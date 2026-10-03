//go:build !windows

package videoio

import "os/exec"

func hideWindow(cmd *exec.Cmd) { _ = cmd }
