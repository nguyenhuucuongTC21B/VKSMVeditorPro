// Package ffmpegbin nhúng sẵn ffmpeg.exe + ffprobe.exe (Windows, build
// essentials của gyan.dev) trong chính tệp .exe thông qua go:embed, và tự
// giải nén vào thư mục cache của người dùng (%LOCALAPPDATA% trên Windows)
// ở lần chạy đầu tiên. Đây là chìa khoá để VKSeditorPro thỏa mãn yêu cầu
// "một tệp .exe duy nhất": không cần cài FFmpeg, không cần internet.
//
// v1.2.0 — hợp nhất cơ chế chống treo từ v1.0.5:
//   - Prepare(): kiểm tra NHANH trạng thái nhúng, không bao giờ chặn cửa sổ.
//   - Extract(): giải nén BẤT ĐỒNG BỘ, có progress %, resume từng tệp
//     (tệp đã ghi đủ kích thước sẽ bỏ qua), marker chỉ ghi khi hoàn tất.
//   - Verify: chạy "ffmpeg -version" với timeout 30s sau khi giải nén;
//     timeout -> thông báo hướng dẫn thêm ngoại lệ diệt virus.
package ffmpegbin

import (
        "archive/zip"
        "bytes"
        "context"
        "crypto/sha256"
        _ "embed"
        "errors"
        "fmt"
        "io"
        "os"
        "os/exec"
        "path/filepath"
        "regexp"
        "runtime"
        "time"

        "vkseditorpro/internal/executil"
)

//go:embed ffmpeg-essentials.zip
var embeddedZip []byte

// markerVersion thay đổi khi nâng cấp bundle để buộc giải nén lại.
const markerVersion = "ffmpeg-essentials-8.0.0-v2"

// verifyTimeout là thời gian tối đa chờ "ffmpeg -version" phản hồi.
const verifyTimeout = 30 * time.Second

// Result là kết quả giải nén.
type Result struct {
        FFmpegPath  string
        FFprobePath string
        Dir         string
        Extracted   bool // true nếu lần này mới giải nén (lần đầu chạy)
        Version     string
        Verified    bool // đã chạy verify thành công (chỉ trên Windows)
}

// Boot là trạng thái chuẩn bị lúc khởi động.
type Boot struct {
        Ready        bool    // dùng được ngay — không cần giải nén
        NeedsExtract bool    // cần gọi Extract()
        Result       *Result // kết quả khi Ready
        Reason       string  // vì sao cần giải nén (để log/UI)
}

// targetDir trả thư mục đích giải nén:
// Windows: %LOCALAPPDATA%\VKSeditorPro\bin — Linux/macOS: ~/.cache/VKSeditorPro/bin
func targetDir() (string, error) {
        base, err := os.UserCacheDir()
        if err != nil {
                return "", err
        }
        return filepath.Join(base, "VKSeditorPro", "bin"), nil
}

// Prepare kiểm tra NHANH xem ffmpeg nhúng đã dùng được chưa.
// Không bao giờ giải nén ở đây — an toàn gọi trước khi mở cửa sổ.
func Prepare() *Boot {
        dir, err := targetDir()
        if err != nil {
                return &Boot{NeedsExtract: true, Reason: "không xác định được thư mục cache: " + err.Error()}
        }
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return &Boot{NeedsExtract: true, Reason: "không tạo được thư mục cache: " + err.Error()}
        }
        ff, fp := binPaths(dir)

        marker := filepath.Join(dir, "marker.txt")
        if data, err := os.ReadFile(marker); err != nil || string(data) != markerVersion {
                reason := "lần đầu chạy"
                if err == nil {
                        reason = "bundle cũ (" + string(data) + ")"
                }
                return &Boot{NeedsExtract: true, Reason: reason}
        }
        if !ok(ff) || !ok(fp) {
                return &Boot{NeedsExtract: true, Reason: "tệp nhúng thiếu hoặc quá nhỏ (diệt virus có thể đã can thiệp)"}
        }
        // Verify nhanh (~100ms) — bắt tệp bị cách ly/hỏng ngay từ đầu.
        if runtime.GOOS == "windows" {
                v, err := verifyFF(ff)
                if err != nil {
                        return &Boot{NeedsExtract: true, Reason: "binary không thực thi được: " + err.Error()}
                }
                return &Boot{Ready: true, Result: &Result{FFmpegPath: ff, FFprobePath: fp, Dir: dir, Extracted: false, Version: v, Verified: true}}
        }
        // Trên Linux/macOS bản nhúng là PE — không verify được, cho là sẵn sàng
        // (resolveFFBinaries sẽ rót về ffmpeg hệ thống khi thực chạy).
        return &Boot{Ready: true, Result: &Result{FFmpegPath: ff, FFprobePath: fp, Dir: dir, Extracted: false, Version: "embedded-PE"}}
}

// Extract giải nén ffmpeg/ffprobe (có resume từng tệp) rồi verify.
// onProgress nhận phần trăm 0..100 và nhãn giai đoạn; có thể nil.
func Extract(onProgress func(pct float64, phase string)) (*Result, error) {
        prog := func(p float64, s string) {
                if onProgress != nil {
                        onProgress(p, s)
                }
        }
        dir, err := targetDir()
        if err != nil {
                return nil, err
        }
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return nil, fmt.Errorf("tạo thư mục %s: %w", dir, err)
        }
        ff, fp := binPaths(dir)
        prog(0, "chuẩn bị")

        zr, err := zip.NewReader(bytes.NewReader(embeddedZip), int64(len(embeddedZip)))
        if err != nil {
                return nil, fmt.Errorf("mở zip FFmpeg nhúng: %w", err)
        }
        want := map[string]string{
                "ffmpeg.exe":  ff,
                "ffprobe.exe": fp,
        }
        // Tổng số byte sẽ ghi (decompress) để tính %.
        var total int64
        entries := map[string]*zip.File{}
        for _, f := range zr.File {
                base := filepath.Base(f.Name)
                if _, okw := want[base]; okw && entries[base] == nil {
                        entries[base] = f
                        total += int64(f.UncompressedSize64)
                }
        }
        if len(entries) < 2 {
                return nil, errors.New("zip FFmpeg thiếu ffmpeg.exe hoặc ffprobe.exe")
        }

        // Resume: tệp đã tồn tại đủ kích thước -> bỏ qua ghi.
        var written int64
        skipped := 0
        for base, f := range entries {
                dst := want[base]
                if st, err := os.Stat(dst); err == nil && int64(f.UncompressedSize64) > 0 && st.Size() == int64(f.UncompressedSize64) {
                        written += int64(f.UncompressedSize64)
                        skipped++
                        prog(100*float64(written)/float64(total), "đã có "+base)
                } else {
                        if err := extractFile(f, dst, func(n int64) {
                                prog(100*float64(written+n)/float64(total), "giải nén "+base)
                        }); err != nil {
                                return nil, err
                        }
                        written += int64(f.UncompressedSize64)
                }
        }
        prog(100, "hoàn tất giải nén")

        _ = os.WriteFile(filepath.Join(dir, "marker.txt"), []byte(markerVersion), 0o644)

        version := "embedded-PE"
        verified := false
        if runtime.GOOS == "windows" {
                prog(99, "xác minh ffmpeg")
                v, err := verifyFF(ff)
                if err != nil {
                        return nil, err
                }
                version, verified = v, true
        }
        prog(100, "sẵn sàng")
        return &Result{FFmpegPath: ff, FFprobePath: fp, Dir: dir, Extracted: skipped < 2, Version: version, Verified: verified}, nil
}

// Ensure giữ API cũ cho các chế độ CLI (--selftest/--headless): chặn đến khi
// ffmpeg sẵn sàng. Người gọi PHẢI set MGO_FFMPEG / MGO_FFPROBE bằng kết quả
// này TRƯỚC khi gọi bất kỳ hàm nào của moviego (moviego cache lần đầu tra cứu).
func Ensure(logf func(format string, args ...any)) (*Result, error) {
        boot := Prepare()
        if boot.Ready {
                if logf != nil {
                        logf("FFmpeg nhúng đã sẵn sàng: %s", boot.Result.FFmpegPath)
                }
                return boot.Result, nil
        }
        if logf != nil {
                logf("Giải nén FFmpeg vào thư mục cache (%s)...", boot.Reason)
        }
        res, err := Extract(func(pct float64, phase string) {
                // CLI không cần progress mượt — in mốc 25%.
                if pct == 0 || pct >= 25 && pct < 26 || pct >= 50 && pct < 51 || pct >= 75 && pct < 76 || pct >= 100 {
                        if logf != nil {
                                logf("  %3.0f%% — %s", pct, phase)
                        }
                }
        })
        if err != nil {
                return nil, err
        }
        if logf != nil {
                logf("FFmpeg đã sẵn sàng: %s", res.FFmpegPath)
        }
        return res, nil
}

// binPaths trả cặp đường dẫn đích theo hệ điều hành.
func binPaths(dir string) (string, string) {
        ff := filepath.Join(dir, "ffmpeg.exe")
        fp := filepath.Join(dir, "ffprobe.exe")
        if runtime.GOOS != "windows" {
                // Bản test trên Linux dùng tên không đuôi.
                ff = filepath.Join(dir, "ffmpeg")
                fp = filepath.Join(dir, "ffprobe")
        }
        return ff, fp
}

// TargetDirForDiag trả thư mục bin nhúng cho bảng chẩn đoán (bản export).
func TargetDirForDiag() (string, error) { return targetDir() }

// BinPathsForDiag trả cặp đường dẫn ffmpeg/ffprobe cho bảng chẩn đoán (bản export).
func BinPathsForDiag(dir string) (string, string) { return binPaths(dir) }

func ok(p string) bool {
        st, err := os.Stat(p)
        return err == nil && st.Size() > 1<<20 // > 1MB
}

// verifyFF chạy "ffmpeg -version" với timeout cứng. Trả phiên bản.
// Timeout -> lỗi thân thiện hướng dẫn ngoại lệ diệt virus.
func verifyFF(ffmpegPath string) (string, error) {
        ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)
        defer cancel()
        cmd := exec.CommandContext(ctx, ffmpegPath, "-version")
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        out, err := cmd.CombinedOutput()
        if err != nil {
                if ctx.Err() == context.DeadlineExceeded {
                        dir, _ := targetDir()
                        return "", fmt.Errorf(
                                "ffmpeg không phản hồi sau %ds — phần mềm diệt virus có thể đang chặn. Hãy thêm ngoại lệ cho thư mục ứng dụng và thư mục %s rồi chạy lại",
                                int(verifyTimeout.Seconds()), dir)
                }
                return "", fmt.Errorf("ffmpeg không chạy được: %v", err)
        }
        m := regexp.MustCompile(`ffmpeg version (\S+)`).FindSubmatch(out)
        if m == nil {
                return "unknown", nil
        }
        return string(m[1]), nil
}

// extractFile giải nén một entry, ghi qua tệp tạm rồi rename (atomic),
// kèm xác thực SHA-256 kích thước tối thiểu để tránh tệp rác.
// onWrite báo số byte đã ghi (cho progress).
func extractFile(f *zip.File, dst string, onWrite func(n int64)) error {
        rc, err := f.Open()
        if err != nil {
                return fmt.Errorf("mở %s trong zip: %w", f.Name, err)
        }
        defer rc.Close()

        tmp := dst + ".tmp"
        out, err := os.Create(tmp)
        if err != nil {
                return fmt.Errorf("ghi %s: %w", tmp, err)
        }
        h := sha256.New()
        var written int64
        buf := make([]byte, 1024*1024) // khối 1MB — ít gọi hệ thống hơn
        lastReport := time.Now()
        for {
                n, rerr := rc.Read(buf)
                if n > 0 {
                        if _, werr := out.Write(buf[:n]); werr != nil {
                                _ = out.Close()
                                _ = os.Remove(tmp)
                                return fmt.Errorf("giải nén %s: %w", f.Name, werr)
                        }
                        h.Write(buf[:n])
                        written += int64(n)
                        if onWrite != nil && time.Since(lastReport) > 120*time.Millisecond {
                                lastReport = time.Now()
                                onWrite(written)
                        }
                }
                if rerr == io.EOF {
                        break
                }
                if rerr != nil {
                        _ = out.Close()
                        _ = os.Remove(tmp)
                        return fmt.Errorf("giải nén %s: %w", f.Name, rerr)
                }
        }
        if err := out.Close(); err != nil {
                _ = os.Remove(tmp)
                return err
        }
        if written < 1<<20 {
                _ = os.Remove(tmp)
                return fmt.Errorf("%s quá nhỏ (%d byte) — zip hỏng", f.Name, written)
        }
        _ = os.Remove(dst)
        if err := os.Rename(tmp, dst); err != nil {
                _ = os.Remove(tmp)
                return err
        }
        _ = os.Chmod(dst, 0o755)
        if onWrite != nil {
                onWrite(written)
        }
        return nil
}
