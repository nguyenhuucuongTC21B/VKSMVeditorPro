════════════════════════════════════════════════════════════
  VKSeditorPro v1.2.3  —  Trình dựng video chuyên nghiệp
  (một tệp exe duy nhất)
════════════════════════════════════════════════════════════

CÁCH DÙNG
─────────
1. Giải nén ZIP, được VKSeditorPro.exe + README.txt
2. Chạy VKSeditorPro.exe (không cần cài đặt, không cần quyền admin)
3. Bấm "＋ Nhập tư liệu" hoặc kéo–thả video/ảnh/âm thanh vào cửa sổ

KHÔNG CẦN CÀI ĐẶT — KHÔNG CẦN INTERNET — FFmpeg đã nằm sẵn trong exe.

TÍNH NĂNG (kế thừa đầy đủ v1.1)
───────────────────────────────
• Thư viện tư liệu: video / ảnh / âm thanh, tự tạo thumbnail
• Timeline kéo–thả: cắt, tách, di chuyển, xén In/Out
• Chuyển cảnh: crossfade, dissolve, qua đen, trượt, đẩy, iris…
• Lớp phủ: chữ động, hình vẽ, video PiP, tách nền xanh
• Phụ đề SRT nhập/xuất + đốt trực tiếp; watermark; timecode; credits
• v1.1: tốc độ 0.25–4x, reverse, boomerang, lặp, freeze, fade audio,
  hiệu ứng chỉnh màu, LUT .cube, keyframes scale/opacity
• Công cụ: TTS giọng Windows, cắt khoảng lặng tự động, QC tự động,
  phát hiện cảnh, AI (STT/dịch/phân tích — cần API key), proxy 480p,
  FastCut không re-encode, nối nhanh
• Xuất MP4 / GIF / MOV alpha; GPU (NVENC/QSV); render headless bằng dòng lệnh:
  VKSeditorPro.exe --headless --project=duan.vksproj --out=ketqua.mp4

MỚI TRONG v1.2.3 — HOÀN THIỆN WORKSPACE + CÔNG CỤ DỰNG CHUẨN PRO
────────────────────────────────────────────────────────────────
CÔNG CỤ TIMELINE:
• 🗡 Dao cắt (phím C): bấm vào clip là cắt đúng chỗ bấm; Ctrl+B = cắt tại con
  trỏ; V = về công cụ chọn. Alt + kéo thân clip = SLIP (đổi khung bên trong,
  giữ vị trí + độ dài).
• 🧲 Nam châm: viền clip tự dính con trỏ/viền clip khác/ranh giây khi kéo.
• 🔒 Khóa timeline chặn sửa nhầm · 🔇 tắt tiếng toàn bộ · 👁 ẩn/hiện lớp phủ.
• Phím I/O đặt Đầu/Cuối clip tại con trỏ · Shift+I/O nhảy tới Đầu/Cuối clip.
• ⏮ ⏭ về đầu/cuối timeline. Space = phát/dừng (chế độ Khung: phát khung
  từng bước; chế độ Bản render: phát video).

THUỘC TÍNH CLIP MỚI:
• Vị trí ngang/dọc (pan khung khi zoom > 1×) — nền tảng Auto Reframe.
• Độ mờ đục (Opacity) — đã sửa lỗi tiềm ẩn v1.1: fade + xuất MP4.
• Âm lượng hiển thị dB · Lọc tạp âm tiếng (afftdn) · Làm sạch giọng nói.
• Tông màu vùng Tối/Trung/Sáng (bánh xe màu rút gọn) · 🎨 Khớp màu tự động
  theo clip chuẩn · ⧉ Áp hiệu ứng cho TẤT CẢ clip.

TÍNH NĂNG MỚI:
• 🔗 Tách âm thanh (Ctrl+L): xuất tiếng của clip thành tư liệu riêng.
• Click đôi tư liệu = xem tệp gốc (Source Monitor).
• 📱 Đổi khung dự án 16:9 / 9:16 TikTok / 1:1 / 4:5 + Auto Reframe căn giữa.
• Đã sửa lỗi tiềm ẩn v1.1: clip có fade in/out (keyframe) + xuất MP4 bị
  lỗi alpha — giờ tự ép xuống nền đặc, xuất MP4 luôn thành công.

MỚI TRONG v1.2.2 — SỬA LỖI "KHUNG XEM TRƯỚC / THUMBNAIL KHÔNG HIỆN"
────────────────────────────────────────────────────────────────
Timeline đã hiện đúng (v1.2.1) nhưng khung xem trước giữa màn hình
vẫn gãy trên Windows. Nguyên nhân: bộ phục vụ tệp nội bộ của app xử
lý đường dẫn URL bằng hàm tạo đường dẫn của Windows (dấu "\"), làm
mọi yêu cầu /local/... bị từ chối 403 — chỉ lỗi trên Windows.
v1.2.2:
• Sửa triệt để: đường dẫn URL giờ xử lý theo chuẩn "/" thuần, độc lập
  hệ điều hành → khung xem trước, thumbnail tư liệu, video xem trước
  hoạt động bình thường trên Windows.
• Thêm 4 unit test chốt chặn hồi quy cho bộ phục vụ tệp (phục vụ đúng
  thư mục trắng, chặn lộ tệp ngoài whitelist, chống ../).

MỚI TRONG v1.2.1 — SỬA LỖI "TIMELINE TRỐNG, KHÔNG XEM TRƯỚC ĐƯỢC"
────────────────────────────────────────────────────────────────
Nguyên nhân: dự án cũ do phiên bản trước ghi vào %APPDATA%\VKSeditorPro
chứa dữ liệu lệch chuẩn (clip rác 0 giây / trỏ tới tệp không có / mảng null),
khi nạp lại làm timeline trống và xem trước bị hỏng. v1.2.1:
• Tự sửa clip rác khi nạp: gán lại thời lượng hợp lệ, bỏ clip trỏ tới
  tư liệu không tồn tại — log ghi rõ "Tự sửa dự án khi nạp: ...".
• Phát hiện tệp dự án định dạng lạ -> chuyển thành .foreign.bak và tạo
  dự án mới sạch (không bao giờ kẹt lại lần nữa).
• Nhập tệp TRÙNG không còn im lặng: báo "đã có trong thư viện" hoặc tự
  làm mới tư liệu cũ thiếu dữ liệu.
• Giao diện chống chết: dữ liệu lạ không thể làm tắt canvas timeline.
• Ghi log đầy đủ mọi thao tác nhập/thêm clip/xem khung — nút 🧯 sẽ thấy.
• Nếu vẫn thấy trống: bấm "Mới" trên thanh công cụ -> chọn KHÔNG giữ
  thư viện -> nhập lại video -> ＋ Timeline.

MỚI TRONG v1.2.0 — KHẮC PHỤC TRIỆT ĐỂ LỖI "ĐANG XỬ LÝ"
────────────────────────────────────────────────────────
• Cửa sổ mở NGAY LẬP TỨC: lần đầu chạy, FFmpeg được giải nén NGAY TRONG
  ứng dụng với thanh tiến trình % trên thanh công cụ — không còn cảnh
  chờ nhiều phút mà màn hình im lặng.
• Tiến trình giải nén có khả năng PHỤC HỒI (resume): nếu bị gián đoạn
  (tắt máy, mất điện), lần sau chạy tiếp từ nơi đã giải nén.
• Sau khi giải nén, ứng dụng tự KIỂM TRA ffmpeg -version với giới hạn
  30 giây — nếu diệt virus chặn sẽ có thông báo hướng dẫn cụ thể ngay.
• Nhật ký tự động: %APPDATA%\VKSeditorPro\log.txt + nút 🧯 CHẨN ĐOÁN
  mới trên thanh công cụ (xem log, sao chép, mở thư mục dữ liệu).
• Mọi thao tác dùng FFmpeg (nhập, xem trước, xuất, công cụ) đều chờ
  FFmpeg sẵn sàng — không còn lỗi "bí ẩn" khi nhập quá sớm.
• Mọi tiến trình công cụ đều có giới hạn thời gian (timeout).

NẾU GẶP LỖI
───────────
• Khởi động chậm lần đầu: bình thường khi giải nén FFmpeg (~200MB) —
  chỉ xảy ra ĐÚNG MỘT LẦN; các lần mở sau khởi động tức thì.
• Diệt virus cảnh báo/chặn: thêm ngoại lệ cho:
    - Thư mục chứa VKSeditorPro.exe
    - Thư mục cache: %LOCALAPPDATA%\VKSeditorPro
    - Thư mục dữ liệu: %APPDATA%\VKSeditorPro
• Xem log: nút 🧯 trên thanh công cụ → "Sao chép" → gửi người hỗ trợ.

Yêu cầu: Windows 7 SP1 → 11 (64-bit). RAM 4GB+.
Phiên bản: 1.2.0 — nền Wails (Go) + FFmpeg 8.0 nhúng.
