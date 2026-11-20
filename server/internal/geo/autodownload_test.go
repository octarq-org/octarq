package geo

import (
	"context"
	"errors"
	"testing"
)

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

func TestAutoDownloadFailureThenCancel(t *testing.T) {
	dir := t.TempDir()
	writeMMDB(t, dir, false)

	orig := downloadFn
	downloadFn = func(context.Context, downloadOptions) (string, error) {
		return "", errors.New("stub network failure")
	}
	defer func() { downloadFn = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	r := &Resolver{}
	r.autoDownload(ctx, dir, "license-key")
	r.Close()
}

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
	<-called
	downloadFn = orig
	close(release)
	r.Close()
}

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
