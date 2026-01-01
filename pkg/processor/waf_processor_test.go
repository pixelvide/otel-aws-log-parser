package processor

import (
	"testing"

	"github.com/pixelvide/otel-aws-log-parser/pkg/parser"
)

func TestWAFProcessor_Matches(t *testing.T) {
	proc := &WAFProcessor{}

	tests := []struct {
		name   string
		bucket string
		key    string
		want   bool
	}{
		{
			name:   "User provided format",
			bucket: "aws-waf-logs-test",
			key:    "KEY-NAME-PREFIX/AWSLogs/123456789012/WAFLogs/us-east-1/TEST-WEBACL/2023/01/01/00/00/123456789012_waflogs_us-east-1_TEST-WEBACL_20230101T0000Z_hash.log.gz",
			want:   true,
		},
		{
			name:   "Standard WAFLogs path",
			bucket: "aws-waf-logs-prod",
			key:    "AWSLogs/123/WAFLogs/us-east-1/my-acl/123_waflogs_file.log",
			want:   true,
		},
		{
			name:   "Alternative prefix with correct format",
			bucket: "aws-waf-logs-custom",
			key:    "some/prefix/WAFLogs/us-east-1/my-acl/123_waflogs_file.log",
			want:   true,
		},
		{
			name:   "ALB log",
			bucket: "my-bucket",
			key:    "AWSLogs/123/elasticloadbalancing/us-east-1/2023/01/01/123_elasticloadbalancing_us-east-1_app.my-lb.123_20230101T0000Z_1.2.3.4_5678.log.gz",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := proc.Matches(tt.bucket, tt.key); got != tt.want {
				t.Errorf("WAFProcessor.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWAFAdapter_GetResourceAttributes(t *testing.T) {
	// ARN provided by user (anonymized)
	arn := "arn:aws:wafv2:ap-south-1:123456789012:regional/webacl/TEST-WEBACL/11111111-2222-3333-4444-555555555555"

	entry := &parser.WAFLogEntry{
		WebACLID: arn,
	}

	// Create adapter without S3 fallback initially to test ARN parsing
	adapter := &WAFAdapter{
		WAFLogEntry: entry,
		AccountID:   "",
		Region:      "",
	}

	attrs := adapter.GetResourceAttributes()

	attrMap := make(map[string]string)
	for _, a := range attrs {
		if a.Value.StringValue != nil {
			attrMap[a.Key] = *a.Value.StringValue
		}
	}

	expected := map[string]string{
		"cloud.provider":     "aws",
		"cloud.platform":     "aws_waf",
		"cloud.service":      "waf",
		"cloud.account.id":   "123456789012",
		"cloud.region":       "ap-south-1",
		"aws.waf.web_acl_id": arn,
	}

	for k, v := range expected {
		if got, ok := attrMap[k]; !ok || got != v {
			t.Errorf("Attribute %q = %q, want %q", k, got, v)
		}
	}
}
