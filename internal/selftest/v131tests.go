// v131tests.go — nhóm test engine v1.3.1: CHẾ ĐỘ LỚP KIỂU KINEMASTER.
// Render timeline có: PiP blend screen + preset trắng đen, ảnh overlay
// sepia + hiện dần, chữ mờ 70% — xuất MP4 và kiểm tra tệp kết quả sống khỏe.
package selftest

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"vkseditorpro/internal/engine"
	"vkseditorpro/internal/media"
	"vkseditorpro/internal/project"
)

// runV131Tests trả 0 nếu mọi bước đạt.
func runV131Tests(out io.Writer, work, ffBin string, ctx context.Context, vAsset, imgAsset *project.Asset) int {
	fmt.Fprintln(out, "--- V1.3.1: chế độ lớp (blend/preset/fade) ---")

	doc := project.NewDocument()
	doc.Assets = []*project.Asset{vAsset, imgAsset}
	c1 := &project.Clip{ID: project.NewID("c"), AssetID: vAsset.ID, InMs: 0, OutMs: 4000,
		Volume: 1, Transform: project.Transform{Zoom: 1}}
	doc.Clips = []*project.Clip{c1}

	// Lớp PiP video: hòa trộn SCREEN + trắng đen + hiện dần vào/ra.
	pip := &project.Overlay{
		ID: project.NewID("ov"), Kind: project.OverlayMedia,
		StartMs: 500, EndMs: 3500, PosX: 0.7, PosY: 0.1,
		AssetID: vAsset.ID, ScalePct: 30, Volume: 1,
		Blend: "screen", FilterPreset: "bw", Opacity: 0.9, FadeInMs: 400, FadeOutMs: 400,
	}
	// Lớp ảnh: sepia + hiện dần vào.
	ovImg := &project.Overlay{
		ID: project.NewID("ov"), Kind: project.OverlayMedia,
		StartMs: 1000, EndMs: 3000, PosX: 0.1, PosY: 0.6,
		AssetID: imgAsset.ID, ScalePct: 25,
		FilterPreset: "sepia", FadeInMs: 600,
	}
	// Lớp chữ: mờ 70% (dự án cũ opacity 0 → engine phải coi là đặc).
	ovTxt := &project.Overlay{
		ID: project.NewID("ov"), Kind: project.OverlayText,
		StartMs: 0, EndMs: 3000, PosX: 0.5, PosY: 0.85,
		Text: "VKS 1.3.1", FontSize: 64, ColorHex: "FFFFFF",
		Opacity: 0.7,
	}
	doc.Overlays = []*project.Overlay{pip, ovImg, ovTxt}
	doc.SortOverlays()

	// 1) Biên dịch + xuất 360p.
	g, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 360})
	if !step("v1.3.1 biên dịch graph (PiP blend+filter+fade, ảnh sepia, chữ mờ)", err) {
		return 1
	}
	outMp4 := filepath.Join(work, "out131.mp4")
	err = engine.WriteVideo(ctx, g, engine.ExportRequest{
		Format: engine.FormatMP4, OutputPath: outMp4, CRF: 28, Preset: "ultrafast",
	}, nil)
	g.Close()
	if !step("v1.3.1 xuất MP4 với các chế độ lớp", err) {
		return 1
	}

	// 2) Probe kết quả: đủ dài, có video.
	p, err := media.Probe(outMp4)
	if !step("v1.3.1 probe kết quả", err) {
		return 1
	}
	if p.DurationMs < 3500 {
		step("v1.3.1 thời lượng kết quả", fmt.Errorf("kỳ vọng >= 3500ms, nhận %dms", p.DurationMs))
		return 1
	}
	if p.Width <= 0 || p.Height <= 0 {
		step("v1.3.1 kích thước kết quả", fmt.Errorf("%dx%d", p.Width, p.Height))
		return 1
	}
	step(fmt.Sprintf("v1.3.1 tệp hoàn chỉnh → %.2fs %dx%d", float64(p.DurationMs)/1000, p.Width, p.Height), nil)

	// 3) Các chế độ hòa trộn còn lại phải biên dịch được (không crash).
	for _, mode := range []string{"multiply", "overlay", "darken", "lighten", "add"} {
		doc.Overlays[0].Blend = mode
		g2, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 180})
		if !step("v1.3.1 biên dịch blend="+mode, err) {
			return 1
		}
		outMode := filepath.Join(work, "out131_"+mode+".mp4")
		err = engine.WriteVideo(ctx, g2, engine.ExportRequest{
			Format: engine.FormatMP4, OutputPath: outMode, CRF: 30, Preset: "ultrafast",
		}, nil)
		g2.Close()
		if !step("v1.3.1 render blend="+mode, err) {
			return 1
		}
	}
	return 0
}
