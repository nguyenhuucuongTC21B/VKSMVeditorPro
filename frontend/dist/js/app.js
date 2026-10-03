/* app.js — khởi động, thanh công cụ, modal xuất video, selftest, phím tắt. */
(function () {
    "use strict";

    const { call, onEvent, fmtMs, fmtBytes } = window.VKSApi;
    const State = window.VKSState;

    const $ = id => document.getElementById(id);

    // ─────────────── Toast ───────────────
    function toast(msg, kind) {
        const box = document.createElement("div");
        box.className = "toast " + (kind || "");
        box.textContent = msg;
        $("toasts").appendChild(box);
        setTimeout(() => {
            box.style.transition = "opacity .3s";
            box.style.opacity = "0";
            setTimeout(() => box.remove(), 320);
        }, kind === "err" ? 5200 : 2800);
    }

    // ─────────────── Khởi động ───────────────
    async function waitForBackend(ms) {
        const t0 = Date.now();
        while (Date.now() - t0 < (ms || 5000)) {
            if (window.go && window.go.main && window.go.main.App) return true;
            await new Promise(r => setTimeout(r, 80));
        }
        return false;
    }

    async function boot() {
        if (!(await waitForBackend(6000))) {
            toast("Không kết nối được backend Go — hãy chạy qua VKSeditorPro.exe", "err");
            return;
        }
        try {
            const s = await call("GetState");
            State.setDoc(s.doc);
            if (s.previewUrl) {
                State.previewURL = s.previewUrl;
                window.VKSPanels.setPreviewURL(s.previewUrl);
            }
            if (s.os !== "windows") {
                // Cảnh báo nhẹ khi chạy trên môi trường không phải Windows.
                toast("VKSeditorPro được tối ưu cho Windows (bản exe đơn)", "ok");
            }
        } catch (e) {
            // v1.2.1: lỗi nạp trạng thái KHÔNG được phép giết timeline —
            // vẫn vẽ khung trống để người dùng có thể bấm Mới / Nhập tư liệu.
            State.setDoc(null);
            toast("Không nạp được dự án cũ (" + e.message + ") — đã tạo khung trống. Bấm Mới hoặc Nhập tư liệu để bắt đầu.", "err");
        } finally {
            try {
                $("tl-total-time").textContent = fmtMs(State.totalMs());
                window.VKSTimeline.resize();
            } catch (_) { /* bỏ qua */ }
        }

        // Sự kiện từ backend
        onEvent("import:error", p => toast(p && p.message ? p.message : "Nhập tư liệu lỗi", "err"));
        onEvent("import:done", () => { refresh(); toast("Nhập tư liệu thành công", "ok"); });
        bindExportEvents();
        bindSelftestEvents();
    }

    function refresh() {
        call("GetState").then(s => State.setDoc(s.doc)).catch(() => {});
    }

    // ─────────────── Thanh công cụ ───────────────
    $("btn-import").addEventListener("click", doImport);
    $("btn-import-2").addEventListener("click", doImport);
    async function doImport() {
        try {
            const added = await call("ImportMediaDialog");
            if (added && added.length) {
                toast("Đã nhập/cập nhật " + added.length + " tư liệu", "ok");
                refresh();
            } else {
                // v1.2.1: không còn im lặng khi tệp trùng/không hỗ trợ.
                toast("Không có tư liệu mới — tệp đã có trong thư viện hoặc không hỗ trợ", "err");
            }
        } catch (e) { if (e.message && !e.message.includes("đã huỷ")) toast(e.message, "err"); }
    }

    $("btn-new").addEventListener("click", async () => {
        if (!confirm("Tạo dự án MỚI? Timeline hiện tại sẽ bị xoá.")) return;
        try {
            const keep = confirm("Giữ lại thư viện tư liệu?\nOK = giữ · Cancel = xoá sạch");
            await call("NewProject", keep);
            State.select(null);
            State.setPlayhead(0);
            refresh();
            toast("Đã tạo dự án mới", "ok");
        } catch (e) { toast(e.message, "err"); }
    });

    $("btn-open").addEventListener("click", async () => {
        try {
            const doc = await call("OpenProjectDialog");
            if (doc) {
                State.select(null);
                State.setPlayhead(0);
                State.setDoc(doc);
                toast("Đã mở dự án «" + doc.name + "»", "ok");
            }
        } catch (e) { if (e.message) toast(e.message, "err"); }
    });

    $("btn-save").addEventListener("click", async () => {
        try {
            const p = await call("SaveProject");
            toast("Đã lưu: " + p, "ok");
        } catch (e) { toast(e.message, "err"); }
    });

    $("btn-preview").addEventListener("click", async () => {
        if (State.doc.clips.length === 0) { toast("Timeline trống — thêm clip trước đã"); return; }
        setJob("Đang tạo bản xem trước…");
        try {
            await call("RenderPreview");
            toast("Đang render bản xem trước 540p…", "ok");
        } catch (e) {
            setJob("");
            toast(e.message, "err");
        }
    });

    $("btn-export").addEventListener("click", openExportModal);
    $("btn-selftest").addEventListener("click", () => { $("selftest-modal").hidden = false; });

    function setJob(text) {
        $("job-status").textContent = text || "";
    }

    // ─────────────── Modal xuất video ───────────────
    const expModal = $("export-modal");
    let expFormat = "mp4";
    let expPath = "";
    const crfNames = { 14: "Siêu nét (CRF 14)", 18: "Rất nét (CRF 18)", 20: "Cân bằng (CRF 20)", 24: "Nhỏ gọn (CRF 24)", 28: "Rất nhỏ (CRF 28)" };

    async function openExportModal() {
        if (State.doc.clips.length === 0) { toast("Timeline trống — không có gì để xuất"); return; }
        expModal.hidden = false;
        $("export-form").hidden = false;
        $("export-progress").hidden = true;
        expPath = "";
        $("exp-path").value = "";
        buildResOptions();
        syncFormatUI();
        $("tl-total-time").textContent = fmtMs(State.totalMs());
    }

    function buildResOptions() {
        const sel = $("exp-res");
        sel.innerHTML = "";
        const H = State.doc.canvas.height;
        const opts = [[0, "Giữ nguyên (" + H + "p)"]];
        for (const h of [1080, 720, 480, 360]) {
            if (h < H) opts.push([h, h + "p"]);
        }
        for (const [v, label] of opts) {
            const o = document.createElement("option");
            o.value = v; o.textContent = label;
            sel.appendChild(o);
        }
    }

    function syncFormatUI() {
        const isGif = expFormat === "gif";
        const isAlpha = expFormat === "movalpha";
        $("field-crf").hidden = isGif || isAlpha;
        $("field-giffps").hidden = !isGif;
        $("field-res").hidden = isAlpha;
        $("field-gpu").hidden = isGif || isAlpha;
        document.querySelectorAll("#exp-format button").forEach(b =>
            b.classList.toggle("active", b.dataset.v === expFormat));
    }

    $("exp-format").addEventListener("click", e => {
        const b = e.target.closest("button");
        if (!b) return;
        expFormat = b.dataset.v;
        syncFormatUI();
    });
    $("exp-crf").addEventListener("input", e => {
        const v = Number(e.target.value);
        $("crf-label").textContent = crfNames[v] || ("CRF " + v);
    });
    $("exp-giffps").addEventListener("input", e => {
        $("giffps-label").textContent = e.target.value;
    });
    $("exp-browse").addEventListener("click", async () => {
        try {
            const p = await call("ChooseExportPath", expFormat);
            if (p) { expPath = p; $("exp-path").value = p; }
        } catch (e) { toast(e.message, "err"); }
    });
    $("export-close").addEventListener("click", () => { expModal.hidden = true; });
    expModal.addEventListener("click", e => { if (e.target === expModal) expModal.hidden = true; });

    $("exp-start").addEventListener("click", async () => {
        if (!expPath) { toast("Hãy chọn nơi lưu trước"); return; }
        const req = {
            format: expFormat,
            outputPath: expPath,
            targetH: Number($("exp-res").value) || 0,
            crf: Number($("exp-crf").value) || 20,
            preset: "medium",
            gifFps: Number($("exp-giffps").value) || 12,
            hwAccel: $("exp-gpu") ? $("exp-gpu").value : "",
        };
        try {
            await call("StartExport", req);
            $("export-form").hidden = true;
            $("export-progress").hidden = false;
            $("exp-open").hidden = true;
            $("exp-close2").hidden = true;
            $("exp-fill").style.width = "0%";
            $("exp-text").textContent = "Đang dựng…";
            setJob("Đang xuất video…");
        } catch (e) { toast(e.message, "err"); }
    });
    $("exp-cancel").addEventListener("click", async () => {
        try { await call("CancelExport"); } catch (e) { /* ignore */ }
    });
    $("exp-open").addEventListener("click", async () => {
        try { await call("OpenInFolder", expPath); } catch (e) { toast(e.message, "err"); }
    });
    $("exp-close2").addEventListener("click", () => {
        expModal.hidden = true;
        setJob("");
        refresh();
    });

    function bindExportEvents() {
        onEvent("export:progress", p => {
            $("exp-fill").style.width = (p.pct || 0) + "%";
            $("exp-text").textContent = "Đang dựng… " + (p.pct || 0) + "% (" + p.done + "/" + p.total + " khung)";
        });
        onEvent("export:done", p => {
            $("exp-fill").style.width = "100%";
            $("exp-text").textContent = "Hoàn tất! " + fmtBytes(p.size) + " · " + (p.took || "");
            $("exp-open").hidden = false;
            $("exp-close2").hidden = false;
            $("exp-cancel").hidden = true;
            setJob("");
            toast("Xuất video thành công 🎉", "ok");
        });
        onEvent("export:error", p => {
            $("exp-text").textContent = p && p.message ? p.message : "Lỗi không rõ";
            $("exp-close2").hidden = false;
            $("exp-cancel").hidden = true;
            setJob("");
            if (!(p && p.canceled)) toast(p && p.message ? p.message : "Xuất video lỗi", "err");
        });
        // Xem trước
        onEvent("preview:progress", p => {
            setJob("Đang tạo bản xem trước… " + (p.pct || 0) + "%");
        });
        onEvent("preview:done", p => {
            setJob("");
            window.VKSPanels.setPreviewURL(p.url);
            State.previewDurationMs = State.totalMs();
            toast("Bản xem trước đã sẵn sàng — đang tự phát (nút ▶/⏸ dưới khung)", "ok");
            // Tự chuyển sang chế độ video + TỰ PHÁT (v1.3.0)
            document.querySelector('#preview-mode [data-mode="video"]').click();
            const v = $("preview-video");
            if (v) { v.play().catch(() => {}); }
        });
        onEvent("preview:error", p => {
            setJob("");
            if (!(p && p.canceled)) toast(p && p.message ? p.message : "Lỗi xem trước", "err");
        });
        // v1.3.0: cảnh báo không chặn khi trộn âm A1 lỗi sau khi xuất
        onEvent("export:note", p => {
            if (p && p.message) toast(p.message, "err");
        });
    }

    // ─────────────── Selftest ───────────────
    function bindSelftestEvents() {
        const log = $("selftest-log");
        onEvent("selftest:log", p => {
            log.textContent += (p && p.line ? p.line : "") + "\n";
            log.scrollTop = log.scrollHeight;
        });
        onEvent("selftest:done", p => {
            log.textContent += "\n===> " + (p && p.ok ? "TẤT CẢ ĐẠT ✓" : "CÓ LỖI ✗ — gửi file log ở dưới cho đội phát triển") + "\n";
            if (p && p.logPath) log.textContent += "Log: " + p.logPath + "\n";
            log.scrollTop = log.scrollHeight;
            setJob("");
        });
        $("selftest-run").addEventListener("click", async () => {
            log.textContent = "";
            setJob("Đang tự kiểm tra…");
            try { await call("RunSelfTest"); }
            catch (e) { toast(e.message, "err"); setJob(""); }
        });
        $("selftest-close").addEventListener("click", () => { $("selftest-modal").hidden = true; });
    }

    // ─────────────── Timeline nút ───────────────
    $("btn-split").addEventListener("click", doSplit);
    $("btn-delete").addEventListener("click", doDelete);
    $("btn-back").addEventListener("click", () => moveSel(-1));
    $("btn-fwd").addEventListener("click", () => moveSel(1));

    // ─────────────── v1.2.5: Lưu khung hình PNG ───────────────
    // Lưu khung ĐANG xem trên timeline (clip dưới con trỏ, hoặc clip đang chọn)
    // thành tệp PNG — khớp khung canvas, đã áp zoom/xoay/lật.
    $("btn-snap-png").addEventListener("click", async () => {
        if (State.doc.clips.length === 0) { toast("Timeline trống — không có khung để lưu"); return; }
        const at = State.clipAtTimeline(State.playheadMs) ||
            (State.selection ? { clip: State.findClip(State.selection), localMs: 0 } : null);
        if (!at || !at.clip) { toast("Chưa có clip dưới con trỏ — kéo con trỏ lên một clip"); return; }
        try {
            const p = await call("SaveFramePNG", at.clip.id, Math.round(at.localMs));
            if (p) toast("📷 Đã lưu khung: " + p, "ok");
            // p rỗng = người dùng đã bấm Huỷ trong hộp thoại — không báo
        } catch (e) { toast(e.message, "err"); }
    });

    // ─────────────── v1.2.5: Bảng phím tắt (?) ───────────────
    const keysModal = $("keys-modal");
    $("btn-keys").addEventListener("click", () => { keysModal.hidden = false; });
    $("keys-close").addEventListener("click", () => { keysModal.hidden = true; });
    keysModal.addEventListener("click", e => { if (e.target === keysModal) keysModal.hidden = true; });

    async function doSplit() {
        const c = State.clipAtTimeline(State.playheadMs);
        if (!c) { toast("Đặt con trỏ lên một clip rồi bấm Tách"); return; }
        try {
            await call("SplitClip", c.clip.id, State.playheadMs);
            refresh();
            toast("Đã tách clip", "ok");
        } catch (e) { toast(e.message, "err"); }
    }

    async function doDelete() {
        if (!State.selection) { toast("Chưa chọn gì để xóa — bấm vào clip, khối lớp phủ hoặc đoạn âm thanh trên timeline trước"); return; }
        try {
            // v1.2.6 SỬA LỖI: lựa chọn có thể là CLIP (V1) hoặc LỚP PHỦ (V2) —
            // trước đây luôn gọi RemoveClip → lớp phủ báo "không tìm thấy clip".
            // v1.3.0: thêm nhánh ĐOẠN ÂM THANH A1.
            if (State.findClip(State.selection)) {
                await call("RemoveClip", State.selection);
                State.select(null);
                refresh();
                toast("🗑 Đã xóa clip — các clip phía sau tự dồn vào (ghép trơn tru)", "ok");
            } else if (State.findOverlay(State.selection)) {
                await call("RemoveOverlay", State.selection);
                State.select(null);
                refresh();
                toast("🗑 Đã xóa lớp phủ V2", "ok");
            } else if (State.findAudioClip(State.selection)) {
                await call("RemoveAudioClip", State.selection);
                State.select(null);
                refresh();
                toast("🗑 Đã xóa đoạn âm thanh A1", "ok");
            } else {
                State.select(null);
                toast("Lựa chọn không còn tồn tại trên timeline", "err");
            }
        } catch (e) { toast(e.message, "err"); }
    }

    async function moveSel(dir) {
        if (!State.selection) return;
        const i = State.doc.clips.findIndex(c => c.id === State.selection);
        if (i < 0) return;
        try {
            await call("MoveClip", State.selection, i + dir);
            refresh();
        } catch (e) { toast(e.message, "err"); }
    }

    // ─────────────── Công cụ timeline (v1.2.3) ───────────────
    // "select" = mặc định; "razor" = bấm vào clip là cắt tại chỗ đó.
    window.VKSTool = { current: "select", snapping: true, locked: false };

    document.querySelectorAll("#tl-tools button").forEach(b => {
        b.addEventListener("click", () => {
            window.VKSTool.current = b.dataset.tool;
            document.querySelectorAll("#tl-tools button").forEach(x =>
                x.classList.toggle("active", x === b));
            $("tl-hint").textContent = window.VKSTool.current === "razor"
                ? "🗡 Dao: BẤM VÀO CLIP để cắt đúng chỗ bấm (Ctrl+B = cắt tại con trỏ)"
                : "Kéo viền clip để cắt · kéo thân để dời · Alt+kéo thân = Slip · click timeline để đưa con trỏ";
        });
    });

    $("btn-snap").addEventListener("click", () => {
        window.VKSTool.snapping = !window.VKSTool.snapping;
        $("btn-snap").classList.toggle("active", window.VKSTool.snapping);
        toast(window.VKSTool.snapping ? "🧲 Nam châm BẬT — viền clip tự dính điểm gần nhất" : "Nam châm TẮT", "ok");
    });

    // ─────────────── Công tắc track: 🔒 / 🔇 / 👁 (v1.2.3) ───────────────
    $("btn-tl-lock").addEventListener("click", () => {
        window.VKSTool.locked = !window.VKSTool.locked;
        $("btn-tl-lock").classList.toggle("active", window.VKSTool.locked);
        toast(window.VKSTool.locked ? "🔒 Đã KHÓA timeline — mọi chỉnh sửa bị chặn" : "Đã MỞ khóa timeline", "ok");
    });
    $("btn-tl-mute").addEventListener("click", async () => {
        const nv = !State.doc.muteAll;
        try { await call("SetDocFlags", nv, !!State.doc.hideOverlays); State.doc.muteAll = nv; }
        catch (e) { toast(e.message, "err"); return; }
        $("btn-tl-mute").classList.toggle("active", nv);
        toast(nv ? "🔇 TOÀN BỘ timeline sẽ IM LẶNG khi dựng/xuất" : "Bật lại âm thanh timeline", nv ? "ok" : "ok");
    });
    $("btn-tl-ovis").addEventListener("click", async () => {
        const nv = !State.doc.hideOverlays;
        try { await call("SetDocFlags", !!State.doc.muteAll, nv); State.doc.hideOverlays = nv; }
        catch (e) { toast(e.message, "err"); return; }
        $("btn-tl-ovis").classList.toggle("active", !nv);
        toast(nv ? "👁 ĐÃ ẨN lớp phủ (chữ/hình/PiP) khi dựng" : "Hiện lại lớp phủ", "ok");
    });
    // Đồng bộ trạng thái nút khi doc đổi (mở dự án khác…)
    document.addEventListener("vks:doc-changed", () => {
        $("btn-tl-mute").classList.toggle("active", !!State.doc.muteAll);
        $("btn-tl-ovis").classList.toggle("active", !State.doc.hideOverlays);
    });

    // ─────────────── Thu phóng timeline (v1.2.4) ───────────────
    // Ctrl+lăn chuột trên timeline = nhanh nhất; các nút dưới đây cho chuột thường.
    document.addEventListener("vks:zoom-changed", () => {
        const s = $("zoom-slider");
        if (document.activeElement !== s) s.value = String(Math.round(State.pxPerSec));
    });
    $("zoom-in").addEventListener("click", () => window.VKSTimeline && window.VKSTimeline.zoomBy(1.3));
    $("zoom-out").addEventListener("click", () => window.VKSTimeline && window.VKSTimeline.zoomBy(1 / 1.3));
    $("zoom-fit").addEventListener("click", () => window.VKSTimeline && window.VKSTimeline.zoomFit());

    // Nút nhỏ trên track header V2/A1 — tái dùng đúng logic nút trên thanh công cụ
    $("hd-ovis").addEventListener("click", () => $("btn-tl-ovis").click());
    $("hd-mute").addEventListener("click", () => $("btn-tl-mute").click());
    document.addEventListener("vks:doc-changed", () => {
        $("hd-mute").classList.toggle("active", !!State.doc.muteAll);
        $("hd-ovis").classList.toggle("active", !State.doc.hideOverlays);
    });

    // ─────────────── Transport: về đầu/cuối, In/Out của clip (v1.2.3) ───────────────
    $("btn-tl-start").addEventListener("click", () => State.setPlayhead(0));
    $("btn-tl-end").addEventListener("click", () => State.setPlayhead(State.totalMs()));
    function jumpClipIn() {
        const c = State.selection ? State.findClip(State.selection) : (State.clipAtTimeline(State.playheadMs) || {}).clip;
        if (!c) { toast("Chưa chọn clip"); return; }
        const i = State.doc.clips.findIndex(x => x.id === c.id);
        State.setPlayhead(State.clipStarts()[i]);
    }
    function jumpClipOut() {
        const c = State.selection ? State.findClip(State.selection) : (State.clipAtTimeline(State.playheadMs) || {}).clip;
        if (!c) { toast("Chưa chọn clip"); return; }
        const i = State.doc.clips.findIndex(x => x.id === c.id);
        State.setPlayhead(State.clipStarts()[i] + State.effMs(c));
    }
    $("btn-tl-in").addEventListener("click", jumpClipIn);
    $("btn-tl-out").addEventListener("click", jumpClipOut);
    window.VKSJump = { jumpClipIn, jumpClipOut };

    // ─────────────── Đặt In/Out tại con trỏ (phím I/O) ───────────────
    async function setInAtPlayhead() {
        const at = State.clipAtTimeline(State.playheadMs);
        if (!at) { toast("Đặt con trỏ lên một clip trước"); return; }
        const local = Math.max(0, at.localMs);
        try { await call("UpdateClip", at.clip.id, { inMs: Math.round(local) }); refresh(); toast("Đã đặt ĐẦU clip = con trỏ"); }
        catch (e) { toast(e.message, "err"); }
    }
    async function setOutAtPlayhead() {
        const at = State.clipAtTimeline(State.playheadMs);
        if (!at) { toast("Đặt con trỏ lên một clip trước"); return; }
        const local = Math.max(0, at.localMs);
        try { await call("UpdateClip", at.clip.id, { outMs: Math.round(local) }); refresh(); toast("Đã đặt CUỐI clip = con trỏ"); }
        catch (e) { toast(e.message, "err"); }
    }
    window.VKSInOut = { setInAtPlayhead, setOutAtPlayhead };

    // ─────────────── v1.3.0: THANH ĐIỀU KHIỂN PHÁT (Play/Stop) ───────────────
    // Khung xem trước từ trước đến nay KHÔNG có nút Play/Stop. Giờ có:
    // ⏮ về đầu · ◀ khung trước · ▶/⏸ Phát/Dừng · ▶ khung sau · ⏭ về cuối,
    // kèm đồng hồ «đang phát / tổng». Chế độ «Khung»: phát từng khung render
    // (đã có từ v1.2.3); chế độ «Bản render»: điều khiển video 540p có âm thanh.
    const btnPlay = $("btn-play");
    function setPlayIcon(icon) { if (btnPlay) btnPlay.textContent = icon; }
    function updateTransportTime() {
        const el = $("transport-time");
        if (el) el.textContent = fmtMs(State.playheadMs) + " / " + fmtMs(State.totalMs());
    }
    document.addEventListener("vks:playhead-changed", updateTransportTime);
    document.addEventListener("vks:doc-changed", updateTransportTime);

    function togglePlay() {
        if (State.previewMode === "video") {
            const v = $("preview-video");
            if (v.paused) { v.play().catch(() => {}); } else { v.pause(); }
            return;
        }
        if (framePlay) { stopFramePlay(); setPlayIcon("▶"); toast("⏸ Đã dừng phát khung"); }
        else { startFramePlay(); }
    }
    if (btnPlay) btnPlay.addEventListener("click", togglePlay);

    function stepFrame(dir) {
        const fps = Math.max(1, (State.doc.canvas.rate.num || 30) / (State.doc.canvas.rate.den || 1));
        State.setPlayhead(State.playheadMs + dir * (1000 / fps));
    }
    const bf = $("btn-prev-frame"), bnx = $("btn-next-frame");
    if (bf) bf.addEventListener("click", () => stepFrame(-1));
    if (bnx) bnx.addEventListener("click", () => stepFrame(1));
    const bst = $("btn-tl-start-t"), ben = $("btn-tl-end-t");
    if (bst) bst.addEventListener("click", () => State.setPlayhead(0));
    if (ben) ben.addEventListener("click", () => State.setPlayhead(State.totalMs()));
    // nhãn chế độ trên thanh transport (khung | bản render)
    document.getElementById("preview-mode").addEventListener("click", () => {
        const m = $("transport-mode");
        if (m) m.textContent = State.previewMode === "video" ? "bản render" : "khung";
    });

    // Đồng bộ video «Bản render» ⇔ con trỏ timeline
    const pvEl = $("preview-video");
    if (pvEl) {
        pvEl.addEventListener("play", () => setPlayIcon("⏸"));
        pvEl.addEventListener("pause", () => setPlayIcon("▶"));
        pvEl.addEventListener("ended", () => setPlayIcon("▶"));
        pvEl.addEventListener("timeupdate", () => {
            if (State.previewMode === "video" && !pvEl.seeking) {
                const ms = pvEl.currentTime * 1000;
                if (Math.abs(ms - State.playheadMs) > 60) State.setPlayhead(ms);
            }
        });
    }
    let pvSeeking = false;
    document.addEventListener("vks:playhead-changed", () => {
        if (State.previewMode === "video" && pvEl && pvEl.src && !pvSeeking) {
            const want = State.playheadMs / 1000;
            if (Math.abs(pvEl.currentTime - want) > 0.45) {
                pvSeeking = true;
                try { pvEl.currentTime = want; } catch (_) { /* chưa nạp xong */ }
                pvEl.addEventListener("seeked", () => { pvSeeking = false; }, { once: true });
                setTimeout(() => { pvSeeking = false; }, 1200); // phòng ngừa sự kiện seeked không tới
            }
        }
    });

    // ─────────────── Phát khung từng bước trong chế độ "Khung" (v1.2.3) ───────────────
    let framePlay = null;
    function stopFramePlay() {
        if (framePlay) { clearTimeout(framePlay); framePlay = null; }
        setPlayIcon("▶");
    }
    function startFramePlay() {
        stopFramePlay();
        if (State.previewMode === "video") {
            // đang ở chế độ bản render — chuyển phát video thay vì phát khung
            if (pvEl && pvEl.paused) pvEl.play().catch(() => {});
            return;
        }
        setPlayIcon("⏸");
        const fps = Math.max(1, Math.min(30, (State.doc.canvas.rate.num / (State.doc.canvas.rate.den || 1))));
        const tick = () => {
            if (State.previewMode !== "frame") { stopFramePlay(); return; }
            const next = State.playheadMs + 1000 / fps;
            if (next >= State.totalMs()) { State.setPlayhead(0); }
            else { State.setPlayhead(next); }
            framePlay = setTimeout(tick, 1000 / fps); // chạy tiếp; khung render chậm sẽ tự trôi
        };
        framePlay = setTimeout(tick, 1000 / fps);
        toast("▶ Phát (khung) — nhấn Space hoặc ⏸ để dừng");
    }

    // ─────────────── Source Monitor: xem tư liệu gốc (v1.2.3) ───────────────
    const srcmonModal = $("srcmon-modal");
    $("srcmon-close").addEventListener("click", () => { srcmonModal.hidden = true; $("srcmon-stage").innerHTML = ""; });
    srcmonModal.addEventListener("click", e => { if (e.target === srcmonModal) { srcmonModal.hidden = true; $("srcmon-stage").innerHTML = ""; } });
    async function openSourceMonitor(assetId) {
        const a = State.findAsset(assetId);
        if (!a) return;
        try {
            const url = await call("GetAssetSourceURL", assetId);
            const stage = $("srcmon-stage");
            if (a.kind === "image") {
                stage.innerHTML = '<img src="' + url + '?v=' + Date.now() + '" style="max-width:100%;max-height:56vh;border-radius:8px" alt="' + a.name + '">';
            } else if (a.kind === "audio") {
                stage.innerHTML = '<div style="font-size:64px">🎵</div><audio src="' + url + '?v=' + Date.now() + '" controls style="width:100%"></audio>';
            } else {
                stage.innerHTML = '<video src="' + url + '?v=' + Date.now() + '" controls autoplay style="max-width:100%;max-height:56vh;border-radius:8px"></video>';
            }
            $("srcmon-info").textContent = a.name + " · " + a.width + "×" + a.height +
                (a.kind !== "image" ? " · " + fmtMs(a.durationMs) : "") + " · " + fmtBytes(a.sizeBytes);
            srcmonModal.hidden = false;
        } catch (e) { toast(e.message, "err"); }
    }
    window.VKSSourceMonitor = { open: openSourceMonitor };

    // ─────────────── Phím tắt ───────────────
    document.addEventListener("keydown", e => {
        if (e.target.tagName === "INPUT" || e.target.tagName === "SELECT" || e.target.tagName === "TEXTAREA") return;
        switch (e.key) {
            case "Delete":
            case "Backspace":
                doDelete();
                break;
            case "s": case "S":
                doSplit();
                break;
            case "v": case "V": // v1.2.3: công cụ chọn
                document.querySelector('#tl-tools [data-tool="select"]').click();
                break;
            case "c": case "C": // v1.2.3: công cụ dao
                document.querySelector('#tl-tools [data-tool="razor"]').click();
                break;
            case "b": case "B": // v1.2.3: Ctrl+B = cắt tại con trỏ (chuẩn Premiere)
                if (e.ctrlKey || e.metaKey) { e.preventDefault(); doSplit(); }
                break;
            case "i": case "I": // v1.2.3
                if (e.shiftKey) window.VKSJump.jumpClipIn();
                else window.VKSInOut.setInAtPlayhead();
                break;
            case "o": case "O": // v1.2.3
                if (e.shiftKey) window.VKSJump.jumpClipOut();
                else window.VKSInOut.setOutAtPlayhead();
                break;
            case "l": case "L": // v1.2.3: Ctrl+L = tách âm thanh (Link/Unlink)
                if (e.ctrlKey || e.metaKey) {
                    e.preventDefault();
                    if (!State.selection) { toast("Chưa chọn clip"); break; }
                    (async () => {
                        try {
                            const as = await call("DetachAudio", State.selection);
                            refresh();
                            toast("🔗 Đã tách âm thanh «" + (as && as.name ? as.name : "?") + "» — clip gốc tắt tiếng", "ok");
                        } catch (err) { toast(err.message, "err"); }
                    })();
                }
                break;
            case " ": // v1.3.0: Space = Phát/Dừng theo chế độ xem hiện tại
                e.preventDefault();
                if (!previewVideoHidden() && State.previewMode === "video") {
                    togglePlay();
                } else if (e.shiftKey) {
                    $("btn-preview").click();
                } else {
                    togglePlay();
                }
                break;
            case "ArrowLeft": {
                // v1.3.1: đang chọn LỚP PHỦ → mũi tên = xê dịch lớp (kiểu KineMaster)
                const ov = State.selection ? State.findOverlay(State.selection) : null;
                if (ov) { e.preventDefault(); nudgeOverlay(ov, (e.shiftKey ? -0.005 : -0.02), 0); break; }
                const stepMs = e.shiftKey ? 1000 : (1000 / (State.doc.canvas.rate.num / State.doc.canvas.rate.den));
                State.setPlayhead(State.playheadMs - stepMs);
                break;
            }
            case "ArrowRight": {
                const ov = State.selection ? State.findOverlay(State.selection) : null;
                if (ov) { e.preventDefault(); nudgeOverlay(ov, (e.shiftKey ? 0.005 : 0.02), 0); break; }
                const stepMs = e.shiftKey ? 1000 : (1000 / (State.doc.canvas.rate.num / State.doc.canvas.rate.den));
                State.setPlayhead(State.playheadMs + stepMs);
                break;
            }
            case "ArrowUp": { // v1.3.1: xê dịch lớp phủ theo phương dọc
                const ov = State.selection ? State.findOverlay(State.selection) : null;
                if (ov) { e.preventDefault(); nudgeOverlay(ov, 0, (e.shiftKey ? -0.005 : -0.02)); }
                break;
            }
            case "ArrowDown": {
                const ov = State.selection ? State.findOverlay(State.selection) : null;
                if (ov) { e.preventDefault(); nudgeOverlay(ov, 0, (e.shiftKey ? 0.005 : 0.02)); }
                break;
            }
            case "Home": State.setPlayhead(0); break;
            case "End": State.setPlayhead(State.totalMs()); break;
            case "?": // v1.2.5: bảng phím tắt (Shift+/ sinh "?")
            case "F1":
                e.preventDefault();
                $("keys-modal").hidden = false;
                break;
            case "Escape": // v1.2.5: đóng mọi modal đang mở
                document.querySelectorAll(".modal-mask:not([hidden])").forEach(m => { m.hidden = true; });
                break;
        }
    });
    function previewVideoHidden() { return $("preview-video").hidden; }

    // v1.3.1: xê dịch lớp phủ bằng phím mũi tên (dời nhẹ 2%, Shift = 0.5%)
    let nudgeTimer = null, nudgePending = null;
    function nudgeOverlay(ov, dx, dy) {
        ov.posX = Math.max(0, Math.min(1, (ov.posX ?? 0.5) + dx));
        ov.posY = Math.max(0, Math.min(1, (ov.posY ?? 0.5) + dy));
        nudgePending = { id: ov.id, posX: ov.posX, posY: ov.posY };
        if (window.VKSPanels) window.VKSPanels.renderOverlayPreview();
        clearTimeout(nudgeTimer);
        nudgeTimer = setTimeout(async () => { // gom phím liên tiếp thành 1 lần lưu
            const p = nudgePending; nudgePending = null;
            if (!p) return;
            try { await call("UpdateOverlay", p.id, { posX: p.posX, posY: p.posY }); refresh(); }
            catch (err) { toast(err.message, "err"); refresh(); }
        }, 350);
    }

    // Cập nhật tổng thời lượng khi doc đổi
    document.addEventListener("vks:doc-changed", () => {
        $("tl-total-time").textContent = fmtMs(State.totalMs());
    });

    // Phơi ra ngoài cho panels.js dùng
    window.VKSApp = { toast };

    boot();
})();
