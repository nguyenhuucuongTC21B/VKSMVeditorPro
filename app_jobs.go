package main

import (
        "context"
        "crypto/sha1"
        "encoding/hex"
        "errors"
        "fmt"
        "image"
        "image/jpeg"
        "net/http"
        "os"
        "path"
        "path/filepath"
        "strings"
        "sync"
        "time"

        wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

        "vkseditorpro/internal/applog"
        "vkseditorpro/internal/engine"
        "vkseditorpro/internal/selftest"
)

// ---------------------------------------------------------------------------
// Quản lý tác vụ (một tác vụ nặng tại một thời điểm)
// ---------------------------------------------------------------------------

// beginJob đăng ký bắt đầu tác vụ; trả hàm kết thúc. Lỗi nếu đang bận.
// v1.2.7: tác vụ gắn runCtx (thay vì ctx của Wails) — shutdown huỷ runCtx
// là mọi ffmpeg của tác vụ bị kill, không còn tiến trình mồ côi.
func (a *App) beginJob(name string) (context.Context, func(), error) {
        a.jobMu.Lock()
        defer a.jobMu.Unlock()
        if a.jobName != "" {
                return nil, nil, fmt.Errorf("đang bận với tác vụ «%s» — hãy đợi hoặc huỷ trước", a.jobName)
        }
        parent := context.Background()
        if a.runCtx != nil {
                parent = a.runCtx
        }
        ctx, cancel := context.WithCancel(parent)
        a.jobName = name
        a.cancel = cancel
        done := func() {
                a.jobMu.Lock()
                a.jobName = ""
                a.cancel = nil
                a.jobMu.Unlock()
        }
        return ctx, done, nil
}

// currentJobName trả tên tác vụ đang chạy ("" = rảnh).
func (a *App) CurrentJob() string {
        a.jobMu.Lock()
        defer a.jobMu.Unlock()
        return a.jobName
}

// ---------------------------------------------------------------------------
// Khung hình xem trước WYSIWYG của clip
// ---------------------------------------------------------------------------

// v1.2.7 — chống dồn tiến trình render khung (nguồn của "cmd nhấp nháy liên
// tục" + app chết vì quá tải trước đây):
//   - frameSem: tối đa 2 render khung chạy đồng thời, phần còn lại xếp hàng.
//   - frameFl: singleflight theo vân tay yêu cầu — các yêu cầu TRÙNG (cùng
//     clip/cùng khung — ví dụ tua qua lại) chỉ render MỘT lần.
var (
        frameSem = make(chan struct{}, 2)

        frameMu sync.Mutex
        frameFl = map[string]*frameFlight{}
)

type frameFlight struct {
        done chan struct{}
        err  error
}

// GetClipFrameURL render 1 khung của clip (đã biến đổi, khớp canvas) tại
// atMs (trục clip) — trả URL tĩnh /local/frames/... để <img> hiển thị.
// Kết quả được cache theo "dấu vân tay" của clip nên lần 2 sẽ tức thì.
func (a *App) GetClipFrameURL(clipID string, atMs int64) (string, error) {
        if err := a.waitFF(); err != nil {
                return "", err
        }
        doc := a.cloneDoc()
        c := doc.FindClip(clipID)
        if c == nil {
                return "", fmt.Errorf("không tìm thấy clip %s", clipID)
        }
        as := doc.FindAsset(c.AssetID)
        if as == nil || as.Missing {
                return "", errors.New("tệp nguồn không khả dụng")
        }
        if atMs < 0 {
                atMs = 0
        }
        if atMs > c.DurationMs()-1 {
                atMs = c.DurationMs() - 1
        }
        if atMs < 0 {
                atMs = 0
        }
        fp := fmt.Sprintf("%s|%d|%d|%d|%s|%v|%v|%.2f|%.2f|%v|%dx%d|%d",
                clipID, c.InMs, c.OutMs, atMs, as.Path, c.Transform.FlipH, c.Transform.FlipV,
                c.Transform.Zoom, c.Transform.RotateDeg, c.Mute, doc.Canvas.Width, doc.Canvas.Height, as.SizeBytes)
        h := sha1.Sum([]byte(fp))
        name := hex.EncodeToString(h[:]) + ".jpg"
        out := filepath.Join(a.framesDir, name)
        if _, err := os.Stat(out); err == nil {
                return "/local/frames/" + name, nil
        }

        // Singleflight: yêu cầu trùng đang chạy → chờ dùng chung kết quả.
        frameMu.Lock()
        if fl, ok := frameFl[fp]; ok {
                frameMu.Unlock()
                select {
                case <-fl.done:
                        if fl.err != nil {
                                return "", fl.err
                        }
                        return "/local/frames/" + name, nil
                case <-a.runCtx.Done():
                        return "", errors.New("đang đóng ứng dụng")
                }
        }
        fl := &frameFlight{done: make(chan struct{})}
        frameFl[fp] = fl
        frameMu.Unlock()
        defer func() {
                frameMu.Lock()
                delete(frameFl, fp)
                frameMu.Unlock()
                close(fl.done)
        }()

        // Semaphore: tối đa 2 render đồng thời (hoặc thoát nếu app đang đóng).
        select {
        case frameSem <- struct{}{}:
                defer func() { <-frameSem }()
        case <-a.runCtx.Done():
                return "", errors.New("đang đóng ứng dụng")
        }

        img, err := engine.RenderClipFrame(a.runCtx, doc, clipID, atMs, 86)
        if err != nil {
                fl.err = err
                applog.Logf("GetClipFrameURL lỗi (clip %s @%dms): %v", clipID, atMs, err)
                return "", err
        }
        if err := saveJPEG(img, out); err != nil {
                fl.err = err
                return "", err
        }
        return "/local/frames/" + name, nil
}

// ---------------------------------------------------------------------------
// Bản xem trước (render nhanh độ phân giải thấp)
// ---------------------------------------------------------------------------

// previewURL trả URL bản xem trước nếu đã có.
func (a *App) previewURL() string {
        p := filepath.Join(a.previewDir, "preview.mp4")
        if _, err := os.Stat(p); err == nil {
                return "/local/preview/preview.mp4"
        }
        return ""
}

// RenderPreview render timeline hiện tại thành video xem trước 540p (async).
// Bắn sự kiện: preview:progress / preview:done / preview:error.
func (a *App) RenderPreview() error {
        if err := a.waitFF(); err != nil {
                return err
        }
        doc := a.cloneDoc()
        ctx, done, err := a.beginJob("Xem trước")
        if err != nil {
                return err
        }
        applog.Logf("RenderPreview bắt đầu")
        a.safeGo("Xem trước", func() {
                defer done()
                out := filepath.Join(a.previewDir, "preview.mp4")
                _ = os.Remove(out)
                // Fix cứng: 540p, CRF 30, veryfast — nhanh nhưng đủ rõ.
                g, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: 540})
                if err != nil {
                        applog.Logf("RenderPreview Compile lỗi: %v", err)
                        wruntime.EventsEmit(a.ctx, "preview:error", map[string]any{"message": err.Error()})
                        return
                }
                defer g.Close()
                err = engine.WriteVideo(ctx, g, engine.ExportRequest{
                        Format:     engine.FormatMP4,
                        OutputPath: out,
                        CRF:        30,
                        Preset:     "veryfast",
                }, func(doneN, total int) {
                        pct := 0
                        if total > 0 {
                                pct = doneN * 100 / total
                        }
                        wruntime.EventsEmit(a.ctx, "preview:progress", map[string]any{"done": doneN, "total": total, "pct": pct})
                })
                if err != nil {
                        if errors.Is(err, context.Canceled) {
                                wruntime.EventsEmit(a.ctx, "preview:error", map[string]any{"message": "Đã huỷ xem trước", "canceled": true})
                                return
                        }
                        wruntime.EventsEmit(a.ctx, "preview:error", map[string]any{"message": err.Error()})
                        return
                }
                // v1.3.0: trộn TRACK ÂM THANH A1 (mp3/wav người dùng chèn) vào bản
                // xem trước — video copy nguyên, chỉ trộn âm, rất nhanh.
                if _, merr := engine.MixAudioTrack(ctx, out, doc); merr != nil {
                        applog.Logf("RenderPreview: trộn track âm thanh LỖI: %v", merr)
                        wruntime.EventsEmit(a.ctx, "preview:error", map[string]any{"message": "Không trộn được track âm thanh A1: " + merr.Error()})
                }
                wruntime.EventsEmit(a.ctx, "preview:done", map[string]any{"url": "/local/preview/preview.mp4", "ts": nowMs()})
                applog.Logf("RenderPreview xong")
        })
        return nil
}

// CancelPreview huỷ render xem trước.
func (a *App) CancelPreview() error {
        a.jobMu.Lock()
        defer a.jobMu.Unlock()
        if a.jobName == "Xem trước" && a.cancel != nil {
                a.cancel()
        }
        return nil
}

// ---------------------------------------------------------------------------
// Xuất video
// ---------------------------------------------------------------------------

// ChooseExportPath mở hộp thoại chọn nơi lưu theo định dạng.
func (a *App) ChooseExportPath(format string) (string, error) {
        ext := "mp4"
        switch format {
        case "gif":
                ext = "gif"
        case "movalpha":
                ext = "mov"
        }
        doc := a.cloneDoc()
        path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
                Title:           "Chọn nơi lưu video xuất ra",
                DefaultFilename: fmt.Sprintf("VKSeditorPro-%s.%s", time.Now().Format("020106-150405"), ext),
                Filters: []wruntime.FileFilter{
                        {DisplayName: fmt.Sprintf("Tệp %s (.%s)", format, ext), Pattern: "*." + ext},
                },
        })
        _ = doc
        if err != nil {
                return "", err
        }
        return path, nil
}

// StartExport xuất video theo request (async). Sự kiện:
// export:progress {done,total,pct} / export:done {path,size} / export:error {message}
func (a *App) StartExport(req engine.ExportRequest) error {
        if err := a.waitFF(); err != nil {
                return err
        }
        if err := req.Validate(); err != nil {
                return err
        }
        doc := a.cloneDoc()
        ctx, done, err := a.beginJob("Xuất video")
        if err != nil {
                return err
        }
        applog.Logf("StartExport: %s (format=%s, targetH=%d, crf=%d, preset=%s)", req.OutputPath, req.Format, req.TargetH, req.CRF, req.Preset)
        a.safeGo("Xuất video", func() {
                defer done()
                g, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: req.TargetH})
                if err != nil {
                        applog.Logf("StartExport Compile lỗi: %v", err)
                        wruntime.EventsEmit(a.ctx, "export:error", map[string]any{"message": err.Error()})
                        return
                }
                defer g.Close()
                start := time.Now()
                err = engine.WriteVideo(ctx, g, req, func(doneN, total int) {
                        pct := 0
                        if total > 0 {
                                pct = doneN * 100 / total
                        }
                        wruntime.EventsEmit(a.ctx, "export:progress", map[string]any{"done": doneN, "total": total, "pct": pct})
                })
                if err != nil {
                        if errors.Is(err, context.Canceled) {
                                wruntime.EventsEmit(a.ctx, "export:error", map[string]any{"message": "Đã huỷ xuất video", "canceled": true})
                                return
                        }
                        wruntime.EventsEmit(a.ctx, "export:error", map[string]any{"message": err.Error()})
                        applog.Logf("StartExport lỗi: %s", err.Error())
                        return
                }
                st, _ := os.Stat(req.OutputPath)
                size := int64(0)
                if st != nil {
                        size = st.Size()
                }
                // v1.3.0: trộn TRACK ÂM THANH A1 vào video xuất ra (chỉ MP4 —
                // GIF/MOV-alpha vốn không mang âm thanh). Video copy nguyên,
                // chỉ trộn âm nên rất nhanh.
                if req.Format == engine.FormatMP4 || req.Format == "" {
                        if _, merr := engine.MixAudioTrack(ctx, req.OutputPath, doc); merr != nil {
                                applog.Logf("StartExport: trộn track âm thanh LỖI: %v", merr)
                                wruntime.EventsEmit(a.ctx, "export:note", map[string]any{
                                        "message": "Video đã xuất NHƯNG không trộn được track âm thanh A1: " + merr.Error(),
                                })
                        } else if len(doc.ActiveAudioClips()) > 0 {
                                applog.Logf("StartExport: đã trộn %d đoạn âm thanh A1 vào video", len(doc.ActiveAudioClips()))
                        }
                }
                applog.Logf("StartExport XONG: %s (%d byte, mất %s)", req.OutputPath, size, time.Since(start).Round(time.Millisecond))
                wruntime.EventsEmit(a.ctx, "export:done", map[string]any{
                        "path": req.OutputPath,
                        "size": size,
                        "took": time.Since(start).Round(time.Millisecond).String(),
                })
        })
        return nil
}

// CancelExport huỷ tác vụ xuất video.
func (a *App) CancelExport() error {
        a.jobMu.Lock()
        defer a.jobMu.Unlock()
        if a.jobName == "Xuất video" && a.cancel != nil {
                a.cancel()
        }
        return nil
}

// ---------------------------------------------------------------------------
// Selftest từ giao diện
// ---------------------------------------------------------------------------

// RunSelfTest chạy kiểm tra engine (async) và ghi log vào dataDir.
// Sự kiện: selftest:log {line} / selftest:done {ok}
func (a *App) RunSelfTest() error {
        if err := a.waitFF(); err != nil {
                return err
        }
        _, done, err := a.beginJob("Tự kiểm tra")
        if err != nil {
                return err
        }
        a.safeGo("Tự kiểm tra", func() {
                defer done()
                logPath := filepath.Join(a.dataDir, "selftest.log")
                lw := newLineWriter(func(line string) {
                        wruntime.EventsEmit(a.ctx, "selftest:log", map[string]any{"line": line})
                }, logPath)
                code := selftest.Run(lw)
                _ = lw.Close()
                wruntime.EventsEmit(a.ctx, "selftest:done", map[string]any{"ok": code == 0, "logPath": logPath})
        })
        return nil
}

// ---------------------------------------------------------------------------
// HTTP handler phục vụ tệp cục bộ qua assetserver (/local/...)
// ---------------------------------------------------------------------------

// localFileHandler phục vụ CHỈ các thư mục con an toàn của dataDir:
// thumbs/, frames/, preview/. Bất kỳ đường dẫn nào khác → 403.
// v1.2.3: thêm route /local/source/<assetID> (Source Monitor) qua resolver.
type localFileHandler struct {
        root     string
        mu       sync.RWMutex
        sourceFn func(id string) (string, error)
}

func newLocalFileHandler(root string) *localFileHandler {
        return &localFileHandler{root: root}
}

// SetSourceResolver đăng ký hàm tra tư liệu gốc theo ID (gọi lúc startup).
func (h *localFileHandler) SetSourceResolver(fn func(id string) (string, error)) {
        h.mu.Lock()
        h.sourceFn = fn
        h.mu.Unlock()
}

func (h *localFileHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodGet && r.Method != http.MethodHead {
                http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
                return
        }
        // v1.2.2 SỬA LỖI WINDOWS: đường dẫn URL phải xử lý bằng path.Clean (luôn giữ
        // dấu "/"). Trước đây dùng filepath.Clean — trên Windows nó đổi sang "\" khiến
        // TrimPrefix("/local/") KHÔNG khớp → whitelist sai → 403 với MỌI request
        // /local/* (khung xem trước + thumbnail gãy trên Windows, chạy tốt trên Linux).
        clean := path.Clean("/" + r.URL.Path) // luôn dạng "/", chống ../
        if !strings.HasPrefix(clean, "/local/") {
                http.NotFound(w, r)
                return
        }
        // v1.2.3: Source Monitor — phát tệp GỐC của tư liệu theo ID trong doc.
        if strings.HasPrefix(clean, "/local/source/") {
                id := strings.TrimPrefix(clean, "/local/source/")
                h.mu.RLock()
                fn := h.sourceFn
                h.mu.RUnlock()
                if fn == nil || id == "" || strings.Contains(id, "/") {
                        http.NotFound(w, r)
                        return
                }
                p, err := fn(id)
                if err != nil {
                        http.NotFound(w, r)
                        return
                }
                f, err := os.Open(p)
                if err != nil {
                        http.NotFound(w, r)
                        return
                }
                defer f.Close()
                st, err := f.Stat()
                if err != nil || st.IsDir() {
                        http.NotFound(w, r)
                        return
                }
                http.ServeContent(w, r, filepath.Base(p), st.ModTime(), f)
                return
        }
        rel := strings.TrimPrefix(clean, "/local/")
        if rel == "" || rel == "." {
                http.NotFound(w, r)
                return
        }
        sub := strings.SplitN(rel, "/", 2)
        allow := map[string]bool{"thumbs": true, "frames": true, "preview": true, "proxy": true, "tts": true}
        if len(sub) < 2 || !allow[sub[0]] {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
        }
        // Chuyển sang đường dẫn hệ điều hành CHỈ ở bước mở file.
        abs := filepath.Join(h.root, filepath.FromSlash(rel))
        root := filepath.Clean(h.root)
        if !strings.HasPrefix(abs, root+string(filepath.Separator)) {
                http.Error(w, "forbidden", http.StatusForbidden)
                return
        }
        f, err := os.Open(abs)
        if err != nil {
                http.NotFound(w, r)
                return
        }
        defer f.Close()
        st, err := f.Stat()
        if err != nil || st.IsDir() {
                http.NotFound(w, r)
                return
        }
        // ServeContent tự xử lý Range → <video> tua được.
        http.ServeContent(w, r, filepath.Base(abs), st.ModTime(), f)
}

// ---------------------------------------------------------------------------
// lineWriter — ghi log theo dòng cho selftest
// ---------------------------------------------------------------------------

type lineWriter struct {
        mu     sync.Mutex
        buf    []byte
        onLine func(string)
        file   *os.File
}

func newLineWriter(onLine func(string), logPath string) *lineWriter {
        f, err := os.Create(logPath)
        if err != nil {
                f = nil
        }
        return &lineWriter{onLine: onLine, file: f}
}

func (w *lineWriter) Write(p []byte) (int, error) {
        w.mu.Lock()
        defer w.mu.Unlock()
        w.buf = append(w.buf, p...)
        for {
                i := -1
                for j, b := range w.buf {
                        if b == '\n' {
                                i = j
                                break
                        }
                }
                if i < 0 {
                        break
                }
                line := string(w.buf[:i])
                w.buf = w.buf[i+1:]
                if w.onLine != nil {
                        w.onLine(line)
                }
                if w.file != nil {
                        _, _ = w.file.WriteString(line + "\n")
                }
        }
        return len(p), nil
}

func (w *lineWriter) Close() error {
        w.mu.Lock()
        defer w.mu.Unlock()
        if w.file != nil {
                return w.file.Close()
        }
        return nil
}

// saveJPEG mã hoá ảnh image.Image ra tệp JPEG.
func saveJPEG(img image.Image, out string) error {
        f, err := os.Create(out + ".tmp")
        if err != nil {
                return err
        }
        if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 86}); err != nil {
                _ = f.Close()
                _ = os.Remove(out + ".tmp")
                return err
        }
        if err := f.Close(); err != nil {
                _ = os.Remove(out + ".tmp")
                return err
        }
        return os.Rename(out+".tmp", out)
}
