package project

import (
        "encoding/json"
        "testing"
)

// JSON dự án "cũ" — clip rác: assetId không tồn tại + in/out 0 + transitions null.
const staleJSON = `{
  "version": 2,
  "name": "Test",
  "canvas": {"width": 1920, "height": 1080, "rate": {"num": 30, "den": 1}},
  "assets": [
    {"id": "a-ok", "path": "C:/v/video.mp4", "name": "video.mp4", "kind": "video",
     "durationMs": 65000, "width": 1920, "height": 1080, "rate": {"num": 30, "den": 1},
     "hasAudio": true, "codec": "h264", "sizeBytes": 100, "missing": false},
    {"id": "a-gone", "path": "C:/gone/old.mp4", "name": "old.mp4", "kind": "video",
     "durationMs": 0, "width": 0, "height": 0, "missing": true}
  ],
  "clips": [
    {"id": "c-bad-ref", "assetId": "a-khong-ton-tai", "inMs": 0, "outMs": 5000},
    {"id": "c-zero", "assetId": "a-ok", "inMs": 0, "outMs": 0},
    {"id": "c-out-of-range", "assetId": "a-ok", "inMs": 999999, "outMs": 100},
    {"id": "c-on-gone-asset", "assetId": "a-gone", "inMs": 0, "outMs": 0},
    {"id": "c-good", "assetId": "a-ok", "inMs": 1000, "outMs": 5000}
  ],
  "transitions": null,
  "overlays": null
}`

func TestSanitizeRepairsStaleProject(t *testing.T) {
        var d Document
        if err := json.Unmarshal([]byte(staleJSON), &d); err != nil {
                t.Fatalf("unmarshal: %v", err)
        }
        d.sanitize()

        // Kỳ vọng: c-bad-ref và c-on-gone-asset bị BỎ (asset không tồn tại/0 giây),
        // c-zero và c-out-of-range được SỬA thành 0→65000, c-good giữ nguyên.
        if len(d.Clips) != 3 {
                t.Fatalf("mong đợi 3 clip còn lại, thực tế %d", len(d.Clips))
        }
        for _, c := range d.Clips {
                switch c.ID {
                case "c-zero", "c-out-of-range":
                        if c.InMs != 0 || c.OutMs != 65000 {
                                t.Fatalf("%s phải được sửa 0→65000, thực tế %d→%d", c.ID, c.InMs, c.OutMs)
                        }
                case "c-good":
                        if c.InMs != 1000 || c.OutMs != 5000 {
                                t.Fatalf("c-good bị sửa sai: %d→%d", c.InMs, c.OutMs)
                        }
                default:
                        t.Fatalf("clip không mong đợi còn lại: %s", c.ID)
                }
        }
        if d.DroppedClips != 2 {
                t.Fatalf("mong đợi bỏ 2 clip rác, thực tế %d", d.DroppedClips)
        }
        if d.RepairedClips != 2 {
                t.Fatalf("mong đợi sửa 2 clip, thực tế %d", d.RepairedClips)
        }
        if d.Transitions == nil || len(d.Transitions) != 0 {
                t.Fatalf("transitions phải là mảng rỗng, thực tế %#v", d.Transitions)
        }
        if d.Overlays == nil || len(d.Overlays) != 0 {
                t.Fatalf("overlays phải là mảng rỗng, thực tế %#v", d.Overlays)
        }
        if d.ForeignFormat {
                t.Fatal("không phải foreign format (asset có path)")
        }
}

// Clip trỏ asset hợp lệ nhưng out<=in -> tự gán lại toàn bộ thời lượng.
func TestSanitizeRepairsDegenerateClip(t *testing.T) {
        var d Document
        if err := json.Unmarshal([]byte(staleJSON), &d); err != nil {
                t.Fatalf("unmarshal: %v", err)
        }
        // Giữ 1 clip "zero" trỏ a-ok, bỏ các clip khác để dễ kiểm tra.
        d.Clips = []*Clip{d.Clips[1]}
        d.sanitize()

        if len(d.Clips) != 1 {
                t.Fatalf("clip zero phải được SỬA chứ không bỏ: %d clips", len(d.Clips))
        }
        if d.RepairedClips != 1 || d.DroppedClips != 0 {
                t.Fatalf("repaired=%d dropped=%d, mong đợi 1/0", d.RepairedClips, d.DroppedClips)
        }
        if d.Clips[0].InMs != 0 || d.Clips[0].OutMs != 65000 {
                t.Fatalf("clip zero phải được gán 0→65000, thực tế %d→%d", d.Clips[0].InMs, d.Clips[0].OutMs)
        }
}

// Tệp do phiên bản khác ghi: mọi asset đều không có path -> ForeignFormat.
func TestForeignFormatDetection(t *testing.T) {
        const foreign = `{
                "version": 2, "name": "X",
                "canvas": {"width": 1920, "height": 1080},
                "assets": [{"id": "a1", "name": "khong-ro"}],
                "clips": []
        }`
        var d Document
        if err := json.Unmarshal([]byte(foreign), &d); err != nil {
                t.Fatalf("unmarshal: %v", err)
        }
        d.sanitize()
        if !d.ForeignFormat {
                t.Fatal("phải phát hiện ForeignFormat khi mọi asset thiếu path")
        }
}

// Dự án hợp lệ không bị đụng tới.
func TestSanitizeKeepsValidProject(t *testing.T) {
        var d Document
        if err := json.Unmarshal([]byte(staleJSON), &d); err != nil {
                t.Fatalf("unmarshal: %v", err)
        }
        d.Clips = []*Clip{d.Clips[4]} // c-good
        d.DroppedClips, d.RepairedClips = 0, 0
        d.sanitize()
        if len(d.Clips) != 1 || d.DroppedClips != 0 || d.RepairedClips != 0 {
                t.Fatalf("dự án hợp lệ bị sửa: %d clips, repaired=%d, dropped=%d",
                        len(d.Clips), d.RepairedClips, d.DroppedClips)
        }
}
