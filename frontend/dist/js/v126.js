/* v126.js — v1.2.6: rà soát & bổ sung tính năng.
   1. 🎬 Thư viện chuyển cảnh (gallery 17 hiệu ứng + áp cho mọi mối nối + gỡ hết).
   2. Menu chuột phải trên timeline: tách / nhân bản / xoá / dời / tách âm thanh…
   3. Nhân bản clip (DuplicateClip — Ctrl+D). */
(function () {
    "use strict";

    const { call, fmtMs } = window.VKSApi;
    const State = window.VKSState;
    const $ = id => document.getElementById(id);

    function toast(msg, kind) { if (window.VKSApp) window.VKSApp.toast(msg, kind); }
    function refresh() { call("GetState").then(s => State.setDoc(s.doc)).catch(() => {}); }

    // ═══════════════ 1. THƯ VIỆN CHUYỂN CẢNH ═══════════════
    const transModal = $("trans-modal");
    let lastType = "crossfade";

    const ICONS = {
        crossfade: "🌗", dissolve: "✨", fadethroughblack: "🌑",
        wipeleft: "⬅️", wiperight: "➡️", wipeup: "⬆️", wipedown: "⬇️",
        slideleft: "🎞️", slideright: "🎞️", slideup: "🎞️", slidedown: "🎞️",
        pushleft: "🚚", pushright: "🚚", pushup: "🚚", pushdown: "🚚",
        irisopen: "🔮", irisclose: "🎯",
    };

    async function openTransModal() {
        transModal.hidden = false;
        await buildGallery();
        renderCurrent();
    }

    async function buildGallery() {
        const box = $("trans-gallery");
        box.innerHTML = '<span class="tl-dim">Đang tải danh sách hiệu ứng…</span>';
        let list = [];
        try { list = await call("ListTransitions") || []; } catch (e) { box.textContent = "Lỗi: " + e.message; return; }
        box.innerHTML = "";
        for (const t of list) {
            if (t.type === "none") continue; // "không có" không cần thẻ — gỡ bằng Gỡ hết / dropdown
            const card = document.createElement("div");
            card.className = "trans-card" + (t.type === lastType ? " active" : "");
            card.innerHTML = '<div class="tc-ico">' + (ICONS[t.type] || "🎬") + '</div>' +
                '<div class="tc-name">' + t.name + '</div>';
            card.addEventListener("click", () => applyTransition(t.type, t.name, false));
            card.dataset.type = t.type;
            box.appendChild(card);
        }
    }

    function transDurMs() { return Number($("trans-dur").value) || 800; }

    async function applyTransition(type, name, silent) {
        const c = State.selection ? State.findClip(State.selection) : null;
        if (!c) {
            toast("Hãy CHỌN MỘT CLIP trên timeline trước — hiệu ứng sẽ gắn vào mối nối sau clip đó", "err");
            return;
        }
        const idx = State.doc.clips.findIndex(x => x.id === c.id);
        if (idx >= State.doc.clips.length - 1) {
            toast("Đây là clip CUỐI — chuyển cảnh chỉ gắn giữa 2 clip. Hãy chọn clip đứng trước mối nối.", "err");
            return;
        }
        try {
            await call("SetTransition", c.id, type, transDurMs());
            lastType = type;
            document.querySelectorAll(".trans-card").forEach(k => k.classList.toggle("active", k.dataset.type === type));
            refresh();
            renderCurrent();
            if (!silent) toast("🎬 Đã gắn «" + name + "» (" + (transDurMs() / 1000) + "s) vào mối nối sau «" + (State.findAsset(c.assetId) || {}).name + "»", "ok");
        } catch (e) { toast(e.message, "err"); }
    }

    async function applyToAll() {
        if (State.doc.clips.length < 2) { toast("Cần ít nhất 2 clip trên timeline"); return; }
        const list = await call("ListTransitions");
        const name = (list.find(t => t.type === lastType) || {}).name || lastType;
        if (!confirm("Gắn «" + name + "» (" + (transDurMs() / 1000) + "s) cho TẤT CẢ " + (State.doc.clips.length - 1) + " mối nối?")) return;
        let ok = 0;
        for (let i = 0; i < State.doc.clips.length - 1; i++) {
            try { await call("SetTransition", State.doc.clips[i].id, lastType, transDurMs()); ok++; } catch (e) { /* bỏ qua */ }
        }
        refresh();
        renderCurrent();
        toast("⧉ Đã gắn «" + name + "» cho " + ok + "/" + (State.doc.clips.length - 1) + " mối nối", "ok");
    }

    async function clearAllTransitions() {
        if (!State.doc.transitions.length) { toast("Timeline chưa có chuyển cảnh nào"); return; }
        if (!confirm("Gỡ TOÀN BỘ " + State.doc.transitions.length + " chuyển cảnh?")) return;
        for (const tr of State.doc.transitions) {
            try { await call("SetTransition", tr.afterClipId, "none", 0); } catch (e) { /* bỏ qua */ }
        }
        refresh();
        renderCurrent();
        toast("Đã gỡ toàn bộ chuyển cảnh", "ok");
    }

    function renderCurrent() {
        const box = $("trans-cur");
        const trs = State.doc.transitions || [];
        if (!trs.length) {
            box.innerHTML = '<div class="tl-dim">Timeline chưa có chuyển cảnh nào — bấm một hiệu ứng phía trên để gắn.</div>';
            return;
        }
        let html = '<div class="tl-dim" style="margin-bottom:4px">Chuyển cảnh hiện có (' + trs.length + '):</div>';
        trs.forEach(tr => {
            const i = State.doc.clips.findIndex(c => c.id === tr.afterClipId);
            const a = i >= 0 ? State.findAsset(State.doc.clips[i].assetId) : null;
            const b = i >= 0 && State.doc.clips[i + 1] ? State.findAsset(State.doc.clips[i + 1].assetId) : null;
            html += '<div class="trans-row"><span class="tr-clip">' + (a ? a.name : "?") +
                ' ⇄ ' + (b ? b.name : "?") + '</span>' +
                '<span class="tr-type">' + tr.type + ' · ' + fmtMs(tr.durationMs) + '</span>' +
                '<button class="mini-btn danger" data-clip="' + tr.afterClipId + '">Gỡ</button></div>';
        });
        box.innerHTML = html;
        box.querySelectorAll("button[data-clip]").forEach(b => {
            b.addEventListener("click", async () => {
                try { await call("SetTransition", b.dataset.clip, "none", 0); refresh(); renderCurrent(); }
                catch (e) { toast(e.message, "err"); }
            });
        });
    }

    $("btn-trans").addEventListener("click", openTransModal);
    $("trans-close").addEventListener("click", () => { transModal.hidden = true; });
    transModal.addEventListener("click", e => { if (e.target === transModal) transModal.hidden = true; });
    $("trans-dur").addEventListener("input", e => { $("trans-dur-label").textContent = (Number(e.target.value) / 1000).toFixed(1) + "s"; });
    $("trans-all").addEventListener("click", applyToAll);
    $("trans-clear-all").addEventListener("click", clearAllTransitions);

    // ═══════════════ 2. MENU CHUỘT PHẢI TIMELINE ═══════════════
    const ctxMenu = $("ctx-menu");

    function closeCtx() { ctxMenu.hidden = true; }
    document.addEventListener("pointerdown", e => {
        if (!ctxMenu.hidden && !ctxMenu.contains(e.target)) closeCtx();
    }, true);
    document.addEventListener("vks:doc-changed", closeCtx);
    window.addEventListener("blur", closeCtx);

    function showCtx(x, y, items) {
        ctxMenu.innerHTML = "";
        for (const it of items) {
            if (it === "-") {
                const s = document.createElement("div");
                s.className = "ctx-sep";
                ctxMenu.appendChild(s);
                continue;
            }
            if (it.title) { // dòng tiêu đề mờ (tên clip / "Timeline")
                const t = document.createElement("div");
                t.className = "ctx-title";
                t.textContent = it.title;
                ctxMenu.appendChild(t);
                continue;
            }
            const d = document.createElement("div");
            d.className = "ctx-item" + (it.danger ? " danger" : "");
            d.textContent = (it.icon ? it.icon + " " : "") + it.label;
            d.addEventListener("click", () => { closeCtx(); it.run(); });
            ctxMenu.appendChild(d);
        }
        ctxMenu.hidden = false;
        // kẹp trong khung nhìn
        const r = ctxMenu.getBoundingClientRect();
        ctxMenu.style.left = Math.min(x, window.innerWidth - r.width - 8) + "px";
        ctxMenu.style.top = Math.min(y, window.innerHeight - r.height - 8) + "px";
    }

    // Nhân bản clip — dùng chung cho menu + Ctrl+D
    async function duplicateSelected() {
        const c = State.selection ? State.findClip(State.selection) : null;
        if (!c) { toast("Chọn một clip rồi bấm Ctrl+D để nhân bản"); return; }
        try {
            const nc = await call("DuplicateClip", c.id);
            State.select(nc.id);
            refresh();
            toast("⧉ Đã nhân bản clip — bản sao đứng NGAY SAU bản gốc", "ok");
        } catch (e) { toast(e.message, "err"); }
    }
    window.VKSDuplicate = duplicateSelected;

    const cv = document.getElementById("timeline-canvas");
    if (cv) {
        cv.addEventListener("contextmenu", e => {
            e.preventDefault();
            if (window.VKSTool && window.VKSTool.locked) { toast("🔒 Timeline đang KHÓA — bấm 🔒 để mở trước"); return; }
            const rect = cv.getBoundingClientRect();
            const x = e.clientX - rect.left, y = e.clientY - rect.top;
            const ms = Math.round((x / State.pxPerSec) * 1000);
            const at = State.clipAtTimeline(ms);
            const ovHit = (() => {
                if (y < 28 || y > 78) return null;
                for (const o of (State.doc.overlays || [])) {
                    const x0 = (o.startMs / 1000) * State.pxPerSec, x1 = (o.endMs / 1000) * State.pxPerSec;
                    if (x >= x0 - 2 && x <= x1 + 2) return o;
                }
                return null;
            })();

            // v1.3.0: menu chuột phải trên track âm thanh A1 (dải 168..224px)
            const audHit = (() => {
                if (y < 168 || y > 224) return null;
                for (const ac of (State.doc.audioClips || [])) {
                    const x0 = (ac.startMs / 1000) * State.pxPerSec;
                    const x1 = x0 + ((ac.outMs - ac.inMs) / 1000) * State.pxPerSec;
                    if (x >= x0 - 2 && x <= x1 + 2) return ac;
                }
                return null;
            })();
            if (audHit) {
                State.select(audHit.id);
                const aName = (State.findAsset(audHit.assetId) || {}).name || "Âm thanh";
                showCtx(e.clientX, e.clientY, [
                    { title: "🎵 " + aName + " (A1)" },
                    { icon: "⧉", label: "Nhân bản đoạn", run: async () => {
                        try { const nc = await call("DuplicateAudioClip", audHit.id); State.select(nc.id); refresh(); }
                        catch (err) { toast(err.message, "err"); }
                    } },
                    { icon: audHit.mute ? "🔊" : "🔇", label: audHit.mute ? "Bật tiếng" : "Tắt tiếng đoạn", run: async () => {
                        try { await call("UpdateAudioClip", audHit.id, { mute: !audHit.mute }); refresh(); }
                        catch (err) { toast(err.message, "err"); }
                    } },
                    { icon: "⌖", label: "Đặt tại con trỏ", run: async () => {
                        try { await call("UpdateAudioClip", audHit.id, { startMs: Math.round(State.playheadMs) }); refresh(); }
                        catch (err) { toast(err.message, "err"); }
                    } },
                    "-",
                    { icon: "🗑", label: "Xoá đoạn âm thanh (Del)", danger: true, run: () => doDeleteSel() },
                ]);
                return;
            }

            if (ovHit) {
                State.select(ovHit.id);
                showCtx(e.clientX, e.clientY, [
                    { title: "Lớp phủ V2" },
                    { icon: "✏️", label: "Sửa lớp phủ…", run: () => document.dispatchEvent(new CustomEvent("vks:open-overlay", { detail: ovHit.id })) },
                    { icon: "⧉", label: "Nhân đôi thời lượng", run: async () => {
                        try { await call("UpdateOverlay", ovHit.id, { endMs: ovHit.startMs + (ovHit.endMs - ovHit.startMs) * 2 }); refresh(); }
                        catch (err) { toast(err.message, "err"); }
                    } },
                    "-",
                    { icon: "🗑", label: "Xoá lớp phủ (Del)", danger: true, run: () => doDeleteSel() },
                ]);
                return;
            }

            if (at) {
                State.select(at.clip.id);
                const i = at.index;
                const items = [
                    { title: (State.findAsset(at.clip.assetId) || {}).name || "Clip" },
                    { icon: "✂", label: "Tách tại con trỏ (S)", run: () => document.getElementById("btn-split").click() },
                    { icon: "⧉", label: "Nhân bản clip (Ctrl+D)", run: duplicateSelected },
                    { icon: "🔊", label: "Tách âm thanh (Ctrl+L)", run: () => {
                        window.VKSApi.call("DetachAudio", at.clip.id)
                            .then(() => { refresh(); toast("🔗 Đã tách âm thanh vào thư viện", "ok"); })
                            .catch(err => toast(err.message, "err"));
                    } },
                ];
                if (i > 0) items.push({ icon: "◀", label: "Dời trái", run: () => document.getElementById("btn-back").click() });
                if (i < State.doc.clips.length - 1) items.push({ icon: "▶", label: "Dời phải", run: () => document.getElementById("btn-fwd").click() });
                items.push("-");
                items.push({ icon: "🗑", label: "Xoá clip (Del)", danger: true, run: () => doDeleteSel() });
                showCtx(e.clientX, e.clientY, items);
                return;
            }

            // chỗ trống / thước
            showCtx(e.clientX, e.clientY, [
                { title: "Timeline" },
                { icon: "⏮", label: "Về đầu (Home)", run: () => State.setPlayhead(0) },
                { icon: "⏭", label: "Đến cuối (End)", run: () => State.setPlayhead(State.totalMs()) },
                { icon: "⤢", label: "Thu nhỏ vừa khít", run: () => window.VKSTimeline && window.VKSTimeline.zoomFit() },
            ]);
        });
    }

    function doDeleteSel() { document.getElementById("btn-delete").click(); }

    // ═══════════════ 3. PHÍM Ctrl+D ═══════════════
    document.addEventListener("keydown", e => {
        if (e.target.tagName === "INPUT" || e.target.tagName === "SELECT" || e.target.tagName === "TEXTAREA") return;
        if ((e.ctrlKey || e.metaKey) && (e.key === "d" || e.key === "D")) {
            e.preventDefault();
            duplicateSelected();
        }
    });
})();
