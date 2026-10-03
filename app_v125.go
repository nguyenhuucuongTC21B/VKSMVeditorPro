package main

// app_v125.go — tính năng v1.2.5 "Giao diện linh hoạt":
//   · SetDocument   — thay toàn bộ tài liệu (nền tảng của Undo/Redo phía giao diện).
//   · SaveFramePNG  — lưu khung hình đang xem thành tệp PNG (hộp thoại chọn nơi lưu).
// Không đổi hành vi cũ của các binding v1.2.4 trở xuống.

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"

	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"vkseditorpro/internal/applog"
	"vkseditorpro/internal/engine"
	"vkseditorpro/internal/project"
)

// SetDocument thay toàn bộ tài liệu dự án bằng bản frontend gửi lên.
// Dùng cho Hoàn tác/Làm lại (Ctrl+Z / Ctrl+Y): giao diện giữ snapshot JSON
// của Document và gọi đây để quay về trạng thái cũ. Luôn lưu đĩa sau khi thay.
func (a *App) SetDocument(doc *project.Document) error {
	if doc == nil {
		return errors.New("tài liệu rỗng — không thể hoàn tác")
	}
	if doc.Version == 0 {
		// Dữ liệu frontend luôn đi từ chính backend ra nên Version luôn > 0;
		// chặn giá trị lạ để tránh ghi đè dự án bằng rỗng nhầm lẫn.
		return errors.New("tài liệu thiếu phiên bản — từ chối thay thế")
	}
	a.mu.Lock()
	a.doc = doc
	a.mu.Unlock()
	if err := a.persist(); err != nil {
		applog.Logf("SetDocument: lưu đĩa lỗi: %v", err)
		return fmt.Errorf("đã thay nhưng KHÔNG lưu được đĩa: %w", err)
	}
	applog.Logf("SetDocument: thay tài liệu «%s» (%d tư liệu, %d clip)", doc.Name, len(doc.Assets), len(doc.Clips))
	return nil
}

// SaveFramePNG mở hộp thoại chọn nơi lưu rồi xuất khung hiện tại của clip
// (đã áp biến đổi zoom/xoay/lật, khớp khung canvas) thành PNG.
// Trả về đường dẫn đã lưu; chuỗi rỗng = người dùng đã huỷ hộp thoại.
func (a *App) SaveFramePNG(clipID string, atMs int64) (string, error) {
	if err := a.waitFF(); err != nil {
		return "", err
	}
	if strings.TrimSpace(clipID) == "" {
		return "", errors.New("chưa chọn clip — hãy chọn một clip trên timeline trước")
	}
	path, err := wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:           "Lưu khung hình dưới dạng PNG",
		DefaultFilename: fmt.Sprintf("VKS-khung-%s.png", time.Now().Format("020106-150405")),
		Filters: []wruntime.FileFilter{
			{DisplayName: "Ảnh PNG (.png)", Pattern: "*.png"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // người dùng bấm Huỷ — không phải lỗi
	}
	if !strings.HasSuffix(strings.ToLower(path), ".png") {
		path += ".png"
	}
	if err := a.renderFramePNGTo(clipID, atMs, path); err != nil {
		return "", err
	}
	applog.Logf("SaveFramePNG: đã lưu %s", path)
	return path, nil
}

// renderFramePNGTo dựng 1 khung của clip (tách riêng để unit-test không cần
// hộp thoại GUI) và mã hoá PNG vào out.
func (a *App) renderFramePNGTo(clipID string, atMs int64, out string) error {
	doc := a.cloneDoc()
	c := doc.FindClip(clipID)
	if c == nil {
		return fmt.Errorf("không tìm thấy clip %s", clipID)
	}
	as := doc.FindAsset(c.AssetID)
	if as == nil || as.Missing {
		return errors.New("tệp nguồn không khả dụng")
	}
	if atMs < 0 {
		atMs = 0
	}
	if max := c.DurationMs() - 1; atMs > max {
		atMs = max
	}
	if atMs < 0 {
		atMs = 0
	}
	img, err := engine.RenderClipFrame(context.Background(), doc, clipID, atMs, 100)
	if err != nil {
		return fmt.Errorf("dựng khung lỗi: %w", err)
	}
	return savePNG(img, out)
}

// savePNG ghi ảnh ra tệp PNG an toàn (ghi tạm .tmp rồi đổi tên, như saveJPEG).
func savePNG(img image.Image, out string) error {
	f, err := os.Create(out + ".tmp")
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
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
