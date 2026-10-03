// Command selftest kiểm chứng toàn bộ pipeline engine của VKSeditorPro
// (không cần giao diện): sinh video mẫu bằng ffmpeg → tạo Document → biên dịch
// graph moviego → xuất MP4 + GIF + khung hình xem trước → probe kết quả.
// Chạy được trên Linux (dùng cho CI/dev) và Windows (--selftest của exe chính).
package selftest

import (
        "context"
        "fmt"
        "io"
        "os"
        "os/exec"
        "path/filepath"
        "runtime"
        "time"

        "vkseditorpro/internal/engine"
        "vkseditorpro/internal/executil"
        "vkseditorpro/internal/ffmpegbin"
        "vkseditorpro/internal/media"
        "vkseditorpro/internal/project"
)

var failed int

// testOut là nơi ghi log của lần chạy selftest hiện tại.
var testOut io.Writer = os.Stdout

func step(name string, err error) bool {
        if err != nil {
                fmt.Fprintf(testOut, "  [LỖI] %s: %v\n", name, err)
                failed++
                return false
        }
        fmt.Fprintf(testOut, "  [OK]  %s\n", name)
        return true
}

// genTestVideo sinh video mẫu: màu testsrc2 + tiếng sine, thời lượng s giây.
func genTestVideo(ffmpegBin, dest string, secs int, size string) error {
        ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
        defer cancel()
        cmd := exec.CommandContext(ctx, ffmpegBin, "-y",
                "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%s:rate=30:duration=%d", size, secs),
                "-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=%d:duration=%d", 440+secs*40, secs),
                "-c:v", "libx264", "-preset", "ultrafast", "-crf", "28",
                "-c:a", "aac", "-shortest", dest)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        if b, err := cmd.CombinedOutput(); err != nil {
                return fmt.Errorf("ffmpeg sinh video mẫu: %v: %s", err, tail(string(b)))
        }
        return nil
}

func tail(s string) string {
        if len(s) > 400 {
                return s[len(s)-400:]
        }
        return s
}

// Run thực hiện toàn bộ selftest, ghi kết quả ra out. Trả về 0 nếu thành công.
func Run(out io.Writer) int {
        testOut = out
        fmt.Fprintln(out, "=== VKSeditorPro — SELFTEST ENGINE ===")
        work, err := os.MkdirTemp("", "vks-selftest-")
        if err != nil {
                panic(err)
        }
        defer os.RemoveAll(work)
        fmt.Fprintln(out, "Thư mục làm việc:", work)

        // 1) FFmpeg nhúng → giải nén → set env (đúng luồng như main app).
        res, err := ffmpegbin.Ensure(func(f string, a ...any) { fmt.Fprintf(out, "  … "+f+"\n", a...) })
        if !step("giải nén ffmpeg nhúng", err) {
                return 1
        }
        os.Setenv("MGO_FFMPEG", res.FFmpegPath)
        os.Setenv("MGO_FFPROBE", res.FFprobePath)

        // Trên Linux nếu binary nhúng là exe Windows (không thực thi được) thì
        // chuyển sang ffmpeg hệ thống/static — nhưng vẫn chứng minh zip nhúng OK.
        ffBin := res.FFmpegPath
        if runtime.GOOS != "windows" && isWinExe(ffBin) {
                if p, err := exec.LookPath("ffmpeg"); err == nil {
                        ffBin = p
                        if probe, err2 := exec.LookPath("ffprobe"); err2 == nil {
                                os.Setenv("MGO_FFMPEG", p)
                                os.Setenv("MGO_FFPROBE", probe)
                                fmt.Fprintln(out, "  (Trên Linux: dùng ffmpeg hệ thống để thực thi, exe nhúng đã xác minh)")
                        }
                }
        }
        fmt.Fprintln(out, "Dùng ffmpeg:", ffBin)

        // 2) Sinh tư liệu mẫu.
        v1 := filepath.Join(work, "a.mp4")
        v2 := filepath.Join(work, "b.mp4")
        if step("sinh video mẫu 5s", genTestVideo(ffBin, v1, 5, "1280x720")) &&
                step("sinh video mẫu 4s", genTestVideo(ffBin, v2, 4, "640x480")) {
        } else {
                return 1
        }

        // 3) Probe tư liệu.
        a1, err := media.Probe(v1)
        if !step("probe a.mp4", err) {
                return 1
        }
        a1.ID = project.NewID("a")
        step(fmt.Sprintf("probe a.mp4 → %dx%d @%.3f fps, %d ms, audio=%v",
                a1.Width, a1.Height, a1.Rate.Float(), a1.DurationMs, a1.HasAudio), nil)

        a2, err := media.Probe(v2)
        if !step("probe b.mp4", err) {
                return 1
        }
        a2.ID = project.NewID("a")

        // Sinh ảnh tĩnh + âm thanh mẫu để test các nhánh image/audio.
        imgPng := filepath.Join(work, "img.png")
        wavP := filepath.Join(work, "tone.wav")
        imgCmd := exec.Command(ffBin, "-y", "-f", "lavfi",
                "-i", "testsrc=size=800x600", "-frames:v", "1", imgPng)
        executil.Hide(imgCmd) // v1.2.7: không hiện console trên Windows
        step("sinh ảnh mẫu PNG", imgCmd.Run())
        wavCmd := exec.Command(ffBin, "-y", "-f", "lavfi",
                "-i", "sine=frequency=600:duration=3", wavP)
        executil.Hide(wavCmd) // v1.2.7: không hiện console trên Windows
        step("sinh âm thanh mẫu WAV", wavCmd.Run())
        a3, err := media.Probe(imgPng)
        if !step("probe img.png (ảnh tĩnh)", err) {
                return 1
        }
        a3.ID = project.NewID("a")
        step(fmt.Sprintf("probe img.png → %dx%d kind=%s", a3.Width, a3.Height, a3.Kind), nil)
        a4, err := media.Probe(wavP)
        if !step("probe tone.wav (âm thanh)", err) {
                return 1
        }
        a4.ID = project.NewID("a")
        step(fmt.Sprintf("probe tone.wav → %d ms kind=%s", a4.DurationMs, a4.Kind), nil)

        // 4) Dựng Document: clip video + ảnh tĩnh, xoay/lật/zoom, 3 chuyển cảnh.
        doc := project.NewDocument()
        doc.Assets = []*project.Asset{a1, a2, a3}
        c1 := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 3000, Volume: 1,
                Transform: project.Transform{Zoom: 1}}
        c2 := &project.Clip{ID: project.NewID("c"), AssetID: a2.ID, InMs: 500, OutMs: 3500, Volume: 1,
                Transform: project.Transform{RotateDeg: 90, Zoom: 1.2}}
        c3 := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 1000, OutMs: 4500, Volume: 0.5,
                Transform: project.Transform{FlipH: true, Zoom: 1}}
        c4 := &project.Clip{ID: project.NewID("c"), AssetID: a3.ID, InMs: 0, OutMs: 2000, Volume: 1,
                Transform: project.Transform{Zoom: 1.5}}
        doc.Clips = []*project.Clip{c1, c2, c3, c4}
        doc.Transitions = []*project.Transition{
                {AfterClipID: c1.ID, Type: project.TransCrossfade, DurationMs: 800},
                {AfterClipID: c2.ID, Type: project.TransWipeLeft, DurationMs: 600},
                {AfterClipID: c3.ID, Type: project.TransPushUp, DurationMs: 500},
        }
        doc.SortTransitions()
        fmt.Fprintf(out, "  Tổng thời lượng timeline: %d ms\n", doc.TimelineTotalMs())

        // 5) Biên dịch + xuất MP4.
        ctx := context.Background()
        g, err := engine.Compile(doc, engine.CompileOptions{})
        if !step("biên dịch graph MP4", err) {
                return 1
        }
        outMp4 := filepath.Join(work, "out.mp4")
        lastPct := -1
        err = engine.WriteVideo(ctx, g, engine.ExportRequest{
                Format:     engine.FormatMP4,
                OutputPath: outMp4,
                CRF:        22,
                Preset:     "veryfast",
        }, func(done, total int) {
                if total > 0 {
                        pct := done * 100 / total
                        if pct/20 != lastPct/20 {
                                lastPct = pct
                                fmt.Fprintf(out, "  … tiến trình %d%% (%d/%d khung)\n", pct, done, total)
                        }
                }
        })
        g.Close()
        step(fmt.Sprintf("xuất MP4 → %s", outMp4), err)

        // 6) Probe kết quả MP4.
        if p, err := media.Probe(outMp4); err == nil {
                step(fmt.Sprintf("probe out.mp4 → %dx%d, %.2f s, audio=%v",
                        p.Width, p.Height, float64(p.DurationMs)/1000, p.HasAudio), nil)
        } else {
                step("probe out.mp4", err)
        }

        // 7) Xuất GIF.
        g2, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 360})
        if !step("biên dịch graph GIF (360p)", err) {
                return 1
        }
        outGif := filepath.Join(work, "out.gif")
        err = engine.WriteVideo(ctx, g2, engine.ExportRequest{
                Format:     engine.FormatGIF,
                OutputPath: outGif,
                GifFPS:     12,
        }, nil)
        g2.Close()
        step(fmt.Sprintf("xuất GIF → %s", outGif), err)
        if st, err := os.Stat(outGif); err == nil {
                fmt.Fprintf(out, "  Kích thước GIF: %.1f KB\n", float64(st.Size())/1024)
        }

        // 8) Khung hình xem trước của clip (WYSIWYG).
        img, err := engine.RenderClipFrame(ctx, doc, c2.ID, 500, 85)
        step(fmt.Sprintf("render khung xem trước clip xoay 90° (%v)", img != nil), err)

        // 9) Thumbnail tư liệu.
        thumbDir := filepath.Join(work, "thumbs")
        tp, err := media.EnsureThumbnail(ctx, a1, thumbDir, 1000, 320)
        step(fmt.Sprintf("thumbnail asset → %s", filepath.Base(tp)), err)

        // 10) Xuất với độ phân giải preset 720p.
        g3, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 720})
        if !step("biên dịch graph 720p", err) {
                return 1
        }
        out720 := filepath.Join(work, "out720.mp4")
        err = engine.WriteVideo(ctx, g3, engine.ExportRequest{
                Format: engine.FormatMP4, OutputPath: out720, CRF: 24, Preset: "ultrafast",
        }, nil)
        g3.Close()
        step("xuất MP4 720p", err)
        if p, err := media.Probe(out720); err == nil {
                step(fmt.Sprintf("probe out720.mp4 → %dx%d", p.Width, p.Height), nil)
        }

        // ================================================================
        // v1.1 — Nhóm test tính năng mới
        // ================================================================
        fmt.Fprintln(out, "--- V1.1: hiệu ứng / tốc độ / boomerang / freeze ---")
        if code := runV11Tests(out, work, ffBin, ctx, a1, a2, a3); code != 0 {
                return code
        }

        // ================================================================
        // v1.2.3 — Nhóm test tính năng mới
        // ================================================================
        if code := runV123Tests(out, work, ffBin, ctx, a1, a2); code != 0 {
                return code
        }

        // ================================================================
        // v1.3.0 — Track âm thanh độc lập A1
        // ================================================================
        if code := runV130Tests(out, work, ffBin, ctx, a1, a4); code != 0 {
                return code
        }

        // ================================================================
        // v1.3.1 — Chế độ lớp kiểu KineMaster (blend/preset/fade)
        // ================================================================
        if code := runV131Tests(out, work, ffBin, ctx, a1, a3); code != 0 {
                return code
        }

        fmt.Fprintln(out, "=== KẾT QUẢ:", map[bool]string{true: "THÀNH CÔNG ✓", false: "THẤT BẠI ✗"}[failed == 0], "===")
        if failed > 0 {
                fmt.Fprintln(out, "Số bước lỗi:", failed)
                return 1
        }
        return 0
}

func isWinExe(p string) bool {
        b, err := os.ReadFile(p)
        if err != nil {
                return false
        }
        return len(b) > 2 && b[0] == 'M' && b[1] == 'Z'
}
