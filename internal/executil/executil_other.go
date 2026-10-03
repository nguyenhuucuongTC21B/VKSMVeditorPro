//go:build !windows

package executil

import "os/exec"

// Hide là no-op trên Linux/macOS (không có khái niệm cửa sổ console để ẩn).
func Hide(cmd *exec.Cmd) {
	_ = cmd
}
