/* v11.js — UI tính năng v1.1: lớp phủ (chữ/hình/PiP), phụ đề SRT + AI,
   công cụ (TTS, cắt nhanh, khoảng lặng, QC, cảnh, AI, kịch bản, proxy,
   cập nhật GitHub) và mở rộng modal xuất (alpha/GPU). */
(function () {
    "use strict";

    const { call, onEvent, fmtMs } = window.VKSApi;
    const State = window.VKSState;
    const $ = id => document.getElementById(id);

    function toast(msg, kind) { if (window.VKSApp) window.VKSApp.toast(msg, kind); }
    function refresh() { call("GetState").then(s => State.setDoc(s.doc)).catch(() => {}); }
    // v1.2.6: refresh CHỜ KẾT QUẢ — dùng khi cần doc mới trước khi vẽ UI.
    function refreshNow() {
        return call("GetState").then(s => State.setDoc(s.doc)).catch(() => {});
    }
    // v1.2.6: modal đang mở mà doc đổi (thêm/xoá từ timeline, hoàn tác…) →
    // vẽ lại DANH SÁCH để không bị treo dữ liệu cũ (editor giữ nguyên để không
    // mất nội dung người dùng đang gõ dở).
    document.addEventListener("vks:doc-changed", () => {
        if (ovModal && !ovModal.hidden) renderOvList();
    });

    // ═══════════════════════ MODAL LỚP PHỦ ═══════════════════════
    const ovModal = $("overlay-modal");
    let ovSelected = null;

    $("btn-overlays").addEventListener("click", () => { ovModal.hidden = false; renderOvList(); renderOvEditor(); });
    $("ov-close").addEventListener("click", () => { ovModal.hidden = true; });
    ovModal.addEventListener("click", e => { if (e.target === ovModal) ovModal.hidden = true; });
    document.addEventListener("vks:open-overlay", e => {
        ovModal.hidden = false;
        ovSelected = e.detail;
        renderOvList(); renderOvEditor();
    });

    async function addOverlay(kind) {
        const start = Math.min(State.playheadMs, Math.max(0, State.totalMs() - 1000));
        try {
            const o = await call("AddOverlay", kind, Math.max(0, start), Math.max(0, start) + 3000);
            ovSelected = o.id;
            // v1.2.6 SỬA LỖI: phải nạp doc MỚI về State TRƯỚC khi vẽ — trước đây
            // render chạy trước khi refresh kịp xong → editor không mở, danh sách
            // hiển thị cũ ("Chưa có lớp phủ nào") dù lớp phủ đã được tạo.
            await refreshNow();
            renderOvList(); renderOvEditor();
            toast("Đã thêm lớp phủ — nhập nội dung rồi bấm 💾 Lưu", "ok");
        } catch (e) { toast(e.message, "err"); }
    }
    $("ov-add-text").addEventListener("click", () => addOverlay("text"));
    $("ov-add-shape").addEventListener("click", () => addOverlay("shape"));
    $("ov-add-media").addEventListener("click", () => addOverlay("media"));

    function renderOvList() {
        const list = $("ov-list");
        list.innerHTML = "";
        const ovs = State.doc.overlays || [];
        if (!ovs.length) {
            list.innerHTML = '<div class="tl-dim">Chưa có lớp phủ nào — bấm nút phía trên để thêm.</div>';
            return;
        }
        ovs.forEach(o => {
            const row = document.createElement("div");
            row.className = "ov-row" + (ovSelected === o.id ? " sel" : "");
            const kindName = { text: "Chữ", shape: "Hình", media: "PiP" }[o.kind] || o.kind;
            const icon = { text: "🅣", shape: "◆", media: "▣" }[o.kind] || "•";
            row.innerHTML = `
                <span class="ov-ico">${icon}</span>
                <span class="ov-name grow">${kindName}: ${escapeHtml(ovLabel(o))}</span>
                <span class="tl-dim">${fmtMs(o.startMs)} → ${fmtMs(o.endMs)}</span>
                <button class="mini-btn edit">Sửa</button>
                <button class="mini-btn danger del">🗑</button>`;
            row.querySelector(".edit").addEventListener("click", () => { ovSelected = o.id; renderOvList(); renderOvEditor(); });
            row.querySelector(".del").addEventListener("click", async () => {
                try { await call("RemoveOverlay", o.id); if (ovSelected === o.id) ovSelected = null; refresh(); renderOvList(); renderOvEditor(); }
                catch (e) { toast(e.message, "err"); }
            });
            list.appendChild(row);
        });
    }

    function ovLabel(o) {
        if (o.kind === "text") return o.text || "(trống)";
        if (o.kind === "shape") return ({ rect: "Hình chữ nhật", circle: "Hình tròn", ellipse: "Hình elip", line: "Đường thẳng" }[o.shape] || "Hình");
        const a = State.findAsset(o.assetId);
        return a ? a.name : "(chưa chọn tư liệu)";
    }

    function escapeHtml(s) {
        return String(s || "").replace(/[&<>"']/g, m => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[m]));
    }

    const animNames = {
        "": "Không (tĩnh)", fade: "Mờ dần vào", typewriter: "Gõ chữ (typewriter)",
        slideup: "Trượt lên", slidedown: "Trượt xuống", slideleft: "Trượt trái", slideright: "Trượt phải", pop: "Nảy lên (pop)",
    };

    function renderOvEditor() {
        const box = $("ov-editor");
        const o = ovSelected ? State.findOverlay(ovSelected) : null;
        if (!o) { box.hidden = true; box.innerHTML = ""; return; }
        box.hidden = false;

        const posGrid = [
            ["tl", 0, 0], ["tc", 0.5, 0], ["tr", 1, 0],
            ["ml", 0, 0.5], ["mc", 0.5, 0.5], ["mr", 1, 0.5],
            ["bl", 0, 1], ["bc", 0.5, 1], ["br", 1, 1],
        ];

        let html = `<div class="prop-group">
            <div class="group-title">Sửa lớp phủ</div>
            <div class="row">
                <div class="field grow"><label>Bắt đầu (s)</label><input type="number" id="ove-start" step="0.1" min="0" value="${(o.startMs / 1000).toFixed(1)}"></div>
                <div class="field grow"><label>Kết thúc (s)</label><input type="number" id="ove-end" step="0.1" min="0" value="${(o.endMs / 1000).toFixed(1)}"></div>
            </div>
            <div class="field"><label>Vị trí</label>
                <div class="btn-row pos-grid">`;
        for (const [k, x, y] of posGrid) {
            html += `<button class="rot-btn pos ${Math.abs(o.posX - x) < 0.01 && Math.abs(o.posY - y) < 0.01 ? "active" : ""}" data-x="${x}" data-y="${y}">${k}</button>`;
        }
        html += `</div></div>`;

        if (o.kind === "text") {
            html += `
            <div class="field"><label>Nội dung</label><textarea id="ove-text" rows="2">${escapeHtml(o.text)}</textarea></div>
            <div class="row">
                <div class="field grow"><label>Cỡ chữ — <b id="ove-fs-label">${o.fontSize}</b></label>
                    <input type="range" id="ove-fs" min="16" max="300" step="2" value="${o.fontSize}"></div>
                <div class="field" style="width:110px"><label>Màu</label>
                    <input type="color" id="ove-color" value="#${o.colorHex || "ffffff"}" style="width:100%;height:34px;background:var(--bg2);border:1px solid var(--line);border-radius:6px"></div>
            </div>
            <div class="row">
                <div class="field grow"><label>Hoạt hình</label>
                    <select id="ove-anim">${Object.entries(animNames).map(([v, n]) =>
                        `<option value="${v}" ${o.anim === v ? "selected" : ""}>${n}</option>`).join("")}</select></div>
                <div class="field" style="width:130px"><label>Thời lượng anim (s)</label>
                    <input type="number" id="ove-animms" step="0.1" min="0" max="10" value="${((o.animMs || 0) / 1000).toFixed(1)}"></div>
            </div>
            <div class="btn-row">
                <button class="rot-btn ${o.outline ? "active" : ""}" id="ove-outline">Viền đen</button>
                <button class="rot-btn ${o.bold ? "active" : ""}" id="ove-bold">Đậm</button>
            </div>`;
        } else if (o.kind === "shape") {
            html += `
            <div class="row">
                <div class="field grow"><label>Hình dạng</label>
                    <select id="ove-shape">
                        <option value="rect" ${o.shape === "rect" ? "selected" : ""}>Hình chữ nhật</option>
                        <option value="circle" ${o.shape === "circle" ? "selected" : ""}>Hình tròn</option>
                        <option value="ellipse" ${o.shape === "ellipse" ? "selected" : ""}>Hình elip</option>
                        <option value="line" ${o.shape === "line" ? "selected" : ""}>Đường thẳng</option>
                    </select></div>
                <div class="field" style="width:110px"><label>Màu</label>
                    <input type="color" id="ove-color" value="#${o.colorHex || "ffffff"}" style="width:100%;height:34px;background:var(--bg2);border:1px solid var(--line);border-radius:6px"></div>
            </div>
            <div class="row">
                <div class="field grow"><label>Rộng (% canvas)</label><input type="number" id="ove-w" min="1" max="100" value="${o.widthPct}"></div>
                <div class="field grow"><label>Cao (% canvas)</label><input type="number" id="ove-h" min="1" max="100" value="${o.heightPct}"></div>
            </div>`;
        } else if (o.kind === "media") {
            const opts = State.doc.assets.filter(a => a.kind !== "audio" && !a.missing)
                .map(a => `<option value="${a.id}" ${o.assetId === a.id ? "selected" : ""}>${escapeHtml(a.name)}</option>`).join("");
            html += `
            <div class="field"><label>Tư liệu (video/ảnh)</label><select id="ove-asset">${opts || "<option value=''>— chưa có —</option>"}</select></div>
            <div class="row">
                <div class="field grow"><label>Kích thước (% canvas)</label><input type="number" id="ove-scale" min="2" max="100" value="${o.scalePct}"></div>
                <div class="field grow"><label>Âm lượng %</label><input type="number" id="ove-vol" min="0" max="200" step="10" value="${Math.round((o.volume || 1) * 100)}"></div>
            </div>
            <label class="check-row"><input type="checkbox" id="ove-mute" ${o.muted ? "checked" : ""}> Tắt tiếng</label>
            <div class="tool-hr"></div>
            <label class="check-row"><input type="checkbox" id="ove-ck-on" ${o.chromaKey && o.chromaKey.enabled ? "checked" : ""}> Tách nền xanh (chroma key)</label>
            <div class="row" style="margin-top:8px">
                <div class="field" style="width:120px"><label>Màu nền</label>
                    <input type="color" id="ove-ck-color" value="#${(o.chromaKey && o.chromaKey.hex) || "00ff00"}" style="width:100%;height:34px;background:var(--bg2);border:1px solid var(--line);border-radius:6px"></div>
                <div class="field grow"><label>Độ giống (similarity) — <b id="ove-ck-sim-label">${((o.chromaKey && o.chromaKey.similarity) || 0.1).toFixed(2)}</b></label>
                    <input type="range" id="ove-ck-sim" min="0" max="50" step="1" value="${Math.round(((o.chromaKey && o.chromaKey.similarity) || 0.1) * 100)}"></div>
                <div class="field grow"><label>Mềm viền (blend) — <b id="ove-ck-bl-label">${((o.chromaKey && o.chromaKey.blend) || 0.08).toFixed(2)}</b></label>
                    <input type="range" id="ove-ck-bl" min="0" max="30" step="1" value="${Math.round(((o.chromaKey && o.chromaKey.blend) || 0.08) * 100)}"></div>
            </div>`;
        }
        html += `
            <div class="tool-hr"></div>
            <div class="field"><label>Chế độ hòa trộn (v1.3.1)</label>
                <select id="ove-blend">
                    <option value="">Bình thường</option>
                    <option value="screen" ${o.blend === "screen" ? "selected" : ""}>Screen — hòa sáng</option>
                    <option value="multiply" ${o.blend === "multiply" ? "selected" : ""}>Multiply — nhân</option>
                    <option value="overlay" ${o.blend === "overlay" ? "selected" : ""}>Overlay — chồng tương phản</option>
                    <option value="darken" ${o.blend === "darken" ? "selected" : ""}>Darken — lấy màu tối</option>
                    <option value="lighten" ${o.blend === "lighten" ? "selected" : ""}>Lighten — lấy màu sáng</option>
                    <option value="add" ${o.blend === "add" ? "selected" : ""}>Add — cộng sáng</option>
                </select></div>
            <div class="row">
                <div class="field grow"><label>Độ mờ đục %</label><input type="number" id="ove-op" min="5" max="100" step="5" value="${(o.opacity != null && o.opacity > 0 && o.opacity <= 1) ? Math.round(o.opacity * 100) : 100}"></div>
                <div class="field grow"><label>Hiện dần vào (s)</label><input type="number" id="ove-fin" step="0.1" min="0" max="10" value="${((o.fadeInMs || 0) / 1000).toFixed(1)}"></div>
                <div class="field grow"><label>Biến mất dần (s)</label><input type="number" id="ove-fout" step="0.1" min="0" max="10" value="${((o.fadeOutMs || 0) / 1000).toFixed(1)}"></div>
            </div>
            <div class="field"><label>Preset hiệu ứng (v1.3.1)</label>
                <select id="ove-filter">
                    <option value="">— Không —</option>
                    <option value="bw" ${o.filterPreset === "bw" ? "selected" : ""}>Trắng đen</option>
                    <option value="negative" ${o.filterPreset === "negative" ? "selected" : ""}>Âm bản</option>
                    <option value="sepia" ${o.filterPreset === "sepia" ? "selected" : ""}>Sepia cổ điển</option>
                    <option value="vintage" ${o.filterPreset === "vintage" ? "selected" : ""}>Vintage retro</option>
                    <option value="vivid" ${o.filterPreset === "vivid" ? "selected" : ""}>Rực rỡ</option>
                    <option value="contrast" ${o.filterPreset === "contrast" ? "selected" : ""}>Tương phản đậm</option>
                    <option value="dream" ${o.filterPreset === "dream" ? "selected" : ""}>Mơ màng</option>
                </select></div>`;
        html += `<div class="btn-row" style="margin-top:10px"><button class="tb-btn accent" id="ove-save">💾 Lưu thay đổi</button></div></div>`;
        box.innerHTML = html;

        const patch = {};
        const num = id => Math.round(Number($(id).value) * 1000);
        $("ove-start").addEventListener("change", e => { patch.startMs = Math.round(Number(e.target.value) * 1000); });
        $("ove-end").addEventListener("change", e => { patch.endMs = Math.round(Number(e.target.value) * 1000); });
        box.querySelectorAll(".pos").forEach(b => b.addEventListener("click", async () => {
            patch.posX = Number(b.dataset.x); patch.posY = Number(b.dataset.y);
            box.querySelectorAll(".pos").forEach(x => x.classList.remove("active"));
            b.classList.add("active");
        }));

        // v1.3.1: chế độ lớp trong modal (áp cho mọi loại lớp phủ)
        $("ove-blend").addEventListener("change", e => { patch.blend = e.target.value; });
        $("ove-op").addEventListener("change", e => { patch.opacity = Number(e.target.value) / 100; });
        $("ove-fin").addEventListener("change", e => { patch.fadeInMs = Math.round(Number(e.target.value) * 1000); });
        $("ove-fout").addEventListener("change", e => { patch.fadeOutMs = Math.round(Number(e.target.value) * 1000); });
        $("ove-filter").addEventListener("change", e => { patch.filterPreset = e.target.value; });

        if (o.kind === "text") {
            $("ove-text").addEventListener("change", e => { patch.text = e.target.value; });
            $("ove-fs").addEventListener("input", e => { $("ove-fs-label").textContent = e.target.value; });
            $("ove-fs").addEventListener("change", e => { patch.fontSize = Number(e.target.value); });
            $("ove-color").addEventListener("change", e => { patch.colorHex = e.target.value; });
            $("ove-anim").addEventListener("change", e => { patch.anim = e.target.value; });
            $("ove-animms").addEventListener("change", e => { patch.animMs = Math.round(Number(e.target.value) * 1000); });
            $("ove-outline").addEventListener("click", e => { e.target.classList.toggle("active"); patch.outline = e.target.classList.contains("active"); });
            $("ove-bold").addEventListener("click", e => { e.target.classList.toggle("active"); patch.bold = e.target.classList.contains("active"); });
        } else if (o.kind === "shape") {
            $("ove-shape").addEventListener("change", e => { patch.shape = e.target.value; });
            $("ove-color").addEventListener("change", e => { patch.colorHex = e.target.value; });
            $("ove-w").addEventListener("change", e => { patch.widthPct = Number(e.target.value); });
            $("ove-h").addEventListener("change", e => { patch.heightPct = Number(e.target.value); });
        } else if (o.kind === "media") {
            $("ove-asset").addEventListener("change", e => { patch.assetId = e.target.value; });
            $("ove-scale").addEventListener("change", e => { patch.scalePct = Number(e.target.value); });
            $("ove-vol").addEventListener("change", e => { patch.volume = Number(e.target.value) / 100; });
            $("ove-mute").addEventListener("change", e => { patch.muted = e.target.checked; });
            const baseCK = o.chromaKey || { enabled: false, hex: "00ff00", similarity: 0.1, blend: 0.08 };
            $("ove-ck-on").addEventListener("change", e => { baseCK.enabled = e.target.checked; });
            $("ove-ck-color").addEventListener("change", e => { baseCK.hex = e.target.value.replace("#", ""); });
            $("ove-ck-sim").addEventListener("input", e => { $("ove-ck-sim-label").textContent = (e.target.value / 100).toFixed(2); });
            $("ove-ck-sim").addEventListener("change", e => { baseCK.similarity = Number(e.target.value) / 100; });
            $("ove-ck-bl").addEventListener("input", e => { $("ove-ck-bl-label").textContent = (e.target.value / 100).toFixed(2); });
            $("ove-ck-bl").addEventListener("change", e => { baseCK.blend = Number(e.target.value) / 100; });
            patch.chromaKey = baseCK; // đọc trực tiếp object gốc — mọi field luôn đầy đủ
        }

        $("ove-save").addEventListener("click", async () => {
            try {
                await call("UpdateOverlay", o.id, patch);
                refresh(); renderOvList();
                toast("Đã lưu lớp phủ", "ok");
            } catch (e) { toast(e.message, "err"); }
        });
    }

    // ═══════════════════════ MODAL PHỤ ĐỀ ═══════════════════════
    const subsModal = $("subs-modal");
    $("btn-subs").addEventListener("click", () => { subsModal.hidden = false; renderSubs(); });
    $("subs-close").addEventListener("click", () => { subsModal.hidden = true; });
    subsModal.addEventListener("click", e => { if (e.target === subsModal) subsModal.hidden = true; });

    function renderSubs() {
        const st = State.doc.subs;
        const cues = (st && st.cues) || [];
        $("subs-status").textContent = st
            ? `Nguồn: ${st.sourceName || "—"} · ${cues.length} dòng`
            : "Chưa có phụ đề trong dự án.";
        $("subs-burn").checked = !!(st && st.burn);
        if (st) {
            $("subs-fs").value = st.fontSize || 42;
            $("subs-fs-label").textContent = st.fontSize || 42;
            $("subs-color").value = "#" + (st.colorHex || "ffffff").replace("#", "");
        }
        const box = $("subs-cues");
        box.innerHTML = "";
        cues.slice(0, 400).forEach(c => {
            const row = document.createElement("div");
            row.className = "cue-row";
            row.innerHTML = `<span class="cue-time">${fmtMs(c.startMs)} → ${fmtMs(c.endMs)}</span><span class="cue-text">${escapeHtml(c.text)}</span>`;
            row.addEventListener("click", () => State.setPlayhead(c.startMs));
            box.appendChild(row);
        });
    }

    $("subs-import").addEventListener("click", async () => {
        try {
            const st = await call("ImportSRTDialog");
            if (st) { refresh(); renderSubs(); toast("Đã nhập " + (st.cues || []).length + " dòng phụ đề", "ok"); }
        } catch (e) { if (e.message) toast(e.message, "err"); }
    });
    $("subs-export").addEventListener("click", async () => {
        try {
            const p = await call("ExportSRTDialog");
            if (p) toast("Đã xuất: " + p, "ok");
        } catch (e) { if (e.message) toast(e.message, "err"); }
    });
    $("subs-clear").addEventListener("click", async () => {
        if (!confirm("Xoá phụ đề khỏi dự án?")) return;
        try { await call("ClearSubtitles"); refresh(); renderSubs(); } catch (e) { toast(e.message, "err"); }
    });
    $("subs-fs").addEventListener("input", e => { $("subs-fs-label").textContent = e.target.value; });
    $("subs-style-save").addEventListener("click", async () => {
        try {
            await call("SetSubtitleStyle", $("subs-burn").checked, Number($("subs-fs").value), $("subs-color").value);
            refresh(); renderSubs(); toast("Đã áp style phụ đề", "ok");
        } catch (e) { toast(e.message, "err"); }
    });
    $("subs-burn").addEventListener("change", async e => {
        try { await call("SetSubtitleStyle", e.target.checked, 0, ""); refresh(); }
        catch (err) { toast(err.message, "err"); }
    });
    $("subs-ai-stt").addEventListener("click", async () => {
        const sel = State.selection && State.findClip(State.selection);
        if (!sel) { toast("Chọn một clip trên timeline trước (lấy âm thanh từ clip đó)"); return; }
        try {
            await call("AITranscribe", sel.assetId);
            toast("Đang gửi tới API AI — chờ kết quả…", "ok");
        } catch (e) { toast(e.message, "err"); }
    });
    $("subs-ai-translate").addEventListener("click", async () => {
        const lang = prompt("Dịch phụ đề sang ngôn ngữ nào? (VD: Vietnamese, English, Japanese…)", "English");
        if (!lang) return;
        try {
            await call("AITranslateSubs", lang);
            toast("Đang dịch qua API AI…", "ok");
        } catch (e) { toast(e.message, "err"); }
    });
    onEvent("ai:transcribe:done", p => { refresh(); renderSubs(); toast("AI đã tạo " + (p && p.cues || 0) + " dòng phụ đề 🎉", "ok"); });
    onEvent("ai:transcribe:error", p => toast(p && p.message || "AI STT lỗi", "err"));
    onEvent("ai:translate:done", p => { refresh(); renderSubs(); toast("Đã dịch " + (p && p.count || 0) + " dòng 🌐", "ok"); });
    onEvent("ai:translate:error", p => toast(p && p.message || "AI dịch lỗi", "err"));

    // ═══════════════════════ MODAL CÔNG CỤ ═══════════════════════
    const toolsModal = $("tools-modal");
    $("btn-tools").addEventListener("click", async () => {
        toolsModal.hidden = false;
        fillAssetSelects();
        await loadAISettings();
        await loadTTSPreview();
    });
    $("tools-close").addEventListener("click", () => { toolsModal.hidden = true; });
    toolsModal.addEventListener("click", e => { if (e.target === toolsModal) toolsModal.hidden = true; });

    // ═══════════ v1.2.3: ĐỔI KHUNG DỰ ÁN + AUTO REFRAME CƠ BẢN ═══════════
    const canvasRate = () => {
        const r = State.doc.canvas.rate || { num: 30, den: 1 };
        return [Math.max(1, Number(r.num) || 30), Math.max(1, Number(r.den) || 1)];
    };
    document.querySelectorAll('[data-canvas]').forEach(b => {
        b.addEventListener("click", async () => {
            const [w, h] = b.dataset.canvas.split("x").map(Number);
            const [num, den] = canvasRate();
            try {
                await call("SetCanvas", w, h, num, den);
                refresh();
                $("ar-result").textContent = "✅ Đã đổi khung dự án thành " + w + "×" + h +
                    " — mở Thuộc tính → Vị trí ngang/dọc để chọn vùng khung của từng clip.";
            } catch (e) { $("ar-result").textContent = "Lỗi: " + e.message; }
        });
    });
    const setAllPosCenter = async () => {
        let ok = 0;
        for (const c of State.doc.clips) {
            try { await call("UpdateClip", c.id, { posX: 0.5, posY: 0.5 }); ok++; }
            catch (e) { /* bỏ qua */ }
        }
        refresh();
        return ok + "/" + State.doc.clips.length;
    };
    $("ar-center").addEventListener("click", async () => {
        const [num, den] = canvasRate();
        try {
            await call("SetCanvas", 1080, 1920, num, den);
            const n = await setAllPosCenter();
            $("ar-result").textContent = "✅ Khung 9:16 (1080×1920) + đã căn giữa " + n + " clip. Muốn giữ mặt chủ thể lệch trái/phải: chỉnh «Vị trí ngang» trong Thuộc tính.";
        } catch (e) { $("ar-result").textContent = "Lỗi: " + e.message; }
    });
    $("ar-restore").addEventListener("click", async () => {
        const [num, den] = canvasRate();
        try {
            await call("SetCanvas", 1920, 1080, num, den);
            const n = await setAllPosCenter();
            $("ar-result").textContent = "✅ Khung 16:9 (1920×1080) + đã căn giữa " + n + " clip.";
        } catch (e) { $("ar-result").textContent = "Lỗi: " + e.message; }
    });

    function fillAssetSelects() {
        const vids = State.doc.assets.filter(a => !a.missing);
        for (const [selId, kinds] of [["fc-asset", ["video"]], ["qc-asset", ["video", "audio", "image"]]]) {
            const sel = $(selId);
            sel.innerHTML = "";
            vids.filter(a => kinds.includes(a.kind)).forEach(a => {
                const o = document.createElement("option");
                o.value = a.id; o.textContent = a.name;
                sel.appendChild(o);
            });
        }
    }

    async function loadAISettings() {
        try {
            const s = await call("GetAISettings");
            $("ai-url").value = s.baseUrl || "";
            $("ai-model").value = s.model || "";
            $("ai-asr").value = s.asrModel || "";
            $("ai-key").placeholder = s.hasKey ? "(đã lưu key — để trống để giữ)" : "dán API key…";
        } catch (_) { /* ignore */ }
    }
    $("ai-save").addEventListener("click", async () => {
        try {
            await call("SetAISettings", $("ai-url").value.trim(), $("ai-key").value.trim(),
                $("ai-model").value.trim(), $("ai-asr").value.trim());
            $("ai-key").value = "";
            await loadAISettings();
            toast("Đã lưu cấu hình AI", "ok");
        } catch (e) { toast(e.message, "err"); }
    });
    $("ai-ask").addEventListener("click", async () => {
        const q = $("ai-q").value.trim();
        if (!q) return;
        const box = $("ai-answer");
        box.textContent = "Đang hỏi AI…";
        try {
            const ans = await call("AIAnalyze", q);
            box.textContent = ans;
        } catch (e) { box.textContent = "Lỗi: " + e.message; }
    });

    async function loadTTSPreview() {
        const sel = $("tts-voice");
        if (sel.options.length) return;
        try {
            const voices = await call("ListTTSVoices");
            sel.innerHTML = "";
            (voices || []).forEach(v => {
                const o = document.createElement("option");
                o.value = v.name;
                o.textContent = v.name + (v.language ? " (" + v.language + ")" : "");
                sel.appendChild(o);
            });
            if (!sel.options.length) sel.innerHTML = "<option>Không có giọng đọc</option>";
        } catch (e) {
            sel.innerHTML = "<option>" + (e.message || "TTS chỉ có trên Windows").slice(0, 60) + "</option>";
        }
    }
    $("tts-rate").addEventListener("input", e => { $("tts-rate-label").textContent = e.target.value; });
    $("tts-go").addEventListener("click", async () => {
        const text = $("tts-text").value.trim();
        if (!text) { toast("Nhập nội dung đọc trước"); return; }
        const btn = $("tts-go");
        btn.disabled = true; btn.textContent = "Đang tạo…";
        try {
            await call("TTSGenerate", text, $("tts-voice").value, Number($("tts-rate").value), $("tts-addtl").checked);
            toast("Đã tạo giọng đọc ✓", "ok");
            refresh();
        } catch (e) { toast(e.message, "err"); }
        btn.disabled = false; btn.textContent = "Tạo giọng đọc";
    });

    $("fc-go").addEventListener("click", async () => {
        const assetId = $("fc-asset").value;
        if (!assetId) { toast("Chưa có tư liệu video"); return; }
        try {
            const p = await call("FastCut", assetId, Math.round(Number($("fc-in").value) * 1000), Math.round(Number($("fc-out").value) * 1000));
            toast("Đã cắt nhanh: " + p, "ok");
            refresh();
        } catch (e) { toast(e.message, "err"); }
    });
    $("cc-go").addEventListener("click", async () => {
        // Nối nhanh: dùng tất cả video trong thư viện theo thứ tự (đơn giản).
        const vids = State.doc.assets.filter(a => a.kind === "video" && !a.missing);
        if (vids.length < 2) { toast("Cần ít nhất 2 video trong thư viện"); return; }
        try {
            const p = await call("QuickConcat", vids.map(a => a.id), "");
            toast("Đã nối " + vids.length + " video: " + p, "ok");
            refresh();
        } catch (e) { toast(e.message, "err"); }
    });

    // Khoảng lặng
    let lastSilences = [];
    $("sl-db").addEventListener("input", e => { $("sl-db-label").textContent = e.target.value; });
    $("sl-scan").addEventListener("click", async () => {
        const c = State.selection && State.findClip(State.selection);
        if (!c) { toast("Chọn một clip trên timeline trước"); return; }
        $("sl-result").textContent = "Đang quét…";
        try {
            lastSilences = await call("DetectClipSilence", c.id, Number($("sl-db").value), Math.round(Number($("sl-min").value) * 1000));
            $("sl-result").textContent = lastSilences.length
                ? "Phát hiện " + lastSilences.length + " khoảng lặng: " +
                  lastSilences.map(s => fmtMs(s.startMs) + "→" + fmtMs(s.endMs)).join(", ")
                : "Không có khoảng lặng nào đạt ngưỡng.";
            $("sl-apply").disabled = !lastSilences.length;
        } catch (e) { $("sl-result").textContent = "Lỗi: " + e.message; $("sl-apply").disabled = true; }
    });
    $("sl-apply").addEventListener("click", async () => {
        const c = State.selection && State.findClip(State.selection);
        if (!c || !lastSilences.length) return;
        try {
            const n = await call("ApplySilenceCut", c.id, lastSilences);
            toast("Đã cắt thành " + n + " đoạn bỏ lặng ✓", "ok");
            refresh();
        } catch (e) { toast(e.message, "err"); }
    });

    // QC + cảnh
    function renderReport(el, rep) {
        el.innerHTML = "";
        if (!rep) return;
        const head = document.createElement("div");
        head.className = "tl-dim";
        head.textContent = (rep.file || "").split(/[\\/]/).pop() +
            (rep.durationMs ? " · " + fmtMs(rep.durationMs) : "") +
            (rep.meanVolumeDb != null ? " · âm lượng TB " + rep.meanVolumeDb.toFixed(1) + " dB" : "") +
            (rep.maxVolumeDb != null ? " · đỉnh " + rep.maxVolumeDb.toFixed(1) + " dB" : "");
        el.appendChild(head);
        (rep.problems || []).forEach(p => {
            const row = document.createElement("div");
            row.className = "qc-row " + (p.level === "warn" ? "warn" : "ok");
            row.textContent = (p.level === "warn" ? "⚠ " : "✓ ") + p.message;
            el.appendChild(row);
        });
    }
    $("qc-run").addEventListener("click", async () => {
        const box = $("qc-result");
        box.textContent = "Đang quét QC…";
        try { renderReport(box, await call("RunQCAsset", $("qc-asset").value)); }
        catch (e) { box.textContent = "Lỗi: " + e.message; }
    });
    $("qc-run-prev").addEventListener("click", async () => {
        const box = $("qc-result");
        box.textContent = "Đang quét bản xem trước…";
        try { renderReport(box, await call("RunQCCurrent")); }
        catch (e) { box.textContent = "Lỗi: " + e.message; }
    });
    $("qc-scenes").addEventListener("click", async () => {
        const box = $("qc-result");
        box.textContent = "Đang phân tích cảnh…";
        try {
            const marks = await call("DetectAssetScenes", $("qc-asset").value, Number($("qc-th").value));
            box.innerHTML = "";
            const head = document.createElement("div");
            head.className = "tl-dim";
            head.textContent = marks.length ? "Phát hiện " + marks.length + " điểm chuyển cảnh:" : "Không phát hiện chuyển cảnh nào.";
            box.appendChild(head);
            marks.slice(0, 200).forEach(m => {
                const row = document.createElement("div");
                row.className = "qc-row ok";
                row.textContent = "✂ " + fmtMs(m.atMs) + " (điểm " + m.score.toFixed(2) + ")";
                box.appendChild(row);
            });
        } catch (e) { box.textContent = "Lỗi: " + e.message; }
    });

    // Kịch bản
    $("sc-sample").addEventListener("click", () => {
        const vids = State.doc.assets.filter(a => !a.missing);
        const items = vids.slice(0, 5).map((a, i) => a.kind === "image"
            ? { assetId: a.id, durMs: 3000, transition: i ? "crossfade" : "", transitionMs: 500 }
            : { assetId: a.id, inMs: 0, outMs: Math.min(5000, a.durationMs), transition: i ? "crossfade" : "", transitionMs: 500 });
        $("sc-json").value = JSON.stringify({ items }, null, 2);
    });
    $("sc-go").addEventListener("click", async () => {
        try {
            const n = await call("BuildFromScript", $("sc-json").value);
            refresh();
            toast("Đã dựng " + n + " clip từ kịch bản ✓", "ok");
        } catch (e) { toast(e.message, "err"); }
    });

    // Proxy + cập nhật
    $("px-gen").addEventListener("click", async () => {
        try {
            await call("GenerateProxies");
            toast("Đang tạo proxy…", "ok");
        } catch (e) { toast(e.message, "err"); }
    });
    onEvent("proxy:progress", p => { $("px-up-result").textContent = "Proxy: " + (p.done || 0) + "/" + (p.total || 0) + "…"; });
    onEvent("proxy:done", p => { $("px-up-result").textContent = "Đã tạo " + (p.count || 0) + " proxy 480p ✓"; });
    onEvent("proxy:error", p => { if (!(p && p.canceled)) $("px-up-result").textContent = "Lỗi proxy: " + (p && p.message || "?"); });

    $("up-check").addEventListener("click", async () => {
        const box = $("px-up-result");
        box.textContent = "Đang kiểm tra GitHub Release…";
        try {
            const u = await call("CheckUpdate");
            if (u.error) box.textContent = "⚠ " + u.error;
            else if (u.hasUpdate) box.innerHTML = "🚀 Có bản mới <b>v" + u.latestVersion + "</b> (hiện tại v" + u.currentVersion + ") — " +
                (u.assetUrl ? '<a href="#" id="up-dl">tải về</a>' : "") +
                (u.updateUrl ? ' · <a href="#" id="up-open">xem chi tiết</a>' : "");
            else box.textContent = "✓ Bạn đang dùng bản mới nhất (v" + u.currentVersion + ").";
            const dl = $("up-dl");
            if (dl && u.assetUrl) dl.addEventListener("click", e => { e.preventDefault(); window.open(u.assetUrl, "_blank"); });
            const op = $("up-open");
            if (op && u.updateUrl) op.addEventListener("click", e => { e.preventDefault(); window.open(u.updateUrl, "_blank"); });
        } catch (e) { box.textContent = "Lỗi: " + e.message; }
    });
    $("up-save").addEventListener("click", async () => {
        try { await call("SetGitHubRepo", $("up-repo").value.trim()); toast("Đã lưu repo", "ok"); }
        catch (e) { toast(e.message, "err"); }
    });
})();
