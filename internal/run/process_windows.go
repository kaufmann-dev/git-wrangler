//go:build windows

package run

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

func configureCommandCancellation(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		return cancelCommand(cmd)
	}
}

// configureInteractiveCommand configures a child that must own the terminal,
// such as an editor. Windows has no controlling-terminal process-group issue, so
// this mirrors the standard cancellation setup.
func configureInteractiveCommand(cmd *exec.Cmd) {
	cmd.WaitDelay = commandWaitDelay
	cmd.Cancel = func() error {
		return cancelCommand(cmd)
	}
}

func cancelCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	if err == nil {
		return nil
	}
	killErr := cmd.Process.Kill()
	if errors.Is(killErr, os.ErrProcessDone) {
		return nil
	}
	if killErr != nil {
		return killErr
	}
	return err
}
