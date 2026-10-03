// mixaudio.go — v1.3.0: trộn TRACK ÂM THANH ĐỘC LẬP (A1) vào tệp video đã dựng.
//
// Cách hoạt động: moviego dựng video từ các clip V1 + lớp phủ; sau khi tệp
// MP4 xong, chạy MỘT pass ffmpeg nữa với -c:v copy (video giữ nguyên từng
// bit, chỉ trộn lại âm thanh):
//
//      [0:a]                         = âm thanh gốc của timeline (hoặc im lặng)
//      [1..n] = -ss InMs -t Dur -i … = từng đoạn A1, delay tới StartMs
//      amix=normalize=0              = cộng dồn không giảm âm lượng
//
// Nhờ vậy nhạc nền mp3/wav trên track A1 nghe được đúng như khi xuất.
// Hỗ trợ fade in/out từng đoạn (afade) + âm lượng (volume).
package engine

import (
        "context"
        "fmt"
        "os"
        "os/exec"
        "strconv"
        "strings"

        "vkseditorpro/internal/executil"
        "vkseditorpro/internal/project"
)

// audioTrackSpec dữ liệu phẳng của một đoạn A1 phục vụ dựng lệnh
// (tách khỏi *project.AudioClip để unit-test không cần document).
type AudioTrackSpec struct {
        Path      string
        InMs      int64
        OutMs     int64
        StartMs   int64
        Volume    float64
        FadeInMs  int64
        FadeOutMs int64
}

// specOf map AudioClip → spec (kẹp thời lượng theo asset ở tầng app nên ở đây
// tin thời lượng đã hợp lệ).
func specOf(doc *project.Document, ac *project.AudioClip) (AudioTrackSpec, bool) {
        as := doc.FindAsset(ac.AssetID)
        if as == nil || as.Missing || ac.Mute {
                return AudioTrackSpec{}, false
        }
        return AudioTrackSpec{
                Path:      as.Path,
                InMs:      ac.InMs,
                OutMs:     ac.OutMs,
                StartMs:   ac.StartMs,
                Volume:    ac.Volume,
                FadeInMs:  ac.FadeInMs,
                FadeOutMs: ac.FadeOutMs,
        }, true
}

// buildAudioMixFilter dựng chuỗi filter_complex. CẤU TRÚC INPUT (do caller dựng):
//   input 0                = tệp video (LUÔN CÓ — map 0:v lấy hình từ đây)
//   input 1..n             = từng đoạn âm thanh A1
//   input n+1 (tuỳ chọn)   = anullsrc CHỈ khi video không có stream âm thanh
// Kết quả: [base] + [a1..an] → amix (duration=first: đoạn dài = base).
func BuildAudioMixFilter(videoHasAudio bool, totalMs int64, tracks []AudioTrackSpec) string {
        f := func(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }
        baseSrc := "[0:a]" // video có âm thanh → âm gốc từ video
        if !videoHasAudio {
                // video câm → base = anullsrc ở input cuối (1 + số track)
                baseSrc = "[" + strconv.Itoa(1+len(tracks)) + ":a]"
        }
        parts := []string{baseSrc + "aformat=sample_rates=48000:channel_layouts=stereo[base]"}
        for i, t := range tracks {
                dur := t.OutMs - t.InMs
                var chain []string
                if t.Volume != 1 && t.Volume > 0 {
                        chain = append(chain, "volume="+f(t.Volume))
                }
                if t.FadeInMs > 0 && t.FadeInMs < dur {
                        chain = append(chain, "afade=t=in:st=0:d="+msSec(t.FadeInMs))
                }
                if t.FadeOutMs > 0 && t.FadeOutMs < dur {
                        st := dur - t.FadeOutMs
                        chain = append(chain, "afade=t=out:st="+msSec(st)+":d="+msSec(t.FadeOutMs))
                }
                chain = append(chain,
                        "aformat=sample_rates=48000:channel_layouts=stereo",
                        "adelay=delays="+strconv.FormatInt(t.StartMs, 10)+":all=1")
                label := fmt.Sprintf("[a%d]", i+1)
                parts = append(parts, fmt.Sprintf("[%d:a]%s%s", i+1, strings.Join(chain, ","), label))
        }
        mix := "[base]"
        for i := range tracks {
                mix += fmt.Sprintf("[a%d]", i+1)
        }
        parts = append(parts, mix+"amix=inputs="+strconv.Itoa(len(tracks)+1)+":duration=first:normalize=0[aout]")
        return strings.Join(parts, ";")
}

// msSec định dạng ms → giây với 3 số lẻ (tham số ffmpeg).
func msSec(ms int64) string {
        return strconv.FormatFloat(float64(ms)/1000.0, 'f', 3, 64)
}

// ffprobeBin tìm ffprobe (ưu tiên MGO_FFPROBE).
func ffprobeBin() string {
        if p := os.Getenv("MGO_FFPROBE"); p != "" {
                return p
        }
        if p, err := exec.LookPath("ffprobe"); err == nil {
                return p
        }
        return ""
}

// videoHasAudioStream kiểm tra tệp có stream âm thanh không (ffprobe).
func videoHasAudioStream(bin, path string) bool {
        if bin == "" {
                return false
        }
        cmd := exec.Command(bin, "-v", "error", "-select_streams", "a",
                "-show_entries", "stream=index", "-of", "csv=p=0", path)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        out, err := cmd.Output()
        return err == nil && strings.TrimSpace(string(out)) != ""
}

// MixAudioTrack trộn các đoạn A1 của doc lên video đã dựng. Trả đường dẫn
// kết quả (thường chính là videoPath sau khi ghi đè an toàn). Không cần trộn
// → trả nguyên videoPath mà không chạm vào tệp.
func MixAudioTrack(ctx context.Context, videoPath string, doc *project.Document) (string, error) {
        if videoPath == "" || doc == nil {
                return videoPath, nil
        }
        var tracks []AudioTrackSpec
        for _, ac := range doc.ActiveAudioClips() {
                if s, ok := specOf(doc, ac); ok && s.OutMs > s.InMs {
                        tracks = append(tracks, s)
                }
        }
        if len(tracks) == 0 {
                return videoPath, nil
        }
        ff := ffmpegBin()
        if ff == "" {
                return videoPath, fmt.Errorf("FFmpeg chưa sẵn sàng — không trộn được track âm thanh")
        }

        // Video có sẵn âm thanh không? (mọi clip V1 bị mute → không)
        hasAudio := videoHasAudioStream(ffprobeBin(), videoPath)

        args := []string{"-y", "-i", videoPath} // input 0: video — LUÔN CÓ
        for _, t := range tracks {
                args = append(args,
                        "-ss", msSec(t.InMs),
                        "-t", msSec(t.OutMs-t.InMs),
                        "-i", t.Path)
        }
        if !hasAudio {
                // Video câm → thêm nền im lặng đúng độ dài làm mốc thời gian cho amix.
                total := doc.TimelineTotalMs()
                if total <= 0 {
                        total = doc.AudioTrackEndMs()
                }
                args = append(args, "-f", "lavfi", "-t", msSec(total),
                        "-i", "anullsrc=channel_layout=stereo:sample_rate=48000")
        }
        args = append(args, "-filter_complex", BuildAudioMixFilter(hasAudio, doc.TimelineTotalMs(), tracks))

        tmp := videoPath + ".mixtmp.mp4"
        args = append(args,
                "-map", "0:v", "-map", "[aout]",
                "-c:v", "copy",
                "-c:a", "aac", "-b:a", "192k",
                "-movflags", "+faststart",
                tmp)

        cmd := exec.CommandContext(ctx, ff, args...)
        executil.Hide(cmd) // v1.2.7: không hiện console trên Windows
        if b, err := cmd.CombinedOutput(); err != nil {
                _ = os.Remove(tmp)
                return videoPath, fmt.Errorf("trộn track âm thanh thất bại: %s", tailStr(string(b)))
        }
        // Ghi đè an toàn: đổi tên tệp mới lên chỗ tệp cũ.
        if err := os.Rename(tmp, videoPath); err != nil {
                // Trên Windows rename đè tệp tồn tại thường vẫn OK; nếu lỗi thì giữ tmp
                // và trả lỗi rõ ràng để tầng app báo người dùng.
                return videoPath, fmt.Errorf("ghi tệp đã trộn thất bại: %w", err)
        }
        return videoPath, nil
}
