//go:build windows

package depcruise

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// prepareCommand kills the process and the children it spawned when the context ends.
// A Windows depcruise shim starts Node as a child. Killing only the shim would leave that child running.
func prepareCommand(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		pid := strconv.Itoa(cmd.Process.Pid)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// taskkill is a system binary. The PID is the process this command started.
		killErr := exec.CommandContext(ctx, "taskkill", "/F", "/T", "/PID", pid).Run()
		procErr := cmd.Process.Kill()
		if killErr == nil || procErr == nil {
			return nil
		}
		return killErr
	}
}
