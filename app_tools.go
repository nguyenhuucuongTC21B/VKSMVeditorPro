// app_tools.go — bindings v1.1 cho nhóm công cụ: cắt khoảng lặng, QC tự động,
// phát hiện cảnh, TTS, AI (STT/dịch/phân tích), proxy 480p, cắt nhanh
// không re-encode, nối nhanh và kiểm tra cập nhật GitHub.
package main

import (
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "sync"

        wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

        "vkseditorpro/internal/applog"
        "vkseditorpro/internal/project"
        "vkseditorpro/internal/tools"
)

// ---------------------------------------------------------------------------
// Cắt khoảng lặng tự động
// ---------------------------------------------------------------------------

// DetectClipSilence quét khoảng lặng của NGUỒN asset trong khoảng In/Out của clip.
func (a *App) DetectClipSilence(clipID string, noiseDb float64, minDurMs int64) ([]tools.Silence, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        a.mu.RLock()
        c := a.doc.FindClip(clipID)
        var as *project.Asset
        if c != nil {
                as = a.doc.FindAsset(c.AssetID)
        }
        a.mu.RUnlock()
        if c == nil || as == nil || as.Missing {
                return nil, errors.New("clip hoặc tư liệu không khả dụng")
        }
        sils, err := tools.DetectSilence(context.Background(), as.Path, noiseDb, minDurMs)
        if err != nil {
                return nil, err
        }
        // Giữ lặng cắt vào khoảng [InMs, OutMs] của clip.
        var res []tools.Silence
        for _, s := range sils {
                if s.EndMs <= c.InMs || s.StartMs >= c.OutMs {
                        continue
                }
                if s.StartMs < c.InMs {
                        s.StartMs = c.InMs
                }
                if s.EndMs > c.OutMs {
                        s.EndMs = c.OutMs
                }
                res = append(res, s)
        }
        return res, nil
}

// ApplySilenceCut cắt clip thành nhiều đoạn bỏ qua khoảng lặng. Trả số đoạn mới.
func (a *App) ApplySilenceCut(clipID string, silences []tools.Silence) (int, error) {
        if err := a.waitFF(); err != nil {
                return 0, err
        }
        a.mu.Lock()
        c := a.doc.FindClip(clipID)
        if c == nil {
                a.mu.Unlock()
                return 0, fmt.Errorf("không tìm thấy clip %s", clipID)
        }
        i := a.doc.ClipIndex(clipID)
        sort.Slice(silences, func(x, y int) bool { return silences[x].StartMs < silences[y].StartMs })
        // Tính các đoạn giữ lại trong [InMs, OutMs].
        type seg struct{ s, e int64 }
        var segs []seg
        cur := c.InMs
        for _, sl := range silences {
                s, e := sl.StartMs, sl.EndMs
                if s < c.InMs {
                        s = c.InMs
                }
                if e > c.OutMs {
                        e = c.OutMs
                }
                if e <= s {
                        continue
                }
                if s-cur >= 300 { // đoạn giữ tối thiểu 300ms
                        segs = append(segs, seg{cur, s})
                }
                if e > cur {
                        cur = e
                }
        }
        if c.OutMs-cur >= 300 {
                segs = append(segs, seg{cur, c.OutMs})
        }
        if len(segs) == 0 {
                a.mu.Unlock()
                return 0, errors.New("toàn bộ clip là khoảng lặng — không còn gì để giữ")
        }
        // Đoạn đầu dùng lại clip cũ; các đoạn sau tạo clip mới chèn sau nó.
        var newClips []*project.Clip
        for k, sg := range segs {
                if k == 0 {
                        nc := *c
                        nc.OutMs = sg.e
                        nc.Boomerang, nc.Reverse, nc.LoopN = false, false, 1
                        newClips = append(newClips, &nc)
                        continue
                }
                nc := *c
                nc.ID = project.NewID("c")
                nc.InMs, nc.OutMs = sg.s, sg.e
                nc.Boomerang, nc.Reverse, nc.LoopN = false, false, 1
                newClips = append(newClips, &nc)
        }
        // Thay thế: clip cũ → segs.
        tail := append([]*project.Clip{}, a.doc.Clips[i+1:]...)
        a.doc.Clips = append(append(a.doc.Clips[:i], newClips...), tail...)
        a.mu.Unlock()
        if err := a.persist(); err != nil {
                return 0, err
        }
        return len(newClips), nil
}

// ---------------------------------------------------------------------------
// QC tự động + phát hiện cảnh
// ---------------------------------------------------------------------------

// RunQCAsset quét QC một tư liệu trong thư viện.
func (a *App) RunQCAsset(assetID string) (*tools.QCReport, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil || as.Missing {
                return nil, errors.New("tư liệu không khả dụng")
        }
        return tools.RunQC(context.Background(), as.Path)
}

// RunQCCurrent quét QC trên bản xem trước đã render gần nhất (nếu có).
func (a *App) RunQCCurrent() (*tools.QCReport, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        p := filepath.Join(a.previewDir, "preview.mp4")
        if _, err := os.Stat(p); err != nil {
                return nil, errors.New("chưa có bản xem trước — hãy bấm «Xem trước» trước khi chạy QC")
        }
        return tools.RunQC(context.Background(), p)
}

// DetectAssetScenes phát hiện cảnh của asset; trả danh sách điểm cắt.
func (a *App) DetectAssetScenes(assetID string, threshold float64) ([]tools.SceneMark, error) {
        if err := a.waitFF(); err != nil {
                return nil, err
        }
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil || as.Missing {
                return nil, errors.New("tư liệu không khả dụng")
        }
        return tools.DetectScenes(context.Background(), as.Path, threshold)
}

// ---------------------------------------------------------------------------
// TTS — tạo giọng đọc (Windows SAPI, offline)
// ---------------------------------------------------------------------------

// ListTTSVoices liệt kê giọng đọc trên máy.
func (a *App) ListTTSVoices() ([]tools.VoiceInfo, error) {
        return tools.ListTTSEntries()
}

// TTSGenerate tạo file giọng đọc, tự nhập vào thư viện và (tuỳ chọn) thêm
// vào cuối timeline. Trả assetID.
func (a *App) TTSGenerate(text, voice string, rate int, addToTimeline bool) (string, error) {
        if err := a.waitFF(); err != nil {
                return "", err
        }
        if strings.TrimSpace(text) == "" {
                return "", errors.New("nội dung trống")
        }
        outPath := filepath.Join(a.dataDir, "tts", fmt.Sprintf("tts-%d.wav", nowMs()))
        if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
                return "", err
        }
        if _, err := tools.TTSGenerate(context.Background(), text, voice, rate, outPath); err != nil {
                return "", err
        }
        assets, err := a.AddAssets([]string{outPath})
        if err != nil || len(assets) == 0 {
                return "", fmt.Errorf("nhập file giọng đọc thất bại: %v", err)
        }
        id := assets[0].ID
        if addToTimeline {
                if _, err := a.AddClip(id, -1); err != nil {
                        return id, fmt.Errorf("đã tạo file %s nhưng thêm timeline lỗi: %v", outPath, err)
                }
        }
        applog.Logf("TTSGenerate xong: voice=%s rate=%d → %s", voice, rate, outPath)
        return id, nil
}

// ---------------------------------------------------------------------------
// AI — STT / dịch phụ đề / phân tích (cần cấu hình endpoint)
// ---------------------------------------------------------------------------

// AISettingsResponse trả cấu hình AI (giấu key).
type AISettingsResponse struct {
        BaseURL string `json:"baseUrl"`
        Model   string `json:"model"`
        ASRModel string `json:"asrModel"`
        HasKey  bool   `json:"hasKey"`
}

// settingsPath là file settings.json cục bộ (không nằm trong project).
func (a *App) settingsPath() string { return filepath.Join(a.dataDir, "settings.json") }

// loadAISettings đọc cấu hình AI từ settings.json.
func (a *App) loadAISettings() tools.AISettings {
        var s tools.AISettings
        if b, err := os.ReadFile(a.settingsPath()); err == nil {
                _ = json.Unmarshal(b, &s)
        }
        return s
}

// saveAISettings ghi cấu hình AI xuống settings.json (0600).
func (a *App) saveAISettings(s tools.AISettings) error {
        b, err := json.MarshalIndent(s, "", "  ")
        if err != nil {
                return err
        }
        tmp := a.settingsPath() + ".tmp"
        if err := os.WriteFile(tmp, b, 0o600); err != nil {
                return err
        }
        return os.Rename(tmp, a.settingsPath())
}

// GetAISettings trả cấu hình AI hiện tại (giấu key).
func (a *App) GetAISettings() AISettingsResponse {
        s := a.loadAISettings()
        return AISettingsResponse{BaseURL: s.BaseURL, Model: s.Model, ASRModel: s.ASRModel, HasKey: s.APIKey != ""}
}

// SetAISettings lưu cấu hình AI. key rỗng = giữ key cũ.
func (a *App) SetAISettings(baseUrl, key, model, asrModel string) error {
        s := a.loadAISettings()
        s.BaseURL = strings.TrimSpace(baseUrl)
        if strings.TrimSpace(key) != "" {
                s.APIKey = strings.TrimSpace(key)
        }
        s.Model = strings.TrimSpace(model)
        s.ASRModel = strings.TrimSpace(asrModel)
        return a.saveAISettings(s)
}

// AITranscribe nhận dạng giọng nói asset → tạo phụ đề trong dự án (async).
// Sự kiện: ai:transcribe:progress / ai:transcribe:done {cues} / ai:transcribe:error
func (a *App) AITranscribe(assetID string) error {
        if err := a.waitFF(); err != nil {
                return err
        }
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil || as.Missing {
                return errors.New("tư liệu không khả dụng")
        }
        s := a.loadAISettings()
        ctx, done, err := a.beginJob("AI nhận dạng giọng nói")
        if err != nil {
                return err
        }
        a.safeGo("AI nhận dạng giọng nói", func() {
                defer done()
                wruntime.EventsEmit(a.ctx, "ai:transcribe:progress", map[string]any{"message": "Đang gửi âm thanh tới API AI..."})
                res, err := tools.Transcribe(ctx, s, as.Path)
                if err != nil {
                        wruntime.EventsEmit(a.ctx, "ai:transcribe:error", map[string]any{"message": err.Error()})
                        return
                }
                st := &project.SubtitlesTrack{SourceName: filepath.Base(as.Path), Burn: true, FontSize: 42, ColorHex: "FFFFFF"}
                if len(res.Segments) > 0 {
                        for _, sg := range res.Segments {
                                t := strings.TrimSpace(sg.Text)
                                if t == "" {
                                        continue
                                }
                                st.Cues = append(st.Cues, project.Cue{
                                        StartMs: int64(sg.Start * 1000),
                                        EndMs:   int64(sg.End * 1000),
                                        Text:    t,
                                })
                        }
                } else if strings.TrimSpace(res.Text) != "" {
                        st.Cues = append(st.Cues, project.Cue{StartMs: 0, EndMs: as.DurationMs, Text: strings.TrimSpace(res.Text)})
                }
                a.mu.Lock()
                a.doc.Subs = st
                a.mu.Unlock()
                _ = a.persist()
                wruntime.EventsEmit(a.ctx, "ai:transcribe:done", map[string]any{"cues": len(st.Cues)})
        })
        return nil
}

// AITranslateSubs dịch phụ đề hiện tại sang targetLang (async).
// Sự kiện: ai:translate:done / ai:translate:error
func (a *App) AITranslateSubs(targetLang string) error {
        if err := a.waitFF(); err != nil {
                return err
        }
        if strings.TrimSpace(targetLang) == "" {
                targetLang = "Vietnamese"
        }
        a.mu.RLock()
        st := a.doc.Subs
        a.mu.RUnlock()
        if st == nil || len(st.Cues) == 0 {
                return errors.New("dự án chưa có phụ đề để dịch")
        }
        s := a.loadAISettings()
        ctx, done, err := a.beginJob("AI dịch phụ đề")
        if err != nil {
                return err
        }
        a.safeGo("AI dịch phụ đề", func() {
                defer done()
                lines := make([]string, len(st.Cues))
                for i, c := range st.Cues {
                        lines[i] = strings.ReplaceAll(c.Text, "\n", " ")
                }
                translated, err := tools.TranslateCues(ctx, s, lines, targetLang)
                if err != nil {
                        wruntime.EventsEmit(a.ctx, "ai:translate:error", map[string]any{"message": err.Error()})
                        return
                }
                a.mu.Lock()
                for i := range a.doc.Subs.Cues {
                        a.doc.Subs.Cues[i].Text = translated[i]
                }
                a.mu.Unlock()
                _ = a.persist()
                wruntime.EventsEmit(a.ctx, "ai:translate:done", map[string]any{"count": len(translated)})
        })
        return nil
}

// AIAnalyze gửi câu hỏi + ngữ cảnh dự án tới AI, trả câu trả lời (sync).
func (a *App) AIAnalyze(question string) (string, error) {
        if strings.TrimSpace(question) == "" {
                return "", errors.New("câu hỏi trống")
        }
        doc := a.cloneDoc()
        var sb strings.Builder
        sb.WriteString(fmt.Sprintf("Project: %s | Canvas: %dx%d @ %.2f fps | Timeline: %.1fs\n",
                doc.Name, doc.Canvas.Width, doc.Canvas.Height, doc.Canvas.Rate.Float(), float64(doc.TimelineTotalMs())/1000))
        sb.WriteString(fmt.Sprintf("Assets: %d, Clips: %d, Transitions: %d, Overlays: %d\n",
                len(doc.Assets), len(doc.Clips), len(doc.Transitions), len(doc.Overlays)))
        starts := doc.TimelineMsPerClip()
        for i, c := range doc.Clips {
                as := doc.FindAsset(c.AssetID)
                name := ""
                if as != nil {
                        name = as.Name
                }
                sb.WriteString(fmt.Sprintf("- Clip %d: %s | %.1fs→%.1fs | speed %.2fx | vol %.2f\n",
                        i+1, name, float64(starts[i])/1000, float64(starts[i]+c.EffectiveMs())/1000, c.Speed, c.Volume))
        }
        if doc.Subs != nil {
                sb.WriteString(fmt.Sprintf("Subtitles: %d cues, burn=%v\n", len(doc.Subs.Cues), doc.Subs.Burn))
        }
        s := a.loadAISettings()
        return tools.AnalyzeQuestion(context.Background(), s, sb.String(), question)
}

// ---------------------------------------------------------------------------
// Proxy 480p cho xem trước mượt
// ---------------------------------------------------------------------------

// GenerateProxies tạo proxy 480p cho toàn bộ video ≥720p (async).
// Sự kiện: proxy:progress {done,total} / proxy:done {count} / proxy:error
func (a *App) GenerateProxies() error {
        if err := a.waitFF(); err != nil {
                return err
        }
        a.mu.RLock()
        var targets []*project.Asset
        for _, as := range a.doc.Assets {
                if as.Kind == project.AssetVideo && !as.Missing && as.Height >= 720 {
                        targets = append(targets, as)
                }
        }
        a.mu.RUnlock()
        if len(targets) == 0 {
                return errors.New("không có video nào ≥720p cần tạo proxy")
        }
        ctx, done, err := a.beginJob("Tạo proxy 480p")
        if err != nil {
                return err
        }
        proxyDir := filepath.Join(a.dataDir, "proxies")
        _ = os.MkdirAll(proxyDir, 0o755)
        a.safeGo("Tạo proxy 480p", func() {
                defer done()
                count := 0
                for i, as := range targets {
                        select {
                        case <-ctx.Done():
                                wruntime.EventsEmit(a.ctx, "proxy:error", map[string]any{"message": "Đã huỷ tạo proxy", "canceled": true})
                                return
                        default:
                        }
                        out := filepath.Join(proxyDir, as.ID+".mp4")
                        if _, err := tools.GenerateProxy(ctx, as.Path, out, 480); err == nil {
                                count++
                        }
                        wruntime.EventsEmit(a.ctx, "proxy:progress", map[string]any{"done": i + 1, "total": len(targets)})
                }
                applog.Logf("GenerateProxies xong: %d proxy", count)
                wruntime.EventsEmit(a.ctx, "proxy:done", map[string]any{"count": count})
        })
        return nil
}

// GetProxyURL trả URL proxy của asset (tạo đồng bộ nếu chưa có — dùng cho 1 file).
func (a *App) GetProxyURL(assetID string) (string, error) {
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil || as.Missing || as.Kind != project.AssetVideo {
                return "", errors.New("tư liệu không khả dụng")
        }
        out := filepath.Join(a.dataDir, "proxies", as.ID+".mp4")
        if _, err := os.Stat(out); err != nil {
                _ = os.MkdirAll(filepath.Dir(out), 0o755)
                if _, err := tools.GenerateProxy(context.Background(), as.Path, out, 480); err != nil {
                        return "", err
                }
        }
        return "/local/proxy/" + as.ID + ".mp4", nil
}

// SetPreviewProxy lưu tuỳ chọn dùng proxy khi xem trước (settings cục bộ).
type PreviewSettings struct {
        UseProxy bool `json:"useProxy"`
}

var previewSetMu sync.Mutex

// SetUseProxy bật/tắt proxy trong trình xem trước.
func (a *App) SetUseProxy(enabled bool) error {
        previewSetMu.Lock()
        defer previewSetMu.Unlock()
        return os.WriteFile(a.settingsPath()+"-preview.json",
                []byte(fmt.Sprintf(`{"useProxy":%v}`, enabled)), 0o600)
}

// GetUseProxy đọc tuỳ chọn proxy.
func (a *App) GetUseProxy() bool {
        previewSetMu.Lock()
        defer previewSetMu.Unlock()
        b, err := os.ReadFile(a.settingsPath() + "-preview.json")
        return err == nil && strings.Contains(string(b), "true")
}

// ---------------------------------------------------------------------------
// Cắt nhanh / nối nhanh (không re-encode)
// ---------------------------------------------------------------------------

// FastCut cắt nhanh một asset không re-encode (chính xác keyframe).
func (a *App) FastCut(assetID string, inMs, outMs int64) (string, error) {
        if err := a.waitFF(); err != nil {
                return "", err
        }
        a.mu.RLock()
        as := a.doc.FindAsset(assetID)
        a.mu.RUnlock()
        if as == nil || as.Missing {
                return "", errors.New("tư liệu không khả dụng")
        }
        if outMs <= inMs {
                outMs = as.DurationMs
        }
        out := filepath.Join(filepath.Dir(as.Path), "cut-"+fmt.Sprintf("%d", nowMs())+filepath.Ext(as.Path))
        if err := tools.StreamCopyTrim(context.Background(), as.Path, out, inMs, outMs-inMs); err != nil {
                return "", err
        }
        added, err := a.AddAssets([]string{out})
        if err != nil || len(added) == 0 {
                return out, err
        }
        return out, nil
}

// QuickConcat nối nhanh nhiều asset thành 1 file (không re-encode nếu cùng codec).
func (a *App) QuickConcat(assetIDs []string, outPath string) (string, error) {
        if err := a.waitFF(); err != nil {
                return "", err
        }
        if len(assetIDs) < 2 {
                return "", errors.New("cần ít nhất 2 tư liệu để nối")
        }
        var files []string
        a.mu.RLock()
        for _, id := range assetIDs {
                as := a.doc.FindAsset(id)
                if as == nil || as.Missing {
                        a.mu.RUnlock()
                        return "", fmt.Errorf("tư liệu không khả dụng: %s", id)
                }
                files = append(files, as.Path)
        }
        a.mu.RUnlock()
        if strings.TrimSpace(outPath) == "" {
                outPath = filepath.Join(filepath.Dir(files[0]), "concat-"+fmt.Sprintf("%d", nowMs())+filepath.Ext(files[0]))
        }
        if err := tools.QuickConcat(context.Background(), outPath, files); err != nil {
                return "", err
        }
        _, err := a.AddAssets([]string{outPath})
        return outPath, err
}

// ---------------------------------------------------------------------------
// Kiểm tra cập nhật qua GitHub Release
// ---------------------------------------------------------------------------

// ChooseLUTFile mở hộp thoại chọn file LUT .cube (không tự áp).
func (a *App) ChooseLUTFile() (string, error) {
        return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
                Title: "Chọn file LUT 3D (.cube)",
                Filters: []wruntime.FileFilter{
                        {DisplayName: "LUT 3D (*.cube)", Pattern: "*.cube"},
                },
        })
}

// GitHubRepo mặc định (có thể đổi bằng SetGitHubRepo).
func (a *App) githubRepo() string {
        if b, err := os.ReadFile(a.settingsPath() + "-repo.txt"); err == nil {
                r := strings.TrimSpace(string(b))
                if r != "" {
                        return r
                }
        }
        return "vksstudio/VKSeditorPro"
}

// SetGitHubRepo đổi repo kiểm tra cập nhật.
func (a *App) SetGitHubRepo(repo string) error {
        repo = strings.TrimSpace(repo)
        return os.WriteFile(a.settingsPath()+"-repo.txt", []byte(repo), 0o600)
}

// CheckUpdate kiểm tra phiên bản mới (timeout ngắn, offline an toàn).
func (a *App) CheckUpdate() tools.UpdateInfo {
        return tools.CheckUpdate(a.githubRepo(), AppVersion)
}
