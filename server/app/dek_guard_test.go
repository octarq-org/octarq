package app

import (
	"context"
	"strings"
	"testing"

	"github.com/octarq-org/octarq/server/internal/crypto"
	"github.com/octarq-org/octarq/server/internal/models"
)

// dekProbePlugin answers EncryptedDataReporter for the guard's test.
type dekProbePlugin struct {
	testDummyPlugin
	has   bool
	calls int
}

func (p *dekProbePlugin) HasEncryptedData() (bool, error) {
	p.calls++
	return p.has, nil
}

// A boot that had to create a fresh DEK while a plugin still holds ciphertext
// must stop. Continuing would serve an instance whose plugin data no longer
// decrypts, and the fresh key would hide the fact on every later boot.
func TestBootRefusesAFreshDEKWhenAPluginHoldsCiphertext(t *testing.T) {
	a := bootApp(t)
	probe := &dekProbePlugin{testDummyPlugin: testDummyPlugin{name: "sealed-plugin"}, has: true}
	a.Use(probe)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := a.RunMCP(ctx)
	if err == nil || !strings.Contains(err.Error(), "the key that decrypts them is missing") {
		t.Fatalf("boot must stop when a plugin holds saved values without their key, got %v", err)
	}
	if probe.calls == 0 {
		t.Fatal("the guard must ask the plugin")
	}

	var count int64
	if err := a.gdb.Model(&models.Setting{}).Where("key = ?", crypto.DEKSettingKey).Count(&count).Error; err != nil {
		t.Fatalf("count DEK rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("the fresh DEK must be removed again, found %d row(s)", count)
	}
}

// A plugin with nothing sealed must not stop a boot that legitimately creates
// the first DEK on a fresh instance.
func TestBootCreatesTheFirstDEKWhenNoPluginHoldsCiphertext(t *testing.T) {
	a := bootApp(t)
	probe := &dekProbePlugin{testDummyPlugin: testDummyPlugin{name: "empty-plugin"}, has: false}
	a.Use(probe)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := a.RunMCP(ctx); err != nil && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("boot with no saved ciphertext must proceed, got %v", err)
	}
	if probe.calls == 0 {
		t.Fatal("the guard must ask the plugin even on the first boot")
	}

	var count int64
	if err := a.gdb.Model(&models.Setting{}).Where("key = ?", crypto.DEKSettingKey).Count(&count).Error; err != nil {
		t.Fatalf("count DEK rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("the first DEK must be kept, found %d row(s)", count)
	}
}
