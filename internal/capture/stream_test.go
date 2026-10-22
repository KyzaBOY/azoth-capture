package capture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"
)

const (
	helperEnv        = "AZOTH_STREAM_HELPER"
	helperChild      = "child"
	helperGrandchild = "grandchild"
	helperLifetime   = 30 * time.Second
	testDrainWait    = 500 * time.Millisecond
	maxStopTime      = 10 * time.Second
)

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case helperChild:
		grandchild := exec.Command(os.Args[0])
		grandchild.Env = append(os.Environ(), helperEnv+"="+helperGrandchild)
		grandchild.Stdout = os.Stdout
		grandchild.Stderr = os.Stderr
		if err := grandchild.Start(); err != nil {
			os.Exit(1)
		}
		fmt.Println(grandchild.Process.Pid)
		time.Sleep(helperLifetime)
		os.Exit(0)
	case helperGrandchild:
		time.Sleep(helperLifetime)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Child leaves a grandchild holding stdout, like the game on Windows.
func TestStreamCommandReturnsWhileGrandchildHoldsOutput(t *testing.T) {
	lines := make(chan string, 1)
	r := New(t.TempDir(), func(line string) {
		select {
		case lines <- line:
		default:
		}
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0])
	cmd.Env = append(os.Environ(), helperEnv+"="+helperChild)
	cmd.WaitDelay = testDrainWait

	done := make(chan struct{})
	go func() {
		r.streamCommand(cmd)
		close(done)
	}()

	select {
	case line := <-lines:
		pid, err := strconv.Atoi(line)
		if err != nil {
			t.Fatalf("helper printed %q, want the grandchild pid", line)
		}
		t.Cleanup(func() {
			if p, err := os.FindProcess(pid); err == nil {
				p.Kill()
			}
		})
	case <-time.After(maxStopTime):
		t.Fatal("helper did not start the grandchild")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(maxStopTime):
		t.Fatalf("streamCommand still waiting %v after the child was stopped", maxStopTime)
	}
}
