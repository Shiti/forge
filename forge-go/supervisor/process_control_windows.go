//go:build windows

package supervisor

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"

	gopsprocess "github.com/shirou/gopsutil/v3/process"
)

func configureCommandForProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}

func terminateProcessTree(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}

	// Try a graceful interrupt first.
	_ = proc.Signal(os.Interrupt)

	for i := 0; i < 50; i++ {
		alive, err := gopsprocess.PidExists(int32(pid))
		if err != nil || !alive {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}

	return nil
}
