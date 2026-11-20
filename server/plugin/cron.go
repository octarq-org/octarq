package plugin

import (
	"context"
	"time"
)

type CronService interface {
	Register(name string, spec string, handler func(ctx context.Context) error) error
	Unregister(name string) error
	List() []CronJob
}

type CronJob struct {
	Name    string    `json:"name"`
	Spec    string    `json:"spec"`
	LastRun time.Time `json:"lastRun"`
	NextRun time.Time `json:"nextRun"`
	Status  string    `json:"status"`
	LastErr string    `json:"lastErr"`
}
