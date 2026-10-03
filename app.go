package main

import (
        "context"
        "crypto/sha1"
        "encoding/hex"
        "encoding/json"
        "errors"
        "fmt"
        "math"
        "os"
        "os/exec"
        "path/filepath"
        "runtime"
        "strconv"
        "strings"
        "sync"
        "time"

        "github.com/wailsapp/wails/v2/pkg/options"
        wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

        "vkseditorpro/internal/applog"
        "vkseditorpro/internal/engine"
        "vkseditorpro/internal/executil"
        "vkseditorpro/internal/ffmpegbin"
        "vkseditorpro/internal/media"
        "vkseditorpro/internal/project"
)

// App là cầu nối giữa giao diện (WebView) và backend Go.
// Mọi method export (viết hoa) của struct này đều được Wails tự bind cho JS.
type App struct {
        ctx context.Context

        version string

        dataDir    string // %APPDATA%/VKSeditorPro — project.json, settings
        thumbsDir  string // cache thumbnail (/local/thumbs)
        framesDir  string // cache khung hình xem trước (/local/frames)
        previewDir string // bản xem trước render (/local/preview)
        proxyDir   string // bản proxy 480p (/local/proxy)
        ttsDir     string // file giọng đọc TTS (/local/tts)

        store *project.Store

        mu  sync.RWMutex // bảo vệ doc
        doc *project.Document

        jobMu   sync.Mutex // một tác vụ nặng tại một thời điểm
        jobName string
        cancel  context.CancelFunc

        // v1.2.0 — khởi tạo FFmpeg không chặn cửa sổ:
        ffBoot    *ffmpegbin.Boot // kết quả Prepare() từ main
        ffReady   chan struct{}   // đóng khi FFmpeg dùng được HOẶC lỗi dứt điểm
        ffErr     error           // lỗi khởi tạo FFmpeg (nếu có)
        ffOnce    sync.Once
        ffVersion string // phiên bản ffmpeg đã xác minh

        // v1.2.7 — vòng đời tiến trình con: runCtx bị huỷ RÕ RÀNG trong shutdown
        // để mọi ffmpeg đang chạy bị kill (không còn cmd mồ côi sau khi thoát).
        runCtx    context.Context
        runCancel context.CancelFunc
}

// NewApp khởi tạo App và các thư mục dữ liệu.
func NewApp() (*App, error) {
        cfg, err := os.UserConfigDir()
        if err != nil {
                return nil, err
        }
        dataDir := filepath.Join(cfg, "VKSeditorPro")
        for _, d := range []string{dataDir, filepath.Join(dataDir, "thumbs"), filepath.Join(dataDir, "frames"), filepath.Join(dataDir, "preview"), filepath.Join(dataDir, "proxies"), filepath.Join(dataDir, "tts")} {
                if err := os.MkdirAll(d, 0o755); err != nil {
                        return nil, err
                }
        }
        store, err := project.NewStore(dataDir)
        if err != nil {
                return nil, err
        }
        doc, err := store.Load()
        if err != nil {
                // File hỏng — bắt đầu mới thay vì chết app.
                applog.Logf("Nạp dự án THẤT BẠI (%v) — tạo dự án mới", err)
                doc = project.NewDocument()
        } else {
                // v1.2.1: tệp do phiên bản KHÁC ghi (schema lạ) -> cách ly, bắt đầu mới
                // để tránh trạng thái rác làm treo timeline/xem trước.
                if doc.ForeignFormat {
                        quarantine := store.Path() + ".foreign.bak"
                        _ = os.Rename(store.Path(), quarantine)
                        applog.Logf("Phát hiện tệp dự án định dạng lạ (%s) — đã chuyển sang %s và tạo dự án mới", store.Path(), quarantine)
                        doc = project.NewDocument()
                }
                if doc.RepairedClips > 0 || doc.DroppedClips > 0 {
                        applog.Logf("Tự sửa dự án khi nạp: %d clip được gán lại thời lượng, %d clip rác bị loại", doc.RepairedClips, doc.DroppedClips)
                }
                applog.Logf("Nạp dự án: %d tư liệu, %d clip", len(doc.Assets), len(doc.Clips))
        }
        return &App{
                version:    AppVersion,
                dataDir:    dataDir,
                thumbsDir:  filepath.Join(dataDir, "thumbs"),
                framesDir:  filepath.Join(dataDir, "frames"),
                previewDir: filepath.Join(dataDir, "preview"),
                proxyDir:   filepath.Join(dataDir, "proxies"),
                ttsDir:     filepath.Join(dataDir, "tts"),
                store:      store,
                doc:        doc,
                ffReady:    make(chan struct{}),
        }, nil
}

// startup được gọi bởi Wails khi ứng dụng khởi động.
func (a *App) startup(ctx context.Context) {
        a.ctx = ctx
        // v1.2.7: vòng đời độc lập với Wails — huỷ RÕ trong shutdown().
        a.runCtx, a.runCancel = context.WithCancel(context.Background())
        applog.Logf("Wails startup bắt đầu")
        // Kéo-thả tệp vào cửa sổ → tự nhập vào thư viện.
        wruntime.OnFileDrop(ctx, func(x, y int, paths []string) {
                defer func() {
                        if r := recover(); r != nil {
                                applog.Logf("PANIC trong OnFileDrop: %v", r)
                        }
                }()
                if _, err := a.AddAssets(paths); err != nil {
                        // Báo qua sự kiện để UI hiện toast.
                        wruntime.EventsEmit(a.ctx, "import:error", map[string]any{"message": err.Error()})
                } else {
                        wruntime.EventsEmit(a.ctx, "import:done", nil)
                }
        })

        // v1.2.0 — hoàn tất FFmpeg NGAY KHI cửa sổ đã mở (không chặn nữa).
        // Giao diện nhận "ffinit" {phase, pct} và "ffcheck" {ok, version, error}.
        // v1.2.7: chạy qua safeGo — panic trong khởi tạo không giết ứng dụng.
        a.safeGo("khởi tạo FFmpeg", a.bootFFmpeg)

        // Đánh dấu asset thiếu nếu tệp nguồn không còn.
        changed := false
        a.mu.Lock()
        for _, as := range a.doc.Assets {
                if _, err := os.Stat(as.Path); err != nil {
                        if !as.Missing {
                                as.Missing = true
                                changed = true
                        }
                }
        }
        a.mu.Unlock()
        if changed {
                _ = a.persist()
        }
        applog.Logf("Wails startup xong (assets: %d, clips: %d)", len(a.doc.Assets), len(a.doc.Clips))
}

// bootFFmpeg chạy nền: nếu Prepare đã báo sẵn sàng thì chỉ đóng gate;
// nếu chưa, giải nén có progress + verify, đặt env moviego rồi báo UI.
func (a *App) bootFFmpeg() {
        emitFFInit := func(phase string, pct float64) {
                wruntime.EventsEmit(a.ctx, "ffinit", map[string]any{"phase": phase, "pct": pct})
        }
        finishErr := func(err error) {
                a.ffOnce.Do(func() {
                        a.ffErr = err
                        close(a.ffReady)
                })
                applog.Logf("FFmpeg khởi tạo THẤT BẠI: %v", err)
                wruntime.EventsEmit(a.ctx, "ffcheck", map[string]any{"ok": false, "error": err.Error()})
        }

        if a.ffBoot != nil && a.ffBoot.Ready {
                a.ffVersion = a.ffBoot.Result.Version
                a.ffOnce.Do(func() { close(a.ffReady) })
                applog.Logf("ffcheck OK (sẵn sàng từ Prepare): v%s", a.ffVersion)
                wruntime.EventsEmit(a.ctx, "ffcheck", map[string]any{"ok": true, "version": a.ffVersion, "extracted": false})
                return
        }

        emitFFInit("chuẩn bị", 0)
        applog.Logf("Bắt đầu giải nén FFmpeg nhúng...")
        lastPct := -1.0
        res, err := ffmpegbin.Extract(func(pct float64, phase string) {
                // Chỉ phát khi % thay đổi đủ 1 điểm để đỡ nhiễu sự kiện.
                if pct-lastPct >= 1 || pct >= 100 {
                        lastPct = pct
                        emitFFInit(phase, pct)
                }
        })
        if err != nil {
                finishErr(err)
                return
        }
        // Đặt env TRƯỚC mọi lời gọi moviego — gate waitFF bảo đảm thứ tự này.
        ff, fp := resolveFFBinaries(res)
        _ = os.Setenv("MGO_FFMPEG", ff)
        _ = os.Setenv("MGO_FFPROBE", fp)
        a.ffVersion = res.Version
        a.ffOnce.Do(func() { close(a.ffReady) })
        applog.Logf("FFmpeg sẵn sàng: %s (v%s, extracted=%v, verified=%v)", ff, res.Version, res.Extracted, res.Verified)
        wruntime.EventsEmit(a.ctx, "ffcheck", map[string]any{"ok": true, "version": res.Version, "extracted": res.Extracted})
}

// waitFF chờ FFmpeg khởi tạo xong (tối đa 15 phút) trước khi cho phép các
// thao tác cần ffmpeg/probe. Gọi ở đầu mọi method chạm vào moviego/engine.
func (a *App) waitFF() error {
        select {
        case <-a.ffReady:
                if a.ffErr != nil {
                        return a.ffErr
                }
                return nil
        case <-time.After(15 * time.Minute):
                applog.Logf("waitFF TIMEOUT sau 15 phút")
                return errors.New("chờ FFmpeg khởi tạo quá 15 phút — hãy đóng ứng dụng, thêm ngoại lệ diệt virus cho thư mục ứng dụng rồi mở lại")
        }
}

// shutdown được gọi khi ứng dụng đóng — huỷ tác vụ đang chạy.
// v1.2.7: huỷ runCtx TRƯỚC để mọi ffmpeg/ffprobe đang chạy bị kill —
// không còn cửa sổ cmd mồ côi sau khi thoát ứng dụng.
func (a *App) shutdown(ctx context.Context) {
        if a.runCancel != nil {
                a.runCancel() // kill mọi tiến trình con gắn runCtx + huỷ tác vụ
        }
        a.jobMu.Lock()
        if a.cancel != nil {
                a.cancel()
        }
        a.jobMu.Unlock()
        applog.Logf("Shutdown: đã yêu cầu huỷ toàn bộ tiến trình con")
}

// safeGo chạy fn trong goroutine có recover — panic trong tác vụ nền chỉ
// được ghi log, KHÔNG giết ứng dụng (nguồn "phần mềm tự tắt" trước đây).
func (a *App) safeGo(name string, fn func()) {
        go func() {
                defer func() {
                        if r := recover(); r != nil {
                                applog.Logf("PANIC trong tác vụ «%s»: %v", name, r)
                        }
                }()
                fn()
        }()
}

// onSecondInstance — lần chạy thứ 2 chỉ đưa cửa sổ cũ lên trước.
func (a *App) onSecondInstance(_ options.SecondInstanceData) {
        wruntime.WindowUnminimise(a.ctx)
        wruntime.WindowSetAlwaysOnTop(a.ctx, true)
        wruntime.WindowSetAlwaysOnTop(a.ctx, false)
}

// ---------------------------------------------------------------------------
// Trạng thái & dự án
// ---------------------------------------------------------------------------

// StateResponse là toàn bộ trạng thái trả về cho frontend lúc khởi động.
type StateResponse struct {
        Doc        *project.Document `json:"doc"`
        Version    string            `json:"version"`
        DataDir    string            `json:"dataDir"`
        PreviewURL string            `json:"previewUrl"`
        OS         string            `json:"os"`
}

// cloneDoc tạo bản sao độc lập của document (an toàn luồng).
func (a *App) cloneDoc() *project.Document {
        a.mu.RLock()
        defer a.mu.RUnlock()
        b, _ := json.Marshal(a.doc)
        var d project.Document
        _ = json.Unmarshal(b, &d)
        return &d
}

// persist lưu document hiện tại xuống đĩa.
func (a *App) persist() error {
        a.mu.RLock()
        doc := a.doc
        a.mu.RUnlock()
        return a.store.Save(doc)
}

// GetState trả trạng thái đầy đủ cho frontend.
func (a *App) GetState() (*StateResponse, error) {
        return &StateResponse{
                Doc:        a.cloneDoc(),
                Version:    a.version,
                DataDir:    a.dataDir,
                PreviewURL: a.previewURL(),
                OS:         runtime.GOOS,
        }, nil
}

// NewProject tạo dự án trắng (giữ thư viện nếu giữLibrary = true).
func (a *App) NewProject(keepLibrary bool) error {
        a.mu.Lock()
        old := a.doc
        nd := project.NewDocument()
        if keepLibrary {
                nd.Assets = old.Assets
        }
        a.doc = nd
        a.mu.Unlock()
        return a.persist()
}

// SaveProject lưu dự án vào vị trí mặc định (project.json).
func (a *App) SaveProject() (string, error) {
        if err := a.persist(); err != nil {
                return "", err
        }
        return a.store.Path(), nil
}

// SaveProjectAs mở hộp thoại lưu và ghi bản sao .vksproj.
func (a *App) SaveProjectAs() (string, error) {
        path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
                Title:           "Lưu dự án",
                DefaultFilename: a.cloneDoc().Name + ".vksproj",
                Filters: []wruntime.FileFilter{
                        {DisplayName: "Dự án VKSeditorPro (*.vksproj)", Pattern: "*.vksproj"},
                },
        })
        if err != nil || path == "" {
                return "", err
        }
        doc := a.cloneDoc()
        b, err := json.MarshalIndent(doc, "", "  ")
        if err != nil {
                return "", err
        }
        if err := os.WriteFile(path, b, 0o644); err != nil {
                return "", err
        }
        return path, nil
}

// OpenProjectDialog mở hộp thoại chọn .vksproj và nạp vào app.
func (a *App) OpenProjectDialog() (*project.Document, error) {
        path, err := wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
                Title: "Mở dự án",
                Filters: []wruntime.FileFilter{
                        {DisplayName: "Dự án VKSeditorPro (*.vksproj)", Pattern: "*.vksproj"},
                },
        })
        if err != nil || path == "" {
                return nil, err
        }
        b, err := os.ReadFile(path)
        if err != nil {
                return nil, err
        }
        var doc project.Document
        if err := json.Unmarshal(b, &doc); err != nil {
                return nil, fmt.Errorf("file dự án không hợp lệ: %w", err)
        }
        // sanitize như store.Load
        tmpStore, err := project.NewStore(filepath.Join(os.TempDir(), "vks-sanitize"))
        if err == nil {
                _ = tmpStore.Save(&doc)
                if fresh, err2 := tmpStore.Load(); err2 == nil {
                        doc = *fresh
                }
                _ = os.RemoveAll(filepath.Join(os.TempDir(), "vks-sanitize"))
        }
        for _, as := range doc.Assets {
                if _, err := os.Stat(as.Path); err != nil {
                        as.Missing = true
                }
        }
        a.mu.Lock()
        a.doc = &doc
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        return a.cloneDoc(), nil
}

// SetCanvas đặt kích thước khung + frame rate của dự án.
func (a *App) SetCanvas(w, h, num, den int) error {
        if w < 64 || w > 7680 || h < 64 || h > 4320 || num <= 0 || den <= 0 {
                return errors.New("tham số canvas không hợp lệ")
        }
        a.mu.Lock()
        a.doc.Canvas = project.Canvas{Width: w, Height: h, Rate: project.Rate{Num: num, Den: den}}
        a.mu.Unlock()
        return a.persist()
}

// ---------------------------------------------------------------------------
// Thư viện tư liệu
// ---------------------------------------------------------------------------

// ImportMediaDialog mở hộp thoại chọn nhiều tệp rồi nhập vào thư viện.
func (a *App) ImportMediaDialog() ([]*project.Asset, error) {
        paths, err := wruntime.OpenMultipleFilesDialog(a.ctx, wruntime.OpenDialogOptions{
                Title: "Nhập tư liệu (video / ảnh / âm thanh)",
                Filters: []wruntime.FileFilter{
                        {DisplayName: "Tất cả định dạng hỗ trợ", Pattern: "*.mp4;*.mov;*.mkv;*.webm;*.avi;*.m4v;*.wmv;*.flv;*.ts;*.png;*.jpg;*.jpeg;*.webp;*.bmp;*.gif;*.mp3;*.wav;*.m4a;*.aac;*.ogg;*.flac"},
                        {DisplayName: "Video", Pattern: "*.mp4;*.mov;*.mkv;*.webm;*.avi;*.m4v;*.wmv;*.flv"},
                        {DisplayName: "Ảnh", Pattern: "*.png;*.jpg;*.jpeg;*.webp;*.bmp;*.gif"},
                        {DisplayName: "Âm thanh", Pattern: "*.mp3;*.wav;*.m4a;*.aac;*.ogg;*.flac"},
                },
        })
        if err != nil {
                return nil, err
        }
        if len(paths) == 0 {
                return nil, nil
        }
        return a.AddAssets(paths)
}

// AddAssets nhập danh sách tệp vào thư viện (probe + thêm + lưu).
func (a *App) AddAssets(paths []string) ([]*project.Asset, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        t0 := nowMs()
        var added []*project.Asset
        var errs []string
        refreshed, dups := 0, 0
        for _, p := range paths {
                if !media.Importable(p) {
                        errs = append(errs, fmt.Sprintf("%s: định dạng không hỗ trợ", filepath.Base(p)))
                        applog.Logf("Nhập BỎ: %s (định dạng không hỗ trợ)", filepath.Base(p))
                        continue
                }
                as, err := media.Probe(p)
                if err != nil {
                        errs = append(errs, fmt.Sprintf("%s: %v", filepath.Base(p), err))
                        applog.Logf("Probe lỗi: %s: %v", filepath.Base(p), err)
                        continue
                }
                as.ID = project.NewID("a")
                a.mu.Lock()
                // v1.2.1: nhập trùng tệp đã có — KHÔNG bỏ qua im lặng nữa:
                //  - tư liệu cũ khoẻ mạnh -> trả về chính nó để UI báo "đã có"
                //  - tư liệu cũ Missing/dữ liệu 0 -> làm mới tại chỗ (giữ ID để clip cũ còn dùng được)
                var dup *project.Asset
                for _, ex := range a.doc.Assets {
                        if ex.Path == as.Path {
                                dup = ex
                                break
                        }
                }
                if dup != nil {
                        if !dup.Missing && dup.DurationMs > 0 {
                                a.mu.Unlock()
                                added = append(added, dup)
                                dups++
                                applog.Logf("Nhập trùng: %s đã có trong thư viện (tư liệu khoẻ) — không thêm lần nữa", filepath.Base(p))
                                continue
                        }
                        dup.Missing = false
                        dup.DurationMs = as.DurationMs
                        dup.Width = as.Width
                        dup.Height = as.Height
                        dup.Rate = as.Rate
                        dup.HasAudio = as.HasAudio
                        dup.Codec = as.Codec
                        dup.SizeBytes = as.SizeBytes
                        a.mu.Unlock()
                        added = append(added, dup)
                        refreshed++
                        applog.Logf("Làm mới tư liệu cũ: %s (bản ghi trước đó thiếu dữ liệu/tệp đã quay lại)", filepath.Base(p))
                        continue
                }
                a.doc.Assets = append(a.doc.Assets, as)
                a.mu.Unlock()
                added = append(added, as)
                applog.Logf("Nhập OK: %s (%dx%d, %dms, %s)", filepath.Base(p), as.Width, as.Height, as.DurationMs, as.Codec)
        }
        if err := a.persist(); err != nil {
                return added, err
        }
        applog.Logf("AddAssets xong: %d thêm mới / %d làm mới / %d trùng / %d lỗi / %dms", len(added)-refreshed-dups, refreshed, dups, len(errs), nowMs()-t0)
        if len(errs) > 0 {
                return added, errors.New(strings.Join(errs, "\n"))
        }
        return added, nil
}

// RemoveAsset xoá tư liệu khỏi thư viện (kèm xoá các clip đang dùng nó).
func (a *App) RemoveAsset(id string) error {
        a.mu.Lock()
        var kept []*project.Asset
        found := false
        for _, as := range a.doc.Assets {
                if as.ID == id {
                        found = true
                        continue
                }
                kept = append(kept, as)
        }
        if !found {
                a.mu.Unlock()
                return fmt.Errorf("không tìm thấy tư liệu %s", id)
        }
        a.doc.Assets = kept
        // Xoá clip tham chiếu + chuyển cảnh nối vào chúng.
        var clips []*project.Clip
        for _, c := range a.doc.Clips {
                if c.AssetID == id {
                        a.doc.RemoveTransitionAfter(c.ID)
                        continue
                }
                clips = append(clips, c)
        }
        a.doc.Clips = clips
        a.doc.Transitions = cleanTransitions(a.doc)
        // v1.3.0: xoá cả đoạn âm thanh A1 đang dùng tư liệu này.
        a.doc.RemoveAudioClipsForAsset(id)
        a.mu.Unlock()
        return a.persist()
}

// GetAssetThumb tạo (nếu cần) và trả URL thumbnail cho tư liệu tại tMs.
func (a *App) GetAssetThumb(assetID string, tMs int64) (string, error) {
        if err := a.waitFF(); err != nil {
                return "", err
        }
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil {
                return "", fmt.Errorf("không tìm thấy tư liệu %s", assetID)
        }
        if as.Missing {
                return "", fmt.Errorf("tệp nguồn không còn: %s", as.Path)
        }
        path, err := media.EnsureThumbnail(a.ctx, as, a.thumbsDir, tMs, 320)
        if err != nil {
                applog.Logf("GetAssetThumb lỗi (%s @%dms): %v", filepath.Base(as.Path), tMs, err)
                return "", err
        }
        return "/local/thumbs/" + filepath.Base(path), nil
}

// ---------------------------------------------------------------------------
// Timeline
// ---------------------------------------------------------------------------

// AddClip thêm clip từ asset vào vị trí atIndex (-1 = cuối timeline).
// Video: mặc định toàn bộ; ảnh: 5 giây.
func (a *App) AddClip(assetID string, atIndex int) (*project.Clip, error) {
        a.mu.Lock()
        as := a.doc.FindAsset(assetID)
        if as == nil {
                a.mu.Unlock()
                applog.Logf("AddClip LỖI: không tìm thấy tư liệu %s", assetID)
                return nil, fmt.Errorf("không tìm thấy tư liệu %s", assetID)
        }
        if as.Missing {
                a.mu.Unlock()
                applog.Logf("AddClip LỖI: tệp nguồn không còn: %s", as.Path)
                return nil, fmt.Errorf("tệp nguồn không còn: %s", as.Path)
        }
        in, out := int64(0), as.DurationMs
        if as.Kind == project.AssetImage {
                out = 5000
        }
        if out <= 0 {
                a.mu.Unlock()
                applog.Logf("AddClip LỖI: tư liệu %s có thời lượng 0", as.Name)
                return nil, errors.New("tư liệu có thời lượng 0, không thể thêm vào timeline")
        }
        c := &project.Clip{
                ID:        project.NewID("c"),
                AssetID:   assetID,
                InMs:      in,
                OutMs:     out,
                Volume:    1,
                Transform: project.Transform{Zoom: 1},
        }
        n := len(a.doc.Clips)
        if atIndex < 0 || atIndex > n {
                atIndex = n
        }
        // Chèn: cắt mảng.
        a.doc.Clips = append(a.doc.Clips, c)
        copy(a.doc.Clips[atIndex+1:], a.doc.Clips[atIndex:])
        a.doc.Clips[atIndex] = c
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        applog.Logf("AddClip OK: %s -> timeline[%d] (%s, %d→%dms)", as.Name, atIndex, c.ID, c.InMs, c.OutMs)
        return c, nil
}

// ClipPatch là bản vá thuộc tính clip — trường nil = không đổi.
type ClipPatch struct {
        InMs      *int64   `json:"inMs"`
        OutMs     *int64   `json:"outMs"`
        Mute      *bool    `json:"mute"`
        Volume    *float64 `json:"volume"`
        RotateDeg *float64 `json:"rotateDeg"`
        FlipH     *bool    `json:"flipH"`
        FlipV     *bool    `json:"flipV"`
        Zoom      *float64 `json:"zoom"`
        // v1.1
        Speed          *float64                 `json:"speed"`
        Reverse        *bool                    `json:"reverse"`
        Boomerang      *bool                    `json:"boomerang"`
        LoopN          *int                     `json:"loopN"`
        FreezeStartMs  *int64                   `json:"freezeStartMs"`
        FreezeEndMs    *int64                   `json:"freezeEndMs"`
        AudioFadeInMs  *int64                   `json:"audioFadeInMs"`
        AudioFadeOutMs *int64                   `json:"audioFadeOutMs"`
        WaterDropMs    *int64                   `json:"waterDropMs"`
        Effects        *project.ClipEffects     `json:"effects"`
        LUTPath        *string                  `json:"lutPath"`
        Keyframes      *[]project.KeyframePoint `json:"keyframes"`
        // v1.2.3
        Opacity *float64 `json:"opacity"`
        PosX    *float64 `json:"posX"`
        PosY    *float64 `json:"posY"`
}

// clampPatch01 kẹp giá trị patch vào [0,1]; NaN → fallback.
func clampPatch01(v, fb float64) float64 {
        if math.IsNaN(v) || v < 0 || v > 1 {
                return fb
        }
        return v
}

// UpdateClip áp bản vá cho clip.
func (a *App) UpdateClip(id string, patch ClipPatch) (*project.Clip, error) {
        a.mu.Lock()
        c := a.doc.FindClip(id)
        if c == nil {
                a.mu.Unlock()
                return nil, fmt.Errorf("không tìm thấy clip %s", id)
        }
        as := a.doc.FindAsset(c.AssetID)
        maxDur := int64(0)
        isImage := false
        if as != nil {
                maxDur = as.DurationMs
                isImage = as.Kind == project.AssetImage
        }
        if isImage {
                maxDur = 10 * 60 * 1000 // ảnh: giới hạn ảo 10 phút
        }
        if patch.InMs != nil {
                v := *patch.InMs
                if v < 0 {
                        v = 0
                }
                c.InMs = v
        }
        if patch.OutMs != nil {
                v := *patch.OutMs
                if v > maxDur {
                        v = maxDur
                }
                c.OutMs = v
        }
        if c.OutMs-c.InMs < 100 {
                c.OutMs = c.InMs + 100
                if !isImage && maxDur > 0 && c.OutMs > maxDur {
                        c.InMs = maxDur - 100
                        c.OutMs = maxDur
                }
        }
        if patch.Mute != nil {
                c.Mute = *patch.Mute
        }
        if patch.Volume != nil {
                v := *patch.Volume
                if v < 0 {
                        v = 0
                }
                if v > 2 {
                        v = 2
                }
                c.Volume = v
        }
        if patch.RotateDeg != nil {
                c.Transform.RotateDeg = *patch.RotateDeg
        }
        if patch.FlipH != nil {
                c.Transform.FlipH = *patch.FlipH
        }
        if patch.FlipV != nil {
                c.Transform.FlipV = *patch.FlipV
        }
        if patch.Zoom != nil {
                c.Transform.Zoom = *patch.Zoom
        }
        // v1.1: thuộc tính thời gian / hiệu ứng / keyframes.
        if patch.Speed != nil {
                s := *patch.Speed
                if s < 0.25 {
                        s = 0.25
                }
                if s > 4 {
                        s = 4
                }
                c.Speed = s
        }
        if patch.Reverse != nil {
                c.Reverse = *patch.Reverse
        }
        if patch.Boomerang != nil {
                c.Boomerang = *patch.Boomerang
        }
        if patch.LoopN != nil {
                n := *patch.LoopN
                if n < 1 {
                        n = 1
                }
                if n > 20 {
                        n = 20
                }
                c.LoopN = n
                if n > 1 {
                        c.Boomerang = false
                }
        }
        if c.Boomerang {
                c.LoopN = 1
        }
        if patch.FreezeStartMs != nil {
                v := *patch.FreezeStartMs
                if v < 0 {
                        v = 0
                }
                if v > 30000 {
                        v = 30000
                }
                c.FreezeStartMs = v
        }
        if patch.FreezeEndMs != nil {
                v := *patch.FreezeEndMs
                if v < 0 {
                        v = 0
                }
                if v > 30000 {
                        v = 30000
                }
                c.FreezeEndMs = v
        }
        if patch.AudioFadeInMs != nil {
                v := *patch.AudioFadeInMs
                if v < 0 {
                        v = 0
                }
                if v > 60000 {
                        v = 60000
                }
                c.AudioFadeInMs = v
        }
        if patch.AudioFadeOutMs != nil {
                v := *patch.AudioFadeOutMs
                if v < 0 {
                        v = 0
                }
                if v > 60000 {
                        v = 60000
                }
                c.AudioFadeOutMs = v
        }
        if patch.WaterDropMs != nil {
                v := *patch.WaterDropMs
                if v < 0 {
                        v = 0
                }
                if v > 30000 {
                        v = 30000
                }
                c.WaterDropMs = v
        }
        if patch.Effects != nil {
                c.Effects = patch.Effects.Normalize()
        }
        if patch.LUTPath != nil {
                c.LUTPath = strings.TrimSpace(*patch.LUTPath)
                if c.LUTPath != "" {
                        if _, err := os.Stat(c.LUTPath); err != nil {
                                c.LUTPath = ""
                                a.mu.Unlock()
                                return nil, errors.New("file LUT .cube không tồn tại")
                        }
                }
        }
        if patch.Keyframes != nil {
                ks := make([]project.KeyframePoint, 0, len(*patch.Keyframes))
                for _, k := range *patch.Keyframes {
                        if k.Prop != "scale" && k.Prop != "opacity" {
                                continue
                        }
                        if patch.Opacity != nil { // v1.2.3
                                v := *patch.Opacity
                                if v < 0 {
                                        v = 0
                                }
                                if v > 1 {
                                        v = 1
                                }
                                c.Opacity = v
                        }
                        if patch.PosX != nil {
                                c.Transform.PosX = clampPatch01(*patch.PosX, 0.5)
                        }
                        if patch.PosY != nil {
                                c.Transform.PosY = clampPatch01(*patch.PosY, 0.5)
                        }
                        if k.AtMs < 0 {
                                k.AtMs = 0
                        }
                        if k.Prop == "scale" && (k.Val < 0.1 || k.Val > 4) {
                                k.Val = 1
                        }
                        if k.Prop == "opacity" && (k.Val < 0 || k.Val > 1) {
                                k.Val = 1
                        }
                        ks = append(ks, k)
                }
                c.Keyframes = ks
        }
        c.Transform = c.Transform.Normalize()
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        return c, nil
}

// MoveClip di chuyển clip tới vị trí toIndex.
// v1.2.6 SỬA DEADLOCK NGHIÊM TRỌNG (tồn tại từ v1.2.3): trước đây dùng
// `defer a.mu.Unlock()` rồi gọi a.persist() (lấy RLock) trong cùng goroutine —
// sync.RWMutex KHÔng vào lại được → khoá mu treo VĨNH VIỄN. Người dùng chỉ cần
// KÉO DỜI một clip là toàn bộ backend chết (xoá clip, chẩn đoán, lớp phủ…
// đều đứng im) — đúng các lỗi user báo. Nay: nhả khoá TRƯỚC khi persist.
func (a *App) MoveClip(id string, toIndex int) error {
        a.mu.Lock()
        i := a.doc.ClipIndex(id)
        if i < 0 {
                a.mu.Unlock()
                return fmt.Errorf("không tìm thấy clip %s", id)
        }
        n := len(a.doc.Clips)
        if toIndex < 0 {
                toIndex = 0
        }
        if toIndex >= n {
                toIndex = n - 1
        }
        if toIndex == i {
                a.mu.Unlock()
                return a.persist()
        }
        // Bước 1: gỡ khỏi vị trí i.
        c := a.doc.Clips[i]
        a.doc.Clips = append(a.doc.Clips[:i], a.doc.Clips[i+1:]...)
        // Bước 2: chèn lại tại toIndex.
        a.doc.Clips = append(a.doc.Clips, nil)
        copy(a.doc.Clips[toIndex+1:], a.doc.Clips[toIndex:])
        a.doc.Clips[toIndex] = c
        a.doc.SortTransitions()
        a.mu.Unlock()
        return a.persist()
}

// RemoveClip xoá clip (kèm chuyển cảnh nối vào nó).
func (a *App) RemoveClip(id string) error {
        a.mu.Lock()
        i := a.doc.ClipIndex(id)
        if i < 0 {
                a.mu.Unlock()
                return fmt.Errorf("không tìm thấy clip %s", id)
        }
        a.doc.Clips = append(a.doc.Clips[:i], a.doc.Clips[i+1:]...)
        a.doc.RemoveTransitionAfter(id)
        a.doc.Transitions = cleanTransitions(a.doc)
        a.mu.Unlock()
        return a.persist()
}

// SplitClip tách clip tại vị trí timelineMs; trả về clip mới (nửa trái).
func (a *App) SplitClip(id string, timelineMs int64) (*project.Clip, error) {
        a.mu.Lock()
        i := a.doc.ClipIndex(id)
        if i < 0 {
                a.mu.Unlock()
                return nil, fmt.Errorf("không tìm thấy clip %s", id)
        }
        starts := a.doc.TimelineMsPerClip()
        c := a.doc.Clips[i]
        local := timelineMs - starts[i]
        if local <= c.InMs+80 || local >= c.OutMs-80 {
                a.mu.Unlock()
                return nil, errors.New("vị trí tách quá sát đầu/cuối clip")
        }
        left := *c
        left.ID = project.NewID("c")
        left.OutMs = local
        right := *c
        right.InMs = local
        a.doc.Clips[i] = &right
        a.doc.Clips = append(a.doc.Clips, nil)
        copy(a.doc.Clips[i+1:], a.doc.Clips[i:])
        a.doc.Clips[i] = &left
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        return &left, nil
}

// DuplicateClip v1.2.6: NHÂN BẢN clip (giống Ctrl+D Premiere/CapCut) — chèn
// bản sao NGAY SAU clip gốc, giữ nguyên mọi hiệu ứng/cắt/âm thanh, kèm
// chuyển cảnh nối sau gốc (nếu có) chuyển sang bản sao.
func (a *App) DuplicateClip(id string) (*project.Clip, error) {
        a.mu.Lock()
        i := a.doc.ClipIndex(id)
        if i < 0 {
                a.mu.Unlock()
                return nil, fmt.Errorf("không tìm thấy clip %s", id)
        }
        src := a.doc.Clips[i]
        clone := *src
        clone.ID = project.NewID("c")
        // Sao chép sâu keyframes + effects để 2 clip không dính tham chiếu.
        if src.Keyframes != nil {
                clone.Keyframes = make([]project.KeyframePoint, len(src.Keyframes))
                copy(clone.Keyframes, src.Keyframes)
        }
        a.doc.Clips = append(a.doc.Clips, nil)
        copy(a.doc.Clips[i+2:], a.doc.Clips[i+1:])
        a.doc.Clips[i+1] = &clone
        // Chuyển cảnh nối sau clip gốc → dời sang nối sau BẢN SAO (đứng cuối cặp).
        for _, tr := range a.doc.Transitions {
                if tr.AfterClipID == id {
                        tr.AfterClipID = clone.ID
                        break
                }
        }
        a.doc.SortTransitions()
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        applog.Logf("DuplicateClip: %s → %s (vị trí %d)", id, clone.ID, i+1)
        return &clone, nil
}

// TransInfo là thông tin 1 chuyển cảnh cho dropdown UI.
type TransInfo struct {
        Type string `json:"type"`
        Name string `json:"name"`
}

// ListTransitions trả danh sách chuyển cảnh hỗ trợ (tiếng Việt).
func (a *App) ListTransitions() []TransInfo {
        list := []TransInfo{
                {Type: "none", Name: "— Không có —"},
                {Type: string(project.TransCrossfade), Name: "Hòa mềm (Crossfade)"},
                {Type: string(project.TransDissolve), Name: "Tan biến (Dissolve)"},
                {Type: string(project.TransFadeThroughBlk), Name: "Qua đen (Fade qua đen)"},
                {Type: string(project.TransWipeLeft), Name: "Trượt khoá — sang trái"},
                {Type: string(project.TransWipeRight), Name: "Trượt khoá — sang phải"},
                {Type: string(project.TransWipeUp), Name: "Trượt khoá — lên"},
                {Type: string(project.TransWipeDown), Name: "Trượt khoá — xuống"},
                {Type: string(project.TransSlideLeft), Name: "Trượt khung — sang trái"},
                {Type: string(project.TransSlideRight), Name: "Trượt khung — sang phải"},
                {Type: string(project.TransSlideUp), Name: "Trượt khung — lên"},
                {Type: string(project.TransSlideDown), Name: "Trượt khung — xuống"},
                {Type: string(project.TransPushLeft), Name: "Đẩy — sang trái"},
                {Type: string(project.TransPushRight), Name: "Đẩy — sang phải"},
                {Type: string(project.TransPushUp), Name: "Đẩy — lên"},
                {Type: string(project.TransPushDown), Name: "Đẩy — xuống"},
                {Type: string(project.TransIrisOpen), Name: "Mống mở (Iris mở)"},
                {Type: string(project.TransIrisClose), Name: "Mống đóng (Iris đóng)"},
        }
        return list
}

// SetTransition đặt/xoá chuyển cảnh sau afterClipID. typeName "none" = xoá.
// v1.2.6 SỬA DEADLOCK (như MoveClip): nhả khoá TRƯỚC khi persist.
func (a *App) SetTransition(afterClipID string, typeName string, durMs int64) error {
        a.mu.Lock()
        i := a.doc.ClipIndex(afterClipID)
        if i < 0 {
                a.mu.Unlock()
                return fmt.Errorf("không tìm thấy clip %s", afterClipID)
        }
        if i == len(a.doc.Clips)-1 || typeName == "none" || typeName == "" {
                a.doc.RemoveTransitionAfter(afterClipID)
                a.doc.Transitions = cleanTransitions(a.doc)
                a.mu.Unlock()
                return a.persist()
        }
        t, ok := parseTransition(typeName)
        if !ok {
                return fmt.Errorf("loại chuyển cảnh không hỗ trợ: %s", typeName)
        }
        // Giới hạn thời lượng an toàn.
        limit := a.doc.Clips[i].DurationMs()
        if i+1 < len(a.doc.Clips) {
                if n := a.doc.Clips[i+1].DurationMs(); n < limit {
                        limit = n
                }
        }
        if durMs < 120 {
                durMs = 300
        }
        if durMs > limit-120 {
                durMs = limit - 120
        }
        if durMs < 120 {
                return errors.New("clip quá ngắn để áp chuyển cảnh")
        }
        if ex := a.doc.TransitionAfter(afterClipID); ex != nil {
                ex.Type = t
                ex.DurationMs = durMs
        } else {
                a.doc.Transitions = append(a.doc.Transitions, &project.Transition{
                        AfterClipID: afterClipID, Type: t, DurationMs: durMs,
                })
        }
        a.doc.SortTransitions()
        a.mu.Unlock()
        return a.persist()
}

// cleanTransitions loại transition trỏ tới clip không tồn tại.
func cleanTransitions(doc *project.Document) []*project.Transition {
        out := doc.Transitions[:0]
        for _, t := range doc.Transitions {
                if doc.FindClip(t.AfterClipID) != nil {
                        out = append(out, t)
                }
        }
        return out
}

// parseTransition map chuỗi type sang project.TransitionType.
func parseTransition(s string) (project.TransitionType, bool) {
        t := project.TransitionType(s)
        switch t {
        case project.TransCrossfade, project.TransDissolve, project.TransFadeThroughBlk,
                project.TransWipeLeft, project.TransWipeRight, project.TransWipeUp, project.TransWipeDown,
                project.TransSlideLeft, project.TransSlideRight, project.TransSlideUp, project.TransSlideDown,
                project.TransPushLeft, project.TransPushRight, project.TransPushUp, project.TransPushDown,
                project.TransIrisOpen, project.TransIrisClose:
                return t, true
        }
        return "", false
}

// ---------------------------------------------------------------------------
// Tiện ích hệ thống
// ---------------------------------------------------------------------------

// OpenInFolder mở Explorer tại file/thư mục.
func (a *App) OpenInFolder(path string) error {
        if _, err := os.Stat(path); err != nil {
                return err
        }
        switch runtime.GOOS {
        case "windows":
                return exec.Command("explorer", "/select,", path).Start()
        case "darwin":
                return exec.Command("open", "-R", path).Start()
        default:
                return exec.Command("xdg-open", filepath.Dir(path)).Start()
        }
}

// GetVersion trả phiên bản app.
func (a *App) GetVersion() string { return a.version }

// DiagnosticsResponse là báo cáo chẩn đoán cho bảng 🧯 phía giao diện.
type DiagnosticsResponse struct {
        Version     string `json:"version"`
        OS          string `json:"os"`
        Arch        string `json:"arch"`
        DataDir     string `json:"dataDir"`
        FFmpegPath  string `json:"ffmpegPath"`
        FFmpegOK    bool   `json:"ffmpegOk"`
        FFmpegSize  int64  `json:"ffmpegSize"`
        FFprobePath string `json:"ffprobePath"`
        FFprobeOK   bool   `json:"ffprobeOk"`
        FFprobeSize int64  `json:"ffprobeSize"`
        FFVersion   string `json:"ffVersion"`
        FFReady     bool   `json:"ffReady"`
        Assets      int    `json:"assets"`
        Clips       int    `json:"clips"`
        CurrentJob  string `json:"currentJob"`
        LogPath     string `json:"logPath"`
        LogTail     string `json:"logTail"`
}

// GetDiagnostics trả thông tin chẩn đoán + log tail (nút 🧯 trên thanh công cụ).
// v1.2.6: KHÔNG BAO GIỜ treo — đếm asset/clip bằng khoá ngắn thay vì cloneDoc,
// log đọc bằng TryTail (không chờ khoá ghi); toàn bộ thân hàm có timeout 4s,
// quá hạn trả báo cáo đầy đủ nhất có thể kèm ghi chú "hệ thống bận".
func (a *App) GetDiagnostics() DiagnosticsResponse {
        done := make(chan DiagnosticsResponse, 1)
        go func() {
                defer func() {
                        if r := recover(); r != nil {
                                applog.Logf("PANIC trong diagnosticsSlow: %v", r)
                                done <- DiagnosticsResponse{Version: a.version, OS: runtime.GOOS, Arch: runtime.GOARCH}
                        }
                }()
                done <- a.diagnosticsSlow()
        }()
        select {
        case d := <-done:
                return d
        case <-time.After(4 * time.Second):
                applog.Logf("GetDiagnostics quá 4s — trả báo cáo tối giản (hệ thống bận)")
                return DiagnosticsResponse{
                        Version: a.version, OS: runtime.GOOS, Arch: runtime.GOARCH,
                        DataDir: a.dataDir, LogPath: applog.TryPath(),
                        LogTail: "(hệ thống đang bận — không đọc được log ngay, thử mở lại bảng này)",
                        FFVersion: a.ffVersion,
                }
        }
}

// diagnosticsSlow dựng báo cáo đầy đủ (tách ra để bọc timeout).
func (a *App) diagnosticsSlow() DiagnosticsResponse {
        // v1.2.6: đếm bằng khoá NGẮN — cloneDoc (marshal toàn bộ doc) có thể
        // chờ vô hạn nếu một thao tác khác đang giữ khoá ghi lâu.
        a.mu.RLock()
        assets, clips := len(a.doc.Assets), len(a.doc.Clips)
        a.mu.RUnlock()
        d := DiagnosticsResponse{
                Version:    a.version,
                OS:         runtime.GOOS,
                Arch:       runtime.GOARCH,
                DataDir:    a.dataDir,
                LogPath:    applog.TryPath(),
                LogTail:    applog.TryTail(120),
                CurrentJob: a.CurrentJob(),
                Assets:     assets,
                Clips:      clips,
                FFVersion:  a.ffVersion,
        }
        dir, err := ffmpegbin.TargetDirForDiag()
        if err == nil {
                ff, fp := ffmpegbin.BinPathsForDiag(dir)
                d.FFmpegPath, d.FFprobePath = ff, fp
                if st, e := os.Stat(ff); e == nil {
                        d.FFmpegOK, d.FFmpegSize = true, st.Size()
                }
                if st, e := os.Stat(fp); e == nil {
                        d.FFprobeOK, d.FFprobeSize = true, st.Size()
                }
        }
        select {
        case <-a.ffReady:
                d.FFReady = a.ffErr == nil
        default:
        }
        return d
}

// AppVersion được set lúc build bằng ldflags.
var AppVersion = "1.3.1"

// nowMs helper.
func nowMs() int64 { return time.Now().UnixNano() / 1e6 }

// ---------------------------------------------------------------------------
// v1.2.3 — công tắc track, tách âm thanh, khớp màu, xem trước tư liệu
// ---------------------------------------------------------------------------

// SetDocFlags đặt công tắc cấp timeline: 🔇 tắt tiếng toàn bộ, 👁 ẩn lớp phủ.
func (a *App) SetDocFlags(muteAll, hideOverlays bool) error {
        a.mu.Lock()
        a.doc.MuteAll = muteAll
        a.doc.HideOverlays = hideOverlays
        a.mu.Unlock()
        applog.Logf("SetDocFlags: muteAll=%v hideOverlays=%v", muteAll, hideOverlays)
        return a.persist()
}

// DetachAudio tách âm thanh của clip thành TƯ LIỆU RIÊNG trong thư viện
// (giống Link/Unlink: clip gốc được tắt tiếng, âm thanh đứng riêng — người dùng
// kéo vào nơi cần). Kết quả: asset âm thanh mới (aac 192k), clip gốc mute=true.
func (a *App) DetachAudio(clipID string) (*project.Asset, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        a.mu.RLock()
        c := a.doc.FindClip(clipID)
        if c == nil {
                a.mu.RUnlock()
                return nil, fmt.Errorf("không tìm thấy clip %s", clipID)
        }
        as := a.doc.FindAsset(c.AssetID)
        a.mu.RUnlock()
        if as == nil || as.Missing {
                return nil, errors.New("tệp nguồn không khả dụng")
        }
        if as.Kind == project.AssetAudio {
                return nil, errors.New("clip này đã là âm thanh thuần — không cần tách")
        }
        bin := engine.FFmpegBin()
        if bin == "" {
                return nil, errors.New("FFmpeg chưa sẵn sàng")
        }
        st, err := os.Stat(as.Path)
        if err != nil {
                return nil, err
        }
        h := sha1.Sum([]byte("detach-audio|" + as.Path + "|" + fmt.Sprint(st.Size())))
        outDir := filepath.Join(a.dataDir, "audio")
        _ = os.MkdirAll(outDir, 0o755)
        out := filepath.Join(outDir, hex.EncodeToString(h[:])+".m4a")
        if _, err := os.Stat(out); err != nil {
                cmd := exec.Command(bin, "-y", "-i", as.Path, "-vn",
                        "-c:a", "aac", "-b:a", "192k", out)
                executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
                if b, err := cmd.CombinedOutput(); err != nil {
                        applog.Logf("DetachAudio LỖI (%s): %v: %s", filepath.Base(as.Path), err, tail300(string(b)))
                        return nil, fmt.Errorf("tách âm thanh thất bại: %s", tail300(string(b)))
                }
        }
        added, err := a.AddAssets([]string{out})
        if err != nil {
                return nil, err
        }
        if len(added) == 0 {
                // Đã tách trước đó — asset đã có trong thư viện; tìm theo path.
                a.mu.RLock()
                for _, x := range a.doc.Assets {
                        if x.Path == out {
                                added = []*project.Asset{x}
                                break
                        }
                }
                a.mu.RUnlock()
                if added == nil {
                        return nil, errors.New("không xác định được tư liệu âm thanh đã tách")
                }
        }
        // Tắt tiếng clip gốc (unlink).
        if _, err := a.UpdateClip(clipID, ClipPatch{Mute: boolPtr(true)}); err != nil {
                applog.Logf("DetachAudio: mute clip gốc lỗi: %v", err)
        }
        applog.Logf("DetachAudio: %s → %s (clip gốc đã tắt tiếng)", filepath.Base(as.Path), filepath.Base(out))
        return added[0], nil
}

// ColorMatch phân tích trung bình độ sáng (YAVG) + độ bão hoà (SATAVG) của
// clip chuẩn và clip đích (signalstats), rồi tự chỉnh Brightness/Saturation
// của clip đích về gần tone màu của clip chuẩn. Best-effort: người dùng vẫn
// tinh chỉnh tiếp bằng thanh trượt sau khi khớp.
func (a *App) ColorMatch(refClipID, targetClipID string) (*project.Clip, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        bin := engine.FFmpegBin()
        if bin == "" {
                return nil, errors.New("FFmpeg chưa sẵn sàng")
        }
        statOf := func(clipID string) (yAvg, satAvg float64, path string, err error) {
                a.mu.RLock()
                c := a.doc.FindClip(clipID)
                var as *project.Asset
                if c != nil {
                        as = a.doc.FindAsset(c.AssetID)
                }
                a.mu.RUnlock()
                if c == nil || as == nil || as.Missing {
                        return 0, 0, "", errors.New("clip/tư liệu không khả dụng")
                }
                if as.Kind == project.AssetAudio {
                        return 0, 0, "", errors.New("không khớp màu cho âm thanh")
                }
                path = as.Path
                // Lấy mẫu 6 khung đều nhau trong khoảng in/out của clip.
                dur := float64(c.OutMs-c.InMs) / 1000
                if dur <= 0 {
                        dur = float64(as.DurationMs) / 1000
                }
                seek := fmt.Sprintf("%.2f", float64(c.InMs)/1000+math.Min(1, dur*0.1))
                args := []string{"-hide_banner", "-ss", seek, "-t", "3", "-i", as.Path,
                        "-vf", "fps=2,signalstats,metadata=print:file=-", "-f", "null", "-"}
                ccmd := exec.Command(bin, args...)
                executil.Hide(ccmd) // v1.2.7: không hiện console trên Windows
                out, err := ccmd.Output()
                if err != nil {
                        return 0, 0, path, fmt.Errorf("phân tích %s: %w", filepath.Base(as.Path), err)
                }
                var ys, ss []float64
                for _, ln := range strings.Split(string(out), "\n") {
                        if i := strings.Index(ln, "YAVG="); i >= 0 {
                                if v, e := strconv.ParseFloat(strings.TrimSpace(ln[i+5:]), 64); e == nil {
                                        ys = append(ys, v)
                                }
                        } else if i := strings.Index(ln, "SATAVG="); i >= 0 {
                                if v, e := strconv.ParseFloat(strings.TrimSpace(ln[i+7:]), 64); e == nil {
                                        ss = append(ss, v)
                                }
                        }
                }
                if len(ys) == 0 {
                        return 0, 0, path, errors.New("không đọc được chỉ số màu (YAVG)")
                }
                for _, v := range ys {
                        yAvg += v
                }
                yAvg /= float64(len(ys))
                if len(ss) > 0 {
                        for _, v := range ss {
                                satAvg += v
                        }
                        satAvg /= float64(len(ss))
                }
                return yAvg, satAvg, path, nil
        }
        refY, refS, refPath, err := statOf(refClipID)
        if err != nil {
                return nil, fmt.Errorf("clip chuẩn: %w", err)
        }
        tgtY, tgtS, tgtPath, err := statOf(targetClipID)
        if err != nil {
                return nil, fmt.Errorf("clip đích: %w", err)
        }
        a.mu.Lock()
        tc := a.doc.FindClip(targetClipID)
        if tc == nil {
                a.mu.Unlock()
                return nil, errors.New("clip đích không còn")
        }
        fx := tc.Effects
        dY := (refY - tgtY) / 255.0                       // ±0.5 kẹp trong Normalize
        fx.Brightness = clampPatch01(fx.Brightness+dY, 0) // không clamp — Normalize lo
        if tgtS > 4 && refS > 4 {
                fx.Saturation = fx.Saturation * (refS / tgtS)
                if fx.Saturation < 0.2 {
                        fx.Saturation = 0.2
                }
                if fx.Saturation > 3 {
                        fx.Saturation = 3
                }
        }
        tc.Effects = fx.Normalize()
        tc.Transform = tc.Transform.Normalize()
        result := *tc
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return nil, err
        }
        applog.Logf("ColorMatch: ref=%s (Y=%.1f S=%.1f) → target=%s (Y=%.1f S=%.1f) brightness+=%.3f sat→%.2f",
                filepath.Base(refPath), refY, refS, filepath.Base(tgtPath), tgtY, tgtS, dY, fx.Saturation)
        return &result, nil
}

// GetAssetSourceURL trả URL phát TỆP GỐC của tư liệu qua assetserver
// (/local/source/<id>) — dùng cho màn hình xem trước tư liệu (Source Monitor).
func (a *App) GetAssetSourceURL(assetID string) (string, error) {
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil {
                return "", fmt.Errorf("không tìm thấy tư liệu %s", assetID)
        }
        if as.Missing {
                return "", fmt.Errorf("tệp nguồn không còn: %s", as.Path)
        }
        return "/local/source/" + assetID, nil
}

// tail300 cắt chuỗi xuống tối đa 300 ký tự cuối (log gọn).
func tail300(s string) string {
        s = strings.TrimSpace(s)
        if len(s) > 300 {
                return s[len(s)-300:]
        }
        return s
}

// boolPtr helper.
func boolPtr(v bool) *bool { return &v }
