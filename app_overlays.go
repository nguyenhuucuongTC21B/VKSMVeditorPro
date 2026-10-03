// app_overlays.go — bindings v1.1 cho lớp phủ (chữ / hình / video PiP),
// phụ đề SRT, watermark, timecode, credits và dựng timeline từ kịch bản JSON.
package main

import (
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "strings"

        wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

        "vkseditorpro/internal/applog"
        "vkseditorpro/internal/project"
        "vkseditorpro/internal/tools"
)

// ---------------------------------------------------------------------------
// Lớp phủ (overlay)
// ---------------------------------------------------------------------------

// AddOverlay thêm lớp phủ mới theo loại; trả lớp phủ đã tạo.
func (a *App) AddOverlay(kind string, startMs, endMs int64) (*project.Overlay, error) {
        var k project.OverlayKind
        switch kind {
        case "text":
                k = project.OverlayText
        case "shape":
                k = project.OverlayShape
        case "media":
                k = project.OverlayMedia
        default:
                return nil, fmt.Errorf("loại lớp phủ không hỗ trợ: %s", kind)
        }
        if endMs <= startMs {
                startMs, endMs = 0, 3000
        }
        o := &project.Overlay{
                ID:      project.NewID("ov"),
                Kind:    k,
                StartMs: startMs,
                EndMs:   endMs,
                PosX:    0.5,
                PosY:    0.5,
                Text:    "Nhập chữ tại đây",
                Shape:   "rect",
                ColorHex: "FFFFFF",
        }
        a.mu.Lock()
        total := a.doc.TimelineTotalMs()
        if endMs > total && total > 0 {
                o.EndMs = total
                if o.EndMs <= o.StartMs {
                        o.StartMs = 0
                        o.EndMs = 3000
                }
        }
        a.doc.Overlays = append(a.doc.Overlays, o)
        a.doc.SortOverlays()
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        return o, nil
}

// AddMediaOverlay v1.2.4: thêm lớp phủ media (PiP) từ tư liệu tại khoảng
// [startMs, endMs) — phục vụ kéo-thả tư liệu thẳng vào track V2 trên timeline
// (kiểu CapCut/KineMaster). Ảnh dùng 5s; video/audio bị chặn tối đa theo
// thời lượng tư liệu và theo chiều dài timeline.
func (a *App) AddMediaOverlay(assetID string, startMs, endMs int64) (*project.Overlay, error) {
        a.mu.Lock()
        as := a.doc.FindAsset(assetID)
        if as == nil {
                a.mu.Unlock()
                applog.Logf("AddMediaOverlay LỖI: không tìm thấy tư liệu %s", assetID)
                return nil, fmt.Errorf("không tìm thấy tư liệu %s", assetID)
        }
        if as.Missing {
                a.mu.Unlock()
                return nil, fmt.Errorf("tệp nguồn không còn: %s", as.Path)
        }
        dur := as.DurationMs
        if as.Kind == project.AssetImage {
                dur = 5000
        }
        if dur <= 0 {
                a.mu.Unlock()
                return nil, errors.New("tư liệu có thời lượng 0, không thể thêm lớp phủ")
        }
        if startMs < 0 {
                startMs = 0
        }
        if endMs <= startMs {
                endMs = startMs + dur
        }
        // Không dài hơn tư liệu (trừ ảnh) và không vượt quá chiều dài timeline.
        if as.Kind != project.AssetImage && endMs-startMs > dur {
                endMs = startMs + dur
        }
        if total := a.doc.TimelineTotalMs(); total > 0 {
                if startMs >= total {
                        startMs = total - 1000
                        if startMs < 0 {
                                startMs = 0
                        }
                }
                if endMs > total {
                        endMs = total
                }
        }
        if endMs-startMs < 500 {
                endMs = startMs + 500
        }
        o := &project.Overlay{
                ID:       project.NewID("ov"),
                Kind:     project.OverlayMedia,
                StartMs:  startMs,
                EndMs:    endMs,
                PosX:     0.5,
                PosY:     0.5,
                AssetID:  assetID,
                ScalePct: 30,
                Volume:   1,
        }
        a.doc.Overlays = append(a.doc.Overlays, o)
        a.doc.SortOverlays()
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        applog.Logf("AddMediaOverlay OK: %s [%d→%dms] (%s)", as.Name, o.StartMs, o.EndMs, o.ID)
        return o, nil
}

// OverlayPatch là bản vá thuộc tính lớp phủ — trường nil = không đổi.
type OverlayPatch struct {
        StartMs   *int64   `json:"startMs"`
        EndMs     *int64   `json:"endMs"`
        PosX      *float64 `json:"posX"`
        PosY      *float64 `json:"posY"`
        Text      *string  `json:"text"`
        FontSize  *float64 `json:"fontSize"`
        ColorHex  *string  `json:"colorHex"`
        Outline   *bool    `json:"outline"`
        Bold      *bool    `json:"bold"`
        Anim      *string  `json:"anim"`
        AnimMs    *int64   `json:"animMs"`
        Shape     *string  `json:"shape"`
        WidthPct  *float64 `json:"widthPct"`
        HeightPct *float64 `json:"heightPct"`
        AssetID   *string  `json:"assetId"`
        ScalePct  *float64 `json:"scalePct"`
        Volume    *float64 `json:"volume"`
        Muted     *bool    `json:"muted"`
        ChromaKey *project.ChromaKeySettings `json:"chromaKey"`

        // v1.3.1: chế độ lớp.
        Blend        *string  `json:"blend"`
        Opacity      *float64 `json:"opacity"`
        FadeInMs     *int64   `json:"fadeInMs"`
        FadeOutMs    *int64   `json:"fadeOutMs"`
        FilterPreset *string  `json:"filterPreset"`
}

// UpdateOverlay áp bản vá cho lớp phủ.
func (a *App) UpdateOverlay(id string, patch OverlayPatch) (*project.Overlay, error) {
        a.mu.Lock()
        o := a.doc.FindOverlay(id)
        if o == nil {
                a.mu.Unlock()
                return nil, fmt.Errorf("không tìm thấy lớp phủ %s", id)
        }
        if patch.StartMs != nil {
                v := *patch.StartMs
                if v < 0 {
                        v = 0
                }
                o.StartMs = v
        }
        if patch.EndMs != nil {
                v := *patch.EndMs
                if v < o.StartMs+100 {
                        v = o.StartMs + 100
                }
                o.EndMs = v
        }
        if patch.PosX != nil {
                o.PosX = clampF(*patch.PosX)
        }
        if patch.PosY != nil {
                o.PosY = clampF(*patch.PosY)
        }
        if patch.Text != nil {
                o.Text = *patch.Text
        }
        if patch.FontSize != nil {
                o.FontSize = *patch.FontSize
        }
        if patch.ColorHex != nil {
                o.ColorHex = strings.TrimPrefix(strings.TrimSpace(*patch.ColorHex), "#")
        }
        if patch.Outline != nil {
                o.Outline = *patch.Outline
        }
        if patch.Bold != nil {
                o.Bold = *patch.Bold
        }
        if patch.Anim != nil {
                switch project.TextAnimKind(*patch.Anim) {
                case project.TextAnimNone, project.TextAnimFadeIn, project.TextAnimTypewriter,
                        project.TextAnimSlideUp, project.TextAnimSlideDown, project.TextAnimSlideLeft,
                        project.TextAnimSlideRight, project.TextAnimPop:
                        o.Anim = project.TextAnimKind(*patch.Anim)
                default:
                        o.Anim = project.TextAnimNone
                }
        }
        if patch.AnimMs != nil {
                v := *patch.AnimMs
                if v < 0 {
                        v = 0
                }
                if v > 10000 {
                        v = 10000
                }
                o.AnimMs = v
        }
        if patch.Shape != nil {
                switch *patch.Shape {
                case "rect", "circle", "ellipse", "line":
                        o.Shape = *patch.Shape
                }
        }
        if patch.WidthPct != nil {
                o.WidthPct = *patch.WidthPct
        }
        if patch.HeightPct != nil {
                o.HeightPct = *patch.HeightPct
        }
        if patch.AssetID != nil {
                o.AssetID = *patch.AssetID
        }
        if patch.ScalePct != nil {
                o.ScalePct = *patch.ScalePct
        }
        if patch.Volume != nil {
                v := *patch.Volume
                if v < 0 {
                        v = 0
                }
                if v > 2 {
                        v = 2
                }
                o.Volume = v
        }
        if patch.Muted != nil {
                o.Muted = *patch.Muted
        }
        if patch.ChromaKey != nil {
                o.ChromaKey = *patch.ChromaKey
                o.ChromaKey.Similarity = clampF(o.ChromaKey.Similarity)
                o.ChromaKey.Blend = clampF(o.ChromaKey.Blend)
        }
        // v1.3.1: chế độ lớp (giá trị lạ → Normalize sẽ trả về mặc định an toàn).
        if patch.Blend != nil {
                o.Blend = strings.ToLower(strings.TrimSpace(*patch.Blend))
        }
        if patch.Opacity != nil {
                o.Opacity = *patch.Opacity
        }
        if patch.FadeInMs != nil {
                o.FadeInMs = *patch.FadeInMs
        }
        if patch.FadeOutMs != nil {
                o.FadeOutMs = *patch.FadeOutMs
        }
        if patch.FilterPreset != nil {
                o.FilterPreset = strings.ToLower(strings.TrimSpace(*patch.FilterPreset))
        }
        o.Normalize()
        a.doc.SortOverlays()
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        return o, nil
}

// RemoveOverlay xoá lớp phủ.
func (a *App) RemoveOverlay(id string) error {
        a.mu.Lock()
        a.doc.RemoveOverlay(id)
        a.mu.Unlock()
        return a.persist()
}

func clampF(v float64) float64 {
        if v < 0 {
                return 0
        }
        if v > 1 {
                return 1
        }
        return v
}

// ---------------------------------------------------------------------------
// Phụ đề
// ---------------------------------------------------------------------------

// ImportSRTDialog mở hộp thoại chọn file .srt, đọc vào dự án.
func (a *App) ImportSRTDialog() (*project.SubtitlesTrack, error) {
        path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
                Title: "Nhập file phụ đề SRT",
                Filters: []wruntime.FileFilter{
                        {DisplayName: "Phụ đề SRT (*.srt)", Pattern: "*.srt"},
                        {DisplayName: "Phụ đề VTT (*.vtt)", Pattern: "*.vtt"},
                },
        })
        if err != nil || path == "" {
                return nil, err
        }
        b, err := os.ReadFile(path)
        if err != nil {
                return nil, err
        }
        parsed, err := tools.ParseSRT(string(b))
        if err != nil {
                return nil, err
        }
        if len(parsed) == 0 {
                return nil, errors.New("file phụ đề không có dòng nào")
        }
        st := &project.SubtitlesTrack{
                SourceName: baseName(path),
                Cues:       make([]project.Cue, 0, len(parsed)),
                Burn:       true,
                FontSize:   42,
                ColorHex:   "FFFFFF",
        }
        for _, c := range parsed {
                st.Cues = append(st.Cues, project.Cue{StartMs: c.StartMs, EndMs: c.EndMs, Text: c.Text})
        }
        a.mu.Lock()
        a.doc.Subs = st
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        return st, nil
}

// ExportSRTDialog xuất phụ đề hiện tại ra file .srt.
func (a *App) ExportSRTDialog() (string, error) {
        a.mu.RLock()
        st := a.doc.Subs
        a.mu.RUnlock()
        if st == nil || len(st.Cues) == 0 {
                return "", errors.New("dự án chưa có phụ đề")
        }
        path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
                Title:           "Xuất phụ đề SRT",
                DefaultFilename: strings.TrimSuffix(st.SourceName, ".srt") + "-vks.srt",
                Filters:         []wruntime.FileFilter{{DisplayName: "Phụ đề SRT (*.srt)", Pattern: "*.srt"}},
        })
        if err != nil || path == "" {
                return "", err
        }
        cues := make([]struct {
                StartMs int64
                EndMs   int64
                Text    string
        }, len(st.Cues))
        for i, c := range st.Cues {
                cues[i] = struct {
                        StartMs int64
                        EndMs   int64
                        Text    string
                }{c.StartMs, c.EndMs, c.Text}
        }
        if err := os.WriteFile(path, []byte(tools.BuildSRT(cues)), 0o644); err != nil {
                return "", err
        }
        return path, nil
}

// SetSubtitleStyle đặt style phụ đề + bật/tắt ghi lên video.
func (a *App) SetSubtitleStyle(burn bool, fontSize float64, colorHex string) error {
        a.mu.Lock()
        if a.doc.Subs == nil {
                a.doc.Subs = &project.SubtitlesTrack{Cues: []project.Cue{}}
        }
        a.doc.Subs.Burn = burn
        if fontSize > 0 {
                a.doc.Subs.FontSize = fontSize
        }
        a.doc.Subs.ColorHex = strings.TrimPrefix(strings.TrimSpace(colorHex), "#")
        a.mu.Unlock()
        return a.persist()
}

// ClearSubtitles xoá phụ đề khỏi dự án.
func (a *App) ClearSubtitles() error {
        a.mu.Lock()
        a.doc.Subs = nil
        a.mu.Unlock()
        return a.persist()
}

func baseName(p string) string {
        if i := strings.LastIndexByte(p, '/'); i >= 0 {
                p = p[i+1:]
        }
        if i := strings.LastIndexByte(p, '\\'); i >= 0 {
                p = p[i+1:]
        }
        return p
}

// ---------------------------------------------------------------------------
// Watermark / Timecode / Credits
// ---------------------------------------------------------------------------

// SetWatermark cấu hình watermark logo (asset ảnh). assetID rỗng = tắt.
func (a *App) SetWatermark(assetID, corner string, marginPx int, opacity, scalePct float64) error {
        a.mu.Lock()
        if assetID == "" {
                a.doc.Watermark = nil
        } else {
                if a.doc.FindAsset(assetID) == nil {
                        a.mu.Unlock()
                        return fmt.Errorf("không tìm thấy tư liệu watermark %s", assetID)
                }
                c := project.CornerBR
                switch corner {
                case "tl":
                        c = project.CornerTL
                case "tr":
                        c = project.CornerTR
                case "bl":
                        c = project.CornerBL
                }
                if opacity <= 0 {
                        opacity = 1
                }
                if opacity > 1 {
                        opacity = 1
                }
                a.doc.Watermark = &project.Watermark{
                        AssetID: assetID, Corner: c, MarginPx: marginPx,
                        Opacity: opacity, ScalePct: scalePct,
                }
        }
        a.mu.Unlock()
        return a.persist()
}

// SetTimecode bật/tắt ghi timecode lên video.
func (a *App) SetTimecode(enabled bool, format string, fontSize, posX, posY float64) error {
        if format != "clock" && format != "millis" && format != "frames" {
                format = "clock"
        }
        a.mu.Lock()
        if enabled {
                a.doc.Timecode = &project.Timecode{
                        Enabled: true, Format: format,
                        FontSize: fontSize, PosX: clampF(posX), PosY: clampF(posY),
                }
        } else {
                a.doc.Timecode = nil
        }
        a.mu.Unlock()
        return a.persist()
}

// SetCredits cấu hình credits chạy cuối. lines rỗng = tắt.
func (a *App) SetCredits(lines []string, fontSize, speed float64) error {
        a.mu.Lock()
        if len(lines) == 0 {
                a.doc.Credits = nil
        } else {
                var clean []string
                for _, l := range lines {
                        l = strings.TrimRight(l, "\r")
                        if strings.TrimSpace(l) == "" {
                                continue
                        }
                        clean = append(clean, l)
                }
                if len(clean) == 0 {
                        a.doc.Credits = nil
                } else {
                        a.doc.Credits = &project.Credits{Lines: clean, FontSize: fontSize, SpeedPXS: speed}
                }
        }
        a.mu.Unlock()
        return a.persist()
}

// ---------------------------------------------------------------------------
// Dựng timeline từ kịch bản (script-based editing)
// ---------------------------------------------------------------------------

// ScriptItem là một mục kịch bản: lấy asset vào timeline.
type ScriptItem struct {
        AssetID      string  `json:"assetId"`
        InMs         int64   `json:"inMs,omitempty"`
        OutMs        int64   `json:"outMs,omitempty"`
        DurMs        int64   `json:"durMs,omitempty"` // cho ảnh
        Transition   string  `json:"transition,omitempty"`
        TransitionMs int64   `json:"transitionMs,omitempty"`
        Volume       float64 `json:"volume,omitempty"`
}

// ScriptDoc là kịch bản dựng phim JSON.
type ScriptDoc struct {
        Width  int          `json:"width,omitempty"`
        Height int          `json:"height,omitempty"`
        Items  []*ScriptItem `json:"items"`
}

// BuildFromScript thay timeline bằng kịch bản JSON (giữ nguyên thư viện).
// Trả về số clip đã dựng.
func (a *App) BuildFromScript(scriptJSON string) (int, error) {
        var sc ScriptDoc
        if err := json.Unmarshal([]byte(scriptJSON), &sc); err != nil {
                return 0, fmt.Errorf("kịch bản JSON không hợp lệ: %w", err)
        }
        if len(sc.Items) == 0 {
                return 0, errors.New("kịch bản không có mục nào")
        }
        a.mu.Lock()
        // v1.2.6 SỬA DEADLOCK (như MoveClip): KHÔNG giữ khoá khi persist.
        var clips []*project.Clip
        var trans []*project.Transition
        for i, it := range sc.Items {
                as := a.doc.FindAsset(it.AssetID)
                if as == nil || as.Missing {
                        a.mu.Unlock()
                        return 0, fmt.Errorf("mục %d: tư liệu không tồn tại: %s", i+1, it.AssetID)
                }
                in, out := it.InMs, it.OutMs
                if as.Kind == project.AssetImage {
                        dur := it.DurMs
                        if dur <= 0 {
                                dur = 5000
                        }
                        in, out = 0, dur
                } else if out <= in {
                        in, out = 0, as.DurationMs
                }
                if out-in < 100 {
                        a.mu.Unlock()
                        return 0, fmt.Errorf("mục %d: thời lượng quá ngắn", i+1)
                }
                c := &project.Clip{
                        ID:      project.NewID("c"),
                        AssetID: it.AssetID,
                        InMs:    in,
                        OutMs:   out,
                        Volume:  1,
                        Transform: project.Transform{Zoom: 1},
                }
                if it.Volume > 0 {
                        if it.Volume > 2 {
                                it.Volume = 2
                        }
                        c.Volume = it.Volume
                }
                clips = append(clips, c)
                if i > 0 && it.Transition != "" && it.Transition != "none" {
                        t, ok := parseTransition(it.Transition)
                        if ok {
                                dur := it.TransitionMs
                                if dur <= 0 {
                                        dur = 500
                                }
                                trans = append(trans, &project.Transition{
                                        AfterClipID: clips[i-1].ID, Type: t, DurationMs: dur,
                                })
                        }
                }
        }
        if sc.Width > 0 && sc.Height > 0 {
                a.doc.Canvas.Width, a.doc.Canvas.Height = sc.Width, sc.Height
        }
        a.doc.Clips = clips
        a.doc.Transitions = trans
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return 0, err
        }
        return len(clips), nil
}
