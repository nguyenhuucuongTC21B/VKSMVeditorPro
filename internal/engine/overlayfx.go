// overlayfx.go — v1.3.1: chế độ lớp kiểu KineMaster/CapCut cho lớp phủ.
// Hòa trộn (blend), độ mờ + hiện dần (opacity/fade) và preset hiệu ứng
// nghệ thuật (trắng đen, âm bản, sepia, vintage, rực rỡ, tương phản, mơ màng)
// áp cho MỌI loại lớp phủ (chữ / hình / video / ảnh PiP).
package engine

import (
	"time"

	"github.com/mowshon/moviego/v2/clip"
	mgo "github.com/mowshon/moviego/v2"
	"github.com/mowshon/moviego/v2/effect"

	"vkseditorpro/internal/project"
)

// blendModeOf map chế độ hòa trộn (chuỗi project) → BlendMode moviego.
func blendModeOf(o *project.Overlay) mgo.BlendMode {
	switch o.Blend {
	case "screen":
		return mgo.BlendScreen
	case "multiply":
		return mgo.BlendMultiply
	case "overlay":
		return mgo.BlendOverlay
	case "darken":
		return mgo.BlendDarken
	case "lighten":
		return mgo.BlendLighten
	case "add":
		return mgo.BlendAdd
	}
	return mgo.BlendNormal
}

// applyOverlayFilter áp preset hiệu ứng nghệ thuật cho lớp phủ media/ảnh.
// Giá trị "" = giữ nguyên.
func applyOverlayFilter(v *mgo.Video, o *project.Overlay) *mgo.Video {
	switch o.FilterPreset {
	case "bw":
		return v.Grayscale()
	case "negative":
		return v.Invert()
	case "sepia":
		return v.Fx(effect.PixelFn{Name: "vks-sepia", Fn: sepiaInto})
	case "vintage":
		v = v.Fx(effect.PixelFn{Name: "vks-vintage", Fn: vintageInto})
		return v.Vignette(0.55)
	case "vivid":
		return v.Saturation(1.6).Contrast(1.08)
	case "contrast":
		return v.Contrast(1.38).Saturation(0.92)
	case "dream":
		return v.GaussianBlur(1.6).Brightness(0.07)
	}
	return v
}

// applyOverlayOpacity áp độ mờ tĩnh + hiện dần (fade in/out) cho lớp phủ.
// Dùng hoạt hình alpha (PropOpacity) thay vì FadeIn luminance: lớp phủ mờ dần
// theo ĐỘ ĐẶC — nền lộ ra xuyên suốt — đúng kiểu KineMaster, không bùng đen.
// win là độ dài hiệu dụng của lớp (ms); fade được kẹp trong khoảng đó.
func applyOverlayOpacity(v *mgo.Video, o *project.Overlay, win int64) *mgo.Video {
	base := o.Opacity
	if base <= 0 || base > 1 { // dự án cũ / thiếu = đặc
		base = 1
	}
	clamp := func(vv int64) int64 {
		if vv < 0 {
			return 0
		}
		if vv > win {
			return win
		}
		return vv
	}
	fi, fo := clamp(o.FadeInMs), clamp(o.FadeOutMs)
	if fi+fo > win { // fade chồng nhau — rút ngắn còn một nửa mỗi bên
		half := win / 2
		fi, fo = min64(fi, half), min64(fo, half)
	}
	if base >= 1 && fi <= 0 && fo <= 0 {
		return v
	}
	ms := func(vv int64) mgo.Time { return mgo.Time(time.Duration(vv) * time.Millisecond) }
	var keys []mgo.Keyframe
	switch {
	case fi > 0 && fo > 0:
		keys = []mgo.Keyframe{
			{At: 0, Val: 0}, {At: ms(fi), Val: base},
			{At: ms(win - fo), Val: base}, {At: ms(win), Val: 0},
		}
	case fi > 0:
		keys = []mgo.Keyframe{{At: 0, Val: 0}, {At: ms(fi), Val: base}}
	case fo > 0:
		keys = []mgo.Keyframe{{At: 0, Val: base}, {At: ms(win - fo), Val: base}, {At: ms(win), Val: 0}}
	default:
		keys = []mgo.Keyframe{{At: 0, Val: base}}
	}
	return v.Animate(mgo.PropOpacity, keys, mgo.EaseInOut)
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// sepiaInto biến hệ màu sepia cổ điển (ma trận chuẩn, per-pixel thuần Go —
// không cần ffmpeg filter nên chạy đúng cả bản xem trước lẫn bản xuất).
func sepiaInto(t clip.Time, dst, src *clip.Frame) error {
	for y := 0; y < src.H; y++ {
		s := src.Pix[y*src.Stride:]
		d := dst.Pix[y*dst.Stride:]
		for x := 0; x < src.W; x++ {
			i := x * 3
			r, g, b := int(s[i]), int(s[i+1]), int(s[i+2])
			nr := (393*r + 769*g + 189*b) / 1000
			ng := (349*r + 686*g + 168*b) / 1000
			nb := (272*r + 498*g + 131*b) / 1000
			d[i], d[i+1], d[i+2] = clampByteF(nr), clampByteF(ng), clampByteF(nb)
		}
	}
	return nil
}

// vintageInto tông màu retro: giảm bão hòa nhẹ + nghiêng vàng ấm + nâng tối.
func vintageInto(t clip.Time, dst, src *clip.Frame) error {
	for y := 0; y < src.H; y++ {
		s := src.Pix[y*src.Stride:]
		d := dst.Pix[y*dst.Stride:]
		for x := 0; x < src.W; x++ {
			i := x * 3
			r, g, b := int(s[i]), int(s[i+1]), int(s[i+2])
			// giảm bão hòa 35%
			gray := (77*r + 150*g + 29*b) >> 8
			r = gray + (r-gray)*65/100
			g = gray + (g-gray)*65/100
			b = gray + (b-gray)*65/100
			// ấm lên: + đỏ/vàng, hạ xanh lam một chút
			r = r*106/100 + 8
			g = g*102/100 + 4
			b = b*94/100
			d[i], d[i+1], d[i+2] = clampByteF(r), clampByteF(g), clampByteF(b)
		}
	}
	return nil
}

func clampByteF(v int) byte {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return byte(v)
}
