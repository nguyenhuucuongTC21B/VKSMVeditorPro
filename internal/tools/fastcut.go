package tools

import (
        "context"
        "encoding/json"
        "fmt"
        "net/http"
        "os"
        "os/exec"
        "path/filepath"
        "regexp"
        "strconv"
        "strings"
        "time"

        mgo "github.com/mowshon/moviego/v2"

        "vkseditorpro/internal/executil"
)

// UpdateInfo là kết quả kiểm tra phiên bản mới qua GitHub Release.
type UpdateInfo struct {
        CurrentVersion string `json:"currentVersion"`
        LatestVersion  string `json:"latestVersion,omitempty"`
        UpdateURL      string `json:"updateUrl,omitempty"`
        AssetURL       string `json:"assetUrl,omitempty"`
        Notes          string `json:"notes,omitempty"`
        HasUpdate      bool   `json:"hasUpdate"`
        Error          string `json:"error,omitempty"`
}

var reSemver = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)

// CheckUpdate truy vấn releases/latest của repo GitHub, so sánh với current.
// Offline an toàn: timeout 8s, lỗi trả trong UpdateInfo.Error.
func CheckUpdate(repo, current string) UpdateInfo {
        info := UpdateInfo{CurrentVersion: current}
        url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
        cctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
        defer cancel()
        req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
        if err != nil {
                info.Error = err.Error()
                return info
        }
        req.Header.Set("Accept", "application/vnd.github+json")
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
                info.Error = "không kết nối được GitHub (chế độ offline?): " + err.Error()
                return info
        }
        defer resp.Body.Close()
        if resp.StatusCode != http.StatusOK {
                info.Error = fmt.Sprintf("GitHub trả %d — kiểm tra lại tên repo trong Cài đặt", resp.StatusCode)
                return info
        }
        var rel struct {
                TagName string `json:"tag_name"`
                HTMLURL string `json:"html_url"`
                Body    string `json:"body"`
                Assets  []struct {
                        Name               string `json:"name"`
                        BrowserDownloadURL string `json:"browser_download_url"`
                } `json:"assets"`
        }
        if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
                info.Error = err.Error()
                return info
        }
        latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
        info.LatestVersion = latest
        info.UpdateURL = rel.HTMLURL
        info.Notes = rel.Body
        if newer(latest, current) {
                info.HasUpdate = true
                for _, a := range rel.Assets {
                        if strings.EqualFold(filepath.Ext(a.Name), ".exe") {
                                info.AssetURL = a.BrowserDownloadURL
                                break
                        }
                }
        }
        return info
}

// newer so sánh semver a > b.
func newer(a, b string) bool {
        pa := semverParts(a)
        pb := semverParts(b)
        for i := 0; i < 3; i++ {
                if pa[i] > pb[i] {
                        return true
                }
                if pa[i] < pb[i] {
                        return false
                }
        }
        return false
}

func semverParts(s string) [3]int {
        m := reSemver.FindStringSubmatch(s)
        var r [3]int
        if m == nil {
                return r
        }
        r[0], _ = strconv.Atoi(m[1])
        r[1], _ = strconv.Atoi(m[2])
        r[2], _ = strconv.Atoi(m[3])
        return r
}

// StreamCopyTrim cắt nhanh [startMs, startMs+durMs) không re-encode
// (chính xác tới keyframe). durMs <= 0 = tới cuối file.
func StreamCopyTrim(ctx context.Context, in, out string, startMs, durMs int64) error {
        start := mgo.Time(time.Duration(startMs) * time.Millisecond)
        var dur mgo.Time
        if durMs > 0 {
                dur = mgo.Time(time.Duration(durMs) * time.Millisecond)
        }
        return mgo.StreamCopyTrim(ctx, in, out, start, dur)
}

// QuickConcat nối nhanh danh sách file (ưu tiên concat demuxer không re-encode,
// tự rơi về re-encode khi codec khác nhau).
func QuickConcat(ctx context.Context, out string, files []string) error {
        return mgo.ConcatFiles(ctx, out, files...)
}

// GenerateProxy tạo bản proxy 480p cho xem trước mượt (CRF 30, veryfast).
// Trả đường dẫn file; nếu đã tồn tại thì trả luôn.
func GenerateProxy(ctx context.Context, src, outPath string, height int) (string, error) {
        if st, err := os.Stat(outPath); err == nil && st.Size() > 10_000 {
                return outPath, nil
        }
        if height <= 0 || height%2 != 0 {
                height = 480
        }
        args := []string{"-y", "-nostdin", "-i", src,
                "-vf", fmt.Sprintf("scale=-2:%d", height),
                "-c:v", "libx264", "-crf", "30", "-preset", "veryfast",
                "-c:a", "aac", "-b:a", "96k", "-movflags", "+faststart", outPath}
        cmd := exec.CommandContext(ctx, FFmpegBin(), args...)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        if b, err := cmd.CombinedOutput(); err != nil {
                return "", fmt.Errorf("tạo proxy: %w: %s", err, tail(string(b)))
        }
        st, err := os.Stat(outPath)
        if err != nil || st.Size() < 10_000 {
                return "", fmt.Errorf("proxy không được tạo đúng cách: %s", outPath)
        }
        return outPath, nil
}
