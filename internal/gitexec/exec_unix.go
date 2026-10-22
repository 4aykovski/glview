//go:build unix

package gitexec

import (
	"os/exec"
	"syscall"
)

const supported = true

// detach starts git in a new session without a controlling terminal, so
// ssh and credential helpers cannot open /dev/tty; cancelling kills the
// whole process group (ssh included).
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
