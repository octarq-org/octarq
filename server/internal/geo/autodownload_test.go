package geo

import (
	"context"
	"errors"
	"testing"
)

// TestAutoDownloadSuccess stubs the entire download pipeline (which normally
// hits MaxMind over the network) and drives autoDownload directly, avoiding
// the 30s retry delay and the background goroutine entirely.
func TestAutoDownloadSuccess(t *testing.T) {
	dir := t.TempDir()
	mmdbPath := writeMMDB(t, dir, false)

	orig := downloadFn
	downloadFn = func(_ context.Context, opts downloadOptions) (string, error) {
		if opts.dir != dir {
			t.Errorf("download dir = %q, want %q", opts.dir, dir)
		}
		return mmdbPath, nil
	}
	defer func() { downloadFn = orig }()

	r := &Resolver{}
	r.autoDownload(context.Background(), dir, "license-key")
	defer r.Close()

	if country, _, _ := r.Locate("203.0.113.5"); country != "US" {
		t.Errorf("autoDownload load produced country %q, want US", country)
	}
}

// TestAutoDownloadFailureThenCancel covers the failure path: after the first
// failed download the retry waits for either the delay or ctx cancellation;
// a canceled ctx aborts the retry immediately instead of sleeping 30s.
func TestAutoDownloadFailureThenCancel(t *testing.T) {
	dir := t.TempDir()
	writeMMDB(t, dir, false)

	orig := downloadFn
	downloadFn = func(context.Context, downloadOptions) (string, error) {
		return "", errors.New("stub network failure")
	}
	defer func() { downloadFn = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled: the retry select must take ctx.Done, not time.After

	r := &Resolver{}
	r.autoDownload(ctx, dir, "license-key")
	r.Close() // no database was loaded, but Close must be safe
}

// TestOpenAutoDownloadMode starts the background auto-download only when a
// license key is present and no cached database exists; without a key the
// resolver stays disabled. The dir must be empty so decideMode picks download,
// not cached. The stub blocks on a channel so the test can restore the real
// downloadFn only once the goroutine is provably inside the stub.
func TestOpenAutoDownloadMode(t *testing.T) {
	emptyDir := t.TempDir()
	readyDir := t.TempDir()
	mmdbPath := writeMMDB(t, readyDir, false)

	called := make(chan struct{})
	release := make(chan struct{})
	orig := downloadFn
	downloadFn = func(_ context.Context, opts downloadOptions) (string, error) {
		close(called)
		<-release
		if opts.dir != emptyDir {
			t.Errorf("download dir = %q, want %q", opts.dir, emptyDir)
		}
		return mmdbPath, nil
	}
	defer func() { downloadFn = orig }()

	r, err := openAuto(emptyDir, "super-secret-key")
	if err != nil {
		t.Fatalf("openAuto download mode: %v", err)
	}
	<-called          // goroutine is blocked inside the stub
	downloadFn = orig // safe now: the goroutine will not read downloadFn again
	close(release)    // let the goroutine finish its Load in the background
	r.Close()
}

// TestOpenAutoDownloadModeDisabled pins that a missing license key with no
// cached database yields a disabled resolver with no download goroutine.
func TestOpenAutoDownloadModeDisabled(t *testing.T) {
	r, err := openAuto(t.TempDir(), "")
	if err != nil {
		t.Fatalf("openAuto: %v", err)
	}
	defer r.Close()
	if c, _, _ := r.Locate("203.0.113.1"); c != "" {
		t.Errorf("disabled resolver locate = %q, want empty", c)
	}
}
