package tools

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseSRT đọc nội dung file SRT thành danh sách cue (ms).
// Chấp nhận cả dấu ',' hoặc '.' trong mili-giây.
func ParseSRT(data string) (cues []struct {
	StartMs int64
	EndMs   int64
	Text    string
}, err error) {
	type rawCue struct {
		start, end int64
		text       string
	}
	var raws []rawCue
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var cur *rawCue
	flush := func() {
		if cur != nil && cur.text != "" {
			raws = append(raws, *cur)
		}
		cur = nil
	}
	for _, ln := range lines {
		ln = strings.TrimRight(ln, "\r")
		switch {
		case strings.Contains(ln, "-->"):
			parts := strings.Split(ln, "-->")
			if len(parts) != 2 {
				continue
			}
			s, err1 := parseSRTTime(strings.TrimSpace(parts[0]))
			e, err2 := parseSRTTime(strings.TrimSpace(parts[1]))
			if err1 != nil || err2 != nil {
				continue
			}
			flush()
			cur = &rawCue{start: s, end: e}
		case ln == "":
			flush()
		case cur != nil:
			if cur.text != "" {
				cur.text += "\n"
			}
			cur.text += ln
		}
	}
	flush()
	for _, r := range raws {
		cues = append(cues, struct {
			StartMs int64
			EndMs   int64
			Text    string
		}{r.start, r.end, r.text})
	}
	return cues, nil
}

// parseSRTTime đọc "00:01:02,500" → ms.
func parseSRTTime(s string) (int64, error) {
	s = strings.ReplaceAll(s, ",", ".")
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("thời gian SRT không hợp lệ: %s", s)
	}
	h, err1 := strconv.ParseFloat(parts[0], 64)
	m, err2 := strconv.ParseFloat(parts[1], 64)
	sec, err3 := strconv.ParseFloat(parts[2], 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, fmt.Errorf("thời gian SRT không hợp lệ: %s", s)
	}
	return int64((h*3600 + m*60 + sec) * 1000), nil
}

// BuildSRT tạo nội dung file SRT từ danh sách cue (ms).
func BuildSRT(cues []struct {
	StartMs int64
	EndMs   int64
	Text    string
}) string {
	var sb strings.Builder
	for i, c := range cues {
		sb.WriteString(fmt.Sprintf("%d\n", i+1))
		sb.WriteString(fmt.Sprintf("%s --> %s\n", msToSRT(c.StartMs), msToSRT(c.EndMs)))
		sb.WriteString(c.Text)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// msToSRT đổi ms → "00:00:01,500".
func msToSRT(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	mm := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, mm)
}
