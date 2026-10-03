// v11tests.go — bộ kiểm tra v1.1: hiệu ứng + chỉnh màu, LUT, tốc độ, phát
// ngược, boomerang, freeze, keyframe, lớp phủ (chữ/hình/PiP), phụ đề SRT,
// watermark, timecode, credits, alpha export, stream-copy trim, proxy,
// silencedetect, scene detect, QC và round-trip SRT.
package selftest

import (
        "context"
        "fmt"
        "io"
        "os"
        "os/exec"
        "path/filepath"

        "vkseditorpro/internal/engine"
        "vkseditorpro/internal/executil"
        "vkseditorpro/internal/media"
        "vkseditorpro/internal/project"
        "vkseditorpro/internal/tools"
)

// approx so sánh ms sai số cho phép.
func approx(got, want, tolMs int64) bool {
        d := got - want
        if d < 0 {
                d = -d
        }
        return d <= tolMs
}

// runV11Tests chạy toàn bộ nhóm test v1.1; trả 0 nếu mọi bước OK.
func runV11Tests(out io.Writer, work, ffBin string, ctx context.Context,
        a1, a2, a3 *project.Asset) int {

        // ---------------------------------------------------------------
        // T1 — Clip hiệu ứng + chỉnh màu + LUT + keyframe + reverse
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: hiệu ứng / chỉnh màu / LUT / reverse ---")
        doc := project.NewDocument()
        doc.Canvas = project.Canvas{Width: 640, Height: 360, Rate: project.Rate{Num: 30, Den: 1}}
        doc.Assets = []*project.Asset{a1, a2, a3}

        // Viết file .cube LUT tối giản (2x2x2): giữ gần nguyên màu.
        cube := filepath.Join(work, "mini.cube")
        _ = os.WriteFile(cube, []byte(miniCube()), 0o644)

        fx := project.ClipEffects{
                Brightness: 0.05, Contrast: 1.15, Saturation: 1.3, Gamma: 1.1,
                Blur: 0, Sharpen: 0.8, Vignette: 0.35, Grayscale: false, Invert: false,
        }
        cFx := &project.Clip{
                ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 2000,
                Volume: 1, Transform: project.Transform{Zoom: 1},
                Effects: fx, LUTPath: cube,
                Reverse: true,
        }
        doc.Clips = []*project.Clip{cFx}
        g, err := engine.Compile(doc, engine.CompileOptions{})
        if !step("biên dịch clip hiệu ứng + LUT + reverse", err) {
                return 1
        }
        outFx := filepath.Join(work, "v11-fx.mp4")
        err = engine.WriteVideo(ctx, g, engine.ExportRequest{
                Format: engine.FormatMP4, OutputPath: outFx, CRF: 26, Preset: "ultrafast",
        }, nil)
        g.Close()
        step("xuất clip hiệu ứng + LUT + reverse (2s)", err)
        if err == nil {
                if p, e := media.Probe(outFx); e == nil {
                        step(fmt.Sprintf("thời lượng reverse 2s → %.2fs", float64(p.DurationMs)/1000),
                                check(approx(p.DurationMs, 2000, 300)))
                }
        }

        // ---------------------------------------------------------------
        // T2 — Tốc độ + boomerang + freeze: 2s / 2x * 2 (boomerang) + 0.5s freeze = 2.5s
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: tốc độ / boomerang / freeze ---")
        doc2 := project.NewDocument()
        doc2.Canvas = doc.Canvas
        doc2.Assets = doc.Assets
        cSpd := &project.Clip{
                ID: project.NewID("c"), AssetID: a2.ID, InMs: 0, OutMs: 2000,
                Volume: 1, Speed: 2, Boomerang: true, FreezeEndMs: 500,
        }
        doc2.Clips = []*project.Clip{cSpd}
        step("EffectiveMs = 2500 (2s/2x ×2 boomerang +500ms freeze)",
                check(doc2.TimelineTotalMs() == 2500))
        g2, err := engine.Compile(doc2, engine.CompileOptions{})
        if !step("biên dịch tốc độ + boomerang + freeze", err) {
                return 1
        }
        outBm := filepath.Join(work, "v11-boomerang.mp4")
        err = engine.WriteVideo(ctx, g2, engine.ExportRequest{
                Format: engine.FormatMP4, OutputPath: outBm, CRF: 26, Preset: "ultrafast",
        }, nil)
        g2.Close()
        step("xuất boomerang + freeze", err)
        if err == nil {
                if p, e := media.Probe(outBm); e == nil {
                        step(fmt.Sprintf("thời lượng boomerang 2.5s → %.2fs", float64(p.DurationMs)/1000),
                                check(approx(p.DurationMs, 2500, 400)))
                }
        }

        // ---------------------------------------------------------------
        // T3 — Lớp phủ + phụ đề + watermark + timecode + credits
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: lớp phủ / phụ đề / watermark / timecode / credits ---")
        doc3 := project.NewDocument()
        doc3.Canvas = doc.Canvas
        doc3.Assets = doc.Assets
        cMain := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 4000,
                Volume: 1, Transform: project.Transform{Zoom: 1}}
        doc3.Clips = []*project.Clip{cMain}
        doc3.Overlays = []*project.Overlay{
                {ID: "ov1", Kind: project.OverlayText, StartMs: 500, EndMs: 2500,
                        PosX: 0.5, PosY: 0.2, Text: "Xin chào VKSeditorPro", FontSize: 56,
                        ColorHex: "FFD54A", Outline: true, Anim: project.TextAnimFadeIn, AnimMs: 400},
                {ID: "ov2", Kind: project.OverlayShape, StartMs: 0, EndMs: 1500,
                        PosX: 0.8, PosY: 0.75, Shape: "circle", ColorHex: "E53935",
                        WidthPct: 12, HeightPct: 12},
                {ID: "ov3", Kind: project.OverlayMedia, StartMs: 2000, EndMs: 4000,
                        PosX: 0.85, PosY: 0.85, AssetID: a2.ID, ScalePct: 25, Muted: true},
        }
        doc3.Subs = &project.SubtitlesTrack{
                SourceName: "test", Burn: true, FontSize: 36, ColorHex: "FFFFFF",
                Cues: []project.Cue{{StartMs: 300, EndMs: 1500, Text: "Phụ đề tiếng Việt có dấu"}},
        }
        doc3.Watermark = &project.Watermark{AssetID: a3.ID, Corner: project.CornerTL,
                MarginPx: 16, Opacity: 0.9, ScalePct: 12}
        doc3.Timecode = &project.Timecode{Enabled: true, Format: "clock", FontSize: 24,
                PosX: 0.85, PosY: 0.08}
        doc3.Credits = &project.Credits{Lines: []string{"VKSeditorPro v1.1", "Dựng video Go + moviego"},
                FontSize: 28, SpeedPXS: 40}
        step("Tổng timeline doc3 = 4000ms", check(doc3.TimelineTotalMs() == 4000))
        g3, err := engine.Compile(doc3, engine.CompileOptions{})
        if !step("biên dịch graph đầy đủ lớp phủ", err) {
                return 1
        }
        outOv := filepath.Join(work, "v11-overlays.mp4")
        err = engine.WriteVideo(ctx, g3, engine.ExportRequest{
                Format: engine.FormatMP4, OutputPath: outOv, CRF: 26, Preset: "ultrafast",
        }, nil)
        g3.Close()
        step("xuất video có lớp phủ + phụ đề + watermark + timecode + credits", err)
        if err == nil {
                if p, e := media.Probe(outOv); e == nil {
                        step(fmt.Sprintf("thời lượng overlays 4s → %.2fs", float64(p.DurationMs)/1000),
                                check(approx(p.DurationMs, 4000, 400)))
                }
        }

        // ---------------------------------------------------------------
        // T4 — Xuất MOV alpha (qtrle)
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: xuất MOV alpha ---")
        g4, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 360})
        if !step("biên dịch graph alpha", err) {
                return 1
        }
        outAlpha := filepath.Join(work, "v11-alpha.mov")
        err = engine.WriteVideo(ctx, g4, engine.ExportRequest{
                Format: engine.FormatAlpha, OutputPath: outAlpha,
        }, nil)
        g4.Close()
        step("xuất MOV alpha (qtrle)", err)
        if err == nil {
                if st, e := os.Stat(outAlpha); e == nil {
                        step(fmt.Sprintf("MOV alpha kích thước %.1f KB", float64(st.Size())/1024),
                                check(st.Size() > 10_000))
                }
        }

        // ---------------------------------------------------------------
        // T5 — Stream copy trim (không re-encode)
        // Nguồn dùng GOP 1s (-g 30) để keyframe khớp điểm cắt: [1s,3s) → ~2s.
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: cắt nhanh stream copy ---")
        srcGOP := filepath.Join(work, "src-gop.mp4")
        gopCmd := exec.Command(ffBin, "-y", "-f", "lavfi",
                "-i", "testsrc2=size=640x360:rate=30:duration=5",
                "-c:v", "libx264", "-preset", "ultrafast", "-crf", "28", "-g", "30",
                srcGOP)
        executil.Hide(gopCmd) // v1.2.7: không hiện console trên Windows
        if !step("sinh nguồn GOP 1s cho stream copy", gopCmd.Run()) {
                return 1
        }
        outCut := filepath.Join(work, "v11-cut.mp4")
        err = tools.StreamCopyTrim(ctx, srcGOP, outCut, 1000, 2000)
        step("cắt nhanh 1s→3s không re-encode", err)
        if err == nil {
                if p, e := media.Probe(outCut); e == nil {
                        step(fmt.Sprintf("thời lượng cắt nhanh ≈2s → %.2fs", float64(p.DurationMs)/1000),
                                check(p.DurationMs >= 1400 && p.DurationMs <= 2600))
                }
        }

        // ---------------------------------------------------------------
        // T6 — DetectSilence / DetectScenes / QC
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: khoảng lặng / phát hiện cảnh / QC ---")
        wavSil := filepath.Join(work, "silence.mp4")
        silCmd := exec.Command(ffBin, "-y", "-f", "lavfi",
                "-i", "aevalsrc=if(lt(t\\,1)\\,0.5*sin(880*2*PI*t)\\,0):d=4:s=44100",
                "-f", "lavfi", "-i", "color=c=blue:s=320x240:r=30:d=4",
                "-c:v", "libx264", "-preset", "ultrafast", "-c:a", "aac",
                "-shortest", wavSil)
        executil.Hide(silCmd) // v1.2.7: không hiện console trên Windows
        err = silCmd.Run()
        if !step("sinh video có 3s im lặng", err) {
                return 1
        }
        sils, err := tools.DetectSilence(ctx, wavSil, -35, 300)
        if !step("silencedetect phát hiện khoảng lặng", err) {
                return 1
        }
        found := false
        for _, s := range sils {
                if s.StartMs >= 700 && s.StartMs <= 1400 && s.EndMs >= 3000 {
                        found = true
                }
        }
        step(fmt.Sprintf("khoảng lặng bắt đầu ~1s (%d vùng)", len(sils)), check(found))

        marks, err := tools.DetectScenes(ctx, a1.Path, 0.35)
        if err == nil && len(marks) == 0 {
                // testsrc2 vẫn có thể không vượt ngưỡng ở cấu hình nhất định — thử thấp hơn.
                marks, err = tools.DetectScenes(ctx, a1.Path, 0.2)
        }
        step(fmt.Sprintf("phát hiện cảnh (%d điểm)", len(marks)), err)

        qc, err := tools.RunQC(ctx, a2.Path)
        step("QC tự động (blackdetect + volumedetect)", err)
        if err == nil {
                step(fmt.Sprintf("QC có báo cáo (%d nhận xét)", len(qc.Problems)), nil)
        }

        // ---------------------------------------------------------------
        // T7 — Proxy 480p
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: proxy 480p ---")
        outProxy := filepath.Join(work, "v11-proxy.mp4")
        _, err = tools.GenerateProxy(ctx, a1.Path, outProxy, 480)
        step("tạo proxy 480p", err)
        if err == nil {
                if p, e := media.Probe(outProxy); e == nil {
                        step(fmt.Sprintf("proxy → %dx%d", p.Width, p.Height),
                                check(p.Height == 480 || p.Height == 482))
                }
        }

        // ---------------------------------------------------------------
        // T8 — Round-trip SRT
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: SRT parse / serialize ---")
        srt := "1\n00:00:00,500 --> 00:00:01,750\nXin chào\nthế giới\n\n2\n00:00:02,000 --> 00:00:03,250\nDòng thứ hai\n"
        parsed, err := tools.ParseSRT(srt)
        step("parse SRT 2 cue", check(err == nil && len(parsed) == 2))
        if err == nil && len(parsed) == 2 {
                step("cue 1 = 500..1750ms «Xin chào\\nthế giới»",
                        check(parsed[0].StartMs == 500 && parsed[0].EndMs == 1750 && parsed[0].Text == "Xin chào\nthế giới"))
                built := tools.BuildSRT(parsed)
                p2, err2 := tools.ParseSRT(built)
                step("round-trip build→parse giữ nguyên nội dung",
                        check(err2 == nil && len(p2) == 2 && p2[1].Text == "Dòng thứ hai"))
        }

        // ---------------------------------------------------------------
        // T9 — Headless compile (dùng chung engine với CLI)
        // ---------------------------------------------------------------
        fmt.Fprintln(out, "--- V1.1: headless (CLI) dùng chung engine ---")
        gh, err := engine.Compile(doc2, engine.CompileOptions{TargetHeight: 360})
        if !step("biên dịch headless-style graph", err) {
                return 1
        }
        outH := filepath.Join(work, "v11-headless.mp4")
        err = engine.WriteVideo(context.Background(), gh, engine.ExportRequest{
                Format: engine.FormatMP4, OutputPath: outH, CRF: 28, Preset: "ultrafast",
        }, nil)
        gh.Close()
        step("xuất headless MP4", err)

        return 0
}

// check chuyển bool thành error cho step().
func check(ok bool) error {
        if ok {
                return nil
        }
        return errFail{}
}

type errFail struct{}

func (errFail) Error() string { return "giá trị kiểm tra không khớp" }

// miniCube sinh LUT 2x2x2 tối giản gần như identity.
func miniCube() string {
        var sb []byte
        sb = append(sb, []byte("TITLE \"VKS mini\"\nLUT_3D_SIZE 2\nDOMAIN_MIN 0.0 0.0 0.0\nDOMAIN_MAX 1.0 1.0 1.0\n")...)
        for b := 0; b < 2; b++ {
                for g := 0; g < 2; g++ {
                        for r := 0; r < 2; r++ {
                                sb = append(sb, []byte(fmt.Sprintf("%.4f %.4f %.4f\n",
                                        float64(r)/1, float64(g)/1, float64(b)/1))...)
                        }
                }
        }
        return string(sb)
}
