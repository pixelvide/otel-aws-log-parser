package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"github.com/pixelvide/otel-alb-log-parser/pkg/converter"
	"github.com/pixelvide/otel-alb-log-parser/pkg/parser"
)

var (
	s3Client      *s3.S3
	otlpEndpoint  string
	basicAuthUser string
	basicAuthPass string
	maxBatchSize  int
	maxRetries    int
	retryBaseSec  float64
	logger        *slog.Logger
	maxConcurrent int
)

func init() {
	// Initialize structured logger (JSON format)
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Initialize AWS session
	sess := session.Must(session.NewSession())
	s3Client = s3.New(sess)

	// Load configuration from environment
	otlpEndpoint = getEnv("SIGNOZ_OTLP_ENDPOINT", "http://localhost:4318/v1/logs")
	basicAuthUser = os.Getenv("BASIC_AUTH_USERNAME")
	basicAuthPass = os.Getenv("BASIC_AUTH_PASSWORD")
	maxBatchSize = getEnvInt("MAX_BATCH_SIZE", 500)
	maxRetries = getEnvInt("MAX_RETRIES", 3)
	maxConcurrent = getEnvInt("MAX_CONCURRENT", 10)
	retryBaseSec = 1.0
}

func handler(ctx context.Context, s3Event events.S3Event) error {
	logger.Info("Lambda triggered", "record_count", len(s3Event.Records))

	for _, record := range s3Event.Records {
		bucket := record.S3.Bucket.Name
		key := record.S3.Object.Key

		log := logger.With("bucket", bucket, "key", key)
		log.Info("Processing S3 object")

		// Read and parse logs from S3
		entries, err := readAndParseFromS3(bucket, key)
		if err != nil {
			log.Error("Error processing S3 object", "error", err)
			return err
		}

		if len(entries) == 0 {
			log.Info("No entries found")
			continue
		}

		log.Info("Successfully parsed entries", "count", len(entries))

		// Convert and send to OTLP
		if err := convertAndSend(entries); err != nil {
			log.Error("Error sending to OTLP", "error", err)
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

	// Create channels for parallel processing
	linesChan := make(chan string, maxBatchSize)
	entriesChan := make(chan *parser.ALBLogEntry, maxBatchSize)
	var wg sync.WaitGroup

	// Start workers
	// Use maxConcurrent/2 for parsing to leave room for sending logic, or just use maxConcurrent
	numWorkers := maxConcurrent
	if numWorkers < 1 {
		numWorkers = 1
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for line := range linesChan {
				if line == "" {
					continue
				}
				entry, err := parser.ParseLogLine(line)
				if err == nil && entry != nil {
					entriesChan <- entry
				}
			}
		}()
	}

	// Start a goroutine to read lines and send to workers
	go func() {
		scanner := bufio.NewScanner(reader)
		// Increase buffer size if needed, default is 64k which should be enough for log lines
		// but ALB logs can be long. Let's use a larger buffer just in case.
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024) // 1MB max line size

		for scanner.Scan() {
			linesChan <- scanner.Text()
		}

		if err := scanner.Err(); err != nil {
			logger.Error("Error scanning S3 object", "error", err)
		}

		close(linesChan)
	}()

	// Start a goroutine to close entriesChan when all workers are done
	go func() {
		wg.Wait()
		close(entriesChan)
	}()

	// Collect results
	entries := make([]*parser.ALBLogEntry, 0)
	for entry := range entriesChan {
		entries = append(entries, entry)
	}

	logger.Info("Parsed entries", "count", len(entries))
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

	logger.Info("Grouped logs", "resource_group_count", len(grouped))

	logger.Info("Grouped logs", "resource_group_count", len(grouped))

	// Concurrency control
	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	errChan := make(chan error, 1)

	totalSent := 0
	var sentLock sync.Mutex

	// Send each group in batches
	for resKey, group := range grouped {
		groupLog := logger.With("resource_key", resKey, "total_logs", len(group.LogRecords))
		groupLog.Info("Processing resource group")

		// Split into batches
		batchCount := 0
		for i := 0; i < len(group.LogRecords); i += maxBatchSize {
			// Check for previous errors
			select {
			case err := <-errChan:
				return err
			default:
			}

			end := i + maxBatchSize
			if end > len(group.LogRecords) {
				end = len(group.LogRecords)
			}

			batch := group.LogRecords[i:end]
			payload := buildPayload(group.ResourceAttrs, batch)
			currentBatchCount := batchCount + 1
			currentBatchSize := len(batch)

			wg.Add(1)
			go func(p converter.OTLPPayload, bID int, bSize int, log *slog.Logger) {
				defer wg.Done()

				// Acquire semaphore
				sem <- struct{}{}
				defer func() { <-sem }()

				log.Info("Sending batch", "batch_id", bID, "batch_size", bSize)

				if err := sendWithRetry(p); err != nil {
					log.Error("Failed to send batch", "batch_id", bID, "error", err)
					// Try to report error (non-blocking)
					select {
					case errChan <- fmt.Errorf("failed to send batch %d: %w", bID, err):
					default:
					}
					return
				}

				sentLock.Lock()
				totalSent += bSize
				sentLock.Unlock()
			}(payload, currentBatchCount, currentBatchSize, groupLog)

			batchCount++
		}
	}

	// Wait for all batches to complete
	wg.Wait()

	// Check for any errors that occurred
	select {
	case err := <-errChan:
		return err
	default:
	}

	logger.Info("Successfully sent all logs", "total_sent", totalSent, "resource_groups", len(grouped))
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
			multiplier := 1 << uint(attempt-1)
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
			logger.Warn("Batch send attempt failed", "attempt", attempt+1, "error", err)
			lastErr = err
			continue
		}

		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			logger.Info("Batch sent successfully", "attempt", attempt+1, "status", resp.StatusCode)
			return nil
		}

		respBody, _ := io.ReadAll(resp.Body)
		logger.Warn("Batch send attempt failed", "attempt", attempt+1, "status", resp.StatusCode, "response", string(respBody))
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
