//go:build windows

package videoio

import (
	"os/exec"
	"syscall"
)

// hideWindow v1.2.7: ẩn console trên Windows cho ffmpeg -h encoder=... .
func hideWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
