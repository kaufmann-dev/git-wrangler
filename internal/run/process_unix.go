//go:build !windows

package run

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureCommandCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		return cancelCommand(cmd)
	}
}

// configureInteractiveCommand configures a child that must own the controlling
// terminal, such as an editor. It deliberately does not set Setpgid: keeping the
// child in git-wrangler's foreground process group lets it read and write the
// terminal, whereas a background process group would raise SIGTTIN/SIGTTOU and
// stop the editor before it could open.
func configureInteractiveCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := cmd.Process.Kill()
		if errors.Is(err, os.ErrProcessDone) {
			return nil
		}
		return err
	}
}

func cancelCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return err
		}
		return nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	err = cmd.Process.Kill()
	if errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return err
}
