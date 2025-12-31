package parser

import (
	"testing"
)

func TestParseNLBLogLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    *NLBLogEntry
		wantErr bool
	}{
		{
			name: "Valid TLS log",
			line: "tls 2.0 2023-10-01T00:00:00.000000Z app/net-lb/1234567890abcdef listener/net-lb/1234567890abcdef/1234567890abcdef 1.2.3.4:12345 5.6.7.8:80 0.001 0.002 100 200 - arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012 - ECDHE-RSA-AES128-GCM-SHA256 TLSv1.2 - example.com h2 - 2.000",
			want: &NLBLogEntry{
				Type:               "tls",
				Version:            "2.0",
				Time:               "2023-10-01T00:00:00.000000Z",
				ELB:                "app/net-lb/1234567890abcdef",
				ListenerID:         "listener/net-lb/1234567890abcdef/1234567890abcdef",
				ClientIP:           "1.2.3.4",
				ClientPort:         12345,
				TargetIP:           "5.6.7.8",
				TargetPort:         80,
				ConnectionTime:     0.001,
				TLSHandshakeTime:   0.002,
				ReceivedBytes:      100,
				SentBytes:          200,
				ChosenCertARN:      "arn:aws:acm:us-east-1:123456789012:certificate/12345678-1234-1234-1234-123456789012",
				TLSCipher:          "ECDHE-RSA-AES128-GCM-SHA256",
				TLSProtocolVersion: "TLSv1.2",
				DomainName:         "example.com",
				ALPNProtocol:       "h2",
				TLSHandshakeTimeMS: 2.000,
			},
			wantErr: false,
		},
		{
			name:    "Invalid log line",
			line:    "invalid log line",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseNLBLogLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseNLBLogLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Type != tt.want.Type {
					t.Errorf("ParseNLBLogLine() Type = %v, want %v", got.Type, tt.want.Type)
				}
				if got.ClientIP != tt.want.ClientIP {
					t.Errorf("ParseNLBLogLine() ClientIP = %v, want %v", got.ClientIP, tt.want.ClientIP)
				}
				if got.TargetIP != tt.want.TargetIP {
					t.Errorf("ParseNLBLogLine() TargetIP = %v, want %v", got.TargetIP, tt.want.TargetIP)
				}
			}
		})
	}
}
