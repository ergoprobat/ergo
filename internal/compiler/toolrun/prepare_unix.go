//go:build unix

package toolrun

import (
	"errors"
	"os/exec"
	"syscall"
)

// prepare puts the process in its own group and kills that group when the context ends.
// Killing only the parent would leave a child running past the deadline.
func prepare(cmd *exec.Cmd) {
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
