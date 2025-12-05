package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pixelvide/otel-alb-log-parser/pkg/converter"
	"github.com/pixelvide/otel-alb-log-parser/pkg/parser"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <log-file-path>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s /path/to/alb.log.gz\n", os.Args[0])
		os.Exit(1)
	}

	filePath := os.Args[1]

	// Parse the log file
	entries, err := parser.ParseLogFile(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Parsed %d log entries from %s\n", len(entries), filePath)
	fmt.Fprintf(os.Stderr, "Converting to OTLP format...\n\n")

	// Group by resource
	grouped := make(map[string]*resourceGroup)

	for _, entry := range entries {
		// Extract resource key (region + account)
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

	// Build OTLP payload
	payload := converter.OTLPPayload{
		ResourceLogs: []converter.ResourceLog{},
	}

	for _, group := range grouped {
		payload.ResourceLogs = append(payload.ResourceLogs, converter.ResourceLog{
			Resource: converter.ResourceAttributes{
				Attributes: group.ResourceAttrs,
			},
			ScopeLogs: []converter.ScopeLog{
				{
					Scope: converter.Scope{
						Name:    "alb-log-parser",
						Version: "1.0.0",
					},
					LogRecords: group.LogRecords,
				},
			},
		})
	}

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	
	if err := encoder.Encode(payload); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
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
