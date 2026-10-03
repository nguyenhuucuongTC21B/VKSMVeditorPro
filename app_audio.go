// app_audio.go — v1.3.0: track âm thanh độc lập A1 (kiểu KineMaster).
// Người dùng kéo mp3/wav vào track A1 → nhạc nền/giọng đọc song song với
// video; tắt tiếng clip video gốc (checkbox "Tắt tiếng clip" hoặc Ctrl+L tách
// âm) rồi dùng âm thanh chèn để biên tập video hoàn chỉnh. Khi xuất/xem trước,
// engine trộn các đoạn A1 lên âm thanh hiện có (engine.MixAudioTrack).
package main

import (
	"errors"
	"fmt"

	"vkseditorpro/internal/applog"
	"vkseditorpro/internal/project"
)

// AudioClipPatch là bản vá thuộc tính đoạn âm thanh — trường nil = không đổi.
type AudioClipPatch struct {
	StartMs   *int64   `json:"startMs"`
	InMs      *int64   `json:"inMs"`
	OutMs     *int64   `json:"outMs"`
	Volume    *float64 `json:"volume"`
	Mute      *bool    `json:"mute"`
	FadeInMs  *int64   `json:"fadeInMs"`
	FadeOutMs *int64   `json:"fadeOutMs"`
}

// AddAudioClip thêm đoạn âm thanh từ asset (chỉ nhận asset kind=audio) vào
// track A1 tại startMs. startMs < 0 = đặt SAU CUỐI track hiện có (tiện
// "＋ Timeline" trong thư viện). Trả đoạn đã tạo.
func (a *App) AddAudioClip(assetID string, startMs int64) (*project.AudioClip, error) {
	a.mu.Lock()
	as := a.doc.FindAsset(assetID)
	if as == nil {
		a.mu.Unlock()
		applog.Logf("AddAudioClip LỖI: không tìm thấy tư liệu %s", assetID)
		return nil, fmt.Errorf("không tìm thấy tư liệu %s", assetID)
	}
	if as.Missing {
		a.mu.Unlock()
		return nil, fmt.Errorf("tệp nguồn không còn: %s", as.Path)
	}
	if as.Kind != project.AssetAudio {
		a.mu.Unlock()
		return nil, errors.New("chỉ tệp ÂM THANH (mp3/wav/m4a…) mới đặt được vào track A1 — video/ảnh hãy thả vào V1/V2")
	}
	dur := as.DurationMs
	if dur <= 0 {
		a.mu.Unlock()
		return nil, errors.New("tệp âm thanh có thời lượng 0")
	}
	ac := &project.AudioClip{
		ID:      project.NewID("aud"),
		AssetID: assetID,
		InMs:    0,
		OutMs:   dur,
		Volume:  1,
	}
	// Vị trí: <0 = sau cuối track; kẹp để không tràn quá chiều dài timeline
	// (vẫn cho phép khi timeline trống — người dùng có thể dựng âm trước).
	if startMs < 0 {
		startMs = a.doc.AudioTrackEndMs()
	}
	if startMs < 0 {
		startMs = 0
	}
	ac.StartMs = startMs
	if total := a.doc.TimelineTotalMs(); total > 0 {
		if ac.StartMs >= total {
			ac.StartMs = total - 500
			if ac.StartMs < 0 {
				ac.StartMs = 0
			}
		}
	}
	ac.Normalize()
	a.doc.AudioClips = append(a.doc.AudioClips, ac)
	a.doc.SortAudioClips()
	a.mu.Unlock()
	if err := a.persist(); err != nil {
		return nil, err
	}
	applog.Logf("AddAudioClip OK: %s → A1 @%dms (%s, %d→%dms)", as.Name, ac.StartMs, ac.ID, ac.InMs, ac.OutMs)
	return ac, nil
}

// UpdateAudioClip áp bản vá cho đoạn âm thanh (kéo dời / cắt in/out /
// âm lượng / mute / fade). Không cho phép kéo ra ngoài tệp nguồn.
func (a *App) UpdateAudioClip(id string, patch AudioClipPatch) (*project.AudioClip, error) {
	a.mu.Lock()
	ac := a.doc.FindAudioClip(id)
	if ac == nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("không tìm thấy đoạn âm thanh %s", id)
	}
	as := a.doc.FindAsset(ac.AssetID)
	maxDur := int64(0)
	if as != nil {
		maxDur = as.DurationMs
	}
	if patch.StartMs != nil {
		v := *patch.StartMs
		if v < 0 {
			v = 0
		}
		ac.StartMs = v
	}
	if patch.InMs != nil {
		v := *patch.InMs
		if v < 0 {
			v = 0
		}
		ac.InMs = v
	}
	if patch.OutMs != nil && maxDur > 0 {
		v := *patch.OutMs
		if v > maxDur {
			v = maxDur
		}
		ac.OutMs = v
	}
	if ac.OutMs-ac.InMs < 100 {
		if maxDur > 0 && ac.InMs+100 <= maxDur {
			ac.OutMs = ac.InMs + 100
		} else {
			ac.InMs = 0
			ac.OutMs = maxDur
		}
	}
	if patch.Volume != nil {
		v := *patch.Volume
		if v < 0 {
			v = 0
		}
		if v > 2 {
			v = 2
		}
		ac.Volume = v
	}
	if patch.Mute != nil {
		ac.Mute = *patch.Mute
	}
	if patch.FadeInMs != nil {
		v := *patch.FadeInMs
		if v < 0 {
			v = 0
		}
		if v > 60000 {
			v = 60000
		}
		ac.FadeInMs = v
	}
	if patch.FadeOutMs != nil {
		v := *patch.FadeOutMs
		if v < 0 {
			v = 0
		}
		if v > 60000 {
			v = 60000
		}
		ac.FadeOutMs = v
	}
	ac.Normalize()
	a.doc.SortAudioClips()
	a.mu.Unlock()
	if err := a.persist(); err != nil {
		return nil, err
	}
	return ac, nil
}

// RemoveAudioClip xoá đoạn âm thanh khỏi track A1.
func (a *App) RemoveAudioClip(id string) error {
	a.mu.Lock()
	removed := a.doc.RemoveAudioClip(id)
	a.mu.Unlock()
	if !removed {
		return fmt.Errorf("không tìm thấy đoạn âm thanh %s", id)
	}
	return a.persist()
}

// DuplicateAudioClip nhân bản đoạn âm thanh — bản sao đặt NGAY SAU bản gốc
// (nếu tràn chiều dài timeline thì kẹp sát cuối).
func (a *App) DuplicateAudioClip(id string) (*project.AudioClip, error) {
	a.mu.Lock()
	src := a.doc.FindAudioClip(id)
	if src == nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("không tìm thấy đoạn âm thanh %s", id)
	}
	clone := *src
	clone.ID = project.NewID("aud")
	clone.StartMs = src.EndMs()
	if total := a.doc.TimelineTotalMs(); total > 0 && clone.StartMs >= total {
		clone.StartMs = total - clone.DurationMs()
		if clone.StartMs < 0 {
			clone.StartMs = 0
		}
	}
	a.doc.AudioClips = append(a.doc.AudioClips, &clone)
	a.doc.SortAudioClips()
	a.mu.Unlock()
	if err := a.persist(); err != nil {
		return nil, err
	}
	applog.Logf("DuplicateAudioClip: %s → %s @%dms", id, clone.ID, clone.StartMs)
	return &clone, nil
}
