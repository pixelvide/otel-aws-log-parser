package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	
	"github.com/pixelvide/go-alb-processor/pkg/converter"
	"github.com/pixelvide/go-alb-processor/pkg/parser"
)

var (
	s3Client           *s3.S3
	otlpEndpoint       string
	basicAuthUser      string
	basicAuthPass      string
	maxBatchSize       int
	maxRetries         int
	retryBaseSec       float64
)

func init() {
	// Initialize AWS session
	sess := session.Must(session.NewSession())
	s3Client = s3.New(sess)
	
	// Load configuration from environment
	otlpEndpoint = getEnv("SIGNOZ_OTLP_ENDPOINT", "http://localhost:4318/v1/logs")
	basicAuthUser = os.Getenv("BASIC_AUTH_USERNAME")
	basicAuthPass = os.Getenv("BASIC_AUTH_PASSWORD")
	maxBatchSize = getEnvInt("MAX_BATCH_SIZE", 500)
	maxRetries = getEnvInt("MAX_RETRIES", 3)
	retryBaseSec = 1.0
}

func handler(ctx context.Context, s3Event events.S3Event) error {
	for _, record := range s3Event.Records {
		bucket := record.S3.Bucket.Name
		key := record.S3.Object.Key
		
		fmt.Printf("Processing s3://%s/%s\n", bucket, key)
		
		// Read and parse logs from S3
		entries, err := readAndParseFromS3(bucket, key)
		if err != nil {
			fmt.Printf("Error processing %s: %v\n", key, err)
			return err
		}
		
		if len(entries) == 0 {
			fmt.Printf("No entries found in %s\n", key)
			continue
		}
		
		fmt.Printf("Parsed %d entries from %s\n", len(entries), key)
		
		// Convert and send to OTLP
		if err := convertAndSend(entries); err != nil {
			fmt.Printf("Error sending to OTLP: %v\n", err)
			return err
		}
	}
	
	return nil
}

func readAndParseFromS3(bucket, key string) ([]*parser.ALBLogEntry, error) {
	// Get object from S3
	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 object: %w", err)
	}
	defer result.Body.Close()

	var reader io.Reader = result.Body

	// Handle gzip compression
	if strings.HasSuffix(key, ".gz") {
		gzReader, err := gzip.NewReader(result.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}
	
	// Read content
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object: %w", err)
	}
	
	lines := strings.Split(string(content), "\n")
	entries := make([]*parser.ALBLogEntry, 0, len(lines))
	
	for _, line := range lines {
		if line == "" {
			continue
		}
		
		entry, err := parser.ParseLogLine(line)
		if err != nil {
			// Skip malformed lines
			continue
		}
		if entry != nil {
			entries = append(entries, entry)
		}
	}
	
	return entries, nil
}

func convertAndSend(entries []*parser.ALBLogEntry) error {
	// Group by resource
	grouped := make(map[string]*resourceGroup)
	
	for _, entry := range entries {
		resKey := getResourceKey(entry)
		
		if _, exists := grouped[resKey]; !exists {
			grouped[resKey] = &resourceGroup{
				ResourceAttrs: converter.ExtractResourceAttributes(entry),
				LogRecords:    []converter.OTelLogRecord{},
			}
		}
		
		logRecord := converter.ConvertToOTel(entry)
		grouped[resKey].LogRecords = append(grouped[resKey].LogRecords, logRecord)
	}
	
	fmt.Printf("Grouped into %d resource groups\n", len(grouped))
	
	// Send each group in batches
	for resKey, group := range grouped {
		fmt.Printf("Sending %d logs for resource %s\n", len(group.LogRecords), resKey)
		
		// Split into batches
		for i := 0; i < len(group.LogRecords); i += maxBatchSize {
			end := i + maxBatchSize
			if end > len(group.LogRecords) {
				end = len(group.LogRecords)
			}
			
			batch := group.LogRecords[i:end]
			payload := buildPayload(group.ResourceAttrs, batch)
			
			if err := sendWithRetry(payload); err != nil {
				return fmt.Errorf("failed to send batch: %w", err)
			}
		}
	}
	
	return nil
}

func buildPayload(resourceAttrs []converter.OTelAttribute, logRecords []converter.OTelLogRecord) converter.OTLPPayload {
	return converter.OTLPPayload{
		ResourceLogs: []converter.ResourceLog{
			{
				Resource: converter.ResourceAttributes{
					Attributes: resourceAttrs,
				},
				ScopeLogs: []converter.ScopeLog{
					{
						Scope: converter.Scope{
							Name:    "alb-log-parser",
							Version: "1.0.0",
						},
						LogRecords: logRecords,
					},
				},
			},
		},
	}
}

func sendWithRetry(payload converter.OTLPPayload) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			multiplier := 1 << uint(attempt-1) // 1, 2, 4, 8...
			sleep := time.Duration(retryBaseSec*float64(multiplier)) * time.Second
			time.Sleep(sleep)
		}
		
		req, err := http.NewRequest("POST", otlpEndpoint, bytes.NewBuffer(body))
		if err != nil {
			lastErr = err
			continue
		}
		
		req.Header.Set("Content-Type", "application/json")
		
		if basicAuthUser != "" && basicAuthPass != "" {
			req.SetBasicAuth(basicAuthUser, basicAuthPass)
		}
		
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		
		defer resp.Body.Close()
		
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			fmt.Printf("Successfully sent batch (attempt %d)\n", attempt+1)
			return nil
		}
		
		respBody, _ := io.ReadAll(resp.Body)
		lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	
	return fmt.Errorf("failed after %d attempts: %w", maxRetries+1, lastErr)
}

type resourceGroup struct {
	ResourceAttrs []converter.OTelAttribute
	LogRecords    []converter.OTelLogRecord
}

func getResourceKey(entry *parser.ALBLogEntry) string {
	arn := entry.TargetGroupARN
	if arn == "" || arn == "-" {
		arn = entry.ChosenCertARN
	}
	return arn
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		var result int
		fmt.Sscanf(value, "%d", &result)
		return result
	}
	return defaultValue
}

func main() {
	lambda.Start(handler)
}
