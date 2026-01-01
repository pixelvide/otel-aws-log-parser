package processor

import (
	"context"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pixelvide/otel-aws-log-parser/cmd/lambda/adapter"
	"github.com/pixelvide/otel-aws-log-parser/pkg/converter"
	"github.com/pixelvide/otel-aws-log-parser/pkg/parser"
)

type ALBProcessor struct {
	MaxBatchSize  int
	MaxConcurrent int
}

func (p *ALBProcessor) Name() string {
	return "ALB"
}

func (p *ALBProcessor) Matches(bucket, key string) bool {
	return strings.Contains(key, "/elasticloadbalancing/") && strings.Contains(key, "_app.")
}

func (p *ALBProcessor) Process(ctx context.Context, logger *slog.Logger, s3Client *s3.S3, bucket, key string) ([]adapter.LogAdapter, error) {
	// Extract common attributes from S3 key
	accountID, region := ParseRegionAccountFromS3Key(key)

	return ReadAndParseFromS3(logger, s3Client, bucket, key, p.MaxBatchSize, p.MaxConcurrent, func(line string) (adapter.LogAdapter, error) {
		entry, err := parser.ParseLogLine(line)
		if err != nil {
			return nil, err
		}
		return ALBAdapter{
			ALBLogEntry: entry,
			AccountID:   accountID,
			Region:      region,
		}, nil
	})
}

// ALBAdapter implementation
type ALBAdapter struct {
	*parser.ALBLogEntry
	AccountID string
	Region    string
}

func (a ALBAdapter) GetResourceKey() string {
	arn := a.ALBLogEntry.TargetGroupARN
	if arn == "" || arn == "-" {
		arn = a.ALBLogEntry.ChosenCertARN
	}
	return arn
}

func (a ALBAdapter) GetResourceAttributes() []converter.OTelAttribute {
	attrs := converter.ExtractResourceAttributes(a.ALBLogEntry)

	// Check if cloud attributes are missing and fill from S3 key context
	hasAccount := false
	hasRegion := false
	for _, attr := range attrs {
		if attr.Key == "cloud.account.id" {
			hasAccount = true
		}
		if attr.Key == "cloud.region" {
			hasRegion = true
		}
	}

	if !hasAccount && a.AccountID != "" {
		attrs = append(attrs, converter.OTelAttribute{Key: "cloud.account.id", Value: converter.OTelAnyValue{StringValue: &a.AccountID}})
	}
	if !hasRegion && a.Region != "" {
		attrs = append(attrs, converter.OTelAttribute{Key: "cloud.region", Value: converter.OTelAnyValue{StringValue: &a.Region}})
	}

	return attrs
}

func (a ALBAdapter) ToOTel() converter.OTelLogRecord {
	return converter.ConvertToOTel(a.ALBLogEntry)
}
