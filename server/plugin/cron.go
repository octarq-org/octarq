package plugin

import (
	"context"
	"errors"
	"time"
)

var (
	ErrJobNotFound = errors.New("cron: job not found")
	ErrJobBusy     = errors.New("cron: job is already running")
)

type CronService interface {
	Register(name string, spec string, handler func(ctx context.Context) error) error
	Unregister(name string) error
	List() []CronJob
	StartJob(name string) error
	SetEnabled(name string, enabled bool) error
	SetSpec(name string, spec string) error
}

type CronJob struct {
	Name        string    `json:"name"`
	Spec        string    `json:"spec"`
	DefaultSpec string    `json:"defaultSpec"`
	Enabled     bool      `json:"enabled"`
	LastRun     time.Time `json:"lastRun"`
	NextRun     time.Time `json:"nextRun"`
	Status      string    `json:"status"`
	LastErr     string    `json:"lastErr"`
}
