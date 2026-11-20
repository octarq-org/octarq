package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"connectrpc.com/connect"
)

const (
	ServiceName      = "octarq.ipc.v1.IPCService"
	StatusProcedure  = "/" + ServiceName + "/Status"
	ReloadProcedure  = "/" + ServiceName + "/Reload"
	MetricsProcedure = "/" + ServiceName + "/Metrics"
)

type StatusRequest struct{}

type StatusReply struct {
	Version    string `json:"version"`
	UptimeSecs int64  `json:"uptime_secs"`
	PID        int    `json:"pid"`
	Goroutines int    `json:"goroutines"`
	MemAlloc   uint64 `json:"mem_alloc"`
	Socket     string `json:"socket"`
}

type ReloadRequest struct{}

type ReloadReply struct {
	Applied []string `json:"applied"`
}

type MetricsRequest struct{}

type ProviderMetric struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type MetricsReply struct {
	Overall   string           `json:"overall"`
	Providers []ProviderMetric `json:"providers"`
	CheckedAt string           `json:"checked_at"`
}

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

func Listen(socketPath string) (net.Listener, error) {
	if socketPath == "" {
		return nil, fmt.Errorf("ipc: empty socket path")
	}
	dir := filepath.Dir(socketPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: mkdir %s: %w", dir, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ipc: chmod dir %s: %w", dir, err)
	}
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("ipc: remove stale socket %s: %w", socketPath, err)
	}
	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("ipc: listen %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		if closeErr := lis.Close(); closeErr != nil {
			return nil, fmt.Errorf("ipc: chmod %s: %w (close error: %v)", socketPath, err, closeErr)
		}
		return nil, fmt.Errorf("ipc: chmod %s: %w", socketPath, err)
	}
	return lis, nil
}

type Daemon struct {
	Version   string
	Started   time.Time
	OnReload  func(ctx context.Context) ([]string, error)
	OnMetrics func(ctx context.Context) (*MetricsReply, error)
}

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

func (d *Daemon) Serve(ctx context.Context, lis net.Listener) error {
	srv := &http.Server{Handler: d.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			log.Printf("ipc: shutdown error: %v", err)
		}
	}()
	if err := srv.Serve(lis); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

type Client struct {
	status  *connect.Client[StatusRequest, StatusReply]
	reload  *connect.Client[ReloadRequest, ReloadReply]
	metrics *connect.Client[MetricsRequest, MetricsReply]
}

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

func (c *Client) Status(ctx context.Context) (*StatusReply, error) {
	res, err := c.status.CallUnary(ctx, connect.NewRequest(&StatusRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (c *Client) Reload(ctx context.Context) (*ReloadReply, error) {
	res, err := c.reload.CallUnary(ctx, connect.NewRequest(&ReloadRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}

func (c *Client) Metrics(ctx context.Context) (*MetricsReply, error) {
	res, err := c.metrics.CallUnary(ctx, connect.NewRequest(&MetricsRequest{}))
	if err != nil {
		return nil, err
	}
	return res.Msg, nil
}
