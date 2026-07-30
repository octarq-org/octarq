package mail

import (
	"testing"
)

func TestValidateSMTPTarget(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		port    int
		wantErr bool
	}{
		// Disallowed ports
		{"disallowed port 80", "1.1.1.1", 80, true},
		{"disallowed port 22", "1.1.1.1", 22, true},
		{"disallowed port 8080", "1.1.1.1", 8080, true},
		{"disallowed port 0", "1.1.1.1", 0, true},

		// Disallowed private/loopback/link-local IPs
		{"loopback IPv4", "127.0.0.1", 587, true},
		{"private 10.x", "10.0.0.1", 587, true},
		{"private 172.16.x", "172.16.0.1", 587, true},
		{"private 192.168.x", "192.168.1.1", 587, true},
		{"link-local metadata", "169.254.169.254", 587, true},
		{"loopback IPv6", "::1", 587, true},
		{"localhost domain", "localhost", 587, true},

		// Allowed public targets and allowed ports (25, 465, 587, 2525)
		{"public IP port 25", "1.1.1.1", 25, false},
		{"public IP port 465", "1.1.1.1", 465, false},
		{"public IP port 587", "8.8.8.8", 587, false},
		{"public IP port 2525", "1.1.1.1", 2525, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSMTPTarget(tt.host, tt.port)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSMTPTarget(%q, %d) error = %v, wantErr = %v", tt.host, tt.port, err, tt.wantErr)
			}
		})
	}
}
