// Package media cung cấp thao tác probe tư liệu và sinh thumbnail,
// dựa trên moviego (ffprobe/ffmpeg) — mọi lời gọi đều yêu cầu biến môi trường
// MGO_FFMPEG / MGO_FFPROBE đã được cài trước đó (main.go / ffmpegbin).
package media

import (
        "context"
        "errors"
        "fmt"
        "image"
        "image/jpeg"
        _ "image/gif"
        _ "image/png"
        "os"
        "path/filepath"
        "strings"
        "time"

        mgo "github.com/mowshon/moviego/v2"

        "vkseditorpro/internal/project"
)

// videoExt / imageExt / audioExt — phần mở rộng nhận diện nhanh.
var (
        videoExt = map[string]bool{".mp4": true, ".mov": true, ".mkv": true, ".webm": true, ".avi": true, ".m4v": true, ".wmv": true, ".flv": true, ".ts": true, ".mts": true, ".m2ts": true}
        imageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".bmp": true, ".gif": true}
        audioExt = map[string]bool{".mp3": true, ".wav": true, ".m4a": true, ".aac": true, ".ogg": true, ".flac": true, ".opus": true, ".wma": true}
)

// Importable kiểm tra phần mở rộng có được hỗ trợ không.
func Importable(path string) bool {
        e := strings.ToLower(filepath.Ext(path))
        return videoExt[e] || imageExt[e] || audioExt[e]
}

// Probe phân tích một tệp tư liệu và trả về Asset hoàn chỉnh (chưa có ID).
// Với ảnh tĩnh, decode bằng Go std để lấy kích thước (nhanh, không cần ffmpeg).
func Probe(path string) (*project.Asset, error) {
        st, err := os.Stat(path)
        if err != nil {
                return nil, fmt.Errorf("không truy cập được tệp: %w", err)
        }
        name := filepath.Base(path)
        ext := strings.ToLower(filepath.Ext(path))

        switch {
        case imageExt[ext]:
                f, err := os.Open(path)
                if err != nil {
                        return nil, err
                }
                defer f.Close()
                img, _, err := image.DecodeConfig(f)
                if err != nil {
                        return nil, fmt.Errorf("không đọc được ảnh %s: %w", name, err)
                }
                return &project.Asset{
                        Path:      path,
                        Name:      name,
                        Kind:      project.AssetImage,
                        Width:     img.Width,
                        Height:    img.Height,
                        Rate:      project.Rate{Num: 30, Den: 1},
                        SizeBytes: st.Size(),
                }, nil

        case audioExt[ext]:
                info, err := mgo.Probe(path)
                if err != nil {
                        return nil, fmt.Errorf("không đọc được âm thanh %s: %w", name, err)
                }
                return &project.Asset{
                        Path:       path,
                        Name:       name,
                        Kind:       project.AssetAudio,
                        DurationMs: info.Duration.Milliseconds(),
                        Rate:       project.Rate(info.Rate),
                        HasAudio:   true,
                        SizeBytes:  st.Size(),
                }, nil

        default: // video
                info, err := mgo.Probe(path)
                if err != nil {
                        return nil, fmt.Errorf("không đọc được video %s: %w", name, err)
                }
                kind := project.AssetVideo
                hasAudio := info.Audio != nil
                if info.Codec == "" && hasAudio {
                        // Tệp chỉ có âm thanh nhưng phần mở rộng là video.
                        kind = project.AssetAudio
                }
                a := &project.Asset{
                        Path:       path,
                        Name:       name,
                        Kind:       kind,
                        DurationMs: info.Duration.Milliseconds(),
                        Width:      info.Size.W,
                        Height:     info.Size.H,
                        Rate:       project.Rate(info.Rate),
                        HasAudio:   hasAudio,
                        Codec:      info.Codec,
                        SizeBytes:  st.Size(),
                }
                if a.DurationMs <= 0 {
                        return nil, fmt.Errorf("không xác định được thời lượng của %s", name)
                }
                return a, nil
        }
}

// ThumbPath trả đường dẫn thumbnail chuẩn cho asset tại thời điểm tMs.
func ThumbPath(dir, assetID string, tMs int64) string {
        return filepath.Join(dir, fmt.Sprintf("%s_%d.jpg", assetID, tMs))
}

// EnsureThumbnail tạo (nếu chưa có) thumbnail JPEG cho asset tại tMs và
// trả về đường dẫn. Ảnh tĩnh dùng chính nó; âm thanh không có thumbnail.
func EnsureThumbnail(ctx context.Context, a *project.Asset, dir string, tMs int64, width int) (string, error) {
        if a.Kind == project.AssetAudio {
                return "", errors.New("âm thanh không có thumbnail")
        }
        if err := os.MkdirAll(dir, 0o755); err != nil {
                return "", err
        }
        out := ThumbPath(dir, a.ID, tMs)
        if _, err := os.Stat(out); err == nil {
                return out, nil // đã cache
        }

        var img image.Image
        if a.Kind == project.AssetImage {
                f, err := os.Open(a.Path)
                if err != nil {
                        return "", err
                }
                img, _, err = image.Decode(f)
                _ = f.Close()
                if err != nil {
                        return "", err
                }
        } else {
                t := time.Duration(tMs) * time.Millisecond
                frame, err := mgo.ExtractFrame(ctx, a.Path, mgo.Time(t))
                if err != nil {
                        return "", fmt.Errorf("lấy khung hình: %w", err)
                }
                img = frame
        }

        // Thu nhỏ về chiều rộng tối đa width để tiết kiệm dung lượng.
        img = downscale(img, width)
        f, err := os.CreateTemp(dir, "thumb-*.jpg")
        if err != nil {
                return "", err
        }
        tmpPath := f.Name()
        if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 82}); err != nil {
                _ = f.Close()
                _ = os.Remove(tmpPath)
                return "", err
        }
        _ = f.Close()
        if err := os.Rename(tmpPath, out); err != nil {
                _ = os.Remove(tmpPath)
                return "", err
        }
        return out, nil
}

// downscale thu nhỏ ảnh về chiều rộng tối đa maxW (nếu lớn hơn).
func downscale(img image.Image, maxW int) image.Image {
        b := img.Bounds()
        w, h := b.Dx(), b.Dy()
        if maxW <= 0 || w <= maxW {
                return img
        }
        nw := maxW
        nh := int(float64(h) * float64(maxW) / float64(w))
        if nh < 1 {
                nh = 1
        }
        dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
        // Nearest-neighbour đơn giản là đủ cho thumbnail.
        for y := 0; y < nh; y++ {
                sy := b.Min.Y + y*h/nh
                for x := 0; x < nw; x++ {
                        sx := b.Min.X + x*w/nw
                        dst.Set(x, y, img.At(sx, sy))
                }
        }
        return dst
}
