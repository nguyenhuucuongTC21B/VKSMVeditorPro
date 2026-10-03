// audio.go — v1.3.0: AudioClip là đoạn âm thanh ĐỘC LẬP trên track A1
// (kiểu KineMaster/CapCut): đặt tự do theo StartMs, không phụ thuộc clip video.
// Người dùng tắt tiếng clip video gốc rồi dùng nhạc/giọng đọc ở A1 để dựng
// video hoàn chỉnh. InMs/OutMs là khoảng cắt trong tệp nguồn (mp3/wav/m4a…).
package project

import "sort"

// AudioClip một đoạn trên track âm thanh A1.
type AudioClip struct {
        ID        string  `json:"id"`
        AssetID   string  `json:"assetId"`
        StartMs   int64   `json:"startMs"` // vị trí trên timeline
        InMs      int64   `json:"inMs"`    // cắt trong tệp nguồn
        OutMs     int64   `json:"outMs"`   // loại trừ
        Volume    float64 `json:"volume"`  // 0..2, 1 = giữ nguyên
        Mute      bool    `json:"mute"`
        FadeInMs  int64   `json:"fadeInMs"`
        FadeOutMs int64   `json:"fadeOutMs"`
}

// DurationMs độ dài hiệu dụng của đoạn âm thanh.
func (a *AudioClip) DurationMs() int64 {
        d := a.OutMs - a.InMs
        if d < 0 {
                return 0
        }
        return d
}

// EndMs mốc kết thúc trên timeline.
func (a *AudioClip) EndMs() int64 { return a.StartMs + a.DurationMs() }

// Normalize kẹp giá trị vào khoảng hợp lệ.
func (a *AudioClip) Normalize() {
        if a.StartMs < 0 {
                a.StartMs = 0
        }
        if a.InMs < 0 {
                a.InMs = 0
        }
        if a.Volume <= 0 {
                a.Volume = 1
        }
        if a.Volume > 2 {
                a.Volume = 2
        }
        if a.FadeInMs < 0 {
                a.FadeInMs = 0
        }
        if a.FadeOutMs < 0 {
                a.FadeOutMs = 0
        }
}

// FindAudioClip tìm đoạn âm thanh theo ID.
func (d *Document) FindAudioClip(id string) *AudioClip {
        for _, a := range d.AudioClips {
                if a.ID == id {
                        return a
                }
        }
        return nil
}

// RemoveAudioClip xoá đoạn âm thanh theo ID (true = có xoá).
func (d *Document) RemoveAudioClip(id string) bool {
        out := d.AudioClips[:0]
        removed := false
        for _, a := range d.AudioClips {
                if a.ID == id {
                        removed = true
                        continue
                }
                out = append(out, a)
        }
        d.AudioClips = out
        return removed
}

// RemoveAudioClipsForAsset xoá mọi đoạn âm thanh đang dùng assetID
// (gọi khi gỡ tư liệu khỏi thư viện). Trả số đoạn đã xoá.
func (d *Document) RemoveAudioClipsForAsset(assetID string) int {
        out := d.AudioClips[:0]
        removed := 0
        for _, a := range d.AudioClips {
                if a.AssetID == assetID {
                        removed++
                        continue
                }
                out = append(out, a)
        }
        d.AudioClips = out
        return removed
}

// SortAudioClips sắp xếp đoạn âm thanh theo vị trí bắt đầu.
func (d *Document) SortAudioClips() {
        sort.SliceStable(d.AudioClips, func(i, j int) bool {
                return d.AudioClips[i].StartMs < d.AudioClips[j].StartMs
        })
}

// AudioTrackEndMs trả mốc kết thúc xa nhất của track âm thanh (0 nếu trống).
func (d *Document) AudioTrackEndMs() int64 {
        var end int64
        for _, a := range d.AudioClips {
                if e := a.EndMs(); e > end {
                        end = e
                }
        }
        return end
}

// ActiveAudioClips trả các đoạn âm thanh SẼ được trộn khi dựng
// (không mute, asset hợp lệ, không bật tắt tiếng toàn bộ).
func (d *Document) ActiveAudioClips() []*AudioClip {
        if d.MuteAll {
                return nil
        }
        out := make([]*AudioClip, 0, len(d.AudioClips))
        for _, a := range d.AudioClips {
                if a.Mute {
                        continue
                }
                as := d.FindAsset(a.AssetID)
                if as == nil || as.Missing {
                        continue
                }
                out = append(out, a)
        }
        return out
}

// sanitizeAudioClips dọn đoạn âm thanh rác (asset không còn/không phải âm
// thanh) + kẹp khoảng cắt trong tệp nguồn. Gọi từ Document.sanitize.
func (d *Document) sanitizeAudioClips() {
        if d.AudioClips == nil {
                d.AudioClips = []*AudioClip{}
                return
        }
        aud := d.AudioClips[:0]
        for _, ac := range d.AudioClips {
                if ac == nil {
                        continue
                }
                as := d.FindAsset(ac.AssetID)
                if as == nil || as.Kind != AssetAudio {
                        d.DroppedClips++
                        continue
                }
                if as.DurationMs > 0 {
                        if ac.InMs < 0 || ac.InMs >= as.DurationMs {
                                ac.InMs = 0
                        }
                        if ac.OutMs <= ac.InMs || ac.OutMs > as.DurationMs {
                                ac.OutMs = as.DurationMs
                        }
                }
                if ac.OutMs <= ac.InMs {
                        d.DroppedClips++
                        continue
                }
                if ac.StartMs < 0 {
                        ac.StartMs = 0
                }
                ac.Normalize()
                aud = append(aud, ac)
        }
        d.AudioClips = aud
}
