package capture

import (
	"bufio"
	"context"
	"io"
	"os/exec"
)

// streamCommand starts cmd, sends every output line to the log, and waits for
// it to finish. Standard error is merged into standard output.
//
// Read through an io.Pipe, not StdoutPipe. On Windows the game inherits
// Python's stdout and keeps it open after Stop kills Python. WaitDelay
// can only close pipes it copies itself.
func (r *Runner) streamCommand(cmd *exec.Cmd) error {
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		return err
	}
	logged := make(chan struct{})
	go func() {
		defer close(logged)
		scanner := bufio.NewScanner(pr)
		buf := make([]byte, 0, 256*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			r.log(scanner.Text())
		}
	}()
	err := cmd.Wait()
	pw.Close()
	<-logged
	return err
}

// runCaptureCommand runs the capture process, streams its log, and reports the
// result. A non-zero exit after a Stop is normal, so it is reported as "stopped
// by user" rather than an error. It always ends in the Done status.
func (r *Runner) runCaptureCommand(ctx context.Context, cmd *exec.Cmd) {
	if err := r.streamCommand(cmd); err != nil {
		if ctx.Err() != nil {
			r.log("")
			r.log("Capture stopped by user.")
		} else {
			r.log("Capture ended with error: " + err.Error())
		}
	} else {
		r.log("")
		r.log("Capture finished successfully.")
	}
	r.setStatus(StatusDone)
}
