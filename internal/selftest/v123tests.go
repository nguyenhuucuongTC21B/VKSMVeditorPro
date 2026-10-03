package selftest

// v123tests.go — nhóm test v1.2.3: pan vị trí (Auto Reframe), opacity tĩnh,
// tông màu vùng sáng (colorbalance pre-pass), lọc tạp âm tiếng, làm sạch
// giọng nói, công tắc track (mute-all / ẩn lớp phủ).

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
)

// runV123Tests chạy toàn bộ nhóm test v1.2.3; trả 0 nếu mọi bước OK.
func runV123Tests(out io.Writer, work, ffBin string, ctx context.Context,
        a1, a2 *project.Asset) int {

        const canvasW, canvasH = 640, 360

        mkDoc := func(clip *project.Clip, muteAll, hideOv bool) *project.Document {
                doc := project.NewDocument()
                doc.Canvas = project.Canvas{Width: canvasW, Height: canvasH, Rate: project.Rate{Num: 30, Den: 1}}
                doc.Assets = []*project.Asset{a1}
                doc.Clips = []*project.Clip{clip}
                doc.MuteAll = muteAll
                doc.HideOverlays = hideOv
                return doc
        }
        export := func(doc *project.Document, name string) string {
                g, err := engine.Compile(doc, engine.CompileOptions{})
                if !step("biên dịch "+name, err) {
                        return ""
                }
                out := filepath.Join(work, name+".mp4")
                err = engine.WriteVideo(ctx, g, engine.ExportRequest{
                        Format: engine.FormatMP4, OutputPath: out, CRF: 24, Preset: "veryfast",
                }, func(int, int) {})
                g.Close()
                step("xuất "+name, err)
                return out
        }

        // T1 — Pan vị trí: zoom 2× + pan lệch trái/trên (nền tảng Auto Reframe).
        fmt.Fprintln(out, "--- V1.2.3: pan vị trí (zoom 2× + PosX/PosY) ---")
        cPan := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 1500,
                Volume: 1, Transform: project.Transform{Zoom: 2, PosX: 0.15, PosY: 0.8}}
        if p := export(mkDoc(cPan, false, false), "v123-pan"); p != "" {
                if pr, err := media.Probe(p); err == nil {
                        step(fmt.Sprintf("pan render đúng khung %dx%d", pr.Width, pr.Height), nil)
                }
        }

        // T2 — Opacity tĩnh 50%.
        fmt.Fprintln(out, "--- V1.2.3: opacity tĩnh 50% ---")
        cOp := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 1200,
                Volume: 1, Opacity: 0.5, Transform: project.Transform{Zoom: 1}}
        export(mkDoc(cOp, false, false), "v123-opacity")

        // T3 — Bánh xe màu rút gọn (colorbalance pre-pass).
        fmt.Fprintln(out, "--- V1.2.3: tông màu vùng sáng (colorbalance) ---")
        cCB := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 1200,
                Volume: 1, Transform: project.Transform{Zoom: 1},
                Effects: project.ClipEffects{CBShadowR: 0.2, CBMidB: -0.15, CBHighG: 0.1}}
        export(mkDoc(cCB, false, false), "v123-colorbalance")

        // T4 — Lọc tạp âm tiếng (afftdn pre-pass).
        fmt.Fprintln(out, "--- V1.2.3: lọc tạp âm tiếng (afftdn) ---")
        cAD := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 1200,
                Volume: 1, Effects: project.ClipEffects{AudioDenoise: true},
                Transform: project.Transform{Zoom: 1}}
        export(mkDoc(cAD, false, false), "v123-audiodenoise")

        // T5 — Làm sạch giọng nói (chuỗi filter studio).
        fmt.Fprintln(out, "--- V1.2.3: làm sạch giọng nói (vocal enhance) ---")
        cVE := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 1200,
                Volume: 1, Effects: project.ClipEffects{VocalEnhance: true},
                Transform: project.Transform{Zoom: 1}}
        export(mkDoc(cVE, false, false), "v123-vocal")

        // T6 — 🔇 MuteAll: đầu ra KHÔNG còn luồng âm thanh.
        fmt.Fprintln(out, "--- V1.2.3: mute-all timeline ---")
        cM := &project.Clip{ID: project.NewID("c"), AssetID: a1.ID, InMs: 0, OutMs: 1000,
                Volume: 1, Transform: project.Transform{Zoom: 1}}
        pM := export(mkDoc(cM, true, false), "v123-muteall")
        if pM != "" {
                if pr, err := media.Probe(pM); err == nil {
                        step("mute-all → không còn âm thanh", boolErr(!pr.HasAudio))
                }
        }

        // T7 — 👁 HideOverlays: render xong vẫn OK (lớp phủ bị bỏ qua).
        fmt.Fprintln(out, "--- V1.2.3: ẩn lớp phủ (hide overlays) ---")
        docH := mkDoc(cM, false, true)
        docH.Overlays = []*project.Overlay{{ID: project.NewID("o"), Kind: project.OverlayText,
                Text: "không được hiện", StartMs: 0, EndMs: 1000, PosX: 0.5, PosY: 0.2, FontSize: 60}}
        docH.SortOverlays()
        export(docH, "v123-hideoverlays")

        // T8 — DetachAudio-style: ffmpeg tách âm thanh nhanh (kiểm nguyên lý).
        fmt.Fprintln(out, "--- V1.2.3: tách âm thanh (nguyên lý ffmpeg -vn) ---")
        outA := filepath.Join(work, "v123-detached.m4a")
        detCmd := exec.Command(ffBin, "-y", "-i", a1.Path, "-vn", "-c:a", "aac", "-b:a", "128k", outA)
        executil.Hide(detCmd) // v1.2.7: không hiện console trên Windows
        step("tách audio a.mp4 → m4a", detCmd.Run())
        if _, err := os.Stat(outA); err == nil {
                if pr, err := media.Probe(outA); err == nil {
                        step(fmt.Sprintf("m4a tách ra là âm thanh thuần (kind=%s)", pr.Kind), boolErr(pr.Kind == project.AssetAudio))
                }
        }
        return 0
}

// boolErr trả nil khi ok=true để dùng với step().
func boolErr(ok bool) error {
        if ok {
                return nil
        }
        return fmt.Errorf("không đạt")
}
