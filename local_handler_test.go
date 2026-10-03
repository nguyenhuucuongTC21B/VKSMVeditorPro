package main

// local_handler_test.go — kiểm tra localFileHandler phục vụ đúng các thư mục
// trắng và CHẶN mọi đường dẫn khác (bất kể hệ điều hành, đường dẫn URL luôn "/" ).

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"thumbs", "frames", "preview", "proxy", "tts"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Tệp hợp lệ trong frames/
	if err := os.WriteFile(filepath.Join(root, "frames", "abc.jpg"), []byte("JPEGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Tệp bí mật NẰM NGOÀI thư mục trắng (gốc dataDir)
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Tệp bí mật trong thư mục KHÔNG whitelist
	if err := os.MkdirAll(filepath.Join(root, "evil"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "evil", "x.jpg"), []byte("EVIL"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func get(t *testing.T, h http.Handler, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestLocalHandlerServesWhitelisted(t *testing.T) {
	h := newLocalFileHandler(newTestRoot(t))
	rec := get(t, h, "/local/frames/abc.jpg")
	if rec.Code != 200 {
		t.Fatalf("/local/frames/abc.jpg → %d, muốn 200", rec.Code)
	}
	if got := rec.Body.String(); got != "JPEGDATA" {
		t.Fatalf("nội dung sai: %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "image/jpeg") {
		t.Fatalf("Content-Type = %q, muốn image/jpeg", ct)
	}
}

func TestLocalHandlerBlocksOutside(t *testing.T) {
	h := newLocalFileHandler(newTestRoot(t))
	cases := []string{
		"/local/secret.txt",        // gốc dataDir — không whitelist
		"/local/evil/x.jpg",        // thư mục không whitelist
		"/local/frames/../secret",  // ../ xuyên ra ngoài
		"/local/frames/%2e%2e/secret", // encoded ..
		"/etc/passwd",              // ngoài /local/
		"/local/",                  // trống
		"/local/frames",            // thiếu tên tệp
	}
	for _, c := range cases {
		rec := get(t, h, c)
		if rec.Code == 200 {
			t.Fatalf("%s → 200 (PHẢI bị chặn)", c)
		}
		if strings.Contains(rec.Body.String(), "TOPSECRET") || strings.Contains(rec.Body.String(), "EVIL") {
			t.Fatalf("%s → lộ nội dung bí mật!", c)
		}
	}
}

func TestLocalHandlerHeadAllowed(t *testing.T) {
	h := newLocalFileHandler(newTestRoot(t))
	req := httptest.NewRequest(http.MethodHead, "/local/frames/abc.jpg", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("HEAD → %d, muốn 200", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/local/frames/abc.jpg", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST → %d, muốn 405", rec.Code)
	}
}

// TestLocalHandlerWindowsStyleURL mô phỏng nỗi sợ v1.2.1: nếu đường dẫn bị xử lý
// bằng filepath.Clean trên Windows sẽ thành "\" → 403. Test này chốt chặn hồi
// quy: URL dạng "/" PHẢI được phục vụ (handler mới dùng path.Clean thuần "/").
func TestLocalHandlerWindowsStyleURL(t *testing.T) {
	h := newLocalFileHandler(newTestRoot(t))
	// URL hợp lệ chứa sub-dir trắng phải 200 dù chạy trên hệ điều hành nào
	rec := get(t, h, "/local/frames/abc.jpg?v=1759400000000")
	if rec.Code != 200 {
		t.Fatalf("có querystring → %d, muốn 200", rec.Code)
	}
}
