package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AISettings cấu hình endpoint AI tương thích OpenAI (dùng chung cho STT,
// dịch phụ đề, phân tích). Key lưu cục bộ trên máy người dùng.
type AISettings struct {
	BaseURL string `json:"baseUrl"` // vd https://api.openai.com/v1
	APIKey  string `json:"apiKey"`
	Model   string `json:"model"`       // cho chat (dịch/phân tích)
	ASRModel string `json:"asrModel"`   // cho STT (vd whisper-1)
}

// aiRequest thực hiện 1 HTTP request tới endpoint AI, decode JSON vào out.
func aiRequest(ctx context.Context, s AISettings, method, path string, contentType string, body io.Reader, out any) error {
	if s.BaseURL == "" || s.APIKey == "" {
		return fmt.Errorf("chưa cấu hình AI — mở Cài đặt AI để nhập địa chỉ máy chủ và API key")
	}
	url := strings.TrimRight(s.BaseURL, "/") + path
	cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, method, url, body)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Authorization", "Bearer "+s.APIKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("gọi API AI thất bại: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("API AI trả %d: %s", resp.StatusCode, tail(string(data)))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("đọc phản hồi AI thất bại: %w", err)
	}
	return nil
}

// TranscribeResult là kết quả nhận dạng giọng nói.
type TranscribeResult struct {
	Text     string          `json:"text"`
	Segments []TransSegment  `json:"segments,omitempty"`
}

// TransSegment là một đoạn có thời lượng.
type TransSegment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// Transcribe gọi endpoint /audio/transcriptions (whisper) với file âm thanh.
func Transcribe(ctx context.Context, s AISettings, mediaPath string) (*TranscribeResult, error) {
	model := s.ASRModel
	if model == "" {
		model = "whisper-1"
	}
	f, err := os.Open(mediaPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", filepath.Base(mediaPath))
	if _, err := io.Copy(fw, f); err != nil {
		return nil, err
	}
	_ = mw.WriteField("model", model)
	_ = mw.WriteField("response_format", "verbose_json")
	if err := mw.Close(); err != nil {
		return nil, err
	}

	var res TranscribeResult
	if err := aiRequest(ctx, s, http.MethodPost, "/audio/transcriptions",
		mw.FormDataContentType(), &buf, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// Chat gọi /chat/completions, trả nội dung tin nhắn đầu tiên.
func Chat(ctx context.Context, s AISettings, system, user string) (string, error) {
	model := s.Model
	if model == "" {
		model = "gpt-4o-mini"
	}
	payload := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.2,
	}
	b, _ := json.Marshal(payload)
	var res struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := aiRequest(ctx, s, http.MethodPost, "/chat/completions",
		"application/json", bytes.NewReader(b), &res); err != nil {
		return "", err
	}
	if len(res.Choices) == 0 {
		return "", fmt.Errorf("API AI không trả lựa chọn nào")
	}
	return res.Choices[0].Message.Content, nil
}

// TranslateCues dịch mảng cue sang targetLang bằng API AI, giữ nguyên số dòng.
// Gom theo từng đợt 30 dòng để giữ ngữ cảnh và chống quá tải.
func TranslateCues(ctx context.Context, s AISettings, lines []string, targetLang string) ([]string, error) {
	out := make([]string, len(lines))
	const batch = 30
	for i := 0; i < len(lines); i += batch {
		j := i + batch
		if j > len(lines) {
			j = len(lines)
		}
		var sb strings.Builder
		for k, ln := range lines[i:j] {
			sb.WriteString(fmt.Sprintf("%d. %s\n", k+1, ln))
		}
		sys := "You are a professional subtitle translator. Translate each numbered subtitle line into " + targetLang + ". Keep the numbering exactly. Preserve meaning, tone and brevity. Output ONLY the numbered translated lines, no explanations."
		ans, err := Chat(ctx, s, sys, sb.String())
		if err != nil {
			return nil, err
		}
		got := parseNumbered(ans, j-i)
		for k := i; k < j; k++ {
			if t := got[k-i]; t != "" {
				out[k] = t
			} else {
				out[k] = lines[k] // giữ nguyên khi thiếu bản dịch
			}
		}
	}
	return out, nil
}

// parseNumbered đọc lại các dòng "N. text" từ câu trả lời của AI.
func parseNumbered(ans string, want int) []string {
	res := make([]string, want)
	for _, ln := range strings.Split(ans, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		dot := strings.Index(ln, ".")
		if dot <= 0 {
			continue
		}
		n := 0
		ok := true
		for _, ch := range ln[:dot] {
			if ch < '0' || ch > '9' {
				ok = false
				break
			}
			n = n*10 + int(ch-'0')
		}
		if !ok || n < 1 || n > want {
			continue
		}
		res[n-1] = strings.TrimSpace(ln[dot+1:])
	}
	return res
}

// AnalyzeQuestion gửi câu hỏi phân tích kèm ngữ cảnh metadata tới API AI.
func AnalyzeQuestion(ctx context.Context, s AISettings, contextInfo, question string) (string, error) {
	sys := "You are a video production assistant. Answer concisely in the user's language based on the provided video project context."
	return Chat(ctx, s, sys, "CONTEXT:\n"+contextInfo+"\n\nQUESTION: "+question)
}
