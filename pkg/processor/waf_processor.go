package processor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pixelvide/otel-aws-log-parser/cmd/lambda/adapter"
	"github.com/pixelvide/otel-aws-log-parser/pkg/converter"
	"github.com/pixelvide/otel-aws-log-parser/pkg/parser"
)

type WAFProcessor struct {
	// WAF processor might not need batch/concurrent config for streaming parser yet
	// but keeping them for consistency or future use
}

func (p *WAFProcessor) Name() string {
	return "WAF"
}

func (p *WAFProcessor) Matches(bucket, key string) bool {
	return strings.HasPrefix(bucket, "aws-waf-logs-") && strings.Contains(key, "/WAFLogs/") && strings.Contains(key, "_waflogs_")
}

func (p *WAFProcessor) Process(ctx context.Context, logger *slog.Logger, s3Client *s3.S3, bucket, key string) ([]adapter.LogAdapter, error) {
	// For WAF, we currently download and parse the whole file.
	// (Unless we already refactored to streaming? The plan mentioned streaming refactor,
	// but I am following the "Parser Identification" refactor plan now.
	// I will implement the download-to-temp logic here as it is what currently exists in main.go)

	// Note: If I already implemented streaming in previous steps, I should use that.
	// But I checked the history, and I only *planned* streaming refactor in Step 714, but didn't implement it yet.
	// So I will stick to the temp file approach for now to match current main.go behavior.

	// Extract common attributes from S3 key
	accountID, region := ParseRegionAccountFromS3Key(key)

	result, err := s3Client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 object: %w", err)
	}
	defer result.Body.Close()

	// Download to temp file
	tmpFile, err := os.CreateTemp("", "waf-log-*.json.gz")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, result.Body); err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	// Close file to flush writes before parsing
	tmpFile.Close()

	wafEntries, err := parser.ParseWAFLogFile(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to parse WAF log: %w", err)
	}

	adapters := make([]adapter.LogAdapter, len(wafEntries))
	for i, e := range wafEntries {
		adapters[i] = &WAFAdapter{
			WAFLogEntry: e,
			AccountID:   accountID,
			Region:      region,
		}
	}
	return adapters, nil
}

// WAFAdapter implementation
type WAFAdapter struct {
	*parser.WAFLogEntry
	AccountID string
	Region    string
}

func (a *WAFAdapter) GetResourceKey() string {
	return a.WAFLogEntry.WebACLID
}

func (a *WAFAdapter) GetResourceAttributes() []converter.OTelAttribute {
	attrs := []converter.OTelAttribute{
		{Key: "cloud.provider", Value: converter.OTelAnyValue{StringValue: aws.String("aws")}},
		{Key: "cloud.platform", Value: converter.OTelAnyValue{StringValue: aws.String("aws_waf")}},
		{Key: "cloud.service", Value: converter.OTelAnyValue{StringValue: aws.String("waf")}},
		{Key: "aws.waf.web_acl_id", Value: converter.OTelAnyValue{StringValue: aws.String(a.WAFLogEntry.WebACLID)}},
	}

	// Try extracting from WebACLID
	extractedAccount := ""
	extractedRegion := ""

	if a.WAFLogEntry.WebACLID != "" {
		parts := strings.Split(a.WAFLogEntry.WebACLID, ":")
		if len(parts) >= 6 {
			// Region is parts[3] (can be empty for global)
			extractedRegion = parts[3]
			if extractedRegion == "" {
				extractedRegion = "global"
			}
			extractedAccount = parts[4]
		}
	}

	// Use extracted values, fallback to S3 context
	finalAccount := extractedAccount
	if finalAccount == "" {
		finalAccount = a.AccountID
	}

	finalRegion := extractedRegion
	if finalRegion == "" {
		finalRegion = a.Region
	}

	if finalAccount != "" {
		attrs = append(attrs, converter.OTelAttribute{Key: "cloud.account.id", Value: converter.OTelAnyValue{StringValue: &finalAccount}})
	}
	if finalRegion != "" {
		attrs = append(attrs, converter.OTelAttribute{Key: "cloud.region", Value: converter.OTelAnyValue{StringValue: &finalRegion}})
	}

	return attrs
}

func (a *WAFAdapter) ToOTel() converter.OTelLogRecord {
	return converter.ConvertWAFToOTel(a.WAFLogEntry)
}
