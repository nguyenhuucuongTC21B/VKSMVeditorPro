// Package project định nghĩa mô hình dữ liệu dự án (document) của VKSeditorPro
// và cơ chế lưu/tải JSON an toàn (ghi atomic với backup + rollback), lấy cảm hứng
// từ pattern store của Biz Studio.
package project

import (
        "encoding/json"
        "errors"
        "fmt"
        "math"
        "os"
        "path/filepath"
        "sort"
        "sync"
        "time"
)

// ---------------------------------------------------------------------------
// Mô hình dữ liệu
// ---------------------------------------------------------------------------

// Rate là frame rate dạng số hữu tỉ (Num/Den), tránh sai số dấu phẩy động.
type Rate struct {
        Num int `json:"num"`
        Den int `json:"den"`
}

// Float trả về giá trị hiển thị (chỉ dùng để hiển thị, không dùng để tính toán).
func (r Rate) Float() float64 {
        if r.Den == 0 {
                return 0
        }
        return float64(r.Num) / float64(r.Den)
}

// Valid kiểm tra rate hợp lệ.
func (r Rate) Valid() bool { return r.Num > 0 && r.Den > 0 }

// AssetKind loại tư liệu trong thư viện.
type AssetKind string

const (
        AssetVideo AssetKind = "video"
        AssetAudio AssetKind = "audio"
        AssetImage AssetKind = "image"
)

// Asset là một tư liệu đã được nhập vào thư viện.
type Asset struct {
        ID         string    `json:"id"`
        Path       string    `json:"path"`
        Name       string    `json:"name"`
        Kind       AssetKind `json:"kind"`
        DurationMs int64     `json:"durationMs"`
        Width      int       `json:"width"`
        Height     int       `json:"height"`
        Rate       Rate      `json:"rate"`
        HasAudio   bool      `json:"hasAudio"`
        Codec      string    `json:"codec"`
        SizeBytes  int64     `json:"sizeBytes"`
        Missing    bool      `json:"missing"`
}

// Transform là biến đổi hình học áp cho một clip.
type Transform struct {
        RotateDeg float64 `json:"rotateDeg"` // 0 / 90 / 180 / 270
        FlipH     bool    `json:"flipH"`
        FlipV     bool    `json:"flipV"`
        Zoom      float64 `json:"zoom"` // 1.0 = vừa khung; 1.0..4.0 phóng to (cắt viền)
        // v1.2.3: pan khung nhìn khi zoom > 1 — 0.5 = giữa; 0 = trái/trên; 1 = phải/dưới
        // (nền tảng Auto Reframe: quyết định vùng khung giữ lại khi cắt cúp 16:9 → 9:16).
        PosX float64 `json:"posX"`
        PosY float64 `json:"posY"`
}

// Normalize trả giá trị an toàn mặc định cho Transform.
func (t Transform) Normalize() Transform {
        if t.Zoom < 1.0 {
                t.Zoom = 1.0
        }
        if t.Zoom > 4.0 {
                t.Zoom = 4.0
        }
        t.PosX = clamp01OrDefault(t.PosX, 0.5)
        t.PosY = clamp01OrDefault(t.PosY, 0.5)
        t.RotateDeg = normalizeDeg(t.RotateDeg)
        return t
}

// clamp01OrDefault: NaN, 0 (thiếu — dự án cũ), hoặc ngoài [0,1] → fallback.
// Giá trị sát mép dùng 0.01/0.99 (khác biệt không nhận diện được bằng mắt).
func clamp01OrDefault(v, fb float64) float64 {
        if math.IsNaN(v) || v <= 0 || v > 1 {
                return fb
        }
        return v
}

func normalizeDeg(d float64) float64 {
        d = float64(int(d+0.5) % 360)
        for d < 0 {
                d += 360
        }
        return d
}

// ClipEffects là nhóm hiệu ứng hình ảnh / chỉnh màu áp cho một clip.
// Mọi giá trị 0 / false = không áp dụng.
type ClipEffects struct {
        Brightness float64 `json:"brightness"` // -0.5..0.5
        Contrast   float64 `json:"contrast"`   // 0..2, 1 = giữ nguyên
        Saturation float64 `json:"saturation"` // 0..3, 1 = giữ nguyên
        Gamma      float64 `json:"gamma"`      // 0.2..3, 1 = giữ nguyên
        Blur       float64 `json:"blur"`       // GaussianBlur sigma 0..20
        Sharpen    float64 `json:"sharpen"`    // unsharp amount 0..3
        Vignette   float64 `json:"vignette"`   // 0..1
        Grayscale  bool    `json:"grayscale"`
        Invert     bool    `json:"invert"`
        Denoise    bool    `json:"denoise"` // hqdn3d qua ffmpeg (pre-pass)
        // v1.2.3: âm thanh + tông màu theo vùng sáng.
        AudioDenoise bool `json:"audioDenoise"` // afftdn — lọc tạp âm nền (quạt/gió/ồn)
        VocalEnhance bool `json:"vocalEnhance"` // làm sạch giọng nói: denoise + band-pass + presence EQ
        // ColorBalance (bánh xe màu rút gọn): -1..1 mỗi kênh R/G/B × Tối/Trung/Sáng.
        CBShadowR float64 `json:"cbShadowR"`
        CBShadowG float64 `json:"cbShadowG"`
        CBShadowB float64 `json:"cbShadowB"`
        CBMidR    float64 `json:"cbMidR"`
        CBMidG    float64 `json:"cbMidG"`
        CBMidB    float64 `json:"cbMidB"`
        CBHighR   float64 `json:"cbHighR"`
        CBHighG   float64 `json:"cbHighG"`
        CBHighB   float64 `json:"cbHighB"`
}

// IsZero báo hiệu không có hiệu ứng nào được bật.
func (e ClipEffects) IsZero() bool {
        return e == ClipEffects{}
}

// Normalize kẹp các giá trị hiệu ứng vào khoảng hợp lệ.
func (e ClipEffects) Normalize() ClipEffects {
        clamp := func(v, lo, hi float64) float64 {
                if v < lo {
                        return lo
                }
                if v > hi {
                        return hi
                }
                return v
        }
        e.Brightness = clamp(e.Brightness, -0.5, 0.5)
        e.Contrast = clamp(e.Contrast, 0, 2)
        e.Saturation = clamp(e.Saturation, 0, 3)
        e.Gamma = clamp(e.Gamma, 0.2, 3)
        e.Blur = clamp(e.Blur, 0, 20)
        e.Sharpen = clamp(e.Sharpen, 0, 3)
        e.Vignette = clamp(e.Vignette, 0, 1)
        for _, p := range []*float64{&e.CBShadowR, &e.CBShadowG, &e.CBShadowB,
                &e.CBMidR, &e.CBMidG, &e.CBMidB, &e.CBHighR, &e.CBHighG, &e.CBHighB} {
                *p = clamp(*p, -1, 1)
        }
        return e
}

// HasColorBalance báo hiệu có chỉnh tông màu theo vùng sáng.
func (e ClipEffects) HasColorBalance() bool {
        return e.CBShadowR != 0 || e.CBShadowG != 0 || e.CBShadowB != 0 ||
                e.CBMidR != 0 || e.CBMidG != 0 || e.CBMidB != 0 ||
                e.CBHighR != 0 || e.CBHighG != 0 || e.CBHighB != 0
}

// KeyframePoint ghim giá trị thuộc tính (scale / opacity) tại một thời điểm
// clip-local để tạo hoạt hình keyframe qua moviego Animate.
type KeyframePoint struct {
        Prop string  `json:"prop"` // "scale" | "opacity"
        AtMs int64   `json:"atMs"`
        Val  float64 `json:"val"`
}

// Clip là một đoạn trên timeline, tham chiếu tới một Asset.
type Clip struct {
        ID             string          `json:"id"`
        AssetID        string          `json:"assetId"`
        InMs           int64           `json:"inMs"`
        OutMs          int64           `json:"outMs"` // loại trừ
        Mute           bool            `json:"mute"`
        Volume         float64         `json:"volume"` // 0..2, 1 = giữ nguyên
        Transform      Transform       `json:"transform"`
        Effects        ClipEffects     `json:"effects"`        // v1.1: hiệu ứng + chỉnh màu
        LUTPath        string          `json:"lutPath"`        // v1.1: file .cube
        Speed          float64         `json:"speed"`          // v1.1: 0.25..4, 1 = thường
        Reverse        bool            `json:"reverse"`        // v1.1: phát ngược
        Boomerang      bool            `json:"boomerang"`      // v1.1: xuôi rồi ngược
        LoopN          int             `json:"loopN"`          // v1.1: 1 = không lặp; n = phát n lần
        FreezeStartMs  int64           `json:"freezeStartMs"`  // v1.1: đóng khung đầu
        FreezeEndMs    int64           `json:"freezeEndMs"`    // v1.1: đóng khung cuối
        AudioFadeInMs  int64           `json:"audioFadeInMs"`  // v1.1
        AudioFadeOutMs int64           `json:"audioFadeOutMs"` // v1.1
        WaterDropMs    int64           `json:"waterDropMs"`    // v1.1: gợn sóng nước
        Keyframes      []KeyframePoint `json:"keyframes"`      // v1.1: hoạt hình keyframe
        Opacity        float64         `json:"opacity"`        // v1.2.3: 0..1 độ mờ đục tĩnh, 1 = đặc
}

// EffectiveMs trả về thời lượng hiệu dụng trên timeline sau tốc độ / lặp /
// boomerang / đóng khung. Đây là nguồn chân lý cho mọi phép tính timeline.
func (c *Clip) EffectiveMs() int64 {
        base := c.DurationMs()
        speed := c.Speed
        if speed <= 0 {
                speed = 1
        }
        eff := float64(base) / speed
        if c.Boomerang {
                eff *= 2
        } else if c.LoopN > 1 {
                eff *= float64(c.LoopN)
        }
        eff += float64(c.FreezeStartMs + c.FreezeEndMs)
        if eff < 0 {
                eff = 0
        }
        return int64(eff + 0.5)
}

// TransitionType định danh các loại chuyển cảnh (map sang moviego ở engine).
type TransitionType string

// Tất cả chuyển cảnh hỗ trợ ở v1.
const (
        TransCrossfade      TransitionType = "crossfade"
        TransDissolve       TransitionType = "dissolve"
        TransFadeThroughBlk TransitionType = "fadeblack"
        TransWipeLeft       TransitionType = "wipeleft"
        TransWipeRight      TransitionType = "wiperight"
        TransWipeUp         TransitionType = "wipeup"
        TransWipeDown       TransitionType = "wipedown"
        TransSlideLeft      TransitionType = "slideleft"
        TransSlideRight     TransitionType = "slideright"
        TransSlideUp        TransitionType = "slideup"
        TransSlideDown      TransitionType = "slidedown"
        TransPushLeft       TransitionType = "pushleft"
        TransPushRight      TransitionType = "pushright"
        TransPushUp         TransitionType = "pushup"
        TransPushDown       TransitionType = "pushdown"
        TransIrisOpen       TransitionType = "irisOpen"
        TransIrisClose      TransitionType = "irisOpenClose"
)

// Transition là chuyển cảnh giữa clip có ID AfterClipID và clip kế tiếp.
type Transition struct {
        AfterClipID string         `json:"afterClipId"`
        Type        TransitionType `json:"type"`
        DurationMs  int64          `json:"durationMs"`
}

// Canvas là khung hình xuất của dự án.
type Canvas struct {
        Width  int  `json:"width"`
        Height int  `json:"height"`
        Rate   Rate `json:"rate"`
}

// ---------------------------------------------------------------------------
// v1.1 — Lớp phủ (overlay), phụ đề, watermark, timecode, credits
// ---------------------------------------------------------------------------

// OverlayKind loại lớp phủ trên timeline.
type OverlayKind string

const (
        OverlayText  OverlayKind = "text"  // chữ / tiêu đề (có hoạt hình)
        OverlayShape OverlayKind = "shape" // hình vẽ vector
        OverlayMedia OverlayKind = "media" // video/ảnh xếp chồng (PiP, green-screen)
)

// TextAnimKind loại hoạt hình chữ (map sang moviego Anim*).
type TextAnimKind string

const (
        TextAnimNone       TextAnimKind = ""
        TextAnimFadeIn     TextAnimKind = "fade"
        TextAnimTypewriter TextAnimKind = "typewriter"
        TextAnimSlideUp    TextAnimKind = "slideup"
        TextAnimSlideDown  TextAnimKind = "slidedown"
        TextAnimSlideLeft  TextAnimKind = "slideleft"
        TextAnimSlideRight TextAnimKind = "slideright"
        TextAnimPop        TextAnimKind = "pop"
)

// ChromaKeySettings cấu hình tách nền xanh/lì (chỉ dùng cho lớp phủ media).
type ChromaKeySettings struct {
        Enabled    bool    `json:"enabled"`
        Hex        string  `json:"hex"`        // "00FF00" hoặc "#00FF00"
        Similarity float64 `json:"similarity"` // 0..1
        Blend      float64 `json:"blend"`      // 0..1
}

// Overlay là một lớp phủ xếp chồng lên timeline chính trong khoảng thời gian
// [StartMs, EndMs). Vị trí PosX/PosY theo tỉ lệ 0..1 của khoảng trống
// (0.5 = giữa). Lớp media có thể tách nền chroma-key.
type Overlay struct {
        ID      string      `json:"id"`
        Kind    OverlayKind `json:"kind"`
        StartMs int64       `json:"startMs"`
        EndMs   int64       `json:"endMs"`
        PosX    float64     `json:"posX"` // 0..1 (RelPos)
        PosY    float64     `json:"posY"`
        // Text
        Text     string       `json:"text,omitempty"`
        FontSize float64      `json:"fontSize,omitempty"`
        ColorHex string       `json:"colorHex,omitempty"`
        Outline  bool         `json:"outline,omitempty"`
        Anim     TextAnimKind `json:"anim,omitempty"`
        AnimMs   int64        `json:"animMs,omitempty"`
        Bold     bool         `json:"bold,omitempty"`
        // Shape
        Shape     string  `json:"shape,omitempty"`     // rect|circle|ellipse|line
        WidthPct  float64 `json:"widthPct,omitempty"`  // % bề rộng canvas
        HeightPct float64 `json:"heightPct,omitempty"` // % bề cao canvas
        // Media (PiP)
        AssetID   string            `json:"assetId,omitempty"`
        ScalePct  float64           `json:"scalePct,omitempty"` // % kích thước canvas
        Volume    float64           `json:"volume,omitempty"`
        Muted     bool              `json:"muted,omitempty"`
        ChromaKey ChromaKeySettings `json:"chromaKey,omitempty"`

        // v1.3.1: chế độ lớp kiểu KineMaster/CapCut — hòa trộn (blend), độ mờ,
        // hiện dần (fade) và preset hiệu ứng nghệ thuật cho mọi lớp phủ.
        Blend        string  `json:"blend,omitempty"`        // ""|screen|multiply|overlay|darken|lighten|add
        Opacity      float64 `json:"opacity,omitempty"`      // 0..1; 0/thiếu = đặc (dự án cũ)
        FadeInMs     int64   `json:"fadeInMs,omitempty"`     // hiện dần từ đen trong suốt
        FadeOutMs    int64   `json:"fadeOutMs,omitempty"`
        FilterPreset string  `json:"filterPreset,omitempty"` // ""|bw|negative|sepia|vintage|vivid|contrast|dream
}

// Cue là một dòng phụ đề (ms).
type Cue struct {
        StartMs int64  `json:"startMs"`
        EndMs   int64  `json:"endMs"`
        Text    string `json:"text"`
}

// SubtitlesTrack là nguồn phụ đề nhúng trong dự án + tùy chọn ghi lên video.
type SubtitlesTrack struct {
        SourceName string  `json:"sourceName"` // tên file SRT đã nhập (tham khảo)
        Cues       []Cue   `json:"cues"`
        Burn       bool    `json:"burn"`
        FontSize   float64 `json:"fontSize"`
        ColorHex   string  `json:"colorHex"` // rỗng = trắng
}

// CornerPos góc khung cho watermark.
type CornerPos string

const (
        CornerTL CornerPos = "tl"
        CornerTR CornerPos = "tr"
        CornerBL CornerPos = "bl"
        CornerBR CornerPos = "br"
)

// Watermark cấu hình logo góc màn hình (asset ảnh).
type Watermark struct {
        AssetID  string    `json:"assetId"`
        Corner   CornerPos `json:"corner"`
        MarginPx int       `json:"marginPx"`
        Opacity  float64   `json:"opacity"`  // 0..1, 1 = đậm
        ScalePct float64   `json:"scalePct"` // % bề rộng canvas, 0 = kích thước gốc
}

// Timecode cấu hình ghi timecode chạy lên video.
type Timecode struct {
        Enabled  bool    `json:"enabled"`
        FontSize float64 `json:"fontSize"`
        Format   string  `json:"format"` // clock|millis|frames
        PosX     float64 `json:"posX"`   // 0..1
        PosY     float64 `json:"posY"`
}

// Credits cấu hình credits chạy cuối video.
type Credits struct {
        Lines    []string `json:"lines"`
        FontSize float64  `json:"fontSize"`
        SpeedPXS float64  `json:"speedPxs"` // px/giây
}

// Document là toàn bộ trạng thái dự án (lưu JSON).
type Document struct {
        Version     int           `json:"version"`
        Name        string        `json:"name"`
        Canvas      Canvas        `json:"canvas"`
        Assets      []*Asset      `json:"assets"`
        Clips       []*Clip       `json:"clips"`
        Transitions []*Transition `json:"transitions"`
        // v1.1
        Overlays  []*Overlay      `json:"overlays"`
        Subs      *SubtitlesTrack `json:"subs"`
        Watermark *Watermark      `json:"watermark"`
        Timecode  *Timecode       `json:"timecode"`
        Credits   *Credits        `json:"credits"`

        // v1.2.3: công tắc track (1 track video + lane lớp phủ).
        MuteAll      bool `json:"muteAll"`      // tắt tiếng TOÀN BỘ timeline khi dựng
        HideOverlays bool `json:"hideOverlays"` // ẩn mọi lớp phủ (không render)

        // v1.3.0: track âm thanh độc lập A1 (nhạc nền / giọng đọc song song video).
        AudioClips []*AudioClip `json:"audioClips"`

        // v1.2.1: thống kê tự sửa khi nạp dự án cũ/hỏng (không lưu vào JSON).
        RepairedClips int  `json:"-"` // số clip đã được gán lại in/out
        DroppedClips  int  `json:"-"` // số clip rác đã loại bỏ (asset không tồn tại/không hợp lệ)
        ForeignFormat bool `json:"-"` // tệp do phiên bản khác ghi, dữ liệu không tương thích
}

// ---------------------------------------------------------------------------
// Document helpers
// ---------------------------------------------------------------------------

// FindAsset tìm asset theo ID.
func (d *Document) FindAsset(id string) *Asset {
        for _, a := range d.Assets {
                if a.ID == id {
                        return a
                }
        }
        return nil
}

// FindClip tìm clip theo ID.
func (d *Document) FindClip(id string) *Clip {
        for _, c := range d.Clips {
                if c.ID == id {
                        return c
                }
        }
        return nil
}

// ClipIndex trả về vị trí của clip (hoặc -1).
func (d *Document) ClipIndex(id string) int {
        for i, c := range d.Clips {
                if c.ID == id {
                        return i
                }
        }
        return -1
}

// ClipDurationMs độ dài hiệu dụng của clip.
func (c *Clip) DurationMs() int64 {
        d := c.OutMs - c.InMs
        if d < 0 {
                return 0
        }
        return d
}

// TransitionAfter trả về chuyển cảnh sau clip có ID cho trước.
func (d *Document) TransitionAfter(clipID string) *Transition {
        for _, t := range d.Transitions {
                if t.AfterClipID == clipID {
                        return t
                }
        }
        return nil
}

// RemoveTransitionAfter xoá chuyển cảnh sau clipID (nếu có).
func (d *Document) RemoveTransitionAfter(clipID string) {
        out := d.Transitions[:0]
        for _, t := range d.Transitions {
                if t.AfterClipID != clipID {
                        out = append(out, t)
                }
        }
        d.Transitions = out
}

// TimelineTotalMs tính tổng thời lượng timeline:
// tổng clip hiệu dụng - tổng chuyển cảnh (chuyển cảnh trùng overlap).
// v1.1: dùng EffectiveMs (tính cả tốc độ/lặp/boomerang/freeze).
func (d *Document) TimelineTotalMs() int64 {
        var total int64
        for _, c := range d.Clips {
                total += c.EffectiveMs()
        }
        for _, t := range d.Transitions {
                total -= t.DurationMs
        }
        if total < 0 {
                total = 0
        }
        return total
}

// TimelineMsPerClip trả về mảng thời điểm bắt đầu của từng clip trên timeline
// (đã trừ overlap của chuyển cảnh), song song với d.Clips.
func (d *Document) TimelineMsPerClip() []int64 {
        starts := make([]int64, len(d.Clips))
        var cur int64
        for i, c := range d.Clips {
                starts[i] = cur
                cur += c.EffectiveMs()
                if i < len(d.Clips)-1 {
                        if t := d.TransitionAfter(c.ID); t != nil {
                                ov := t.DurationMs
                                if ov > c.EffectiveMs() {
                                        ov = c.EffectiveMs()
                                }
                                cur -= ov
                        }
                }
        }
        return starts
}

// FindOverlay tìm lớp phủ theo ID.
func (d *Document) FindOverlay(id string) *Overlay {
        for _, o := range d.Overlays {
                if o.ID == id {
                        return o
                }
        }
        return nil
}

// RemoveOverlay xoá lớp phủ theo ID.
func (d *Document) RemoveOverlay(id string) {
        out := d.Overlays[:0]
        for _, o := range d.Overlays {
                if o.ID != id {
                        out = append(out, o)
                }
        }
        d.Overlays = out
}

// SortOverlays sắp xếp lớp phủ theo thời điểm bắt đầu.
func (d *Document) SortOverlays() {
        sort.SliceStable(d.Overlays, func(i, j int) bool {
                return d.Overlays[i].StartMs < d.Overlays[j].StartMs
        })
}

// SortTransitions sắp xếp transitions ổn định (theo thứ tự clip).
func (d *Document) SortTransitions() {
        order := map[string]int{}
        for i, c := range d.Clips {
                order[c.ID] = i
        }
        sort.SliceStable(d.Transitions, func(i, j int) bool {
                return order[d.Transitions[i].AfterClipID] < order[d.Transitions[j].AfterClipID]
        })
}

// ---------------------------------------------------------------------------
// LoadFromBytes đọc document từ mảng byte JSON (dùng cho mở file .vksproj và
// chế độ headless) kèm sanitize đầy đủ như Store.Load.
func LoadFromBytes(b []byte) (*Document, error) {
        var d Document
        if err := json.Unmarshal(b, &d); err != nil {
                return nil, err
        }
        d.sanitize()
        return &d, nil
}

// ---------------------------------------------------------------------------
// Store — lưu/tải JSON an toàn
// ---------------------------------------------------------------------------

// ErrPersistence báo lỗi lưu trữ không khôi phục được.
var ErrPersistence = errors.New("lỗi lưu trữ dữ liệu dự án")

// Store quản lý file project.json với ghi atomic.
type Store struct {
        mu   sync.Mutex
        path string
}

// NewStore tạo store tại dir/project.json (dir tự tạo nếu thiếu).
func NewStore(dir string) (*Store, error) {
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return nil, fmt.Errorf("tạo thư mục dữ liệu: %w", err)
        }
        return &Store{path: filepath.Join(dir, "project.json")}, nil
}

// Path trả về đường dẫn file JSON.
func (s *Store) Path() string { return s.path }

// Save ghi document xuống đĩa theo kiểu atomic:
// marshal → ghi .tmp → rename; đồng thời giữ bản .bak để khôi phục.
func (s *Store) Save(d *Document) error {
        s.mu.Lock()
        defer s.mu.Unlock()
        data, err := json.MarshalIndent(d, "", "  ")
        if err != nil {
                return fmt.Errorf("%w: %v", ErrPersistence, err)
        }
        tmp := s.path + ".tmp"
        bak := s.path + ".bak"
        if err := os.WriteFile(tmp, data, 0o600); err != nil {
                return fmt.Errorf("%w: %v", ErrPersistence, err)
        }
        // Giữ bản cũ thành .bak (nếu có).
        if _, err := os.Stat(s.path); err == nil {
                _ = os.Rename(s.path, bak)
        }
        if err := os.Rename(tmp, s.path); err != nil {
                // Khôi phục .bak nếu rename thất bại.
                if _, err2 := os.Stat(bak); err2 == nil {
                        _ = os.Rename(bak, s.path)
                }
                return fmt.Errorf("%w: %v", ErrPersistence, err)
        }
        return nil
}

// Load đọc document từ đĩa; nếu file hỏng thì thử .bak; nếu chưa có file
// nào trả về document mới rỗng (không lỗi).
func (s *Store) Load() (*Document, error) {
        s.mu.Lock()
        defer s.mu.Unlock()
        d, err := readDoc(s.path)
        if err == nil {
                return d, nil
        }
        if os.IsNotExist(err) {
                return NewDocument(), nil
        }
        // Thử bản backup.
        if d2, err2 := readDoc(s.path + ".bak"); err2 == nil {
                return d2, nil
        }
        return nil, fmt.Errorf("file dự án hỏng: %w", err)
}

func readDoc(path string) (*Document, error) {
        data, err := os.ReadFile(path)
        if err != nil {
                return nil, err
        }
        var d Document
        if err := json.Unmarshal(data, &d); err != nil {
                return nil, err
        }
        d.sanitize()
        return &d, nil
}

// NewDocument trả document rỗng với canvas mặc định 1920x1080@30.
func NewDocument() *Document {
        return &Document{
                Version: 2,
                Name:    "Dự án không tên",
                Canvas: Canvas{
                        Width:  1920,
                        Height: 1080,
                        Rate:   Rate{Num: 30, Den: 1},
                },
                Assets:      []*Asset{},
                Clips:       []*Clip{},
                Transitions: []*Transition{},
                Overlays:    []*Overlay{},
                AudioClips:  []*AudioClip{},
        }
}

// sanitize chuẩn hoá dữ liệu sau khi load để chống trạng thái lạ.
func (d *Document) sanitize() {
        if d.Version <= 0 {
                d.Version = 1
        }
        if d.Name == "" {
                d.Name = "Dự án không tên"
        }
        if d.Canvas.Width <= 0 {
                d.Canvas.Width = 1920
        }
        if d.Canvas.Height <= 0 {
                d.Canvas.Height = 1080
        }
        if !d.Canvas.Rate.Valid() {
                d.Canvas.Rate = Rate{Num: 30, Den: 1}
        }
        if d.Version < 2 {
                d.Version = 2
        }
        if d.Assets == nil {
                d.Assets = []*Asset{}
        }
        if d.Clips == nil {
                d.Clips = []*Clip{}
        }
        if d.Transitions == nil {
                d.Transitions = []*Transition{}
        }
        if d.Overlays == nil {
                d.Overlays = []*Overlay{}
        }
        d.sanitizeAudioClips() // v1.3.0: nil-safe, dọn đoạn âm thanh rác
        for _, a := range d.Assets {
                if a.Kind == "" {
                        a.Kind = AssetVideo
                }
        }
        // v1.2.1: phát hiện tệp do phiên bản khác ghi (schema lạ): mọi asset đều
        // thiếu path -> dữ liệu không tin được, caller sẽ cách ly tệp và bắt đầu mới.
        if len(d.Assets) > 0 {
                emptyPath := 0
                for _, a := range d.Assets {
                        if a.Path == "" {
                                emptyPath++
                        }
                }
                if emptyPath == len(d.Assets) {
                        d.ForeignFormat = true
                }
        }
        // v1.2.1: tự chữa lành clip rác từ dự án cũ — nguyên nhân "timeline trống,
        // không xem trước được" khi nạp dữ liệu do bản cũ ghi.
        kept := d.Clips[:0]
        for _, c := range d.Clips {
                as := d.FindAsset(c.AssetID)
                if as == nil {
                        // Clip trỏ tới asset không tồn tại -> rác thật sự, bỏ kèm chuyển cảnh nối vào nó.
                        d.RemoveTransitionAfter(c.ID)
                        d.DroppedClips++
                        continue
                }
                repaired := false
                if as.Kind != AssetImage {
                        if as.DurationMs > 0 {
                                // Đưa in/out về vùng hợp lệ của asset (so với giá trị gốc
                                // để nhận ra clip rác kiểu 999999→100 và gán lại trọn vẹn).
                                origIn := c.InMs
                                if c.InMs < 0 || c.InMs >= as.DurationMs {
                                        c.InMs = 0
                                        repaired = true
                                }
                                if c.OutMs <= origIn || c.OutMs > as.DurationMs {
                                        c.OutMs = as.DurationMs
                                        repaired = true
                                }
                        } else if c.OutMs <= c.InMs {
                                // Asset 0 giây + clip rỗng -> không dùng được.
                                d.RemoveTransitionAfter(c.ID)
                                d.DroppedClips++
                                continue
                        }
                } else if c.OutMs <= c.InMs {
                        // Clip ảnh phải có chiều dài dương (mặc định 3 giây).
                        c.InMs = 0
                        c.OutMs = 3000
                        repaired = true
                }
                if repaired {
                        d.RepairedClips++
                }
                kept = append(kept, c)
        }
        d.Clips = kept
        // Dọn chuyển cảnh trỏ tới clip không còn.
        validClips := map[string]bool{}
        for _, c := range d.Clips {
                validClips[c.ID] = true
        }
        trs := d.Transitions[:0]
        for _, tr := range d.Transitions {
                if validClips[tr.AfterClipID] {
                        trs = append(trs, tr)
                }
        }
        d.Transitions = trs
        // Chuẩn hoá thuộc tính clip (giữ nguyên logic gốc v1.1).
        for _, c := range d.Clips {
                c.Transform = c.Transform.Normalize()
                if c.Opacity <= 0 || c.Opacity > 1 { // v1.2.3: 0/thiếu = đặc (dự án cũ)
                        c.Opacity = 1
                }
                if c.Volume <= 0 {
                        c.Volume = 1
                }
                if c.Volume > 2 {
                        c.Volume = 2
                }
                c.Effects = c.Effects.Normalize()
                if c.Speed <= 0 {
                        c.Speed = 1
                }
                if c.Speed > 4 {
                        c.Speed = 4
                }
                if c.Speed < 0.25 {
                        c.Speed = 0.25
                }
                if c.LoopN < 1 {
                        c.LoopN = 1
                }
                if c.LoopN > 20 {
                        c.LoopN = 20
                }
                if c.FreezeStartMs < 0 {
                        c.FreezeStartMs = 0
                }
                if c.FreezeEndMs < 0 {
                        c.FreezeEndMs = 0
                }
                if c.AudioFadeInMs < 0 {
                        c.AudioFadeInMs = 0
                }
                if c.AudioFadeOutMs < 0 {
                        c.AudioFadeOutMs = 0
                }
                if c.WaterDropMs < 0 {
                        c.WaterDropMs = 0
                }
        }
        for _, o := range d.Overlays {
                o.Normalize()
        }
        if d.Subs != nil {
                if d.Subs.Cues == nil {
                        d.Subs.Cues = []Cue{}
                }
                if d.Subs.FontSize <= 0 {
                        d.Subs.FontSize = 42
                }
        }
        if d.Watermark != nil {
                if d.Watermark.Opacity <= 0 {
                        d.Watermark.Opacity = 1
                }
                if d.Watermark.Opacity > 1 {
                        d.Watermark.Opacity = 1
                }
                if d.Watermark.MarginPx < 0 {
                        d.Watermark.MarginPx = 0
                }
        }
}

// Normalize kẹp giá trị lớp phủ vào khoảng hợp lệ.
func (o *Overlay) Normalize() {
        if o.PosX < 0 || o.PosX > 1 {
                o.PosX = 0.5
        }
        if o.PosY < 0 || o.PosY > 1 {
                o.PosY = 0.5
        }
        if o.EndMs < o.StartMs {
                o.EndMs = o.StartMs + 3000
        }
        switch o.Kind {
        case OverlayText:
                if o.FontSize <= 0 {
                        o.FontSize = 64
                }
                if o.FontSize > 300 {
                        o.FontSize = 300
                }
                if o.AnimMs < 0 {
                        o.AnimMs = 0
                }
                if o.AnimMs > 10000 {
                        o.AnimMs = 10000
                }
        case OverlayShape:
                if o.WidthPct <= 0 {
                        o.WidthPct = 30
                }
                if o.WidthPct > 100 {
                        o.WidthPct = 100
                }
                if o.HeightPct <= 0 {
                        o.HeightPct = 20
                }
                if o.HeightPct > 100 {
                        o.HeightPct = 100
                }
        case OverlayMedia:
                if o.ScalePct <= 0 {
                        o.ScalePct = 30
                }
                if o.ScalePct > 100 {
                        o.ScalePct = 100
                }
                if o.Volume <= 0 {
                        o.Volume = 1
                }
                if o.Volume > 2 {
                        o.Volume = 2
                }
        }
        // v1.3.1: chế độ lớp.
        if o.Opacity <= 0 || o.Opacity > 1 || math.IsNaN(o.Opacity) {
                o.Opacity = 1
        }
        if o.FadeInMs < 0 {
                o.FadeInMs = 0
        }
        if o.FadeInMs > 10000 {
                o.FadeInMs = 10000
        }
        if o.FadeOutMs < 0 {
                o.FadeOutMs = 0
        }
        if o.FadeOutMs > 10000 {
                o.FadeOutMs = 10000
        }
        if !ValidBlend(o.Blend) {
                o.Blend = ""
        }
        if !ValidOverlayFilter(o.FilterPreset) {
                o.FilterPreset = ""
        }
}

// ValidBlend báo chuỗi chế độ hòa trộn có được hỗ trợ hay không.
func ValidBlend(s string) bool {
        switch s {
        case "", "screen", "multiply", "overlay", "darken", "lighten", "add":
                return true
        }
        return false
}

// ValidOverlayFilter báo preset hiệu ứng lớp phủ có được hỗ trợ hay không.
func ValidOverlayFilter(s string) bool {
        switch s {
        case "", "bw", "negative", "sepia", "vintage", "vivid", "contrast", "dream":
                return true
        }
        return false
}

// NewID sinh ID mới (tiền tố + thời gian).
func NewID(prefix string) string {
        return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano()^int64(os.Getpid())<<8)
}
