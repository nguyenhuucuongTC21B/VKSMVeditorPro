package tools

import (
        "context"
        "fmt"
        "os"
        "os/exec"
        "runtime"
        "strings"
        "time"

        "vkseditorpro/internal/executil"
)

// VoiceInfo là một giọng đọc TTS khả dụng trên hệ thống.
type VoiceInfo struct {
        Name     string `json:"name"`
        Language string `json:"language"`
        Gender   string `json:"gender"`
}

// ListTTSEntries liệt kê giọng đọc cài sẵn. Chỉ hỗ trợ Windows (SAPI qua
// PowerShell) — hệ khác trả lỗi rõ ràng để UI hiển thị.
func ListTTSEntries() ([]VoiceInfo, error) {
        if runtime.GOOS != "windows" {
                return nil, fmt.Errorf("TTS ngoài máy chỉ có trên Windows (SAPI). Hãy cấu hình API AI để dùng giọng đọc đám mây")
        }
        script := `Add-Type -AssemblyName System.Speech
$s = New-Object System.Speech.Synthesis.SpeechSynthesizer
$s.GetInstalledVoices() | ForEach-Object {
  $v = $_.VoiceInfo
  Write-Output ($v.Name + "|" + $v.Culture + "|" + $v.Gender)
}`
        out, err := runPS(script, 30*time.Second)
        if err != nil {
                return nil, fmt.Errorf("liệt kê giọng đọc: %w: %s", err, tail(out))
        }
        var res []VoiceInfo
        for _, ln := range strings.Split(out, "\n") {
                ln = strings.TrimSpace(ln)
                if ln == "" || !strings.Contains(ln, "|") {
                        continue
                }
                parts := strings.SplitN(ln, "|", 3)
                v := VoiceInfo{Name: strings.TrimSpace(parts[0])}
                if len(parts) > 1 {
                        v.Language = strings.TrimSpace(parts[1])
                }
                if len(parts) > 2 {
                        v.Gender = strings.TrimSpace(parts[2])
                }
                res = append(res, v)
        }
        return res, nil
}

// TTSGenerate tạo file .wav đọc to text bằng giọng Windows SAPI.
// rate: -10..10 (0 = thường). Trả đường dẫn file wav đã tạo.
func TTSGenerate(ctx context.Context, text, voice string, rate int, outPath string) (string, error) {
        if runtime.GOOS != "windows" {
                return "", fmt.Errorf("TTS ngoài máy chỉ có trên Windows (SAPI)")
        }
        if strings.TrimSpace(text) == "" {
                return "", fmt.Errorf("nội dung trống")
        }
        if rate < -10 {
                rate = -10
        }
        if rate > 10 {
                rate = 10
        }
        ps := fmt.Sprintf(`Add-Type -AssemblyName System.Speech
$s = New-Object System.Speech.Synthesis.SpeechSynthesizer
$s.SetOutputToWaveFile('%s')
$s.Rate = %d
if ('%s' -ne '') { $s.SelectVoice('%s') }
$s.Speak('%s')
$s.Dispose()`, psEscape(outPath), rate, psEscape(voice), psEscape(voice), psEscape(text))
        out, err := runPSWithContext(ctx, ps, 10*time.Minute)
        if err != nil {
                return "", fmt.Errorf("tạo giọng đọc: %w: %s", err, tail(out))
        }
        st, err := os.Stat(outPath)
        if err != nil || st.Size() < 100 {
                return "", fmt.Errorf("file giọng đọc không được tạo: %s", outPath)
        }
        return outPath, nil
}

// psEscape thoát chuỗi cho nháy đơn trong PowerShell.
func psEscape(s string) string {
        return strings.ReplaceAll(s, "'", "''")
}

// runPS chạy script PowerShell với timeout.
func runPS(script string, timeout time.Duration) (string, error) {
        return runPSWithContext(context.Background(), script, timeout)
}

// runPSWithContext chạy PowerShell (Windows) hoặc pwsh (fallback) kèm ctx.
func runPSWithContext(ctx context.Context, script string, timeout time.Duration) (string, error) {
        cctx, cancel := context.WithTimeout(ctx, timeout)
        defer cancel()
        var cmd *exec.Cmd
        if runtime.GOOS == "windows" {
                cmd = exec.CommandContext(cctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", script)
                executil.Hide(cmd) // v1.2.7: không hiện cửa sổ PowerShell trên Windows
        } else {
                if p, err := exec.LookPath("pwsh"); err != nil {
                        return "", fmt.Errorf("không có PowerShell trên hệ thống này")
                } else {
                        cmd = exec.CommandContext(cctx, p, "-NoProfile", "-Command", script)
                }
        }
        executil.Hide(cmd) // v1.2.7: bảo đảm cả nhánh pwsh cũng ẩn
        out, err := cmd.CombinedOutput()
        return string(out), err
}
