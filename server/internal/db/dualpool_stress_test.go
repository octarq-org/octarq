package db

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/config"
	"gorm.io/gorm"
)

type StressModel struct {
	ID        uint   `gorm:"primaryKey"`
	TenantID  string `gorm:"index;size:64"`
	Key       string `gorm:"index;size:64"`
	Value     string `gorm:"type:text"`
	Counter   int
	UpdatedAt time.Time
}

func TestDualPool_SQLiteMaxConns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "pool_conns.db")

	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    dbPath,
	}

	writeDB, readDB, err := OpenDualPool(cfg)
	if err != nil {
		t.Fatalf("OpenDualPool: %v", err)
	}

	wSQL, err := writeDB.DB()
	if err != nil {
		t.Fatalf("writeDB.DB: %v", err)
	}
	rSQL, err := readDB.DB()
	if err != nil {
		t.Fatalf("readDB.DB: %v", err)
	}

	if got := wSQL.Stats().MaxOpenConnections; got != 1 {
		t.Errorf("writeDB MaxOpenConnections = %d, want 1", got)
	}

	wantReaders := max(4, runtime.NumCPU())
	if got := rSQL.Stats().MaxOpenConnections; got != wantReaders {
		t.Errorf("readDB MaxOpenConnections = %d, want %d", got, wantReaders)
	}

	for name, sdb := range map[string]*gorm.DB{"writeDB": writeDB, "readDB": readDB} {
		sqlDB, err := sdb.DB()
		if err != nil {
			t.Fatalf("%s.DB(): %v", name, err)
		}

		var jm string
		if err := sqlDB.QueryRow("PRAGMA journal_mode").Scan(&jm); err != nil {
			t.Errorf("%s PRAGMA journal_mode: %v", name, err)
		} else if strings.ToLower(jm) != "wal" {
			t.Errorf("%s PRAGMA journal_mode = %q, want wal", name, jm)
		}

		var bt int
		if err := sqlDB.QueryRow("PRAGMA busy_timeout").Scan(&bt); err != nil {
			t.Errorf("%s PRAGMA busy_timeout: %v", name, err)
		} else if bt < 5000 {
			t.Errorf("%s PRAGMA busy_timeout = %d, want >= 5000", name, bt)
		}

		var syncMode int
		if err := sqlDB.QueryRow("PRAGMA synchronous").Scan(&syncMode); err != nil {
			t.Errorf("%s PRAGMA synchronous: %v", name, err)
		} else if syncMode != 1 { // 1 = NORMAL
			t.Errorf("%s PRAGMA synchronous = %d, want 1 (NORMAL)", name, syncMode)
		}

		var fk int
		if err := sqlDB.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
			t.Errorf("%s PRAGMA foreign_keys: %v", name, err)
		} else if fk != 1 {
			t.Errorf("%s PRAGMA foreign_keys = %d, want 1 (ON)", name, fk)
		}
	}
}

func TestDualPool_EnsureSQLiteWALDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{
			name:  "plain filename",
			input: "octarq.db",
			contains: []string{
				"octarq.db?",
				"_pragma=busy_timeout(5000)",
				"_pragma=journal_mode(WAL)",
				"_pragma=synchronous(NORMAL)",
				"_pragma=foreign_keys(1)",
			},
		},
		{
			name:  "with cache query param",
			input: "octarq.db?cache=shared",
			contains: []string{
				"octarq.db?cache=shared&",
				"_pragma=busy_timeout(5000)",
				"_pragma=journal_mode(WAL)",
				"_pragma=synchronous(NORMAL)",
				"_pragma=foreign_keys(1)",
			},
		},
		{
			name:  "existing busy_timeout pragma is not duplicated",
			input: "octarq.db?_pragma=busy_timeout(5000)",
			contains: []string{
				"_pragma=busy_timeout(5000)",
				"_pragma=journal_mode(WAL)",
				"_pragma=synchronous(NORMAL)",
				"_pragma=foreign_keys(1)",
			},
		},
		{
			name:  "empty DSN",
			input: "",
			contains: []string{
				"?_pragma=busy_timeout(5000)",
				"_pragma=journal_mode(WAL)",
				"_pragma=synchronous(NORMAL)",
				"_pragma=foreign_keys(1)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureSQLiteWALDSN(tt.input)
			for _, c := range tt.contains {
				if !strings.Contains(got, c) {
					t.Errorf("EnsureSQLiteWALDSN(%q) = %q, missing %q", tt.input, got, c)
				}
			}
			if strings.Count(got, "busy_timeout") != 1 {
				t.Errorf("EnsureSQLiteWALDSN(%q) = %q has %d busy_timeout occurrences, want 1", tt.input, got, strings.Count(got, "busy_timeout"))
			}
		})
	}
}

func TestDualPool_UnsupportedDriver(t *testing.T) {
	t.Parallel()

	_, _, err := OpenDualPool(&config.Config{DBDriver: "unsupported_driver"})
	if err == nil {
		t.Error("expected error for unsupported driver, got nil")
	}
}

func TestDualPool_ConcurrentReadWriteStress(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stress.db")

	cfg := &config.Config{
		DBDriver: "sqlite",
		DBDSN:    dbPath,
	}

	writeDB, readDB, err := OpenDualPool(cfg)
	if err != nil {
		t.Fatalf("OpenDualPool failed: %v", err)
	}

	// AutoMigrate on the write handle
	if err := writeDB.AutoMigrate(&StressModel{}); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Pre-seed records so readers have data immediately
	const initialRecords = 20
	for i := 1; i <= initialRecords; i++ {
		item := StressModel{
			TenantID:  fmt.Sprintf("tenant-%d", i%5),
			Key:       fmt.Sprintf("seed-key-%d", i),
			Value:     fmt.Sprintf("seed-value-%d", i),
			Counter:   i,
			UpdatedAt: time.Now(),
		}
		if err := writeDB.Create(&item).Error; err != nil {
			t.Fatalf("seed record %d failed: %v", i, err)
		}
	}

	// Run stress test under race detector for 2 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var (
		writerSuccess atomic.Int64
		readerSuccess atomic.Int64
		errMu         sync.Mutex
		errList       []error
	)

	recordError := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		defer errMu.Unlock()
		errList = append(errList, err)
	}

	const readerCount = 20
	var wg sync.WaitGroup
	startCh := make(chan struct{})

	// 1 Writer goroutine continuously inserting/updating data in a loop
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-startCh

		iter := 0
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			iter++
			txErr := writeDB.Transaction(func(tx *gorm.DB) error {
				newItem := StressModel{
					TenantID:  fmt.Sprintf("tenant-%d", iter%5),
					Key:       fmt.Sprintf("dynamic-key-%d", iter),
					Value:     fmt.Sprintf("dynamic-val-%d", iter),
					Counter:   iter,
					UpdatedAt: time.Now(),
				}
				if err := tx.Create(&newItem).Error; err != nil {
					return err
				}

				// Update an existing row in the same transaction
				targetID := (iter % initialRecords) + 1
				return tx.Model(&StressModel{}).
					Where("id = ?", targetID).
					Updates(map[string]any{
						"counter":    iter,
						"updated_at": time.Now(),
					}).Error
			})

			if txErr != nil {
				recordError(fmt.Errorf("writer transaction failed at iter %d: %w", iter, txErr))
			} else {
				writerSuccess.Add(1)
			}
		}
	}()

	// 20 Reader goroutines continuously selecting data
	for i := 0; i < readerCount; i++ {
		wg.Add(1)
		readerID := i
		go func(rid int) {
			defer wg.Done()
			<-startCh

			rIter := 0
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				rIter++
				var queryErr error

				switch rIter % 4 {
				case 0:
					var items []StressModel
					targetTenant := fmt.Sprintf("tenant-%d", (rid+rIter)%5)
					queryErr = readDB.Where("tenant_id = ?", targetTenant).
						Limit(10).
						Find(&items).Error
				case 1:
					var count int64
					queryErr = readDB.Model(&StressModel{}).Count(&count).Error
				case 2:
					var item StressModel
					targetID := (rIter % initialRecords) + 1
					queryErr = readDB.First(&item, targetID).Error
					if errors.Is(queryErr, gorm.ErrRecordNotFound) {
						queryErr = nil
					}
				case 3:
					var items []StressModel
					queryErr = readDB.Order("id desc").Limit(5).Find(&items).Error
				}

				if queryErr != nil {
					recordError(fmt.Errorf("reader %d query failed at iter %d: %w", rid, rIter, queryErr))
				} else {
					readerSuccess.Add(1)
				}
			}
		}(readerID)
	}

	// Release all goroutines simultaneously
	close(startCh)

	// Wait for context to expire and all goroutines to finish
	wg.Wait()

	totalWrites := writerSuccess.Load()
	totalReads := readerSuccess.Load()

	t.Logf("Stress test finished: %d writer transactions succeeded, %d reader queries succeeded", totalWrites, totalReads)

	// Assert 100% success and 0 errors
	errMu.Lock()
	errCount := len(errList)
	var firstFewErrors []string
	for i, e := range errList {
		if i >= 5 {
			break
		}
		firstFewErrors = append(firstFewErrors, e.Error())
	}
	errMu.Unlock()

	if errCount > 0 {
		t.Fatalf("Encountered %d errors during concurrent read/write stress (sample: %v)", errCount, strings.Join(firstFewErrors, "; "))
	}

	if totalWrites == 0 {
		t.Fatal("Expected at least one writer transaction to complete")
	}

	if totalReads == 0 {
		t.Fatal("Expected at least one reader query to complete")
	}
}
