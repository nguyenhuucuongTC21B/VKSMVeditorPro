// Package engine biên dịch Document (mô hình dự án của VKSeditorPro) thành
// clip graph của thư viện moviego v2, rồi xuất ra tệp video. Đây là phần
// "compiler" thuần túy — tách biệt GUI khỏi engine render, kiểm chứng được
// bằng unit test / selftest mà không cần giao diện.
//
// v1.1: hiệu ứng hình ảnh + chỉnh màu per-clip, LUT .cube, tốc độ/phát ngược/
// boomerang/lặp/đóng khung, hoạt hình keyframe, lớp phủ (chữ động, hình vẽ
// vector, video PiP + chroma key), phụ đề SRT, watermark, timecode, credits,
// xuất MOV alpha và tăng tốc GPU (nvenc/qsv).
package engine

import (
        "context"
        "crypto/sha1"
        "encoding/hex"
        "errors"
        "fmt"
        "image"
        "image/color"
        "image/jpeg"
        "math"
        "os"
        "os/exec"
        "path/filepath"
        "runtime"
        "strconv"
        "strings"
        "time"

        mgo "github.com/mowshon/moviego/v2"
        "github.com/mowshon/moviego/v2/effect"

        "vkseditorpro/internal/executil"
        "vkseditorpro/internal/project"
)

// ErrNoClips báo timeline trống.
var ErrNoClips = errors.New("timeline trống — hãy thêm ít nhất một clip trước khi dựng")

// CompileOptions cấu hình chung khi dựng graph.
type CompileOptions struct {
        // TargetHeight ép chiều cao đầu ra (0 = giữ canvas). Dùng cho bản xem trước
        // hoặc preset độ phân giải; chiều rộng được tính lại theo tỉ lệ, làm tròn chẵn.
        TargetHeight int
}

// Graph là một đồ thị moviego đã biên dịch cùng danh sách hàm giải phóng nguồn.
type Graph struct {
        Root   *mgo.Video
        Canvas project.Canvas
        closer []func() error
}

// Close giải phóng toàn bộ nguồn mở trong quá trình biên dịch.
func (g *Graph) Close() {
        for i := len(g.closer) - 1; i >= 0; i-- {
                _ = g.closer[i]()
        }
        g.closer = nil
}

// evenScaleFactor tính hệ số scale để chiều cao đạt target (giữ tỉ lệ),
// đảm bảo cả chiều rộng lẫn chiều cao sau scale đều chẵn (yêu cầu yuv420p).
func evenScaleFactor(w, h, targetH int) float64 {
        if targetH <= 0 || h <= 0 || w <= 0 || targetH == h {
                return 1
        }
        f := float64(targetH) / float64(h)
        // Làm tròn chiều rộng về số chẵn; nếu cần chỉnh lại hệ số.
        w2 := int(math.Round(float64(w) * f))
        if w2%2 != 0 {
                w2++
        }
        h2 := targetH
        if h2%2 != 0 {
                h2++
        }
        return math.Min(float64(w2)/float64(w), float64(h2)/float64(h))
}

// hexColor phân tích "#RRGGBB"/"RRGGBB" thành color.RGBA; lỗi → trắng.
func hexColor(s string) color.RGBA {
        s = strings.TrimPrefix(strings.TrimSpace(s), "#")
        if len(s) == 3 { // "F0A" → "FF00AA"
                s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
        }
        if len(s) != 6 {
                return color.RGBA{R: 255, G: 255, B: 255, A: 255}
        }
        v, err := strconv.ParseUint(s, 16, 32)
        if err != nil {
                return color.RGBA{R: 255, G: 255, B: 255, A: 255}
        }
        return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

// applyTransform áp biến đổi hình học (zoom + pan vị trí / xoay / lật) cho một clip.
// v1.2.3: khi zoom > 1, PosX/PosY (0..1) quyết định vùng khung được giữ lại —
// nền tảng của Auto Reframe (16:9 → 9:16) và pan trái/phải/lên/xuống.
func applyTransform(v *mgo.Video, tr project.Transform) *mgo.Video {
        tr = tr.Normalize()
        if tr.Zoom > 1.0 {
                v = v.Resize(tr.Zoom)
                if tr.PosX != 0.5 || tr.PosY != 0.5 {
                        if s := v.Size(); s.W > 4 && s.H > 4 {
                                cw := int(math.Round(float64(s.W) / tr.Zoom))
                                ch := int(math.Round(float64(s.H) / tr.Zoom))
                                cw -= cw % 2
                                ch -= ch % 2
                                if cw >= 2 && ch >= 2 && cw < s.W && ch < s.H {
                                        x := int(math.Round(float64(s.W-cw) * tr.PosX))
                                        y := int(math.Round(float64(s.H-ch) * tr.PosY))
                                        v = v.Crop(x, y, cw, ch)
                                }
                        }
                }
        }
        if tr.RotateDeg != 0 {
                v = v.Rotate(tr.RotateDeg)
        }
        if tr.FlipH {
                v = v.FlipH()
        }
        if tr.FlipV {
                v = v.FlipV()
        }
        return v
}

// applyEffects áp hiệu ứng hình ảnh + chỉnh màu cho một clip (v1.1).
// Thứ tự: xám → âm bản → độ sáng → tương phản → bão hòa → gamma →
// làm mờ → làm sắc → vignette → gợn sóng nước.
func applyEffects(v *mgo.Video, fx project.ClipEffects, dropDur mgo.Time) *mgo.Video {
        if fx.Grayscale {
                v = v.Grayscale()
        }
        if fx.Invert {
                v = v.Invert()
        }
        if fx.Brightness != 0 {
                v = v.Brightness(fx.Brightness)
        }
        if fx.Contrast > 0 && fx.Contrast != 1 {
                v = v.Contrast(fx.Contrast)
        }
        if fx.Saturation > 0 && fx.Saturation != 1 {
                v = v.Saturation(fx.Saturation)
        }
        if fx.Gamma > 0 && fx.Gamma != 1 {
                v = v.Gamma(fx.Gamma)
        }
        if fx.Blur > 0 {
                v = v.GaussianBlur(fx.Blur)
        }
        if fx.Sharpen > 0 {
                v = v.Sharpen(fx.Sharpen)
        }
        if fx.Vignette > 0 {
                v = v.Vignette(fx.Vignette)
        }
        if dropDur > 0 {
                v = v.WaterDrop(dropDur)
        }
        return v
}

// applyKeyframes áp hoạt hình keyframe scale/opacity cho clip (v1.1).
func applyKeyframes(v *mgo.Video, keys []project.KeyframePoint) *mgo.Video {
        if len(keys) == 0 {
                return v
        }
        var scaleKeys, opacityKeys []mgo.Keyframe
        for _, k := range keys {
                kf := mgo.Keyframe{At: mgo.Time(time.Duration(k.AtMs) * time.Millisecond), Val: k.Val}
                switch k.Prop {
                case "scale":
                        scaleKeys = append(scaleKeys, kf)
                case "opacity":
                        opacityKeys = append(opacityKeys, kf)
                }
        }
        if len(scaleKeys) > 0 {
                v = v.Animate(mgo.PropScale, scaleKeys, nil)
        }
        if len(opacityKeys) > 0 {
                v = v.Animate(mgo.PropOpacity, opacityKeys, nil)
        }
        return v
}

// applyTransparency áp độ mờ đục (v1.2.3): tĩnh (c.Opacity) hoặc qua keyframe
// opacity của v1.1 — rồi ÉP XUỐNG NỀN ĐEN ĐẶC bằng Composite. Lý do: opacity
// sinh kênh alpha, container MP4 (yuv420p) không mang được alpha → từng bị
// lỗi "output container does not support alpha" (lỗi tiềm ẩn từ v1.1 với
// preset fade-in/out + xuất MP4).
func applyTransparency(v *mgo.Video, c *project.Clip, w, h int) *mgo.Video {
        hasKey := false
        for _, k := range c.Keyframes {
                if k.Prop == "opacity" {
                        hasKey = true
                        break
                }
        }
        stat := c.Opacity > 0 && c.Opacity < 1
        if !stat && !hasKey {
                return v
        }
        if stat {
                v = v.Animate(mgo.PropOpacity, []mgo.Keyframe{{At: 0, Val: c.Opacity}}, nil)
        }
        bg := mgo.Color(w, h, [3]byte{0, 0, 0}).WithDuration(v.Duration())
        return mgo.Composite(bg, v)
}

// applyTime áp nhóm biến đổi thời gian: tốc độ → phát ngược / boomerang →
// lặp → đóng khung đầu/cuối (v1.1).
func applyTime(v *mgo.Video, c *project.Clip) *mgo.Video {
        if c.Speed > 0 && c.Speed != 1 {
                v = v.Speed(c.Speed)
        }
        if c.Boomerang {
                v = v.Boomerang()
        } else if c.Reverse {
                v = v.Reverse()
        }
        if !c.Boomerang && c.LoopN > 1 {
                v = v.Loop(c.LoopN)
        }
        if c.FreezeStartMs > 0 {
                v = v.FreezeStart(mgo.Time(time.Duration(c.FreezeStartMs) * time.Millisecond))
        }
        if c.FreezeEndMs > 0 {
                v = v.FreezeEnd(mgo.Time(time.Duration(c.FreezeEndMs) * time.Millisecond))
        }
        return v
}

// transitionStep map TransitionType → TransitionStep của moviego.
func transitionStep(t project.TransitionType, d mgo.Time) (mgo.TransitionStep, bool) {
        switch t {
        case project.TransCrossfade:
                return mgo.Crossfade(d), true
        case project.TransDissolve:
                return mgo.Dissolve(d), true
        case project.TransFadeThroughBlk:
                return mgo.FadeThroughBlack(d), true
        case project.TransWipeLeft:
                return mgo.WipeLeft(d), true
        case project.TransWipeRight:
                return mgo.WipeRight(d), true
        case project.TransWipeUp:
                return mgo.WipeUp(d), true
        case project.TransWipeDown:
                return mgo.WipeDown(d), true
        case project.TransSlideLeft:
                return mgo.SlideLeft(d), true
        case project.TransSlideRight:
                return mgo.SlideRight(d), true
        case project.TransSlideUp:
                return mgo.SlideUp(d), true
        case project.TransSlideDown:
                return mgo.SlideDown(d), true
        case project.TransPushLeft:
                return mgo.PushLeft(d), true
        case project.TransPushRight:
                return mgo.PushRight(d), true
        case project.TransPushUp:
                return mgo.PushUp(d), true
        case project.TransPushDown:
                return mgo.PushDown(d), true
        case project.TransIrisOpen:
                return mgo.IrisOpen(d), true
        case project.TransIrisClose:
                return mgo.IrisClose(d), true
        }
        return mgo.TransitionStep{}, false
}

// FFmpegBin trả đường dẫn binary ffmpeg đang dùng (export cho tầng app:
// tách âm thanh, phân tích màu…). Rỗng nếu không tìm thấy.
func FFmpegBin() string { return ffmpegBin() }

// ffmpegBin tìm binary ffmpeg (biến môi trường MGO_FFMPEG ưu tiên).
func ffmpegBin() string {
        if bin := os.Getenv("MGO_FFMPEG"); bin != "" {
                return bin
        }
        if p, err := exec.LookPath("ffmpeg"); err == nil {
                return p
        }
        return ""
}

// colorbalanceFilter dựng chuỗi tham số colorbalance từ fx (v1.2.3).
func colorbalanceFilter(fx project.ClipEffects) string {
        f := func(v float64) string {
                return strconv.FormatFloat(v, 'f', 3, 64)
        }
        return "colorbalance=" +
                "rs=" + f(fx.CBShadowR) + ":gs=" + f(fx.CBShadowG) + ":bs=" + f(fx.CBShadowB) +
                ":rm=" + f(fx.CBMidR) + ":gm=" + f(fx.CBMidG) + ":bm=" + f(fx.CBMidB) +
                ":rh=" + f(fx.CBHighR) + ":gh=" + f(fx.CBHighG) + ":bh=" + f(fx.CBHighB)
}

// videoPrepass chạy ffmpeg lên file nguồn cho hiệu ứng KHÔNG có trong graph
// moviego: giảm nhiễu hình (hqdn3d) + bánh xe màu rút gọn (colorbalance).
// Cache theo vân tay tệp + tham số trong %TEMP%/vks-prepass.
// Thất bại → trả về file gốc (không chặn dựng video).
func videoPrepass(src string, fx project.ClipEffects) string {
        bin := ffmpegBin()
        if bin == "" {
                return src
        }
        st, err := os.Stat(src)
        if err != nil {
                return src
        }
        var filters []string
        if fx.Denoise {
                filters = append(filters, "hqdn3d=3:2:6:4.5")
        }
        if fx.HasColorBalance() {
                filters = append(filters, colorbalanceFilter(fx.Normalize()))
        }
        if len(filters) == 0 {
                return src
        }
        fp := fmt.Sprintf("%s|%d|%s", src, st.Size(), strings.Join(filters, ","))
        h := sha1.Sum([]byte(fp))
        out := filepath.Join(os.TempDir(), "vks-prepass", hex.EncodeToString(h[:])+".mp4")
        if _, err := os.Stat(out); err == nil {
                return out
        }
        _ = os.MkdirAll(filepath.Dir(out), 0o755)
        args := []string{"-y", "-i", src, "-vf", strings.Join(filters, ","),
                "-c:v", "libx264", "-crf", "18", "-preset", "veryfast", "-c:a", "copy"}
        cmd := exec.Command(bin, append(args, out)...)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        if b, err := cmd.CombinedOutput(); err != nil {
                fmt.Fprintf(os.Stderr, "[engine] pre-pass hình ảnh thất bại, dùng file gốc: %v: %s\n",
                        err, tailStr(string(b)))
                return src
        }
        return out
}

// audioPrepass xử lý ÂM THANH qua ffmpeg (video copy nguyên): v1.2.3
//   - AudioDenoise: afftdn lọc tạp âm nền (quạt/gió/ồn môi trường).
//   - VocalEnhance: làm sạch giọng nói — cắt tiếng ầm dưới 90Hz, khử ồn,
//     lấn át dải "mũi" 250Hz, boost presence 3kHz, giới hạn trên 11kHz.
//
// Cache theo vân tay; thất bại → file gốc.
func audioPrepass(src string, vocal bool) string {
        bin := ffmpegBin()
        if bin == "" {
                return src
        }
        st, err := os.Stat(src)
        if err != nil {
                return src
        }
        var af string
        tag := "adn"
        if vocal {
                af = "highpass=f=90,afftdn=nr=16:nf=-28:tn=1," +
                        "equalizer=f=250:t=q:w=1:g=-3,equalizer=f=3000:t=q:w=2:g=3,lowpass=f=11000"
                tag = "vocal"
        } else {
                af = "afftdn=nr=12:nf=-25:tn=1"
        }
        fp := fmt.Sprintf("%s|%d|%s", src, st.Size(), tag)
        h := sha1.Sum([]byte(fp))
        out := filepath.Join(os.TempDir(), "vks-prepass", "a_"+hex.EncodeToString(h[:])+".m4a.mp4")
        if _, err := os.Stat(out); err == nil {
                return out
        }
        _ = os.MkdirAll(filepath.Dir(out), 0o755)
        cmd := exec.Command(bin, "-y", "-i", src, "-af", af,
                "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", out)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        if b, err := cmd.CombinedOutput(); err != nil {
                fmt.Fprintf(os.Stderr, "[engine] pre-pass âm thanh thất bại, dùng file gốc: %v: %s\n",
                        err, tailStr(string(b)))
                return src
        }
        return out
}

func tailStr(s string) string {
        if len(s) > 300 {
                return s[len(s)-300:]
        }
        return s
}

// textFontPath trả đường dẫn font mặc định cho chữ (hỗ trợ tốt tiếng Việt):
// Windows ưu tiên Segoe UI; Linux/macOS thử các font hệ thống phổ biến có
// glyph Latin Extended Additional (dấu tiếng Việt); rỗng = font nhúng moviego.
func textFontPath() string {
        candidates := []string{}
        switch runtime.GOOS {
        case "windows":
                candidates = append(candidates,
                        `C:\Windows\Fonts\segoeui.ttf`,
                        `C:\Windows\Fonts\arial.ttf`,
                        `C:\Windows\Fonts\tahoma.ttf`,
                )
        case "darwin":
                candidates = append(candidates,
                        "/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
                        "/System/Library/Fonts/Supplemental/Arial.ttf",
                        "/Library/Fonts/Arial.ttf",
                )
        default: // linux và hệ Unix khác
                candidates = append(candidates,
                        "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
                        "/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
                        "/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
                        "/usr/share/fonts/opentype/noto/NotoSans-Regular.ttf",
                        "/usr/share/fonts/truetype/freefont/FreeSans.ttf",
                )
        }
        for _, p := range candidates {
                if _, err := os.Stat(p); err == nil {
                        return p
                }
        }
        return ""
}

// buildTextOptions dựng TextOptions chuẩn cho chữ/tiêu đề.
func buildTextOptions(o *project.Overlay) mgo.TextOptions {
        opts := mgo.TextOptions{
                FontSize: o.FontSize,
                Color:    hexColor(o.ColorHex),
                Align:    mgo.AlignCenter,
        }
        if p := textFontPath(); p != "" {
                opts.FontPath = p
        }
        switch {
        case o.Outline: // viền đen cho chữ nổi
                opts.Stroke = color.RGBA{A: 255}
                opts.StrokeWidth = math.Max(2, o.FontSize/14)
        case o.Bold: // "đậm" giả lập bằng viền cùng màu
                opts.Stroke = opts.Color
                opts.StrokeWidth = math.Max(1, o.FontSize/18)
        }
        return opts
}

// buildOverlayChild dựng clip moviego cho một lớp phủ; trả (clip, closer, ok).
func buildOverlayChild(doc *project.Document, o *project.Overlay, layer int, total int64) (*mgo.Video, func() error, bool) {
        total = doc.TimelineTotalMs()
        start, end := o.StartMs, o.EndMs
        if start < 0 {
                start = 0
        }
        if end > total {
                end = total
        }
        if end <= start {
                return nil, nil, false // ngoài timeline — bỏ qua
        }
        win := mgo.Time(time.Duration(end-start) * time.Millisecond)
        at := mgo.Time(time.Duration(start) * time.Millisecond)
        pos := mgo.RelPos(clamp01(o.PosX), clamp01(o.PosY))

        switch o.Kind {
        case project.OverlayText:
                var (
                        v   *mgo.Video
                        err error
                )
                opts := buildTextOptions(o)
                if o.Anim != "" && o.AnimMs > 0 {
                        v, err = mgo.AnimatedText(o.Text, opts, mgo.TextAnim{
                                Type:   animKind(o.Anim),
                                Dur:    mgo.Time(time.Duration(o.AnimMs) * time.Millisecond),
                                Easing: mgo.EaseOut,
                        })
                } else {
                        v, err = mgo.Text(o.Text, opts)
                }
                if err != nil {
                        fmt.Fprintf(os.Stderr, "[engine] lớp chữ: %v\n", err)
                        return nil, nil, false
                }
                v = v.WithDuration(win).WithStart(at).Position(pos).Layer(layer)
                if m := blendModeOf(o); m != mgo.BlendNormal { // v1.3.1
                        v = v.BlendMode(m)
                }
                if o.Anim == "" || o.AnimMs <= 0 { // hoạt hình chữ đã tự quản alpha
                        v = applyOverlayOpacity(v, o, end-start)
                }
                return v, func() error { return nil }, true

        case project.OverlayShape:
                w := doc.Canvas.Width
                h := doc.Canvas.Height
                sw := int(float64(w) * clampPct(o.WidthPct) / 100)
                sh := int(float64(h) * clampPct(o.HeightPct) / 100)
                if sw <= 0 || sh <= 0 {
                        return nil, nil, false
                }
                col := hexColor(o.ColorHex)
                canvas := mgo.NewCanvas(w, h)
                switch o.Shape {
                case "circle":
                        r := int(math.Min(float64(sw), float64(sh)) / 2)
                        canvas.Circle(w/2, h/2, maxInt(1, r), mgo.Paint{Fill: col})
                case "ellipse":
                        canvas.Ellipse(w/2, h/2, maxInt(1, sw/2), maxInt(1, sh/2), mgo.Paint{Fill: col})
                case "line":
                        canvas.Line(w/2-sw/2, h/2, w/2+sw/2, h/2, col, math.Max(2, float64(sh)/10))
                default: // rect
                        canvas.Rect(w/2-sw/2, h/2-sh/2, sw, sh, mgo.Paint{Fill: col})
                }
                v := canvas.WithDuration(win).WithStart(at).Layer(layer)
                if m := blendModeOf(o); m != mgo.BlendNormal { // v1.3.1
                        v = v.BlendMode(m)
                }
                v = applyOverlayOpacity(v, o, end-start)
                return v, func() error { return nil }, true

        case project.OverlayMedia:
                asset := doc.FindAsset(o.AssetID)
                if asset == nil || asset.Missing {
                        return nil, nil, false
                }
                if asset.Kind == project.AssetImage {
                        img, err := mgo.Image(asset.Path)
                        if err != nil {
                                return nil, nil, false
                        }
                        v := img.WithDuration(win).WithStart(at)
                        v = applyOverlayFilter(v, o) // v1.3.1: preset nghệ thuật
                        v = scaleOverlay(v, doc, o)
                        v = v.Position(pos).Layer(layer)
                        if m := blendModeOf(o); m != mgo.BlendNormal {
                                v = v.BlendMode(m)
                        }
                        v = applyOverlayOpacity(v, o, end-start)
                        return v, func() error { return img.Close() }, true
                }
                src, err := mgo.OpenVideo(asset.Path)
                if err != nil {
                        return nil, nil, false
                }
                v := src.Subclip(0, win)
                if o.ChromaKey.Enabled {
                        v = v.Fx(effect.ChromaKey{
                                Hex:        o.ChromaKey.Hex,
                                Similarity: clamp01(o.ChromaKey.Similarity),
                                Blend:      clamp01(o.ChromaKey.Blend),
                        })
                }
                v = applyOverlayFilter(v, o) // v1.3.1: preset nghệ thuật
                v = scaleOverlay(v, doc, o)
                if o.Muted {
                        v = v.WithoutAudio()
                } else if o.Volume != 1 {
                        v = v.Volume(o.Volume)
                }
                v = v.WithStart(at).Position(pos).Layer(layer)
                if m := blendModeOf(o); m != mgo.BlendNormal { // v1.3.1: hòa trộn
                        v = v.BlendMode(m)
                }
                v = applyOverlayOpacity(v, o, end-start) // v1.3.1: độ mờ + hiện dần
                if err := v.Err(); err != nil {
                        _ = src.Close()
                        return nil, nil, false
                }
                return v, func() error { return src.Close() }, true
        }
        return nil, nil, false
}

// scaleOverlay đổi kích thước lớp phủ media về ScalePct% bề rộng canvas,
// giữ tỉ lệ, làm tròn chẵn.
func scaleOverlay(v *mgo.Video, doc *project.Document, o *project.Overlay) *mgo.Video {
        pct := clampPct(o.ScalePct)
        if pct <= 0 || pct >= 100 {
                return v
        }
        s := v.Size()
        if s.W <= 0 || s.H <= 0 {
                return v
        }
        w := int(float64(doc.Canvas.Width) * pct / 100)
        if w%2 != 0 {
                w++
        }
        if w < 16 {
                w = 16
        }
        h := int(math.Round(float64(s.H) * float64(w) / float64(s.W)))
        if h%2 != 0 {
                h++
        }
        if h < 16 {
                h = 16
        }
        return v.ResizeTo(w, h)
}

func clamp01(v float64) float64 {
        if v < 0 {
                return 0
        }
        if v > 1 {
                return 1
        }
        return v
}

func clampPct(v float64) float64 {
        if v < 1 {
                return 1
        }
        if v > 100 {
                return 100
        }
        return v
}

func maxInt(a, b int) int {
        if a > b {
                return a
        }
        return b
}

// animKind map project.TextAnimKind → mgo.AnimKind.
func animKind(k project.TextAnimKind) mgo.AnimKind {
        switch k {
        case project.TextAnimFadeIn:
                return mgo.AnimFadeIn
        case project.TextAnimTypewriter:
                return mgo.AnimTypewriter
        case project.TextAnimSlideUp:
                return mgo.AnimSlideUp
        case project.TextAnimSlideDown:
                return mgo.AnimSlideDown
        case project.TextAnimSlideLeft:
                return mgo.AnimSlideLeft
        case project.TextAnimSlideRight:
                return mgo.AnimSlideRight
        case project.TextAnimPop:
                return mgo.AnimPop
        }
        return mgo.AnimNone
}

// buildClipChain xây chuỗi moviego cho MỘT clip (mở nguồn mới riêng cho clip).
// Trả về handle cuối và hàm đóng nguồn.
func buildClipChain(doc *project.Document, c *project.Clip) (*mgo.Video, func() error, error) {
        asset := doc.FindAsset(c.AssetID)
        if asset == nil {
                return nil, nil, fmt.Errorf("clip %s tham chiếu asset không tồn tại (%s)", c.ID, c.AssetID)
        }
        if asset.Missing {
                return nil, nil, fmt.Errorf("tệp nguồn không còn tồn tại: %s", asset.Path)
        }

        srcPath := asset.Path
        if asset.Kind == project.AssetVideo {
                if c.Effects.Denoise || c.Effects.HasColorBalance() {
                        srcPath = videoPrepass(srcPath, c.Effects) // v1.2.3: + colorbalance
                }
                if c.Effects.AudioDenoise || c.Effects.VocalEnhance {
                        srcPath = audioPrepass(srcPath, c.Effects.VocalEnhance) // v1.2.3
                }
        }

        // Ảnh tĩnh: không có thời lượng riêng → dùng Image + WithDuration.
        if asset.Kind == project.AssetImage {
                img, err := mgo.Image(srcPath)
                if err != nil {
                        return nil, nil, fmt.Errorf("mở ảnh %s: %w", filepath.Base(asset.Path), err)
                }
                dur := c.OutMs - c.InMs
                if dur < 100 {
                        dur = 100
                }
                v := img.WithDuration(mgo.Time(time.Duration(dur) * time.Millisecond))
                v = applyTransform(v, c.Transform)
                v = applyEffects(v, c.Effects, 0)
                if c.LUTPath != "" {
                        v = v.LUT(c.LUTPath)
                }
                v = applyKeyframes(v, c.Keyframes)
                v = v.FitTo(doc.Canvas.Width, doc.Canvas.Height)
                v = applyTransparency(v, c, doc.Canvas.Width, doc.Canvas.Height) // v1.2.3
                if err := v.Err(); err != nil {
                        _ = img.Close()
                        return nil, nil, fmt.Errorf("xây dựng clip ảnh %s: %w", c.ID, err)
                }
                return v, func() error { return img.Close() }, nil
        }

        src, err := mgo.OpenVideo(srcPath)
        if err != nil {
                return nil, nil, fmt.Errorf("mở %s: %w", filepath.Base(asset.Path), err)
        }
        closeSrc := func() error { return src.Close() }

        v := src
        // Cắt theo In/Out (moviego Time = time.Duration).
        in := mgo.Time(time.Duration(c.InMs) * time.Millisecond)
        if c.InMs > 0 || c.OutMs < asset.DurationMs {
                out := mgo.Time(time.Duration(c.OutMs) * time.Millisecond)
                v = v.Subclip(in, out)
        }
        // Nhóm biến đổi thời gian (tốc độ / ngược / boomerang / lặp / freeze).
        v = applyTime(v, c)
        // Biến đổi hình học rồi hiệu ứng màu/sắc rồi đưa về kích thước canvas.
        v = applyTransform(v, c.Transform)
        var dropDur mgo.Time
        if c.WaterDropMs > 0 {
                dropDur = mgo.Time(time.Duration(c.WaterDropMs) * time.Millisecond)
        }
        v = applyEffects(v, c.Effects, dropDur)
        if c.LUTPath != "" {
                v = v.LUT(c.LUTPath)
        }
        v = applyKeyframes(v, c.Keyframes)
        v = v.FitTo(doc.Canvas.Width, doc.Canvas.Height)
        v = applyTransparency(v, c, doc.Canvas.Width, doc.Canvas.Height) // v1.2.3
        // Âm thanh.
        if c.Mute || doc.MuteAll { // v1.2.3: mute toàn bộ timeline
                v = v.WithoutAudio()
        } else {
                if c.Volume != 1 {
                        v = v.Volume(c.Volume)
                }
                if c.AudioFadeInMs > 0 {
                        v = v.AudioFadeIn(mgo.Time(time.Duration(c.AudioFadeInMs) * time.Millisecond))
                }
                if c.AudioFadeOutMs > 0 {
                        v = v.AudioFadeOut(mgo.Time(time.Duration(c.AudioFadeOutMs) * time.Millisecond))
                }
        }
        if err := v.Err(); err != nil {
                _ = closeSrc()
                return nil, nil, fmt.Errorf("xây dựng clip %s: %w", c.ID, err)
        }
        return v, closeSrc, nil
}

// Compile biên dịch toàn bộ Document thành một graph moviego duy nhất.
// Graph thuần mô tả — chưa xảy ra decode/encode nào cho tới WriteVideo.
func Compile(doc *project.Document, opts CompileOptions) (*Graph, error) {
        if len(doc.Clips) == 0 {
                return nil, ErrNoClips
        }
        g := &Graph{Canvas: doc.Canvas}

        if len(doc.Clips) == 1 {
                v, closeSrc, err := buildClipChain(doc, doc.Clips[0])
                if err != nil {
                        return nil, err
                }
                g.closer = append(g.closer, closeSrc)
                g.Root = v
        } else {
                seq := mgo.NewSequence()
                for i, c := range doc.Clips {
                        v, closeSrc, err := buildClipChain(doc, c)
                        if err != nil {
                                g.Close()
                                return nil, err
                        }
                        g.closer = append(g.closer, closeSrc)

                        if i == 0 {
                                seq = seq.Add(v)
                                continue
                        }
                        // Chuyển cảnh sau clip i-1 (nếu có) — giới hạn overlap an toàn.
                        step, has := transitionStepFor(doc, i-1)
                        if has {
                                seq = seq.Then(step)
                        }
                        seq = seq.Add(v)
                }
                g.Root = seq.Video()
        }

        // Ép độ phân giải đầu ra nếu có preset / xem trước.
        if opts.TargetHeight > 0 && opts.TargetHeight != doc.Canvas.Height {
                f := evenScaleFactor(doc.Canvas.Width, doc.Canvas.Height, opts.TargetHeight)
                if f > 0 && f != 1 {
                        g.Root = g.Root.Resize(f)
                }
        }

        // v1.1: watermark → timecode → lớp phủ → phụ đề → credits.
        if err := g.applyDecorations(doc); err != nil {
                g.Close()
                return nil, err
        }

        if err := g.Root.Err(); err != nil {
                g.Close()
                return nil, err
        }
        return g, nil
}

// applyDecorations gắn watermark, timecode, lớp phủ, phụ đề, credits lên root.
func (g *Graph) applyDecorations(doc *project.Document) error {
        total := doc.TimelineTotalMs()

        // Watermark (logo góc màn hình).
        if wm := doc.Watermark; wm != nil && wm.AssetID != "" {
                as := doc.FindAsset(wm.AssetID)
                if as != nil && !as.Missing && as.Kind == project.AssetImage {
                        logo, err := mgo.Image(as.Path)
                        if err != nil {
                                return fmt.Errorf("mở watermark: %w", err)
                        }
                        g.closer = append(g.closer, func() error { return logo.Close() })
                        lg := logo
                        if wm.ScalePct > 0 && wm.ScalePct < 100 {
                                w := int(float64(doc.Canvas.Width) * clampPct(wm.ScalePct) / 100)
                                if w%2 != 0 {
                                        w++
                                }
                                s := lg.Size()
                                if s.W > 0 {
                                        h := int(math.Round(float64(s.H) * float64(w) / float64(s.W)))
                                        if h%2 != 0 {
                                                h++
                                        }
                                        lg = lg.ResizeTo(w, maxInt(2, h))
                                }
                        }
                        if wm.Opacity > 0 && wm.Opacity < 1 {
                                lg = lg.Animate(mgo.PropOpacity, []mgo.Keyframe{
                                        {At: 0, Val: wm.Opacity},
                                }, nil)
                        }
                        corner := mgo.BottomRight
                        switch wm.Corner {
                        case project.CornerTL:
                                corner = mgo.TopLeft
                        case project.CornerTR:
                                corner = mgo.TopRight
                        case project.CornerBL:
                                corner = mgo.BottomLeft
                        }
                        margin := wm.MarginPx
                        if margin <= 0 {
                                margin = 24
                        }
                        g.Root = g.Root.Watermark(lg, mgo.Corner(corner), mgo.WithMargin(margin))
                }
        }

        // Timecode burn-in.
        if tc := doc.Timecode; tc != nil && tc.Enabled {
                format := mgo.TCClock
                switch tc.Format {
                case "millis":
                        format = mgo.TCMillis
                case "frames":
                        format = mgo.TCFrames
                }
                fs := tc.FontSize
                if fs <= 0 {
                        fs = 28
                }
                g.Root = g.Root.BurnTimecode(mgo.TimecodeOptions{
                        Format:      format,
                        FontSize:    fs,
                        Color:       color.RGBA{R: 255, G: 255, B: 255, A: 255},
                        Stroke:      color.RGBA{A: 255},
                        StrokeWidth: 1,
                        Position:    mgo.RelPos(clamp01(tc.PosX), clamp01(tc.PosY)),
                })
        }

        // Lớp phủ (chữ / hình / media) — v1.2.3: công tắc 👁 ẩn lớp phủ.
        if doc.HideOverlays {
                return g.applySubsAndCredits(doc, total)
        }
        for i, o := range doc.Overlays {
                child, closeFn, ok := buildOverlayChild(doc, o, 10+i, total)
                if !ok {
                        continue
                }
                g.closer = append(g.closer, closeFn)
                g.Root = mgo.Composite(g.Root, child)
        }

        return g.applySubsAndCredits(doc, total)
}

// applySubsAndCredits gắn phụ đề burn-in + credits (tách từ applyDecorations
// để dùng chung khi 👁 ẩn lớp phủ — v1.2.3).
func (g *Graph) applySubsAndCredits(doc *project.Document, total int64) error {
        if st := doc.Subs; st != nil && st.Burn && len(st.Cues) > 0 && total > 0 {
                cues := make([]mgo.Cue, 0, len(st.Cues))
                for _, cu := range st.Cues {
                        s, e := cu.StartMs, cu.EndMs
                        if s < 0 {
                                s = 0
                        }
                        if e > total {
                                e = total
                        }
                        if e <= s || strings.TrimSpace(cu.Text) == "" {
                                continue
                        }
                        cues = append(cues, mgo.Cue{
                                Start: mgo.Time(time.Duration(s) * time.Millisecond),
                                End:   mgo.Time(time.Duration(e) * time.Millisecond),
                                Text:  cu.Text,
                        })
                }
                if len(cues) > 0 {
                        fs := st.FontSize
                        if fs <= 0 {
                                fs = 42
                        }
                        subOpts := mgo.SubtitleOptions{
                                Size:        mgo.Size{W: doc.Canvas.Width, H: doc.Canvas.Height},
                                FontSize:    fs,
                                Color:       hexColor(st.ColorHex),
                                Stroke:      color.RGBA{A: 255},
                                StrokeWidth: math.Max(1, fs/20),
                                Layout:      mgo.LayoutCaption,
                        }
                        if p := textFontPath(); p != "" {
                                subOpts.FontPath = p
                        }
                        subs, err := mgo.Subtitles(cues, subOpts)
                        if err == nil {
                                subs = subs.Layer(1_000_000) // full-canvas → vị trí mặc định (0,0)
                                g.Root = mgo.Composite(g.Root, subs)
                        } else {
                                fmt.Fprintf(os.Stderr, "[engine] phụ đề: %v\n", err)
                        }
                }
        }

        // Credits chạy cuối.
        if cr := doc.Credits; cr != nil && len(cr.Lines) > 0 && total > 0 {
                fs := cr.FontSize
                if fs <= 0 {
                        fs = 32
                }
                speed := cr.SpeedPXS
                if speed <= 0 {
                        speed = 60
                }
                opts := mgo.CreditsOptions{
                        Size:     mgo.Size{W: doc.Canvas.Width, H: doc.Canvas.Height},
                        FontSize: fs,
                        Speed:    speed,
                        Color:    color.RGBA{R: 255, G: 255, B: 255, A: 255},
                }
                if p := textFontPath(); p != "" {
                        opts.FontPath = p
                }
                roll, err := mgo.Credits(cr.Lines, opts)
                if err == nil {
                        d := roll.Duration()
                        startMs := int64(0)
                        if mgo.Finite(d) {
                                durMs := int64(time.Duration(d) / time.Millisecond)
                                startMs = total - durMs
                                if startMs < 0 {
                                        startMs = 0
                                        // Credits dài hơn video: giới hạn trong tổng thời lượng.
                                        roll = roll.WithDuration(mgo.Time(time.Duration(total) * time.Millisecond))
                                }
                        }
                        roll = roll.WithStart(mgo.Time(time.Duration(startMs) * time.Millisecond)).Layer(1_000_000)
                        g.Root = mgo.Composite(g.Root, roll)
                } else {
                        fmt.Fprintf(os.Stderr, "[engine] credits: %v\n", err)
                }
        }
        return nil
}

// transitionStepFor dựng TransitionStep sau clip index i với thời lượng hợp lệ.
func transitionStepFor(doc *project.Document, i int) (mgo.TransitionStep, bool) {
        c := doc.Clips[i]
        t := doc.TransitionAfter(c.ID)
        if t == nil {
                return mgo.TransitionStep{}, false
        }
        // Overlap không được vượt quá thời lượng hiệu dụng hai clip kề nhau (v1.1).
        limit := c.EffectiveMs()
        if i+1 < len(doc.Clips) {
                if n := doc.Clips[i+1].EffectiveMs(); n < limit {
                        limit = n
                }
        }
        dur := t.DurationMs
        if dur > limit-120 { // chừa 120ms để hai clip còn nhìn thấy nhau
                dur = limit - 120
        }
        if dur < 120 {
                return mgo.TransitionStep{}, false
        }
        return transitionStep(t.Type, mgo.Time(time.Duration(dur)*time.Millisecond))
}

// ---------------------------------------------------------------------------
// Xuất video
// ---------------------------------------------------------------------------

// ExportFormat định dạng đầu ra.
type ExportFormat string

const (
        FormatMP4   ExportFormat = "mp4"
        FormatGIF   ExportFormat = "gif"
        FormatAlpha ExportFormat = "movalpha" // v1.1: MOV QuickTime alpha (qtrle)
)

// ExportRequest yêu cầu xuất từ frontend.
type ExportRequest struct {
        Format     ExportFormat `json:"format"`
        OutputPath string       `json:"outputPath"`
        CRF        int          `json:"crf"`     // mp4: 14..34
        TargetH    int          `json:"targetH"` // 0 = giữ canvas
        Preset     string       `json:"preset"`  // veryfast..slow
        GifFPS     int          `json:"gifFps"`  // gif: 10..24
        HWAccel    string       `json:"hwAccel"` // v1.1: ""|"nvenc"|"qsv"
}

// ProgressFn được gọi mỗi ~150ms với (số khung đã hoàn thành, tổng khung).
type ProgressFn func(done, total int)

// progressAdapter hiện thực mgo.Progress và đẩy tiến trình qua callback throttle.
type progressAdapter struct {
        fn       ProgressFn
        done     int64
        total    int
        lastTick time.Time
}

func (p *progressAdapter) SetTotal(n int) {
        p.total = n
        p.emit(true)
}

func (p *progressAdapter) Step() {
        p.done++
        p.emit(false)
}

func (p *progressAdapter) emit(force bool) {
        if p.fn == nil {
                return
        }
        now := time.Now()
        if !force && now.Sub(p.lastTick) < 150*time.Millisecond {
                return
        }
        p.lastTick = now
        p.fn(int(p.done), p.total)
}

// WriteVideo xuất graph ra tệp. ctx cho phép huỷ giữa chừng.
func WriteVideo(ctx context.Context, g *Graph, req ExportRequest, progress ProgressFn) error {
        if req.OutputPath == "" {
                return errors.New("chưa chọn nơi lưu tệp")
        }
        opts := mgo.ExportOptions{Rate: mgo.Rate(g.Canvas.Rate)}
        switch req.Format {
        case FormatGIF:
                fps := req.GifFPS
                if fps <= 0 {
                        fps = 12
                }
                if fps > 24 {
                        fps = 24
                }
                opts.Codec = "gif"
                opts.PixFmt = "pal8"
                opts.DisableAudio = true
                opts.Rate = mgo.Rate{Num: fps, Den: 1}
        case FormatAlpha:
                // Bọc root trong composite trong suốt → .mov qtrle (không âm thanh).
                root := mgo.CompositeWith(mgo.CompositeOptions{
                        Size:        mgo.Size{W: g.Canvas.Width, H: g.Canvas.Height},
                        Transparent: true,
                }, g.Root)
                opts.DisableAudio = true
                opts.Progress = &progressAdapter{fn: progress}
                return mgo.WriteVideo(ctx, root, req.OutputPath, opts)
        case FormatMP4, "":
                crf := req.CRF
                if crf == 0 {
                        crf = 20
                }
                if crf < 14 {
                        crf = 14
                }
                if crf > 34 {
                        crf = 34
                }
                opts.CRF = crf
                preset := req.Preset
                if preset == "" {
                        preset = "medium"
                }
                opts.Preset = preset
                opts.ExtraOutputArgs = []string{"-movflags", "+faststart"}
                if req.HWAccel == "nvenc" || req.HWAccel == "qsv" {
                        opts.HWAccel = req.HWAccel
                }
        default:
                return fmt.Errorf("định dạng không hỗ trợ: %s", req.Format)
        }
        opts.Progress = &progressAdapter{fn: progress}
        return mgo.WriteVideo(ctx, g.Root, req.OutputPath, opts)
}

// Validate kiểm tra request trước khi chạy (để GUI báo lỗi nhanh).
func (r *ExportRequest) Validate() error {
        if r.OutputPath == "" {
                return errors.New("chưa chọn nơi lưu tệp")
        }
        switch r.Format {
        case FormatMP4:
                if r.CRF < 14 {
                        r.CRF = 14
                }
                if r.CRF > 34 {
                        r.CRF = 34
                }
        case FormatGIF, FormatAlpha:
                // ok
        default:
                return fmt.Errorf("định dạng không hỗ trợ: %s", r.Format)
        }
        return nil
}

// ---------------------------------------------------------------------------
// Hỗ trợ xem trước
// ---------------------------------------------------------------------------

// RenderClipFrame xuất MỘT khung hình của clip (đã áp biến đổi + fit canvas)
// tại thời điểm atMs (trong trục clip, không phải timeline) ra JPEG.
// Dùng WriteFrameSequence trên graph 1-khung — WYSIWYG với bản xuất cuối.
func RenderClipFrame(ctx context.Context, doc *project.Document, clipID string, atMs int64, quality int) (image.Image, error) {
        c := doc.FindClip(clipID)
        if c == nil {
                return nil, fmt.Errorf("không tìm thấy clip %s", clipID)
        }
        dur := c.DurationMs()
        if dur <= 0 {
                return nil, errors.New("clip có thời lượng 0")
        }
        at := atMs
        if at < 0 {
                at = 0
        }
        if at > dur-1 {
                at = dur - 1
        }
        // Clip tạm: chỉ clip này, bỏ transition.
        tmp := &project.Document{
                Version: doc.Version,
                Canvas:  doc.Canvas,
                Assets:  doc.Assets,
                Clips:   []*project.Clip{c},
        }
        g, err := Compile(tmp, CompileOptions{})
        if err != nil {
                return nil, err
        }
        defer g.Close()

        frameDur := time.Second / time.Duration(maxInt(1, int(doc.Canvas.Rate.Float())))
        // Chọn rate của asset nếu canvas rate = 0.
        start := mgo.Time(time.Duration(at) * time.Millisecond)
        oneFrame := mgo.Time(frameDur)
        v := g.Root.Subclip(start, start+oneFrame)
        if err := v.Err(); err != nil {
                return nil, fmt.Errorf("lấy khung tại %dms: %w", atMs, err)
        }

        dir, err := os.MkdirTemp("", "vksframe-")
        if err != nil {
                return nil, err
        }
        defer os.RemoveAll(dir)
        n, err := mgo.WriteFrameSequence(ctx, v, dir, mgo.FrameSequenceOptions{
                Format:  mgo.FormatJPEG,
                Quality: quality,
        })
        if err != nil {
                return nil, err
        }
        if n < 1 {
                return nil, errors.New("không xuất được khung hình")
        }
        entries, _ := os.ReadDir(dir)
        for _, e := range entries {
                f, err := os.Open(filepath.Join(dir, e.Name()))
                if err != nil {
                        continue
                }
                img, err := jpeg.Decode(f)
                _ = f.Close()
                if err == nil {
                        return img, nil
                }
        }
        return nil, errors.New("đọc khung hình thất bại")
}
