// Package applog — nhật ký ứng dụng ghi ra %APPDATA%/VKSeditorPro/log.txt.
// Kế thừa cơ chế từ v1.0.5: log mọi bước khởi động/giải nén FFmpeg/lỗi để
// chẩn đoán từ xa; tự xoay vòng khi quá 2MB; không bao giờ làm sập app.
package applog

import (
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"
)

var (
        mu      sync.Mutex
        path    string
        enabled bool
)

// Init bật ghi log vào dir/log.txt (dir = thư mục dữ liệu ứng dụng).
// Gọi một lần lúc khởi động; lỗi chỉ tắt log, không panic.
func Init(dir string) {
        mu.Lock()
        defer mu.Unlock()
        path = filepath.Join(dir, "log.txt")
        if err := os.MkdirAll(dir, 0o755); err != nil {
                enabled = false
                return
        }
        enabled = true
        f("==== VKSeditorPro khởi động ====")
        f("Thư mục dữ liệu: " + dir)
}

// Path trả đường dẫn file log (rỗng nếu chưa Init).
func Path() string {
        mu.Lock()
        defer mu.Unlock()
        return path
}

// TryPath như Path nhưng KHÔNG chờ khoá — bận thì trả "(log đang bận)".
// Dùng cho bảng chẩn đoán để không bao giờ treo giao diện (v1.2.6).
func TryPath() string {
        if !mu.TryLock() {
                return "(log đang bận)"
        }
        defer mu.Unlock()
        return path
}

// Logf ghi một dòng có timestamp. An toàn luồng, không bao giờ panic.
func Logf(format string, args ...any) {
        mu.Lock()
        defer mu.Unlock()
        f(fmt.Sprintf(format, args...))
}

// f gọi khi đã giữ khoá.
func f(msg string) {
        if !enabled {
                return
        }
        defer func() {
                if r := recover(); r != nil {
                        enabled = false
                }
        }()
        line := "[" + time.Now().Format("2006-01-02T15:04:05.000") + "] " + msg + "\n"
        // Xoay vòng khi quá 2MB.
        if st, err := os.Stat(path); err == nil && st.Size() > 2*1024*1024 {
                _ = os.Remove(filepath.Join(filepath.Dir(path), "log.old.txt"))
                _ = os.Rename(path, filepath.Join(filepath.Dir(path), "log.old.txt"))
        }
        fh, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
        if err != nil {
                return
        }
        defer fh.Close()
        _, _ = fh.WriteString(line)
}

// Tail trả N dòng cuối của log (cho bảng chẩn đoán).
func Tail(n int) string {
        mu.Lock()
        defer mu.Unlock()
        return tailLocked(n)
}

// TryTail như Tail nhưng KHÔNG chờ khoá — nếu goroutine ghi log đang bận
// (đĩa chậm / diệt virus quét file) thì trả thông báo thay vì treo bảng
// chẩn đoán mãi mãi (v1.2.6 — lỗi "Đang tải…" không bao giờ xong).
func TryTail(n int) string {
        if !mu.TryLock() {
                return "(log đang bận — có tác vụ ghi lớn, thử lại sau vài giây)"
        }
        defer mu.Unlock()
        return tailLocked(n)
}

// tailLocked gọi khi đã giữ khoá.
func tailLocked(n int) string {
        if !enabled {
                return "(log chưa bật)"
        }
        b, err := os.ReadFile(path)
        if err != nil {
                return "(chưa có log)"
        }
        lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
        if len(lines) > n {
                lines = lines[len(lines)-n:]
        }
        return strings.Join(lines, "\n")
}
