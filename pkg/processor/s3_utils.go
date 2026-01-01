package processor

import (
	"regexp"
)

// Common regex for AWS Logs S3 Key format
// Format: .../AWSLogs/<AccountID>/elasticloadbalancing/<Region>/...
// Format: .../AWSLogs/<AccountID>/WAFLogs/<Region>/...
var awsLogsKeyPattern = regexp.MustCompile(`AWSLogs/(\d+)/[^/]+/([^/]+)/`)

// ParseRegionAccountFromS3Key attempts to extract Account ID and Region from standard AWS S3 Log keys.
func ParseRegionAccountFromS3Key(key string) (string, string) {
	matches := awsLogsKeyPattern.FindStringSubmatch(key)
	if len(matches) >= 3 {
		return matches[1], matches[2]
	}
	// Fallback/Edge case: empty strings
	return "", ""
}
