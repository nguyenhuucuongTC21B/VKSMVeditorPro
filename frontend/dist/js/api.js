/* api.js — lớp bọc gọi backend Wails + quản lý sự kiện.
   Backend được bind qua window.go.main.App (Wails tự sinh lúc chạy). */
(function () {
    "use strict";

    const backend = () => (window.go && window.go.main && window.go.main.App) || null;

    function ensureBackend() {
        if (!backend()) {
            console.warn("[VKS] Backend chưa sẵn sàng — đang chạy ngoài môi trường Wails?");
        }
        return backend();
    }

    /** Gọi method backend; ném lỗi tiếng Việt đã định dạng. */
    async function call(method, ...args) {
        const b = ensureBackend();
        if (!b || typeof b[method] !== "function") {
            throw new Error("Không kết nối được backend (method: " + method + ")");
        }
        try {
            return await b[method](...args);
        } catch (e) {
            let msg = (e && e.message) || String(e);
            // Wails trả lỗi dạng chuỗi đã format từ Go error.
            if (msg.includes(": ")) {
                // Giữ phần thông điệp cuối cho gọn.
            }
            throw new Error(msg);
        }
    }

    /** Đăng ký sự kiện từ backend. */
    function onEvent(name, cb) {
        if (window.runtime && typeof window.runtime.EventsOn === "function") {
            window.runtime.EventsOn(name, cb);
        }
    }

    /** Định dạng ms thành mm:ss.d */
    function fmtMs(ms) {
        if (ms < 0 || ms == null || !isFinite(ms)) ms = 0;
        const m = Math.floor(ms / 60000);
        const s = Math.floor((ms % 60000) / 1000);
        const d = Math.floor((ms % 1000) / 100);
        return String(m).padStart(2, "0") + ":" + String(s).padStart(2, "0") + "." + d;
    }

    function fmtBytes(b) {
        if (!b) return "0 B";
        const u = ["B", "KB", "MB", "GB"];
        let i = 0;
        while (b >= 1024 && i < u.length - 1) { b /= 1024; i++; }
        return b.toFixed(i ? 1 : 0) + " " + u[i];
    }

    function fmtFps(rate) {
        if (!rate || !rate.den) return "?";
        return (rate.num / rate.den).toFixed(2).replace(/\.?0+$/, "") || "?";
    }

    window.VKSApi = { call, onEvent, fmtMs, fmtBytes, fmtFps };
})();
