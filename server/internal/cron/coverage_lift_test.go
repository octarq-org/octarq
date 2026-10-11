package cron

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestCoverageLiftEngineGuardError(t *testing.T) {
	guardErr := errors.New("guard connection broken")
	e := NewEngine(&failClosedGuard{err: guardErr})

	err := e.Register("failing_job", "* * * * *", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("register job: %v", err)
	}

	j := e.jobs["failing_job"]
	e.executeJob(context.Background(), j)

	e.mu.Lock()
	status := j.status
	lastErr := j.lastErr
	e.mu.Unlock()

	if status != "error" {
		t.Errorf("expected status 'error', got %q", status)
	}
	if lastErr != guardErr.Error() {
		t.Errorf("expected lastErr %q, got %q", guardErr.Error(), lastErr)
	}
}

func TestCoverageLiftGuardRedisPingSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err != nil {
				return
			}
			req := strings.ToUpper(string(buf[:n]))
			if strings.Contains(req, "HELLO") {
				_, _ = conn.Write([]byte("-ERR unknown command 'HELLO'\r\n"))
			} else if strings.Contains(req, "PING") {
				_, _ = conn.Write([]byte("+PONG\r\n"))
			} else {
				_, _ = conn.Write([]byte("+OK\r\n+OK\r\n"))
			}
		}
	}()

	guard := NewGuard("redis://" + ln.Addr().String())
	if _, ok := guard.(*distGuard); !ok {
		t.Fatalf("expected *distGuard on successful ping, got %T", guard)
	}
}

func TestCoverageLiftScheduleParsingErrors(t *testing.T) {
	// 6-field parse errors
	cases := []struct {
		spec string
		err  string
	}{
		{"bad 0 0 1 1 1", "second field"},
		{"0 bad 0 1 1 1", "minute field"},
		{"0 0 bad 1 1 1", "hour field"},
		{"0 0 0 bad 1 1", "day of month field"},
		{"0 0 0 1 bad 1", "month field"},
		{"0 0 0 1 1 bad", "day of week field"},
		{"0 0 0 bad-5 1 1", "invalid value"},
		{"0 0 0 1-bad 1 1", "invalid value"},
	}

	for _, tc := range cases {
		_, err := ParseStandard(tc.spec)
		if err == nil || !strings.Contains(err.Error(), tc.err) {
			t.Errorf("spec %q expected error containing %q, got %v", tc.spec, tc.err, err)
		}
	}

	// Step without range (e.g. 10/2)
	sched, err := ParseStandard("0 0 0 10/2 1 1")
	if err != nil {
		t.Fatalf("parse 10/2 step failed: %v", err)
	}
	if sched == nil {
		t.Fatalf("expected non-nil schedule for 10/2")
	}

	// Next returning zero when schedule cannot match within 5 years (Feb 31st)
	feb31, err := ParseStandard("0 0 0 31 2 ?")
	if err != nil {
		t.Fatalf("parse Feb 31: %v", err)
	}
	next := feb31.Next(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if !next.IsZero() {
		t.Errorf("expected zero time for impossible Feb 31, got %v", next)
	}

	// parseField empty string
	_, err = parseField("", 0, 59, nil)
	if err == nil || !strings.Contains(err.Error(), "empty field") {
		t.Errorf("expected empty field error, got %v", err)
	}
}
