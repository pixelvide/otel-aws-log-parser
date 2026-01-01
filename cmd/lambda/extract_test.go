package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestExtractS3Records(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	tests := []struct {
		name        string
		input       string
		wantCount   int
		wantBucket  string
		wantKey     string
		expectError bool
	}{
		{
			name: "Direct S3 Event",
			input: `{
				"Records": [
					{
						"eventSource": "aws:s3",
						"awsRegion": "us-east-1",
						"s3": {
							"bucket": { "name": "direct-bucket" },
							"object": { "key": "direct-key" }
						}
					}
				]
			}`,
			wantCount:  1,
			wantBucket: "direct-bucket",
			wantKey:    "direct-key",
		},
		{
			name: "SQS with Standard S3 Event",
			input: `{
				"Records": [
					{
						"body": "{\"Records\":[{\"eventSource\":\"aws:s3\",\"awsRegion\":\"us-west-2\",\"s3\":{\"bucket\":{\"name\":\"sqs-std-bucket\"},\"object\":{\"key\":\"sqs-std-key\"}}}]}"
					}
				]
			}`,
			wantCount:  1,
			wantBucket: "sqs-std-bucket",
			wantKey:    "sqs-std-key",
		},
		{
			name: "SQS with EventBridge S3 Event",
			input: `{
				"Records": [
					{
						"body": "{\"version\":\"0\",\"id\":\"d1a0...\",\"detail-type\":\"Object Created\",\"source\":\"aws.s3\",\"account\":\"123\",\"time\":\"2026-01-01T00:00:00Z\",\"region\":\"ap-south-1\",\"detail\":{\"bucket\":{\"name\":\"sqs-eb-bucket\"},\"object\":{\"key\":\"sqs-eb-key\"}}}"
					}
				]
			}`,
			wantCount:  1,
			wantBucket: "sqs-eb-bucket",
			wantKey:    "sqs-eb-key",
		},
		{
			name: "SNS with Standard S3 Event",
			input: `{
				"Records": [
					{
						"Sns": {
							"Message": "{\"Records\":[{\"eventSource\":\"aws:s3\",\"awsRegion\":\"eu-central-1\",\"s3\":{\"bucket\":{\"name\":\"sns-bucket\"},\"object\":{\"key\":\"sns-key\"}}}]}"
						}
					}
				]
			}`,
			wantCount:  1,
			wantBucket: "sns-bucket",
			wantKey:    "sns-key",
		},
		{
			name:        "Invalid JSON",
			input:       `{ "foo": "bar" }`,
			expectError: true,
		},
		{
			name: "SQS with Unknown Body",
			input: `{
				"Records": [
					{ "body": "{\"foo\":\"bar\"}" }
				]
			}`,
			expectError: true, // Should error because no valid S3 records found
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractS3Records(logger, []byte(tt.input))

			if tt.expectError {
				if err == nil {
					t.Errorf("extractS3Records() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("extractS3Records() unexpected error: %v", err)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("extractS3Records() got %d records, want %d", len(got), tt.wantCount)
			}

			if len(got) > 0 {
				if got[0].S3.Bucket.Name != tt.wantBucket {
					t.Errorf("Bucket = %s, want %s", got[0].S3.Bucket.Name, tt.wantBucket)
				}
				if got[0].S3.Object.Key != tt.wantKey {
					t.Errorf("Key = %s, want %s", got[0].S3.Object.Key, tt.wantKey)
				}
			}
		})
	}
}
