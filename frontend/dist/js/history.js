/* history.js — v1.2.5 Hoàn tác / Làm lại (Undo / Redo).
   Cách hoạt động:
   · Bọc VKSApi.call: với CÁC METHOD LÀM THAY ĐỔI tài liệu, trước khi gọi
     thành công thì giữ lại snapshot JSON của Document hiện tại vào stack Undo
     (gọi lỗi thì bỏ — không tồn bước để hoàn tác).
   · Ctrl+Z: đẩy trạng thái hiện tại vào Redo, gửi bản snapshot cũ về backend
     qua SetDocument (backend tự lưu đĩa) rồi nạp lại cho giao diện.
   · Mới dự án / Mở dự án = xoá sạch lịch sử (không hoàn tác xuyên dự án).
   · Khi timeline đang KHÓA (🔒) mọi sửa đổi — kể cả hoàn tác — bị chặn. */
(function () {
    "use strict";

    const State = window.VKSState;
    const MAX_STEPS = 60;

    // Các method backend làm thay đổi Document → có bước hoàn tác.
    const MUTATING = new Set([
        "AddClip", "AddAssets", "ImportMediaDialog", "RemoveAsset",
        "UpdateClip", "MoveClip", "RemoveClip", "SplitClip", "DuplicateClip",
        "SetCanvas", "SetTransition", "SetDocFlags", "DetachAudio", "ColorMatch",
        "AddOverlay", "AddMediaOverlay", "UpdateOverlay", "RemoveOverlay",
        "AddAudioClip", "UpdateAudioClip", "RemoveAudioClip", "DuplicateAudioClip", // v1.3.0: track A1
        "SetSubtitleStyle", "ClearSubtitles", "SetWatermark", "SetTimecode",
        "SetCredits", "BuildFromScript", "ApplySilenceCut", "TTSGenerate",
        "AITranscribe", "AITranslateSubs",
    ]);
    // Method thay thế TOÀN BỘ dự án → xoá lịch sử sau khi gọi xong.
    const RESETS = new Set(["NewProject", "OpenProjectDialog"]);

    const undoStack = [];
    const redoStack = [];

    const locked = () => !!(window.VKSTool && window.VKSTool.locked);

    function snapshot() {
        try { return JSON.parse(JSON.stringify(State.doc)); }
        catch (_) { return null; }
    }

    // ───── Toast riêng (không phụ thuộc app.js) ─────
    function toast(msg, kind) {
        const box = document.createElement("div");
        box.className = "toast " + (kind || "");
        box.textContent = msg;
        const host = document.getElementById("toasts");
        if (host) host.appendChild(box);
        setTimeout(() => {
            box.style.transition = "opacity .3s";
            box.style.opacity = "0";
            setTimeout(() => box.remove(), 320);
        }, kind === "err" ? 3600 : 2200);
    }

    function refreshDoc() {
        return window.VKSApi.call("GetState")
            .then(s => State.setDoc(s.doc))
            .catch(() => {});
    }

    function updateButtons() {
        const bu = document.getElementById("btn-undo");
        const br = document.getElementById("btn-redo");
        if (bu) { bu.disabled = undoStack.length === 0; bu.style.opacity = undoStack.length ? "" : ".45"; }
        if (br) { br.disabled = redoStack.length === 0; br.style.opacity = redoStack.length ? "" : ".45"; }
    }

    function push(snap) {
        if (!snap) return;
        undoStack.push(snap);
        if (undoStack.length > MAX_STEPS) undoStack.shift();
        redoStack.length = 0;
        updateButtons();
    }

    function undo() {
        if (locked()) { toast("🔒 Timeline đang KHÓA — bấm 🔒 để mở khóa trước", "err"); return; }
        if (!undoStack.length) { toast("Không còn bước nào để hoàn tác"); return; }
        const cur = snapshot();
        const prev = undoStack.pop();
        redoStack.push(cur);
        window.VKSApi.call("SetDocument", prev)
            .then(() => refreshDoc())
            .then(() => { updateButtons(); toast("↶ Đã hoàn tác", "ok"); })
            .catch(e => { // hoàn tác thất bại — hoàn lại stack cho đúng
                undoStack.push(prev);
                redoStack.pop();
                updateButtons();
                toast("Hoàn tác lỗi: " + (e.message || e), "err");
            });
    }

    function redo() {
        if (locked()) { toast("🔒 Timeline đang KHÓA — bấm 🔒 để mở khóa trước", "err"); return; }
        if (!redoStack.length) { toast("Không có bước nào để làm lại"); return; }
        const cur = snapshot();
        const next = redoStack.pop();
        undoStack.push(cur);
        window.VKSApi.call("SetDocument", next)
            .then(() => refreshDoc())
            .then(() => { updateButtons(); toast("↷ Đã làm lại", "ok"); })
            .catch(e => {
                undoStack.pop();
                redoStack.push(next);
                updateButtons();
                toast("Làm lại lỗi: " + (e.message || e), "err");
            });
    }

    // ───── Bọc VKSApi.call một lần duy nhất ─────
    if (!window.VKSApi.__historyWrapped) {
        const rawCall = window.VKSApi.call.bind(window.VKSApi);
        window.VKSApi.call = function (method, ...args) {
            if (RESETS.has(method)) {
                return rawCall(method, ...args).then(res => {
                    undoStack.length = 0;
                    redoStack.length = 0;
                    updateButtons();
                    return res;
                });
            }
            if (!MUTATING.has(method)) return rawCall(method, ...args);
            if (locked()) {
                return Promise.reject(new Error("🔒 Timeline đang KHÓA — bấm 🔒 trên thanh timeline để mở khóa"));
            }
            const snap = snapshot();
            return rawCall(method, ...args).then(res => { push(snap); return res; });
        };
        window.VKSApi.__historyWrapped = true;
    }

    // ───── Nút + phím ─────
    const bu = document.getElementById("btn-undo");
    const br = document.getElementById("btn-redo");
    if (bu) bu.addEventListener("click", undo);
    if (br) br.addEventListener("click", redo);
    updateButtons();

    document.addEventListener("keydown", e => {
        if (e.target.tagName === "INPUT" || e.target.tagName === "SELECT" || e.target.tagName === "TEXTAREA") return;
        if (!(e.ctrlKey || e.metaKey)) return;
        const k = (e.key || "").toLowerCase();
        if (k === "z" && !e.shiftKey) { e.preventDefault(); undo(); }
        else if (k === "y" || (k === "z" && e.shiftKey)) { e.preventDefault(); redo(); }
    });

    window.VKSHistory = { undo, redo, clear: () => { undoStack.length = 0; redoStack.length = 0; updateButtons(); } };
})();
