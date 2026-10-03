// Package tools gom các công cụ hệ thống của VKSeditorPro v1.1 chạy ngoài
// engine moviego: phân tích khoảng lặng, phát hiện cảnh, kiểm tra QC, tạo
// giọng đọc (TTS), gọi API AI (STT/dịch/phân tích), sinh proxy 480p, cắt
// nhanh không re-encode và kiểm tra cập nhật qua GitHub Release.
package tools

import (
        "context"
        "fmt"
        "os"
        "os/exec"
        "regexp"
        "runtime"
        "strconv"
        "strings"
        "time"

        "vkseditorpro/internal/executil"
        "vkseditorpro/internal/ffmpegbin"
)

// ffmpegPaths trả cặp đường dẫn ffmpeg/ffprobe: ưu tiên env MGO_FFMPEG
// (do main set từ bản nhúng), rồi bản nhúng đã giải nén, rồi PATH.
func ffmpegPaths() (ff, fp string) {
        ff = os.Getenv("MGO_FFMPEG")
        fp = os.Getenv("MGO_FFPROBE")
        if ff != "" && !isBroken(ff) {
                return
        }
        if res, err := ffmpegbin.Ensure(nil); err == nil {
                if !isBroken(res.FFmpegPath) {
                        ff, fp = res.FFmpegPath, res.FFprobePath
                        return
                }
        }
        if p, err := exec.LookPath("ffmpeg"); err == nil {
                ff = p
        }
        if p, err := exec.LookPath("ffprobe"); err == nil {
                fp = p
        }
        return
}

// FFmpegBin trả đường dẫn ffmpeg để chạy trực tiếp.
func FFmpegBin() string {
        ff, _ := ffmpegPaths()
        if ff == "" {
                return "ffmpeg"
        }
        return ff
}

// FFprobeBin trả đường dẫn ffprobe.
func FFprobeBin() string {
        _, fp := ffmpegPaths()
        if fp == "" {
                return "ffprobe"
        }
        return fp
}

// isBroken kiểm tra binary có thực thi được trên hệ hiện tại không:
// PE (MZ) chỉ chạy trên Windows — kể cả khi tên file không có đuôi .exe
// (bản nhúng giải nén trên Linux chính là ffmpeg.exe với tên "ffmpeg").
func isBroken(p string) bool {
        if _, err := os.Stat(p); err != nil {
                return true
        }
        if runtime.GOOS == "windows" {
                return false
        }
        b := make([]byte, 2)
        f, err := os.Open(p)
        if err != nil {
                return true
        }
        _, err = f.Read(b)
        _ = f.Close()
        if err != nil {
                return true
        }
        return b[0] == 'M' && b[1] == 'Z' // PE → không chạy trên Unix
}

// runFF chạy ffmpeg, trả output (stderr chứa kết quả các filter detection).
func runFF(ctx context.Context, bin string, args []string, timeout time.Duration) (string, error) {
        cctx := ctx
        var cancel context.CancelFunc
        if timeout > 0 {
                cctx, cancel = context.WithTimeout(ctx, timeout)
                defer cancel()
        }
        cmd := exec.CommandContext(cctx, bin, args...)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        out, err := cmd.CombinedOutput()
        return string(out), err
}

// Silence là một khoảng lặng phát hiện được.
type Silence struct {
        StartMs int64 `json:"startMs"`
        EndMs   int64 `json:"endMs"`
}

var (
        reSilenceStart = regexp.MustCompile(`silence_start:\s*(-?[\d.]+)`)
        reSilenceEnd   = regexp.MustCompile(`silence_end:\s*([\d.]+)`)
)

// DetectSilence phát hiện khoảng lặng bằng filter silencedetect.
// noiseDb: ngưỡng (vd -40), minDurMs: khoảng ngắn nhất tính là lặng.
func DetectSilence(ctx context.Context, path string, noiseDb float64, minDurMs int64) ([]Silence, error) {
        minDur := float64(minDurMs) / 1000
        if minDur <= 0 {
                minDur = 0.4
        }
        args := []string{"-hide_banner", "-nostdin", "-i", path,
                "-af", fmt.Sprintf("silencedetect=noise=%.0fdB:d=%.2f", noiseDb, minDur),
                "-f", "null", "-"}
        out, err := runFF(ctx, FFmpegBin(), args, 15*time.Minute)
        if err != nil && !strings.Contains(out, "silence_") {
                return nil, fmt.Errorf("silencedetect: %w: %s", err, tail(out))
        }
        returns := reSilenceStart.FindAllStringSubmatch(out, -1)
        ends := reSilenceEnd.FindAllStringSubmatch(out, -1)
        var res []Silence
        for i, s := range returns {
                startSec, _ := strconv.ParseFloat(s[1], 64)
                if startSec < 0 {
                        startSec = 0
                }
                var endSec float64
                if i < len(ends) {
                        endSec, _ = strconv.ParseFloat(ends[i][1], 64)
                }
                res = append(res, Silence{
                        StartMs: int64(startSec * 1000),
                        EndMs:   int64(endSec * 1000),
                })
        }
        return res, nil
}

// SceneMark là một điểm cắt cảnh phát hiện được.
type SceneMark struct {
        AtMs  int64   `json:"atMs"`
        Score float64 `json:"score"`
}

var reScenePts = regexp.MustCompile(`pts_time:([\d.]+)`)
var reSceneSc = regexp.MustCompile(`scene_score=([\d.]+)`)

// DetectScenes phát hiện điểm chuyển cảnh bằng filter select scene.
// threshold 0..1 (0.3 nhạy, 0.5 chuẩn).
func DetectScenes(ctx context.Context, path string, threshold float64) ([]SceneMark, error) {
        if threshold <= 0 {
                threshold = 0.4
        }
        args := []string{"-hide_banner", "-nostdin", "-i", path,
                "-vf", fmt.Sprintf("select='gt(scene,%.2f)',showinfo", threshold),
                "-an", "-f", "null", "-"}
        out, err := runFF(ctx, FFmpegBin(), args, 20*time.Minute)
        if err != nil && !strings.Contains(out, "pts_time") {
                return nil, fmt.Errorf("scene detect: %w: %s", err, tail(out))
        }
        var res []SceneMark
        for _, ln := range strings.Split(out, "\n") {
                if !strings.Contains(ln, "showinfo") || !strings.Contains(ln, "pts_time") {
                        continue
                }
                m := reScenePts.FindStringSubmatch(ln)
                if m == nil {
                        continue
                }
                sec, _ := strconv.ParseFloat(m[1], 64)
                score := 0.0
                if sm := reSceneSc.FindStringSubmatch(ln); sm != nil {
                        score, _ = strconv.ParseFloat(sm[1], 64)
                }
                res = append(res, SceneMark{AtMs: int64(sec * 1000), Score: score})
        }
        return res, nil
}

// QCProblem là một vấn đề QC phát hiện được.
type QCProblem struct {
        Level   string `json:"level"` // "warn" | "info"
        Kind    string `json:"kind"`
        Message string `json:"message"`
}

// QCReport là kết quả kiểm tra chất lượng của một tệp.
type QCReport struct {
        File         string      `json:"file"`
        DurationMs   int64       `json:"durationMs"`
        MeanVolumeDb *float64    `json:"meanVolumeDb,omitempty"`
        MaxVolumeDb  *float64    `json:"maxVolumeDb,omitempty"`
        BlackRanges  []Silence   `json:"blackRanges,omitempty"`
        Problems     []QCProblem `json:"problems,omitempty"`
}

var (
        reBlackStart = regexp.MustCompile(`black_start:([\d.]+)\s+black_end:([\d.]+)`)
        reMeanVol    = regexp.MustCompile(`mean_volume:\s*(-?[\d.]+) dB`)
        reMaxVol     = regexp.MustCompile(`max_volume:\s*(-?[\d.]+) dB`)
        reDuration   = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+\.?\d*)`)
)

// RunQC quét một tệp: phát hiện khung đen (blackdetect) + đo âm lượng
// (volumedetect), trả báo cáo kèm danh sách cảnh báo.
func RunQC(ctx context.Context, path string) (*QCReport, error) {
        rep := &QCReport{File: path}
        args := []string{"-hide_banner", "-nostdin", "-i", path,
                "-vf", "blackdetect=d=0.5:pix_th=0.10",
                "-af", "volumedetect",
                "-f", "null", "-"}
        out, err := runFF(ctx, FFmpegBin(), args, 20*time.Minute)
        if err != nil && !strings.Contains(out, "blackdetect") && !strings.Contains(out, "mean_volume") {
                return nil, fmt.Errorf("qc: %w: %s", err, tail(out))
        }
        if m := reDuration.FindStringSubmatch(out); m != nil {
                h, _ := strconv.ParseFloat(m[1], 64)
                mi, _ := strconv.ParseFloat(m[2], 64)
                s, _ := strconv.ParseFloat(m[3], 64)
                rep.DurationMs = int64((h*3600 + mi*60 + s) * 1000)
        }
        for _, m := range reBlackStart.FindAllStringSubmatch(out, -1) {
                s, _ := strconv.ParseFloat(m[1], 64)
                e, _ := strconv.ParseFloat(m[2], 64)
                rep.BlackRanges = append(rep.BlackRanges, Silence{StartMs: int64(s * 1000), EndMs: int64(e * 1000)})
        }
        if m := reMeanVol.FindStringSubmatch(out); m != nil {
                v, _ := strconv.ParseFloat(m[1], 64)
                rep.MeanVolumeDb = &v
        }
        if m := reMaxVol.FindStringSubmatch(out); m != nil {
                v, _ := strconv.ParseFloat(m[1], 64)
                rep.MaxVolumeDb = &v
        }
        for _, b := range rep.BlackRanges {
                rep.Problems = append(rep.Problems, QCProblem{
                        Level: "warn", Kind: "black",
                        Message: fmt.Sprintf("Nền đen %.1fs → %.1fs", float64(b.StartMs)/1000, float64(b.EndMs)/1000),
                })
        }
        if rep.MeanVolumeDb != nil {
                if *rep.MeanVolumeDb < -35 {
                        rep.Problems = append(rep.Problems, QCProblem{
                                Level: "warn", Kind: "quiet",
                                Message: fmt.Sprintf("Âm lượng trung bình rất nhỏ (%.1f dB) — có thể im lặng", *rep.MeanVolumeDb),
                        })
                }
                if *rep.MaxVolumeDb > -0.5 {
                        rep.Problems = append(rep.Problems, QCProblem{
                                Level: "warn", Kind: "clip",
                                Message: fmt.Sprintf("Âm lượng đỉnh %.1f dB — nguy cơ méo tiếng (clipping)", *rep.MaxVolumeDb),
                        })
                }
        }
        if len(rep.Problems) == 0 {
                rep.Problems = append(rep.Problems, QCProblem{Level: "info", Kind: "ok", Message: "Không phát hiện vấn đề nào"})
        }
        return rep, nil
}

func tail(s string) string {
        if len(s) > 300 {
                return s[len(s)-300:]
        }
        return s
}
