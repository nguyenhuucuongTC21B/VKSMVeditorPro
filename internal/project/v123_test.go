package project

// v123_test.go — kiểm tra tính năng v1.2.3: pan vị trí, opacity, tông màu
// vùng sáng, cờ track, an toàn dữ liệu dự án cũ.

import (
	"math"
	"testing"
)

func TestTransformNormalizePosDefault(t *testing.T) {
	// Dự án cũ không có posX/posY (JSON = 0) → phải hiểu là GIỮA khung.
	tr := Transform{Zoom: 2}.Normalize()
	if tr.PosX != 0.5 || tr.PosY != 0.5 {
		t.Fatalf("pos mặc định = (%v,%v), muốn (0.5,0.5)", tr.PosX, tr.PosY)
	}
	// Giá trị hợp lệ được giữ nguyên.
	tr2 := Transform{Zoom: 2, PosX: 0.1, PosY: 0.9}.Normalize()
	if tr2.PosX != 0.1 || tr2.PosY != 0.9 {
		t.Fatalf("pos giữ nguyên sai: (%v,%v)", tr2.PosX, tr2.PosY)
	}
	// Ngoài khoảng → về giữa.
	tr3 := Transform{Zoom: 2, PosX: 1.5, PosY: -0.2}.Normalize()
	if tr3.PosX != 0.5 || tr3.PosY != 0.5 {
		t.Fatalf("pos ngoài khoảng phải về giữa: (%v,%v)", tr3.PosX, tr3.PosY)
	}
	// NaN → giữa.
	tr4 := Transform{Zoom: 2, PosX: math.NaN()}.Normalize()
	if tr4.PosX != 0.5 {
		t.Fatalf("pos NaN phải về giữa, got %v", tr4.PosX)
	}
}

func TestClipOpacityDefault(t *testing.T) {
	doc := &Document{
		Assets: []*Asset{{ID: "a1", Kind: AssetVideo, Path: "x.mp4", DurationMs: 5000}},
		Clips: []*Clip{
			{ID: "c1", AssetID: "a1", InMs: 0, OutMs: 2000, Opacity: 0},    // dự án cũ → 1
			{ID: "c2", AssetID: "a1", InMs: 0, OutMs: 2000, Opacity: 0.35}, // giữ nguyên
			{ID: "c3", AssetID: "a1", InMs: 0, OutMs: 2000, Opacity: 1.7},  // >1 → 1
		},
	}
	doc.sanitize()
	if doc.Clips[0].Opacity != 1 {
		t.Fatalf("opacity 0 phải về 1, got %v", doc.Clips[0].Opacity)
	}
	if doc.Clips[1].Opacity != 0.35 {
		t.Fatalf("opacity 0.35 phải giữ nguyên, got %v", doc.Clips[1].Opacity)
	}
	if doc.Clips[2].Opacity != 1 {
		t.Fatalf("opacity 1.7 phải về 1, got %v", doc.Clips[2].Opacity)
	}
}

func TestClipEffectsColorBalance(t *testing.T) {
	fx := ClipEffects{CBShadowR: 2.5, CBMidG: -3, CBHighB: 0.4}
	fx = fx.Normalize()
	if fx.CBShadowR != 1 || fx.CBMidG != -1 || fx.CBHighB != 0.4 {
		t.Fatalf("colorbalance kẹp sai: %+v", fx)
	}
	if !fx.HasColorBalance() {
		t.Fatal("HasColorBalance phải true")
	}
	var zero ClipEffects
	if zero.HasColorBalance() {
		t.Fatal("fx rỗng không có colorbalance")
	}
}

func TestDocFlagsRoundTrip(t *testing.T) {
	// MuteAll/HideOverlays phải được serialize + giữ giá trị qua sanitize.
	doc := &Document{Name: "flags", MuteAll: true, HideOverlays: true,
		Clips: []*Clip{}, Assets: []*Asset{}}
	doc.sanitize()
	if !doc.MuteAll || !doc.HideOverlays {
		t.Fatal("cờ track bị mất sau sanitize")
	}
}
