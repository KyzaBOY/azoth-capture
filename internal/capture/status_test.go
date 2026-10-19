package capture

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// installedRunner returns a Runner whose environment looks installed.
func installedRunner(t *testing.T) *Runner {
	t.Helper()
	r := New(t.TempDir(), nil, nil)
	for _, p := range []string{r.pythonExe(), r.captureScript()} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestIsEnvInstalledKeepsBusyStatus(t *testing.T) {
	for _, busy := range []Status{StatusRunning, StatusInstalling} {
		r := installedRunner(t)
		r.setStatus(busy)
		if !r.IsEnvInstalled() {
			t.Fatal("environment should read as installed")
		}
		if got := r.Status(); got != busy {
			t.Errorf("status changed from %q to %q while busy", busy, got)
		}
	}
}

func TestIsEnvInstalledMarksIdleAsInstalled(t *testing.T) {
	r := installedRunner(t)
	r.setStatus(StatusDone)
	r.IsEnvInstalled()
	if got := r.Status(); got != StatusInstalled {
		t.Errorf("status = %q, want %q", got, StatusInstalled)
	}
}

// StopCapture must not return until the capture has finished writing, or the
// ledger copied right after it would miss the final flush.
func TestStopCaptureWaitsForTheFinalWrite(t *testing.T) {
	r := New(t.TempDir(), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	r.cancel, r.finished = cancel, finished

	var finalWrite atomic.Bool
	go func() {
		<-ctx.Done()
		time.Sleep(200 * time.Millisecond) // the helper's final flush
		finalWrite.Store(true)
		close(finished)
	}()

	r.StopCapture()
	if !finalWrite.Load() {
		t.Fatal("StopCapture returned before the capture finished writing")
	}
}

func TestStopCaptureWithoutCaptureReturns(t *testing.T) {
	New(t.TempDir(), nil, nil).StopCapture() // must not block or panic
}
