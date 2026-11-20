package monitor

import (
	"context"
	"database/sql"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

type DBProvider struct {
	db          *gorm.DB
	sqlDB       *sql.DB
	slowQueries atomic.Uint64
}

func NewDBProvider(db *gorm.DB) *DBProvider {
	p := &DBProvider{db: db}
	if db != nil {
		if sdb, err := db.DB(); err == nil {
			p.sqlDB = sdb
		}
	}
	return p
}

func NewDBProviderWithSQLDB(sqlDB *sql.DB) *DBProvider {
	return &DBProvider{sqlDB: sqlDB}
}

func (p *DBProvider) Name() string {
	return "database"
}

func (p *DBProvider) Metadata() plugin.HealthMeta {
	return plugin.HealthMeta{
		Category: plugin.CategoryDatabase,
		Unit:     plugin.UnitCount,
	}
}

func (p *DBProvider) RecordSlowQuery() {
	p.slowQueries.Add(1)
}

func (p *DBProvider) AddSlowQueries(delta uint64) {
	p.slowQueries.Add(delta)
}

func (p *DBProvider) SlowQueries() uint64 {
	return p.slowQueries.Load()
}

func (p *DBProvider) ResetSlowQueries() {
	p.slowQueries.Store(0)
}

func (p *DBProvider) Check(ctx context.Context) plugin.HealthResult {
	sqlDB := p.sqlDB
	if sqlDB == nil && p.db != nil {
		var err error
		sqlDB, err = p.db.DB()
		if err != nil {
			return plugin.HealthResult{
				Status:  plugin.HealthError,
				Message: fmt.Sprintf("failed to get sql.DB: %v", err),
				Metrics: map[string]interface{}{
					"slow_queries": p.slowQueries.Load(),
				},
			}
		}
		p.sqlDB = sqlDB
	}

	if sqlDB == nil {
		return plugin.HealthResult{
			Status:  plugin.HealthError,
			Message: "database handle is nil",
			Metrics: map[string]interface{}{
				"slow_queries": p.slowQueries.Load(),
			},
		}
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	pingStart := time.Now()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return plugin.HealthResult{
			Status:  plugin.HealthError,
			Message: fmt.Sprintf("database ping failed: %v", err),
			Metrics: map[string]interface{}{
				"slow_queries": p.slowQueries.Load(),
			},
		}
	}
	pingLatencyMs := time.Since(pingStart).Milliseconds()

	stats := sqlDB.Stats()
	sq := p.slowQueries.Load()

	metrics := map[string]interface{}{
		"open_connections":     stats.OpenConnections,
		"in_use":               stats.InUse,
		"idle":                 stats.Idle,
		"wait_count":           stats.WaitCount,
		"wait_duration_ms":     stats.WaitDuration.Milliseconds(),
		"max_open_connections": stats.MaxOpenConnections,
		"max_idle_closed":      stats.MaxIdleClosed,
		"max_lifetime_closed":  stats.MaxLifetimeClosed,
		"slow_queries":         sq,
		"ping_latency_ms":      pingLatencyMs,
	}

	if sq > 0 {
		return plugin.HealthResult{
			Status:  plugin.HealthWarn,
			Message: fmt.Sprintf("%d slow queries", sq),
			Metrics: metrics,
		}
	}

	return plugin.HealthResult{
		Status:  plugin.HealthOK,
		Message: "ok",
		Metrics: metrics,
	}
}
