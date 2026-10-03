// ffmpegbin_test.go — kiểm tra Extract: progress, resume, marker.
package ffmpegbin

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// buildFakeZip tạo zip giả chứa 2 tệp "ffmpeg.exe"/"ffprobe.exe" đủ lớn (>1MB)
// nhưng thay thế embeddedZip trong quá trình test bằng zip này.
func buildFakeZip(t *testing.T, dir string) {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for _, name := range []string{"ffmpeg.exe", "ffprobe.exe"} {
		fw, err := zw.Create("build/" + name)
		if err != nil {
			t.Fatal(err)
		}
		payload := bytes.Repeat([]byte("V"), 2<<20) // 2MB
		if _, err := fw.Write(payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fake.zip"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// swapEmbedded thay embeddedZip bằng zip giả trong phạm vi test.
func swapEmbedded(t *testing.T, data []byte) {
	t.Helper()
	old := embeddedZip
	embeddedZip = data
	t.Cleanup(func() { embeddedZip = old })
}

func TestExtractProgressAndMarker(t *testing.T) {
	tmp := t.TempDir()
	buildFakeZip(t, tmp)
	fake, err := os.ReadFile(filepath.Join(tmp, "fake.zip"))
	if err != nil {
		t.Fatal(err)
	}
	swapEmbedded(t, fake)

	// Đổi thư mục cache về tmp — targetDir dùng os.UserCacheDir nên ta
	// kiểm tra thông qua Extract trả về Dir thực tế và marker nằm trong đó.
	phases := map[string]bool{}
	lastPct := 0.0
	res, err := Extract(func(pct float64, phase string) {
		if pct < lastPct-0.001 {
			t.Errorf("pct đi lùi: %v sau %v", pct, lastPct)
		}
		lastPct = pct
		phases[phase] = true
	})
	if err != nil {
		t.Fatalf("Extract lỗi: %v", err)
	}
	if lastPct < 100 {
		t.Errorf("pct cuối = %v, mong 100", lastPct)
	}
	if !phases["hoàn tất giải nén"] {
		t.Errorf("thiếu phase 'hoàn tất giải nén', có: %v", phases)
	}
	// Marker phải tồn tại và đúng version.
	marker, err := os.ReadFile(filepath.Join(res.Dir, "marker.txt"))
	if err != nil || string(marker) != markerVersion {
		t.Fatalf("marker sai: %v %q", err, string(marker))
	}
	// Cả 2 tệp phải tồn tại với kích thước đúng 2MB.
	for _, p := range []string{res.FFmpegPath, res.FFprobePath} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("thiếu %s: %v", p, err)
		}
		if st.Size() != 2<<20 {
			t.Errorf("kích thước %s = %d, mong %d", p, st.Size(), 2<<20)
		}
	}
	if !res.Extracted {
		t.Errorf("lần đầu chạy Extracted phải = true")
	}
}

func TestExtractResume(t *testing.T) {
	tmp := t.TempDir()
	buildFakeZip(t, tmp)
	fake, err := os.ReadFile(filepath.Join(tmp, "fake.zip"))
	if err != nil {
		t.Fatal(err)
	}
	swapEmbedded(t, fake)

	res1, err := Extract(nil)
	if err != nil {
		t.Fatal(err)
	}
	// Chạy lần 2: cả 2 tệp đều đã đủ kích thước → Extracted = false (không ghi lại).
	res2, err := Extract(nil)
	if err != nil {
		t.Fatal(err)
	}
	if res2.Extracted {
		t.Errorf("lần 2 không nên giải nén lại (resume), Extracted=%v dir=%s", res2.Extracted, res2.Dir)
	}
	if res1.Dir != res2.Dir {
		t.Errorf("Dir khác nhau giữa 2 lần: %s vs %s", res1.Dir, res2.Dir)
	}
}

func TestPrepareFastPath(t *testing.T) {
	// Prepare trên Linux: marker + tệp đủ lớn → Ready (không verify PE).
	tmp := t.TempDir()
	buildFakeZip(t, tmp)
	fake, err := os.ReadFile(filepath.Join(tmp, "fake.zip"))
	if err != nil {
		t.Fatal(err)
	}
	swapEmbedded(t, fake)

	if _, err := Extract(nil); err != nil {
		t.Fatal(err)
	}
	boot := Prepare()
	if !boot.Ready {
		t.Errorf("Prepare phải Ready sau Extract thành công, reason=%s", boot.Reason)
	}
	// Xoá ffprobe → Prepare phải yêu cầu giải nén lại.
	_ = os.Remove(boot.Result.FFprobePath)
	boot2 := Prepare()
	if boot2.Ready || !boot2.NeedsExtract {
		t.Errorf("sau khi mất ffprobe, Prepare phải NeedsExtract (ready=%v)", boot2.Ready)
	}
}
