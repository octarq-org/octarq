package ipc_test

import (
	"context"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/ipc"
)

func TestIPCCharacterizationProcedures(t *testing.T) {
	if ipc.ServiceName != "octarq.ipc.v1.IPCService" {
		t.Fatalf("ServiceName mismatch: %q", ipc.ServiceName)
	}
	if ipc.StatusProcedure != "/octarq.ipc.v1.IPCService/Status" {
		t.Fatalf("StatusProcedure mismatch: %q", ipc.StatusProcedure)
	}
	if ipc.ReloadProcedure != "/octarq.ipc.v1.IPCService/Reload" {
		t.Fatalf("ReloadProcedure mismatch: %q", ipc.ReloadProcedure)
	}
	if ipc.MetricsProcedure != "/octarq.ipc.v1.IPCService/Metrics" {
		t.Fatalf("MetricsProcedure mismatch: %q", ipc.MetricsProcedure)
	}
}

func TestIPCCharacterizationSocketEnvOverride(t *testing.T) {
	customSocket := "/tmp/test-custom-octarq-ipc.sock"
	t.Setenv("OCTARQ_IPC_SOCKET", customSocket)

	if got := ipc.SocketPath(); got != customSocket {
		t.Fatalf("SocketPath() = %q, want %q", got, customSocket)
	}
}

func TestIPCCharacterizationDeadDaemonFailsClosed(t *testing.T) {
	client := ipc.NewClient("/tmp/nonexistent-daemon-socket.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := client.Status(ctx)
	if err == nil {
		t.Fatal("expected connection to non-existent socket to fail closed, got nil")
	}
}
