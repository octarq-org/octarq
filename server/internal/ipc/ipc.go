// Package ipc is the lightweight daemon control plane: a Connect-RPC service
// over a Unix domain socket. The CLI (status, reload, top) is a thin client;
// the daemon serves version, uptime, runtime stats and a safe config-reload
// hook. The socket lives at 0600 inside a 0700 directory, so only the
// operator's UID can reach it. Unreachable daemons surface as errors —
// clients never fabricate stats.
package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"connectrpc.com/connect"
)

const (
	// ServiceName is the Connect procedure prefix.
	ServiceName = "octarq.ipc.v1.IPCService"
	// StatusProcedure returns daemon vitals.
	StatusProcedure = "/" + ServiceName + "/Status"
	// ReloadProcedure re-reads safe config keys in the daemon.
	ReloadProcedure = "/" + ServiceName + "/Reload"
	// MetricsProcedure returns provider health for the top console.
	MetricsProcedure = "/" + ServiceName + "/Metrics"
)

// StatusRequest asks for daemon vitals (empty body).
type StatusRequest struct{}

// StatusReply carries daemon vitals.
type StatusReply struct {
	Version    string `json:"version"`
	UptimeSecs int64  `json:"uptime_secs"`
	PID        int    `json:"pid"`
	Goroutines int    `json:"goroutines"`
	MemAlloc   uint64 `json:"mem_alloc"`
	Socket     string `json:"socket"`
}

// ReloadRequest asks the daemon to re-read safe config keys.
type ReloadRequest struct{}

// ReloadReply reports which keys were re-applied.
type ReloadReply struct {
	Applied []string `json:"applied"`
}

// MetricsRequest asks for provider health (empty body).
type MetricsRequest struct{}

// ProviderMetric is one provider's health line.
type ProviderMetric struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// MetricsReply carries the aggregated health report.
type MetricsReply struct {
	Overall   string           `json:"overall"`
	Providers []ProviderMetric `json:"providers"`
	CheckedAt string           `json:"checked_at"`
}

// jsonCodec speaks plain JSON on the Connect protocol without protobuf
// codegen. Both ends must install it.
type jsonCodec struct{}

func (jsonCodec) Name() string { return "json" }

func (jsonCodec) Marshal(msg any) ([]byte, error) {
	return json.Marshal(msg)
}

func (jsonCodec) MarshalAppend(dst []byte, msg any) ([]byte, error) {
	buf, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return append(dst, buf...), nil
}

func (jsonCodec) Unmarshal(data []byte, msg any) error {
	return json.Unmarshal(data, msg)
}

// SocketPath resolves the daemon socket: OCTARQ_IPC_SOCKET wins, otherwise
// ~/.octarq/run.sock.
func SocketPath() string {
	if p := os.Getenv("OCTARQ_IPC_SOCKET"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "octarq-run.sock")
	}
	return filepath.Join(home, ".octarq", "run.sock")
}

// Listen creates the socket directory (0700), removes a stale socket, binds
// the Unix listener and chmods the socket to 0600.
func Listen(socketPath string) (net.Listener, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("ipc: empty socket path")
	}
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: mkdir %s: %w", dir, err)
	}
	_ = os.Chmod(dir, 0o700)
	_ = os.Remove(socketPath)
	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = lis.Close()
		return nil, fmt.Errorf("ipc: chmod %s: %w", socketPath, err)
	}
	return lis, nil
}

// Daemon wires daemon state into the served procedures.
type Daemon struct {
	Version  string
	Started  time.Time
	OnReload func(ctx context.Context) ([]string, error)
	// OnMetrics supplies provider health; nil means "unknown" with no providers.
	OnMetrics func(ctx context.Context) (*MetricsReply, error)
}

// Handler returns the Connect mux serving Status and Reload.
func (d *Daemon) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(StatusProcedure, connect.NewUnaryHandler(
		StatusProcedure,
		func(ctx context.Context, _ *connect.Request[StatusRequest]) (*connect.Response[StatusReply], error) {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			return connect.NewResponse(&StatusReply{
				Version:    d.Version,
				UptimeSecs: int64(time.Since(d.Started).Seconds()),
				PID:        os.Getpid(),
				Goroutines: runtime.NumGoroutine(),
				MemAlloc:   mem.Alloc,
			}), nil
		},
		connect.WithCodec(jsonCodec{}),
	))
	mux.Handle(ReloadProcedure, connect.NewUnaryHandler(
		ReloadProcedure,
		func(ctx context.Context, _ *connect.Request[ReloadRequest]) (*connect.Response[ReloadReply], error) {
			applied := []string{}
			if d.OnReload != nil {
				var err error
				applied, err = d.OnReload(ctx)
				if err != nil {
					return nil, connect.NewError(connect.CodeInternal, err)
				}
			}
			return connect.NewResponse(&ReloadReply{Applied: applied}), nil
		},
		connect.WithCodec(jsonCodec{}),
	))
	mux.Handle(MetricsProcedure, connect.NewUnaryHandler(
		MetricsProcedure,
		func(ctx context.Context, _ *connect.Request[MetricsRequest]) (*connect.Response[MetricsReply], error) {
			if d.OnMetrics == nil {
				return connect.NewResponse(&MetricsReply{Overall: "unknown"}), nil
			}
			rep, err := d.OnMetrics(ctx)
			if err != nil {
				return nil, connect.NewError(connect.CodeInternal, err)
			}
			return connect.NewResponse(rep), nil
		},
		connect.WithCodec(jsonCodec{}),
	))
	return mux
}

// Serve runs the daemon control plane until ctx ends.
func (d *Daemon) Serve(ctx context.Context, lis net.Listener) error {
	srv := &http.Server{Handler: d.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Client is the CLI thin client for the daemon control plane.
type Client struct {
	status  *connect.Client[StatusRequest, StatusReply]
	reload  *connect.Client[ReloadRequest, ReloadReply]
	metrics *connect.Client[MetricsRequest, MetricsReply]
}

// NewClient dials the Unix socket at socketPath.
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	httpClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	base := "http://unix"
	return &Client{
		status: connect.NewClient[StatusRequest, StatusReply](
			httpClient, base+StatusProcedure, connect.WithCodec(jsonCodec{}),
		),
		reload: connect.NewClient[ReloadRequest, ReloadReply](
			httpClient, base+ReloadProcedure, connect.WithCodec(jsonCodec{}),
		),
		metrics: connect.NewClient[MetricsRequest, MetricsReply](
			httpClient, base+MetricsProcedure, connect.WithCodec(jsonCodec{}),
		),
	}
}

// Status fetches daemon vitals. A dead daemon is an error, never a guess.
func (c *Client) Status(ctx context.Context) (*StatusReply, error) {
	res, err := c.status.CallUnary(ctx, connect.NewRequest(&StatusRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// Reload asks the daemon to re-read safe config keys.
func (c *Client) Reload(ctx context.Context) (*ReloadReply, error) {
	res, err := c.reload.CallUnary(ctx, connect.NewRequest(&ReloadRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

// Metrics fetches provider health for the top console.
func (c *Client) Metrics(ctx context.Context) (*MetricsReply, error) {
	res, err := c.metrics.CallUnary(ctx, connect.NewRequest(&MetricsRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}
