// v130tests.go — nhóm test engine v1.3.0: TRỘN TRACK ÂM THANH A1.
// Dựng video mẫu (không âm), chèn 2 đoạn âm thanh (một đoạn có volume + fade),
// chạy MixAudioTrack rồi kiểm tra tệp kết quả CÓ stream âm thanh.
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
)

// runV130Tests trả 0 nếu mọi bước đạt.
func runV130Tests(out io.Writer, work, ffBin string, ctx context.Context, vAsset, aAsset *project.Asset) int {
	fmt.Fprintln(out, "--- V1.3.0: track âm thanh độc lập A1 ---")

	doc := project.NewDocument()
	doc.Assets = []*project.Asset{vAsset, aAsset}
	c1 := &project.Clip{ID: project.NewID("c"), AssetID: vAsset.ID, InMs: 0, OutMs: 5000,
		Volume: 1, Mute: true, Transform: project.Transform{Zoom: 1}} // video gốc TẮT TIẾNG
	doc.Clips = []*project.Clip{c1}
	// Hai đoạn nhạc: volume 0.7 + fade in 300ms; đoạn 2 mute (phải bị bỏ qua).
	doc.AudioClips = []*project.AudioClip{
		{ID: project.NewID("aud"), AssetID: aAsset.ID, StartMs: 1000, InMs: 0, OutMs: aAsset.DurationMs,
			Volume: 0.7, FadeInMs: 300},
		{ID: project.NewID("aud"), AssetID: aAsset.ID, StartMs: 2000, InMs: 0, OutMs: aAsset.DurationMs,
			Mute: true},
	}

	// 1) Dựng video nền (video clip mute → tệp KHÔNG có stream âm thanh).
	g, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 360})
	if !step("v1.3.0 biên dịch graph 360p", err) {
		return 1
	}
	outMp4 := filepath.Join(work, "out130.mp4")
	err = engine.WriteVideo(ctx, g, engine.ExportRequest{
		Format: engine.FormatMP4, OutputPath: outMp4, CRF: 28, Preset: "ultrafast",
	}, nil)
	g.Close()
	if !step("v1.3.0 xuất video nền (clip mute → dự kiến KHÔNG audio)", err) {
		return 1
	}

	// 2) Trộn track A1.
	final, err := engine.MixAudioTrack(ctx, outMp4, doc)
	if !step("v1.3.0 trộn 2 đoạn A1 (1 hoạt động, 1 mute)", err) {
		return 1
	}
	if final != outMp4 {
		step("v1.3.0 đường dẫn kết quả khác thường", fmt.Errorf("kỳ vọng %s, nhận %s", outMp4, final))
		return 1
	}

	// 3) Kiểm tra: tệp giờ PHẢI có stream âm thanh.
	p, err := media.Probe(outMp4)
	if !step("v1.3.0 probe kết quả", err) {
		return 1
	}
	if !p.HasAudio {
		step("v1.3.0 tệp sau trộn phải CÓ âm thanh", fmt.Errorf("HasAudio=false"))
		return 1
	}
	step(fmt.Sprintf("v1.3.0 tệp sau trộn → %.2fs, audio=OK", float64(p.DurationMs)/1000), nil)

	// 4) Nhánh không cần trộn: doc không có đoạn hoạt động → trả nguyên tệp.
	doc2 := project.NewDocument()
	doc2.Assets = doc.Assets
	p2 := filepath.Join(work, "no_mix.mp4")
	if b, err := os.ReadFile(outMp4); err == nil {
		_ = os.WriteFile(p2, b, 0o644)
	}
	got, err := engine.MixAudioTrack(ctx, p2, doc2)
	if !step("v1.3.0 không có đoạn hoạt động → không chạm tệp", err) {
		return 1
	}
	if got != p2 {
		step("v1.3.0 nhánh bỏ qua phải trả cùng đường dẫn", fmt.Errorf("nhận %s", got))
		return 1
	}

	// 5) Sinh âm thanh mẫu để test nhánh "video CÓ audio" — dùng cặp
	// video-đã-có-âm + trộn thêm nhạc: mọi branch amix đều chạy được.
	vidAud := filepath.Join(work, "withaudio.mp4")
	cmd := exec.Command(ffBin, "-y", "-f", "lavfi", "-i",
		"testsrc=size=320x240:rate=10:duration=3", "-f", "lavfi",
		"-i", "sine=frequency=440:duration=3", "-c:v", "libx264", "-preset", "ultrafast",
		"-c:a", "aac", "-shortest", vidAud)
	executil.Hide(cmd)
	if !step("v1.3.0 sinh video mẫu CÓ âm thanh", cmd.Run()) {
		return 1
	}
	va, err := media.Probe(vidAud)
	if !step("v1.3.0 probe video có âm", err) {
		return 1
	}
	va.ID = project.NewID("a")
	doc3 := project.NewDocument()
	doc3.Assets = []*project.Asset{va, aAsset}
	doc3.Clips = []*project.Clip{{ID: project.NewID("c"), AssetID: va.ID, InMs: 0, OutMs: va.DurationMs,
		Volume: 1, Transform: project.Transform{Zoom: 1}}}
	doc3.AudioClips = []*project.AudioClip{{ID: project.NewID("aud"), AssetID: aAsset.ID,
		StartMs: 0, InMs: 0, OutMs: aAsset.DurationMs, Volume: 0.5}}
	out3 := filepath.Join(work, "withaudio_mix.mp4")
	if b, err := copyFile130(vidAud); err == nil {
		_ = os.WriteFile(out3, b, 0o644)
	}
	if _, err := engine.MixAudioTrack(ctx, out3, doc3); !step("v1.3.0 trộn nhạc LÊN video có âm (amix 2 nguồn)", err) {
		return 1
	}
	if p3, err := media.Probe(out3); err == nil {
		if !p3.HasAudio {
			step("v1.3.0 tệp trộn lên video có âm phải còn audio", fmt.Errorf("HasAudio=false"))
			return 1
		}
		step("v1.3.0 video có âm + nhạc nền → audio=OK", nil)
	}
	return 0
}

func copyFile130(path string) ([]byte, error) {
	return os.ReadFile(path)
}
