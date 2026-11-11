package hostlist

import (
	"testing"
)

func TestHostListOperations(t *testing.T) {
	t.Parallel()

	var l HostList
	if err := l.Scan(nil); err != nil || l != nil {
		t.Errorf("Scan(nil) failed: %v", err)
	}
	if err := l.Scan([]byte("")); err != nil || l != nil {
		t.Errorf("Scan(\"\") failed: %v", err)
	}
	if err := l.Scan(12345); err == nil {
		t.Errorf("Scan(int) expected error")
	}

	val, err := l.Value()
	if err != nil || val != "[]" {
		t.Errorf("Value() empty expected \"[]\", got %v", val)
	}

	populated := HostList{
		{Host: "a.com", Enabled: true},
		{Host: "b.com", Enabled: false},
	}
	val, err = populated.Value()
	if err != nil {
		t.Fatalf("Value() populated error: %v", err)
	}

	var scanned HostList
	if err := scanned.Scan(val); err != nil || len(scanned) != 2 {
		t.Fatalf("Scan() populated error: %v", err)
	}

	enabled := populated.Enabled()
	if len(enabled) != 1 || enabled[0] != "a.com" {
		t.Errorf("Enabled() expected [a.com], got %v", enabled)
	}

	if populated.Blocks("a.com") {
		t.Errorf("Blocks(\"a.com\") expected false (since enabled)")
	}
	if !populated.Blocks("b.com") {
		t.Errorf("Blocks(\"b.com\") expected true (since disabled)")
	}
	if populated.Blocks("c.com") {
		t.Errorf("Blocks(\"c.com\") expected false (unlisted)")
	}
}

func TestHostListScanRejectsLegacyFormat(t *testing.T) {
	t.Parallel()

	var l HostList
	if err := l.Scan(`["go.example.com","s.example.com"]`); err == nil {
		t.Errorf("Scan of legacy []string format expected error")
	}

	if err := l.Scan(`invalid-json`); err == nil {
		t.Errorf("Scan invalid json expected error")
	}
}

func TestNormalizeHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"  Go.Example.COM:8443  ", "go.example.com"},
		{"acme.com.", "acme.com"},
		{"Acme.COM:8080.", "acme.com"},
		{"[::1]:8080", "[::1]"},
		{"[2001:db8::1]", "[2001:db8::1]"},
		{"192.168.1.1:80", "192.168.1.1"},
	}

	for _, tt := range tests {
		got := NormalizeHost(tt.in)
		if got != tt.want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
