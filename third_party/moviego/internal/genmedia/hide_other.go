//go:build !windows

package genmedia

import "os/exec"

func hideWindow(cmd *exec.Cmd) { _ = cmd }
