/* boot.js — lớp chống treo phía giao diện (từ v1.2.0, kế thừa v1.0.5).
   Nhiệm vụ:
   1. Hiện tiến trình giải nén FFmpeg lần đầu chạy (sự kiện "ffinit").
   2. Báo kết quả FFmpeg (sự kiện "ffcheck") — lỗi thì mở hộp thoại hướng dẫn.
   3. Watchdog: backend/chrome không phản hồi → cảnh báo thay vì treo im lặng.
   4. Nút 🧯 Chẩn đoán: xem + sao chép nhật ký, mở thư mục dữ liệu. */
(function () {
    "use strict";

    const $ = id => document.getElementById(id);

    /* ─────────── CSS tự tiêm — không đụng file css gốc ─────────── */
    const style = document.createElement("style");
    style.textContent = `
    .vks-ff-chip { font-size:12px; padding:3px 10px; border-radius:999px; border:1px solid #3a3f4d; color:#b7bdcc; white-space:nowrap; }
    .vks-ff-chip.ok { color:#2ecc71; border-color:rgba(46,204,113,.45); }
    .vks-ff-chip.progress { color:#f5b93e; border-color:rgba(245,185,62,.4); }
    .vks-ff-chip.fail { color:#ff5c5c; border-color:rgba(255,92,92,.5); cursor:pointer; }
    .vks-modal-mask { position:fixed; inset:0; background:rgba(0,0,0,.6); display:flex; align-items:center; justify-content:center; z-index:9999; }
    .vks-modal { background:#1a1d24; border:1px solid #343a49; border-radius:12px; padding:18px 20px; width:640px; max-width:94vw; max-height:84vh; overflow:auto; color:#e8eaf0; font-size:13.5px; line-height:1.6; }
    .vks-modal h3 { margin:0 0 10px; font-size:16px; }
    .vks-modal pre { background:#12141a; border:1px solid #2c313e; border-radius:8px; padding:10px; max-height:320px; overflow:auto; font-size:11.5px; white-space:pre-wrap; word-break:break-all; }
    .vks-modal .vks-actions { display:flex; gap:8px; margin-top:12px; }
    .vks-modal button { background:#262b36; border:1px solid #3a4150; color:#e8eaf0; border-radius:8px; padding:7px 14px; cursor:pointer; font-size:13px; }
    .vks-modal button.primary { background:#4f8cff; border-color:#4f8cff; color:#fff; font-weight:600; }
    .vks-modal ol { padding-left:20px; }
    .vks-modal ol li { margin:6px 0; }
    `;
    document.head.appendChild(style);

    /* ─────────── Chip trạng thái FFmpeg trên toolbar ─────────── */
    let chip = document.createElement("span");
    chip.className = "vks-ff-chip progress";
    chip.textContent = "FFmpeg: đang khởi tạo…";
    const anchor = $("job-status");
    if (anchor && anchor.parentNode) anchor.parentNode.insertBefore(chip, anchor);
    else document.body.appendChild(chip);

    let ffResolved = false;

    function setChip(kind, text, clickable) {
        chip.className = "vks-ff-chip " + kind;
        chip.textContent = text;
        chip.onclick = clickable || null;
    }

    /* ─────────── Đợi runtime Wails rồi đăng ký sự kiện ─────────── */
    function onEvent(name, cb) {
        const tryOn = () => {
            if (window.runtime && typeof window.runtime.EventsOn === "function") {
                window.runtime.EventsOn(name, cb);
                return true;
            }
            return false;
        };
        if (tryOn()) return;
        const t0 = Date.now();
        const timer = setInterval(() => {
            if (tryOn() || Date.now() - t0 > 30000) clearInterval(timer);
        }, 100);
    }

    /* ─────────── Hộp thoại hướng dẫn khi FFmpeg lỗi ─────────── */
    function showFFGuidance(detail) {
        const mask = document.createElement("div");
        mask.className = "vks-modal-mask";
        const steps = [
            "Ứng dụng không khởi tạo được FFmpeg nhúng. Cách xử lý:",
            "",
            "1) Thêm ngoại lệ cho phần mềm diệt virus (Windows Security → Virus & threat protection → Exclusions):",
            "    • Thư mục chứa VKSeditorPro.exe (toàn bộ thư mục ứng dụng)",
            "    • Thư mục cache: %LOCALAPPDATA%\\VKSeditorPro",
            "2) Kiểm tra ffmpeg.exe / ffprobe.exe còn tồn tại trong %LOCALAPPDATA%\\VKSeditorPro\\bin",
            "   (diệt virus có thể đã cách ly — khôi phục khỏi Quarantine rồi thêm ngoại lệ).",
            "3) Đóng ứng dụng, xoá tệp log.txt trong %APPDATA%\\VKSeditorPro rồi mở lại để tạo log mới.",
            "4) Vẫn lỗi: bấm 🧯 Chẩn đoán → Sao chép → gửi nội dung cho người hỗ trợ."
        ].join("\n");
        mask.innerHTML = '<div class="vks-modal"><h3>⚠ Không khởi tạo được FFmpeg</h3>' +
            '<div style="white-space:pre-wrap">' + steps.replace(/</g, "&lt;") + '</div>' +
            (detail ? '<div style="margin-top:8px;color:#ff9c9c;word-break:break-all">Chi tiết: ' + String(detail).replace(/</g, "&lt;") + '</div>' : '') +
            '<div class="vks-actions"><button data-a="copy">📋 Sao chép hướng dẫn</button><span style="flex:1"></span>' +
            '<button data-a="close" class="primary">Đã hiểu</button></div></div>';
        mask.addEventListener("click", (e) => {
            const act = e.target && e.target.dataset ? e.target.dataset.a : null;
            if (act === "close" || e.target === mask) mask.remove();
            if (act === "copy") {
                const text = steps + (detail ? "\n\nChi tiết: " + detail : "");
                if (navigator.clipboard) navigator.clipboard.writeText(text);
            }
        });
        document.body.appendChild(mask);
    }

    /* ─────────── Sự kiện từ backend ─────────── */
    onEvent("ffinit", (d) => {
        if (ffResolved || !d) return;
        const pct = Math.max(0, Math.min(100, Math.round(d.pct || 0)));
        setChip("progress", "FFmpeg: " + (d.phase || "đang giải nén") + "… " + pct + "%");
    });

    onEvent("ffcheck", (d) => {
        if (!d) return;
        ffResolved = true;
        if (d.ok) {
            const extra = d.extracted ? "" : "";
            setChip("ok", "FFmpeg sẵn sàng" + (d.version ? " (v" + d.version + ")" : "") + extra);
        } else {
            setChip("fail", "FFmpeg lỗi — bấm để xem hướng dẫn", () => showFFGuidance(d.error));
            showFFGuidance(d.error);
        }
    });

    /* ─────────── Watchdog: cảnh báo thay vì treo im lặng ─────────── */
    setTimeout(() => {
        if (!ffResolved) setChip("progress", "FFmpeg: chưa phản hồi sau 20s — vẫn đang thử…");
    }, 20000);
    setTimeout(() => {
        if (!ffResolved) showFFGuidance("Không nhận được kết quả ffcheck sau 90 giây.");
    }, 90000);

    /* ─────────── Nút 🧯 Chẩn đoán + hộp thoại ─────────── */
    let lastDiag = null; // v1.2.6: nhớ báo cáo gần nhất — "Mở thư mục" không phải gọi lại
    function openDiag() {
        const b = window.go && window.go.main && window.go.main.App;
        const mask = document.createElement("div");
        mask.className = "vks-modal-mask";
        mask.innerHTML = '<div class="vks-modal"><h3>🧯 Chẩn đoán hệ thống</h3>' +
            '<pre id="vks-diag-pre">Đang tải…</pre>' +
            '<div class="vks-actions"><button data-a="copy">📋 Sao chép</button>' +
            '<button data-a="folder">📂 Mở thư mục dữ liệu</button><span style="flex:1"></span>' +
            '<button data-a="close" class="primary">Đóng</button></div></div>';
        mask.addEventListener("click", async (e) => {
            const act = e.target && e.target.dataset ? e.target.dataset.a : null;
            if (act === "close" || e.target === mask) { mask.remove(); return; }
            if (act === "copy") {
                const text = $("vks-diag-pre") ? $("vks-diag-pre").textContent : "";
                if (navigator.clipboard && text) {
                    try { await navigator.clipboard.writeText(text); } catch (_) { /* bỏ qua */ }
                }
            }
            if (act === "folder") {
                try {
                    let dir = (lastDiag && lastDiag.dataDir) || "";
                    if (!dir && b && typeof b.GetDiagnostics === "function") {
                        lastDiag = await b.GetDiagnostics();
                        dir = lastDiag.dataDir || "";
                    }
                    if (b && typeof b.OpenInFolder === "function" && dir) await b.OpenInFolder(dir);
                } catch (err) { /* bỏ qua */ }
            }
        });
        document.body.appendChild(mask);
        const pre = mask.querySelector("#vks-diag-pre");
        if (!b || typeof b.GetDiagnostics !== "function") {
            pre.textContent = "Backend chưa sẵn sàng hoặc phiên bản ứng dụng không có GetDiagnostics.";
            return;
        }
        // v1.2.6: nếu backend trả chậm hơn 4s (đang bận với tác vụ lớn) thì báo
        // rõ thay vì treo "Đang tải…" mãi mãi — kết quả đến sau vẫn được điền.
        const slowTimer = setTimeout(() => {
            if (pre.isConnected && pre.textContent.startsWith("Đang tải")) {
                pre.textContent = "Hệ thống đang bận (có render/xuất video đang chạy?) — vẫn đang chờ dữ liệu…\nNếu màn này kéo dài quá 30 giây: đóng bảng, đợi tác vụ chạy xong rồi mở lại.";
            }
        }, 4000);
        b.GetDiagnostics().then((d) => {
            clearTimeout(slowTimer);
            lastDiag = d;
            if (!pre.isConnected) return;
            const L = [];
            L.push("Phiên bản ứng dụng : " + d.version);
            L.push("Hệ điều hành       : " + d.os + " / " + d.arch);
            L.push("Thư mục dữ liệu    : " + d.dataDir);
            L.push("FFmpeg             : " + (d.ffmpegOk ? "CÓ (" + Math.round(d.ffmpegSize / 1048576) + " MB)" : "THIẾU") + " — " + d.ffmpegPath);
            L.push("FFprobe            : " + (d.ffprobeOk ? "CÓ (" + Math.round(d.ffprobeSize / 1048576) + " MB)" : "THIẾU") + " — " + d.ffprobePath);
            L.push("Phiên bản FFmpeg   : " + (d.ffVersion || "(chưa xác minh)"));
            L.push("Trạng thái FFmpeg  : " + (d.ffReady ? "sẵn sàng" : "đang khởi tạo/lỗi"));
            L.push("Tư liệu / clip     : " + d.assets + " / " + d.clips);
            L.push("Tác vụ đang chạy   : " + (d.currentJob || "(không)"));
            L.push("Log                : " + d.logPath);
            L.push("");
            L.push("----- 120 dòng log cuối -----");
            L.push(d.logTail || "(trống)");
            pre.textContent = L.join("\n");
        }).catch((e) => { clearTimeout(slowTimer); pre.textContent = "Không lấy được chẩn đoán: " + e; });
    }

    function injectDiagButton() {
        if ($("btn-vks-diag") || !$("btn-selftest")) return;
        const b = document.createElement("button");
        b.id = "btn-vks-diag";
        b.className = "tb-btn ghost";
        b.title = "Xem nhật ký & chẩn đoán hệ thống";
        b.textContent = "🧯";
        b.addEventListener("click", openDiag);
        $("btn-selftest").parentNode.insertBefore(b, $("btn-selftest"));
    }

    if (document.readyState === "loading") {
        document.addEventListener("DOMContentLoaded", injectDiagButton);
    } else {
        injectDiagButton();
    }
})();
