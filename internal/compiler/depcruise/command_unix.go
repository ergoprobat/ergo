//go:build unix

package depcruise

import (
	"errors"
	"os/exec"
	"syscall"
)

// prepareCommand puts the process in its own group and kills that group when the context ends.
// Killing only the shell would leave a child, such as a sleep or a worker, running past the deadline.
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
}
