//go:build windows

// Package executil — v1.2.7: giúp mọi tiến trình con (ffmpeg/ffprobe/powershell)
// KHÔNG BAO GIỜ hiện cửa sổ console trên Windows.
//
// Trước v1.2.7, mỗi lần app gọi ffmpeg để dựng khung/probe/xuất video, Windows
// mở một cửa sổ cmd nhấp nháy (vì tiến trình con kế thừa console mặc định).
// Khi người dùng tua/phát ở chế độ "Khung", hàng chục tiến trình sinh ra mỗi
// giây → màn hình nhấp nháy liên tục, CPU/RAM tăng vọt, app có thể chết và để
// lại các cửa sổ cmd mồ côi.
//
// CREATE_NO_WINDOW (0x08000000) tạo tiến trình KHÔNG có console; HideWindow
// ẩn thêm cửa sổ chính nếu tiến trình tự tạo. Hai cờ này KHÔNG ảnh hưởng tới
// stdin/stdout/stderr pipes — ffmpeg vẫn chạy bình thường, vẫn đọc được output.
package executil

import (
	"os/exec"
	"syscall"
)

// createNoWindow — cờ Windows CREATE_NO_WINDOW.
const createNoWindow = 0x08000000

// Hide đặt cờ ẩn console cho tiến trình con trên Windows. No-op trên hệ khác.
func Hide(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
