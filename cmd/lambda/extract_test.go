package main

import (
	"io"
	"log/slog"
	"testing"
)

func TestParseBodyAsS3(t *testing.T) {
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
			name:       "EventBridge S3 Event",
			input:      `{"version":"0","id":"d1a0...","detail-type":"Object Created","source":"aws.s3","account":"123","time":"2026-01-01T00:00:00Z","region":"ap-south-1","detail":{"bucket":{"name":"sqs-eb-bucket"},"object":{"key":"sqs-eb-key"}}}`,
			wantCount:  1,
			wantBucket: "sqs-eb-bucket",
			wantKey:    "sqs-eb-key",
		},
		{
			name:        "Invalid JSON",
			input:       `{ "foo": "bar" }`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBodyAsS3(logger, []byte(tt.input))

			if tt.expectError {
				if err == nil {
					t.Errorf("parseBodyAsS3() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("parseBodyAsS3() unexpected error: %v", err)
				return
			}

			if len(got) != tt.wantCount {
				t.Errorf("parseBodyAsS3() got %d records, want %d", len(got), tt.wantCount)
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
