package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"vkseditorpro/internal/applog"
	"vkseditorpro/internal/engine"
	"vkseditorpro/internal/ffmpegbin"
	"vkseditorpro/internal/project"
	"vkseditorpro/internal/selftest"
)

//go:embed all:frontend/dist
var assets embed.FS

// main — điểm vào của VKSeditorPro:
//  1. --version → in phiên bản rồi thoát.
//  2. --selftest → chuẩn bị FFmpeg, kiểm tra engine rồi thoát (không mở GUI).
//  3. --headless → render dự án .vksproj từ dòng lệnh không GUI (batch/CI).
//  4. Chạy ứng dụng Wails: cửa sổ native + giao diện web nhúng trong exe.
//
// v1.2.0 — quan trọng: GUI KHÔNG BAO GIỜ chờ giải nén FFmpeg.
// Lần đầu chạy, giải nén chạy NGAY TRONG app.startup và bắn sự kiện
// "ffinit" (progress %) + "ffcheck" (kết quả) cho giao diện — người dùng
// thấy cửa sổ mở ngay lập tức và thanh trạng thái FFmpeg chạy %.
func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("VKSeditorPro", AppVersion)
		return
	}

	// Log ứng dụng — bật sớm nhất có thể (không bao giờ làm sập app).
	applog.Init(dataDirOrFallback())

	// FFmpeg nhúng — chìa khoá "1 file .exe duy nhất".
	// MGO_FFMPEG / MGO_FFPROBE phải đặt TRƯỚC mọi lời gọi moviego
	// (thư viện cache kết quả lần đầu tra cứu — bắt buộc đặt sớm).
	setFFEnv := func(res *ffmpegbin.Result) {
		ff, fp := resolveFFBinaries(res)
		_ = os.Setenv("MGO_FFMPEG", ff)
		_ = os.Setenv("MGO_FFPROBE", fp)
	}

	// Chế độ CLI: chặn cho tới khi FFmpeg sẵn sàng (không cần cửa sổ nhanh).
	if len(os.Args) > 1 && os.Args[1] == "--selftest" {
		res, err := ffmpegbin.Ensure(func(f string, a ...any) {
			fmt.Printf("[VKSeditorPro] "+f+"\n", a...)
			applog.Logf(f, a...)
		})
		if err == nil {
			setFFEnv(res)
		} else {
			fmt.Fprintln(os.Stderr, "Không chuẩn bị được FFmpeg, thử ffmpeg hệ thống:", err)
		}
		fmt.Println("VKSeditorPro — chế độ tự kiểm tra engine (--selftest)")
		os.Exit(selftest.Run(os.Stdout))
	}
	if len(os.Args) > 1 && (os.Args[1] == "--headless" || os.Args[1] == "--render") {
		res, err := ffmpegbin.Ensure(func(f string, a ...any) {
			fmt.Printf("[VKSeditorPro] "+f+"\n", a...)
			applog.Logf(f, a...)
		})
		if err == nil {
			setFFEnv(res)
		} else {
			fmt.Fprintln(os.Stderr, "Không chuẩn bị được FFmpeg, thử ffmpeg hệ thống:", err)
		}
		os.Exit(runHeadless(os.Args[2:]))
	}

	// Chế độ GUI: chỉ Prepare NHANH — không bao giờ giải nén ở đây.
	boot := ffmpegbin.Prepare()
	if boot.Ready {
		setFFEnv(boot.Result)
		applog.Logf("FFmpeg nhúng sẵn sàng ngay: %s (v%s)", boot.Result.FFmpegPath, boot.Result.Version)
	} else {
		applog.Logf("FFmpeg cần giải nén khi app mở: %s", boot.Reason)
	}

	app, err := NewApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Khởi tạo thất bại: %v\n", err)
		os.Exit(1)
	}
	app.ffBoot = boot

	handler := newLocalFileHandler(app.dataDir)
	// v1.2.3: Source Monitor — tra tư liệu gốc theo ID (đọc khoá RLock của doc).
	handler.SetSourceResolver(func(id string) (string, error) {
		app.mu.RLock()
		defer app.mu.RUnlock()
		as := app.doc.FindAsset(id)
		if as == nil || as.Missing || as.Path == "" {
			return "", fmt.Errorf("tư liệu không khả dụng")
		}
		return as.Path, nil
	})

	err = wails.Run(&options.App{
		Title:     "VKSeditorPro — Trình dựng video chuyên nghiệp",
		Width:     1440,
		Height:    900,
		MinWidth:  1150,
		MinHeight: 720,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: handler, // /local/thumbs, /local/frames, /local/preview...
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: false,
			CSSDropProperty:    "--wails-drop-target",
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "vkseditorpro-9f3c1a2e-single-instance",
			OnSecondInstanceLaunch: app.onSecondInstance,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// dataDirOrFallback trả thư mục dữ liệu mặc định (không lỗi).
func dataDirOrFallback() string {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	d := filepath.Join(cfg, "VKSeditorPro")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// runHeadless render dự án .vksproj từ dòng lệnh:
//
//	VKSeditorPro.exe --headless --project=video.vksproj --out=ketqua.mp4
//	    [--height=1080] [--crf=20] [--preset=medium] [--gif --giffps=12]
//	    [--alpha] [--gpu=nvenc|qsv] [--quiet]
//
// Trả 0 khi thành công, 1 khi lỗi. Tiến trình in ra stdout dạng "PROGRESS n/t".
func runHeadless(args []string) int {
	var (
		projectPath, outPath string
		height, crf, giffps  int
		preset, gpu          string
		gif, alpha, quiet    bool
	)
	height, crf = 0, 20
	for _, arg := range args {
		key, val := splitArg(arg)
		switch key {
		case "--project":
			projectPath = val
		case "--out":
			outPath = val
		case "--height":
			fmt.Sscanf(val, "%d", &height)
		case "--crf":
			fmt.Sscanf(val, "%d", &crf)
		case "--preset":
			preset = val
		case "--giffps":
			fmt.Sscanf(val, "%d", &giffps)
		case "--gif":
			gif = true
		case "--alpha":
			alpha = true
		case "--gpu":
			gpu = val
		case "--quiet":
			quiet = true
		default:
			fmt.Fprintf(os.Stderr, "Cờ không hiểu: %s\n", arg)
		}
	}
	if projectPath == "" || outPath == "" {
		fmt.Fprintln(os.Stderr, "Thiếu --project=<file.vksproj> hoặc --out=<file.mp4>")
		fmt.Fprintln(os.Stderr, "Ví dụ: VKSeditorPro.exe --headless --project=demo.vksproj --out=demo.mp4 --height=1080")
		return 1
	}
	b, err := os.ReadFile(projectPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Không đọc được dự án: %v\n", err)
		return 1
	}
	doc, err := project.LoadFromBytes(b)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Dự án không hợp lệ: %v\n", err)
		return 1
	}
	g, err := engine.Compile(doc, engine.CompileOptions{TargetHeight: height})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Biên dịch thất bại: %v\n", err)
		return 1
	}
	defer g.Close()
	req := engine.ExportRequest{OutputPath: outPath, CRF: crf, Preset: preset, GifFPS: giffps, HWAccel: gpu}
	switch {
	case gif:
		req.Format = engine.FormatGIF
	case alpha:
		req.Format = engine.FormatAlpha
	default:
		req.Format = engine.FormatMP4
	}
	if !quiet {
		fmt.Printf("[VKSeditorPro] Dựng %s → %s (%d clip, %.1fs)\n",
			doc.Name, outPath, len(doc.Clips), float64(doc.TimelineTotalMs())/1000)
	}
	err = engine.WriteVideo(context.Background(), g, req, func(done, total int) {
		if !quiet && total > 0 {
			fmt.Printf("PROGRESS %d/%d\n", done, total)
		}
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Xuất thất bại: %v\n", err)
		return 1
	}
	if st, err := os.Stat(outPath); err == nil {
		fmt.Printf("DONE %s (%.1f MB)\n", outPath, float64(st.Size())/1024/1024)
	}
	return 0
}

// splitArg tách "--key=value".
func splitArg(arg string) (string, string) {
	for i := 0; i < len(arg); i++ {
		if arg[i] == '=' {
			return arg[:i], arg[i+1:]
		}
	}
	return arg, ""
}

// resolveFFBinaries chọn cặp ffmpeg/ffprobe thực thi được: bản nhúng trên
// Windows, hoặc ffmpeg hệ thống khi bản nhúng là PE (chạy trên Linux/macOS).
func resolveFFBinaries(res *ffmpegbin.Result) (string, string) {
	ff, fp := res.FFmpegPath, res.FFprobePath
	if runtime.GOOS == "windows" || !isPEFile(ff) {
		return ff, fp
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		ff = p
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		fp = p
	}
	return ff, fp
}

// isPEFile kiểm tra 2 byte đầu là "MZ" (định dạng PE của Windows).
func isPEFile(p string) bool {
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	b := make([]byte, 2)
	if _, err := f.Read(b); err != nil {
		return false
	}
	return b[0] == 'M' && b[1] == 'Z'
}
