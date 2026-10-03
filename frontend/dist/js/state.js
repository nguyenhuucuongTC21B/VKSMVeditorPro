/* state.js — trạng thái ứng dụng phía giao diện (bản sao của Document Go).
   v1.1: thời lượng hiệu dụng (tốc độ/boomerang/lặp/freeze) + lớp phủ. */
(function () {
    "use strict";

    const { fmtMs } = window.VKSApi;

    const State = {
        doc: {
            name: "Dự án không tên",
            canvas: { width: 1920, height: 1080, rate: { num: 30, den: 1 } },
            assets: [],
            clips: [],
            transitions: [],
            overlays: [],
        },
        selection: null,     // clipId đang chọn
        playheadMs: 0,       // vị trí con trỏ trên timeline
        pxPerSec: 48,        // độ zoom timeline
        ZOOM_MIN: 4,         // v1.2.4: giới hạn zoom timeline
        ZOOM_MAX: 400,
        previewMode: "frame", // "frame" | "video"
        previewURL: "",
        previewDurationMs: 0,
        dirty: false,

        /** v1.1: thời lượng hiệu dụng của clip trên timeline (khớp Go EffectiveMs).
            v1.2.1: chống NaN — dữ liệu lạ trả về 0 thay vì làm chết canvas. */
        effMs(c) {
            const base = Math.max(0, (Number(c.outMs) || 0) - (Number(c.inMs) || 0));
            const speed = Number(c.speed) > 0 ? Number(c.speed) : 1;
            let eff = base / speed;
            if (c.boomerang) eff *= 2;
            else if (Number(c.loopN) > 1) eff *= Number(c.loopN);
            eff += (Number(c.freezeStartMs) || 0) + (Number(c.freezeEndMs) || 0);
            if (!isFinite(eff) || eff < 0) eff = 0;
            return Math.max(0, Math.round(eff));
        },

        /** Tổng thời lượng timeline (đã trừ overlap chuyển cảnh, dùng thời lượng hiệu dụng). */
        totalMs() {
            let t = 0;
            for (const c of this.doc.clips) t += this.effMs(c);
            for (const tr of (Array.isArray(this.doc.transitions) ? this.doc.transitions : [])) {
                t -= (Number(tr && tr.durationMs) || 0);
            }
            if (!isFinite(t) || t < 0) t = 0;
            return Math.max(0, t);
        },

        /** Mốc thời gian bắt đầu của từng clip (song song với clips). */
        clipStarts() {
            const out = [];
            let cur = 0;
            this.doc.clips.forEach((c, i) => {
                out.push(cur);
                cur += this.effMs(c);
                const tr = i < this.doc.clips.length - 1
                    ? this.doc.transitions.find(t => t.afterClipId === c.id) : null;
                if (tr) cur -= Math.min(tr.durationMs, this.effMs(c));
            });
            return out;
        },

        /** v1.3.0: tìm đoạn âm thanh A1 theo ID. */
        findAudioClip(id) { return (this.doc.audioClips || []).find(a => a.id === id) || null; },

        /** v1.3.0: mốc kết thúc xa nhất của track A1 (0 nếu trống). */
        audioEndMs() {
            let end = 0;
            for (const a of (this.doc.audioClips || [])) {
                const e = (Number(a.startMs) || 0) + Math.max(0, (Number(a.outMs) || 0) - (Number(a.inMs) || 0));
                if (e > end) end = e;
            }
            return end;
        },

        findAsset(id) { return this.doc.assets.find(a => a.id === id) || null; },
        findClip(id) { return this.doc.clips.find(c => c.id === id) || null; },
        transitionAfter(clipId) { return this.doc.transitions.find(t => t.afterClipId === clipId) || null; },
        findOverlay(id) { return (this.doc.overlays || []).find(o => o.id === id) || null; },

        /** v1.2.4: lớp phủ tại mốc thời gian — dùng khi cần biết V2 có gì tại đó. */
        overlayAt(ms) {
            for (const o of (this.doc.overlays || [])) {
                if (ms >= o.startMs && ms < o.endMs) return { ov: o };
            }
            return null;
        },

        /** Clip tại vị trí timeline (trả {clip, index, localMs}). */
        clipAtTimeline(ms) {
            const starts = this.clipStarts();
            for (let i = this.doc.clips.length - 1; i >= 0; i--) {
                const c = this.doc.clips[i];
                const end = starts[i] + this.effMs(c);
                if (ms >= starts[i] && ms < end) {
                    return { clip: c, index: i, localMs: c.inMs + (ms - starts[i]) };
                }
            }
            if (this.doc.clips.length && ms >= this.totalMs()) {
                const c = this.doc.clips[this.doc.clips.length - 1];
                return { clip: c, index: this.doc.clips.length - 1, localMs: c.outMs };
            }
            return null;
        },

        /** Thay thế doc từ backend và báo UI vẽ lại.
            v1.2.1: chuẩn hoá dữ liệu — chặn null/NaN từ dự án cũ làm chết canvas. */
        setDoc(doc) {
            this.doc = doc || this.doc;
            const d = this.doc;
            const num = (v, fb) => { const n = Number(v); return isFinite(n) ? n : fb; };
            if (!Array.isArray(d.assets)) d.assets = [];
            if (!Array.isArray(d.clips)) d.clips = [];
            if (!Array.isArray(d.transitions)) d.transitions = [];
            if (!Array.isArray(d.overlays)) d.overlays = [];
            // v1.3.0: track âm thanh A1
            if (!Array.isArray(d.audioClips)) d.audioClips = [];
            for (const ac of d.audioClips) {
                ac.startMs = Math.max(0, num(ac.startMs, 0));
                ac.inMs = Math.max(0, num(ac.inMs, 0));
                ac.outMs = Math.max(0, num(ac.outMs, 0));
                if (ac.outMs <= ac.inMs) ac.outMs = ac.inMs + 1000;
                const vol = Number(ac.volume);
                ac.volume = (isNaN(vol) || vol <= 0) ? 1 : Math.min(2, vol);
                ac.mute = !!ac.mute;
                ac.fadeInMs = Math.max(0, num(ac.fadeInMs, 0));
                ac.fadeOutMs = Math.max(0, num(ac.fadeOutMs, 0));
            }
            for (const c of d.clips) {
                c.inMs = Math.max(0, num(c.inMs, 0));
                c.outMs = Math.max(0, num(c.outMs, 0));
                if (c.outMs < c.inMs) c.outMs = c.inMs;
                c.speed = num(c.speed, 1) > 0 ? num(c.speed, 1) : 1;
                c.volume = isFinite(Number(c.volume)) ? Math.min(2, Math.max(0, Number(c.volume))) : 1;
                c.loopN = num(c.loopN, 1) > 0 ? Math.round(num(c.loopN, 1)) : 1;
                c.freezeStartMs = Math.max(0, num(c.freezeStartMs, 0));
                c.freezeEndMs = Math.max(0, num(c.freezeEndMs, 0));
                if (!c.transform || typeof c.transform !== "object") c.transform = {};
                // v1.2.3: pan vị trí — 0/thiếu = giữa khung (dự án cũ).
                const pos = v => { const n = Number(v); return (isNaN(n) || n <= 0 || n > 1) ? 0.5 : n; };
                c.transform.posX = pos(c.transform.posX);
                c.transform.posY = pos(c.transform.posY);
                const op = Number(c.opacity);
                c.opacity = (isNaN(op) || op <= 0 || op > 1) ? 1 : op; // 0/thiếu = đặc
                if (!c.effects || typeof c.effects !== "object") c.effects = {};
                c.effects.audioDenoise = !!c.effects.audioDenoise;
                c.effects.vocalEnhance = !!c.effects.vocalEnhance;
            }
            // v1.2.4: chuẩn hóa lớp phủ (V2) — chống dữ liệu lạ làm chết canvas
            for (const o of d.overlays) {
                o.startMs = Math.max(0, num(o.startMs, 0));
                o.endMs = Math.max(0, num(o.endMs, 3000));
                if (o.endMs <= o.startMs) o.endMs = o.startMs + 3000;
                if (!o.assetId) o.assetId = "";
                const sp = Number(o.scalePct);
                o.scalePct = (isNaN(sp) || sp <= 0 || sp > 100) ? 30 : sp;
            }
            // Công tắc track v1.2.3 (v1.2.4: đưa RA KHỎI vòng lặp clips — bug cũ)
            d.muteAll = !!d.muteAll;
            d.hideOverlays = !!d.hideOverlays;
            this.dirty = false;
            // v1.3.0: giữ lựa chọn cho clip / lớp phủ V2 / đoạn âm thanh A1
            if (this.selection && !this.findClip(this.selection) && !this.findOverlay(this.selection) && !this.findAudioClip(this.selection)) this.selection = null;
            document.dispatchEvent(new CustomEvent("vks:doc-changed"));
        },

        select(clipId) {
            this.selection = clipId;
            document.dispatchEvent(new CustomEvent("vks:selection-changed"));
        },

        setPlayhead(ms) {
            const total = this.totalMs();
            this.playheadMs = Math.max(0, Math.min(ms, total));
            document.dispatchEvent(new CustomEvent("vks:playhead-changed"));
        },
    };

    window.VKSState = State;
})();
