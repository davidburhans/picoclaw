//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
	"time"
)

func prepareCommandForTermination(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	// Use setpgid to ensure the child process is in the same process group as the parent
	// This allows us to kill the entire process tree by killing the process group
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := cmd.Process.Pid
	if pid <= 0 {
		return nil
	}

	// First try to kill the process group (negative PID)
	// This should kill the shell and all its children in the same process group
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	
	// Small delay to allow process group kill to propagate
	time.Sleep(50 * time.Millisecond)
	
	// Fallback: try to kill the process directly
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	
	return nil
}
