//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"
	"time"
)

// hideWindow v1.2.7: ẩn cửa sổ console của ffmpeg/ffprobe trên Windows —
// không còn cmd nhấp nháy mỗi lần probe/dựng khung/xuất video.
func hideWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}

// configureProc — Windows: ẩn console + hủy qua killProc khi ctx hết hạn
// + WaitDelay chống treo. (Nhóm process-group kiểu Unix không có trên Windows;
// ffmpeg không sinh tiến trình con nên kill trực tiếp là đủ.)
func configureProc(cmd *exec.Cmd) {
	hideWindow(cmd)
	cmd.Cancel = func() error { return killProc(cmd) }
	cmd.WaitDelay = 10 * time.Second
}

func killProc(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
