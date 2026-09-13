package plugin

import (
	"context"
	"time"
)

// CronService is the SPI contract for registering and inspecting scheduled cron tasks.
type CronService interface {
	// Register 注册一个定时任务
	Register(name string, spec string, handler func(ctx context.Context) error) error
	// Unregister 注销定时任务
	Unregister(name string) error
	// List 列出所有已注册任务及状态
	List() []CronJob
}

// CronJob represents a registered scheduled task and its execution status.
type CronJob struct {
	Name    string    `json:"name"`
	Spec    string    `json:"spec"`
	LastRun time.Time `json:"lastRun"`
	NextRun time.Time `json:"nextRun"`
	Status  string    `json:"status"` // "idle" | "running" | "error"
	LastErr string    `json:"lastErr"`
}
