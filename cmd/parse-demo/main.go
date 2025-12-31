package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pixelvide/otel-lb-log-parser/pkg/parser"
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

	// Print results
	fmt.Fprintf(os.Stderr, "Parsed %d log entries from %s\n\n", len(entries), filePath)

	// Output entries as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(entries); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}
