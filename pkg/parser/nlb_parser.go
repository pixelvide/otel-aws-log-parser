package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// NLBLogEntry represents a parsed NLB log entry
type NLBLogEntry struct {
	Type                     string
	Version                  string
	Time                     string
	ELB                      string
	ListenerID               string
	ClientIP                 string
	ClientPort               int
	TargetIP                 string
	TargetPort               int
	ConnectionTime           float64
	TLSHandshakeTime         float64
	ReceivedBytes            int64
	SentBytes                int64
	IncomingTLSAlert         string
	ChosenCertARN            string
	ChosenCertSerial         string
	TLSCipher                string
	TLSProtocolVersion       string
	TLSNamedGroup            string
	DomainName               string
	ALPNProtocol             string
	ALPNClientPreferenceList string
	TLSHandshakeTimeMS       float64
}

// Regex for NLB logs
// Based on: type version time elb listener client:port destination:port ...
var nlbLogPattern = regexp.MustCompile(
	`^([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*):([0-9]*) ([^ ]*):([0-9]*) ([-.0-9]*) ([-.0-9]*) ([-0-9]*) ([-0-9]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([^ ]*) ([-.0-9]*)`,
)

// ParseNLBLogLine parses a single NLB log line
func ParseNLBLogLine(line string) (*NLBLogEntry, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, nil
	}

	matches := nlbLogPattern.FindStringSubmatch(line)
	if matches == nil {
		// Attempt fallback or simpler parsing if feasible, but for now error out
		return nil, fmt.Errorf("failed to parse NLB log line")
	}

	entry := &NLBLogEntry{
		Type:                     getString(matches, 1),
		Version:                  getString(matches, 2),
		Time:                     getString(matches, 3),
		ELB:                      getString(matches, 4),
		ListenerID:               getString(matches, 5),
		ClientIP:                 getString(matches, 6),
		ClientPort:               getInt(matches, 7),
		TargetIP:                 getString(matches, 8),
		TargetPort:               getInt(matches, 9),
		ConnectionTime:           getFloat(matches, 10),
		TLSHandshakeTime:         getFloat(matches, 11),
		ReceivedBytes:            getInt64(matches, 12),
		SentBytes:                getInt64(matches, 13),
		IncomingTLSAlert:         getString(matches, 14),
		ChosenCertARN:            getString(matches, 15),
		ChosenCertSerial:         getString(matches, 16),
		TLSCipher:                getString(matches, 17),
		TLSProtocolVersion:       getString(matches, 18),
		TLSNamedGroup:            getString(matches, 19),
		DomainName:               getString(matches, 20),
		ALPNProtocol:             getString(matches, 21),
		ALPNClientPreferenceList: getString(matches, 22),
		TLSHandshakeTimeMS:       getFloat(matches, 23),
	}

	return entry, nil
}
