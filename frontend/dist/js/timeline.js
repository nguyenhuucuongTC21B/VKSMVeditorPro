/* timeline.js — vẽ và tương tác timeline trên canvas.
   v1.3.1 — NHIỀU LỚP tự động kiểu KineMaster/CapCut:
   · V2 tách thành N lane tự động khi các lớp phủ chồng thời gian (V2.1, V2.2…)
   · A1 tách thành N lane khi các đoạn nhạc chồng nhau (A1.1, A1.2…)
   · Badge chế độ lớp ngay trên khối: hòa trộn · độ mờ · preset · tách nền
   v1.3.0 — Track âm thanh ĐỘC LẬP kiểu KineMaster:
   · A1 thành track THẬT: kéo mp3/wav vào → đoạn nhạc riêng, dời/cắt tự do,
     song song với video (tắt tiếng video gốc để dùng nhạc chèn)
   · Kéo/trim đoạn A1 + nam châm · bỏ hình chiếu cũ (đã gây hiểu nhầm)
   v1.2.4 — Timeline Pro kiểu CapCut/KineMaster:
   · 3 track: V2 (lớp phủ PiP/chữ/hình — vị trí tự do) · V1 (video chính) · A1
   · Ctrl+lăn chuột = thu phóng quanh con trỏ · lăn thường = cuộn ngang
   · Kéo-thả tư liệu: thả vào V1 = chèn clip · thả vào V2 = lớp phủ PiP
   Tương tác giữ từ v1.2.3: chọn clip · kéo thân dời · kéo viền cắt in/out ·
   Alt+kéo = Slip · dao cắt · nam châm · khóa · thước đưa con trỏ. */
(function () {
    "use strict";

    const { call, fmtMs } = window.VKSApi;
    const State = window.VKSState;

    const canvas = document.getElementById("timeline-canvas");
    const scroll = document.getElementById("tl-scroll");
    const ctx = canvas.getContext("2d");

    // ─── Kích thước track (v1.3.1: số lane V2/A1 động theo chồng lấn) ───
    const RULER_H = 26;
    const OV_H = 48;    // mỗi lane lớp phủ
    const MAIN_H = 88;  // V1 — track chính
    const AUD_H = 56;   // mỗi lane âm thanh
    const GAP = 2;
    const EDGE = 8;         // px vùng bắt viền để cắt/trim
    const OV_EDGE = 6;      // px vùng bắt viền lớp phủ
    // v1.3.1: layout động — relayout() tính lại trước mỗi lần vẽ/hit-test
    let yV2 = RULER_H + GAP;
    let yV1 = yV2 + OV_H + GAP;
    let yAUD = yV1 + MAIN_H + GAP;
    let CONTENT_H = yAUD + AUD_H + 6;
    let ovLaneCount = 1, audLaneCount = 1;
    const ovLaneOf = new Map(), audLaneOf = new Map(); // id → số lane (0..)

    // Đóng gói các mục chồng thời gian vào các lane khác nhau (greedy interval
    // packing) — chỉ phục vụ HIỂN THỊ, không đổi model: dữ liệu vẫn là 1 danh
    // sách, render/xuất video vốn xử lý mọi lớp chồng nhau sẵn.
    function packLanes(items, map, endOf) {
        map.clear();
        if (!items || !items.length) return 1;
        const ends = [];
        items.slice().sort((a, b) => a.startMs - b.startMs).forEach(it => {
            let L = 0;
            while (L < ends.length && it.startMs < ends[L]) L++;
            if (L === ends.length) ends.push(0);
            map.set(it.id, L);
            ends[L] = endOf(it) + 1;
        });
        return ends.length;
    }
    function relayout() {
        ovLaneCount = packLanes(State.doc.overlays || [], ovLaneOf, o => o.endMs);
        audLaneCount = packLanes(State.doc.audioClips || [], audLaneOf,
            ac => ac.startMs + Math.max(100, (ac.outMs || 0) - (ac.inMs || 0)));
        yV2 = RULER_H + GAP;
        yV1 = yV2 + ovLaneCount * (OV_H + GAP) + GAP;
        yAUD = yV1 + MAIN_H + GAP;
        CONTENT_H = yAUD + audLaneCount * (AUD_H + GAP) + 6;
    }
    const ovLaneY = L => yV2 + L * (OV_H + GAP);
    const audLaneY = L => yAUD + L * (AUD_H + GAP);
    const ovBandH = () => ovLaneCount * (OV_H + GAP) - GAP;
    const audBandH = () => audLaneCount * (AUD_H + GAP) - GAP;

    const OV_COLORS = {
        text:  { bg: "#5c4a18", border: "#ffb800", icon: "T" },
        shape: { bg: "#1d4433", border: "#34c77b", icon: "◇" },
        media: { bg: "#1e3a52", border: "#4aa3e0", icon: "▣" },
    };

    const thumbCache = new Map();
    let drag = null;
    let deviceRatio = 1;
    let dropX = null, dropTrack = null; // chỉ báo kéo-thả (v1.2.4: biết track đích)

    // ───────────────────────── Kích thước ─────────────────────────
    function resize() {
        const rect = scroll.getBoundingClientRect();
        deviceRatio = window.devicePixelRatio || 1;
        relayout(); // v1.3.1: số lane có thể đổi → tính lại chiều cao canvas
        // v1.3.0: chiều rộng tính theo cả đuôi track âm thanh A1
        const total = Math.max(State.totalMs() || 0, State.audioEndMs() || 0);
        const want = (total / 1000) * State.pxPerSec + 220;
        const contentPx = Math.max(
            rect.width || 1,
            isFinite(want) ? want : (rect.width || 1)
        );
        const hPx = Math.max(CONTENT_H, Math.round(rect.height || 1));
        canvas.width = Math.max(1, Math.round(contentPx * deviceRatio));
        canvas.height = Math.max(1, Math.round(hPx * deviceRatio));
        canvas.style.width = contentPx + "px";
        canvas.style.height = hPx + "px";
        draw();
    }

    // ───────────────────────── Thumbnail ─────────────────────────
    function thumbURL(assetId, tMs) {
        return "/local/thumbs/" + encodeURIComponent(assetId) + "_" + tMs + ".jpg";
    }

    function ensureThumb(asset, tMs, cb) {
        const key = asset.id + "_" + tMs;
        if (thumbCache.has(key)) { cb(thumbCache.get(key)); return; }
        const url = thumbURL(asset.id, tMs);
        fetch(url).then(r => {
            if (!r.ok) return call("GetAssetThumb", asset.id, tMs);
            return url;
        }).then(u => {
            if (!u) { cb(null); return; }
            const img = new Image();
            img.onload = () => { thumbCache.set(key, img); cb(img); };
            img.onerror = () => cb(null);
            img.src = u;
        }).catch(() => cb(null));
    }

    // ───────────────────────── Vẽ ─────────────────────────
    function xOf(ms) { return (ms / 1000) * State.pxPerSec; }

    function draw() {
        try {
            relayout(); // v1.3.1: tính lane trước khi vẽ
            updateTrackHeads();
            const w = canvas.width / deviceRatio, h = canvas.height / deviceRatio;
            ctx.setTransform(deviceRatio, 0, 0, deviceRatio, 0, 0);
            ctx.clearRect(0, 0, w, h);

            ctx.fillStyle = "#121216";
            ctx.fillRect(0, 0, w, h);

            drawRuler(w);
            drawTrackBands(w, h);
            drawClips();
            drawTransitions();
            drawOverlayTrack(w, h);
            drawAudioTrack(w, h);
            drawPlayhead(h);
            drawDropIndicator();
        } catch (e) {
            console.warn("[VKS] timeline draw lỗi (đã bỏ qua):", e);
        }
    }

    function drawRuler(w) {
        ctx.fillStyle = "#17171c";
        ctx.fillRect(0, 0, w, RULER_H);
        ctx.strokeStyle = "#2a2a34";
        ctx.beginPath();
        ctx.moveTo(0, RULER_H - 0.5); ctx.lineTo(w, RULER_H - 0.5);
        ctx.stroke();

        const candidates = [100, 200, 500, 1000, 2000, 5000, 10000, 30000, 60000];
        let step = candidates[candidates.length - 1];
        for (const c of candidates) {
            if (c / 1000 * State.pxPerSec >= 68) { step = c; break; }
        }
        const total = State.totalMs();
        const endMs = Math.max(total, (scroll.clientWidth / State.pxPerSec) * 1000) + 5000;
        ctx.font = "10px " + getComputedStyle(document.body).fontFamily;
        ctx.textBaseline = "top";
        for (let ms = 0; ms <= endMs; ms += step) {
            const x = xOf(ms);
            ctx.strokeStyle = "#3a3a45";
            ctx.beginPath();
            ctx.moveTo(x + 0.5, RULER_H - 8); ctx.lineTo(x + 0.5, RULER_H - 1);
            ctx.stroke();
            ctx.fillStyle = "#8a8a96";
            ctx.fillText(fmtMs(ms).replace(/\.0$/, ""), x + 4, 4);
            if (State.pxPerSec >= 40) {
                for (let sub = step / 5; sub < step; sub += step / 5) {
                    const sx = xOf(ms + sub);
                    ctx.strokeStyle = "#2e2e38";
                    ctx.beginPath();
                    ctx.moveTo(sx + 0.5, RULER_H - 5); ctx.lineTo(sx + 0.5, RULER_H - 1);
                    ctx.stroke();
                }
            }
        }
    }

    // 3 dải track với màu nền phân biệt + nhãn mờ khi chưa có nội dung
    function drawTrackBands(w, h) {
        // V2 (v1.3.1: chiều cao động theo số lane)
        ctx.fillStyle = "#10131a";
        ctx.fillRect(0, yV2, w, ovBandH());
        // V1
        ctx.fillStyle = "#101014";
        ctx.fillRect(0, yV1, w, MAIN_H);
        // đường kẻ mờ dọc theo giây trên V1
        ctx.strokeStyle = "#1b1b21";
        const total = State.totalMs();
        if (State.pxPerSec >= 24) {
            for (let ms = 0; ms <= total; ms += 1000) {
                const x = xOf(ms);
                ctx.beginPath();
                ctx.moveTo(x + 0.5, yV1); ctx.lineTo(x + 0.5, yV1 + MAIN_H);
                ctx.stroke();
            }
        }
        // A1 (v1.3.1: chiều cao động theo số lane)
        ctx.fillStyle = "#0e1210";
        ctx.fillRect(0, yAUD, w, audBandH());
        // viền phân cách
        ctx.strokeStyle = "#202028";
        for (const y of [yV2 - 0.5, yV1 - 0.5, yAUD - 0.5, yAUD + audBandH() + 0.5]) {
            ctx.beginPath();
            ctx.moveTo(0, y); ctx.lineTo(w, y);
            ctx.stroke();
        }
        // gợi ý khi trống
        if (!State.doc.clips.length) {
            ctx.fillStyle = "#4a4a55";
            ctx.font = "12px " + getComputedStyle(document.body).fontFamily;
            ctx.textBaseline = "middle";
            ctx.fillText("Kéo tư liệu từ thư viện vào đây — hoặc thả vào V2 để làm lớp phủ PiP", 14, yV1 + MAIN_H / 2);
        }
        // v1.3.0: gợi ý track A1 khi trống
        if (!(State.doc.audioClips || []).length) {
            ctx.fillStyle = "#3d5c49";
            ctx.font = "12px " + getComputedStyle(document.body).fontFamily;
            ctx.textBaseline = "middle";
            ctx.fillText("🎵 Kéo tệp mp3/wav vào đây → nhạc nền ĐỘC LẬP song song video (tắt tiếng video gốc nếu muốn)", 14, yAUD + AUD_H / 2);
        }
    }

    function drawClips() {
        const starts = State.clipStarts();
        State.doc.clips.forEach((c, i) => {
            const asset = State.findAsset(c.assetId);
            const eff = State.effMs(c);
            const x = xOf(starts[i]);
            const wpx = Math.max(6, xOf(eff));
            const y = yV1;
            const selected = State.selection === c.id;

            const grad = ctx.createLinearGradient(0, y, 0, y + MAIN_H);
            if (selected) { grad.addColorStop(0, "#2c4a6e"); grad.addColorStop(1, "#223a57"); }
            else { grad.addColorStop(0, "#23303d"); grad.addColorStop(1, "#1b2530"); }
            ctx.fillStyle = grad;
            roundRect(x, y, wpx, MAIN_H, 7);
            ctx.fill();
            ctx.strokeStyle = selected ? "#ff5c38" : "#31404f";
            ctx.lineWidth = selected ? 2 : 1;
            roundRect(x + 0.5, y + 0.5, wpx - 1, MAIN_H - 1, 7);
            ctx.stroke();
            ctx.lineWidth = 1;

            if (asset && asset.kind === "image") {
                ctx.save();
                ctx.setLineDash([5, 4]);
                ctx.strokeStyle = "#5a7d9e";
                roundRect(x + 1.5, y + 1.5, wpx - 3, MAIN_H - 3, 6);
                ctx.stroke();
                ctx.restore();
            }

            ctx.save();
            ctx.beginPath();
            roundRect(x, y, wpx, MAIN_H, 7);
            ctx.clip();

            if (asset && asset.kind !== "audio" && wpx > 24) {
                const n = Math.min(3, Math.max(1, Math.floor(wpx / 90)));
                const per = wpx / n;
                for (let k = 0; k < n; k++) {
                    const localMs = c.inMs + ((c.outMs - c.inMs) * (k + 0.5)) / n;
                    ensureThumb(asset, Math.round(localMs), img => {
                        if (!img) return;
                        ctx.save();
                        ctx.beginPath();
                        roundRect(x, y, wpx, MAIN_H, 7);
                        ctx.clip();
                        ctx.globalAlpha = 0.85;
                        const ih = MAIN_H - 22;
                        const iw = img.width * (ih / img.height);
                        ctx.drawImage(img, x + k * per, y + 22, Math.max(per, iw), ih);
                        ctx.restore();
                    });
                }
            } else if (asset && asset.kind === "audio") {
                drawFakeWave(x, y, wpx, MAIN_H, "#3f7d5a");
            }

            ctx.fillStyle = "rgba(10,10,14,.72)";
            ctx.fillRect(x, y, wpx, 22);
            ctx.fillStyle = selected ? "#ffd9cf" : "#cfd6dd";
            ctx.font = "11px " + getComputedStyle(document.body).fontFamily;
            ctx.textBaseline = "middle";
            const name = asset ? asset.name : "?";
            ctx.fillText(clipLabel(name, c), x + 8, y + 11, wpx - 14);
            ctx.fillStyle = "#9aa5b0";
            ctx.fillText(fmtMs(eff) + (c.mute ? "  🔇" : ""), x + 8, y + MAIN_H - 12, wpx - 14);
            ctx.restore();

            if (selected) {
                ctx.fillStyle = "#ff5c38";
                ctx.fillRect(x, y, 3, MAIN_H);
                ctx.fillRect(x + wpx - 3, y, 3, MAIN_H);
            }
        });
    }

    function clipLabel(name, c) {
        let tag = "";
        if (c.speed && c.speed !== 1) tag += " " + c.speed + "×";
        if (c.boomerang) tag += " ⇄";
        else if (c.reverse) tag += " ◀◀";
        if (c.loopN > 1) tag += " ×" + c.loopN;
        return name + "  [" + fmtMs(c.inMs) + " → " + fmtMs(c.outMs) + "]" + tag;
    }

    // ─── Track V2: lớp phủ PiP/chữ/hình — NHIỀU LANE tự động (v1.3.1) ───
    function drawOverlayTrack(w, h) {
        for (const o of (State.doc.overlays || [])) {
            const ly = ovLaneY(ovLaneOf.get(o.id) || 0);
            const x = xOf(o.startMs);
            const wpx = Math.max(8, xOf(o.endMs - o.startMs));
            const col = OV_COLORS[o.kind] || OV_COLORS.text;
            const selected = State.selection === o.id;
            ctx.fillStyle = col.bg;
            roundRect(x, ly + 2, wpx, OV_H - 4, 6);
            ctx.fill();
            ctx.strokeStyle = selected ? "#ff5c38" : col.border;
            ctx.lineWidth = selected ? 2 : 1;
            roundRect(x + 0.5, ly + 2.5, wpx - 1, OV_H - 5, 6);
            ctx.stroke();
            ctx.lineWidth = 1;

            ctx.save();
            ctx.beginPath();
            ctx.rect(x, ly, wpx, OV_H);
            ctx.clip();
            // Thumbnail cho lớp phủ media
            if (o.kind === "media" && o.assetId && wpx > 30) {
                const a = State.findAsset(o.assetId);
                if (a && a.kind !== "audio") {
                    const mid = Math.round(((o.startMs + o.endMs) / 2));
                    ensureThumb(a, mid, img => {
                        if (!img) return;
                        ctx.save();
                        ctx.beginPath();
                        ctx.rect(x, ly, wpx, OV_H);
                        ctx.clip();
                        ctx.globalAlpha = 0.45;
                        const ih = OV_H - 8;
                        const iw = img.width * (ih / img.height);
                        ctx.drawImage(img, x + wpx - iw - 2, ly + 4, iw, ih);
                        ctx.restore();
                    });
                }
            }
            const a2 = o.kind === "media" ? State.findAsset(o.assetId) : null;
            const label = col.icon + " " + (a2 ? a2.name : overlayLabel(o));
            ctx.fillStyle = "#e8e8ee";
            ctx.font = "10px " + getComputedStyle(document.body).fontFamily;
            ctx.textBaseline = "middle";
            ctx.fillText(label, x + 6, ly + OV_H / 2 - 6);
            ctx.fillStyle = "#9aa5b0";
            ctx.fillText(fmtMs(o.endMs - o.startMs), x + 6, ly + OV_H / 2 + 8);
            // v1.3.1: badge chế độ lớp ngay trên khối (giống CapCut)
            const badges = ovBadges(o);
            if (badges && wpx > 110) {
                ctx.fillStyle = "rgba(255,184,0,.16)";
                const bw = ctx.measureText(badges).width + 12;
                roundRect(x + wpx - bw - 5, ly + OV_H - 20, bw, 15, 4);
                ctx.fill();
                ctx.fillStyle = "#ffcf5c";
                ctx.font = "9px " + getComputedStyle(document.body).fontFamily;
                ctx.fillText(badges, x + wpx - bw + 1, ly + OV_H - 12);
            }
            ctx.restore();

            if (selected) {
                ctx.fillStyle = "#ff5c38";
                ctx.fillRect(x, ly + 2, 3, OV_H - 4);
                ctx.fillRect(x + wpx - 3, ly + 2, 3, OV_H - 4);
            }
        }
    }

    // v1.3.1: nhãn tóm tắt chế độ lớp đang bật (hòa trộn/độ mờ/preset/tách nền)
    function ovBadges(o) {
        const parts = [];
        if (o.chromaKey && o.chromaKey.enabled) parts.push("tách nền");
        if (o.filterPreset) parts.push((OV_FILTER_NAMES[o.filterPreset] || o.filterPreset).toLowerCase());
        if (o.blend) parts.push((OV_BLEND_NAMES[o.blend] || o.blend).toLowerCase());
        if (o.opacity != null && o.opacity > 0 && o.opacity < 1) parts.push("" + Math.round(o.opacity * 100) + "%");
        if (o.fadeInMs > 0 || o.fadeOutMs > 0) parts.push("hiện dần");
        return parts.slice(0, 2).join(" · ");
    }
    const OV_BLEND_NAMES = { screen: "Screen", multiply: "Multiply", overlay: "Overlay", darken: "Darken", lighten: "Lighten", add: "Add" };
    const OV_FILTER_NAMES = { bw: "Trắng đen", negative: "Âm bản", sepia: "Sepia", vintage: "Vintage", vivid: "Rực rỡ", contrast: "Tương phản", dream: "Mơ màng" };

    function overlayLabel(o) {
        if (o.kind === "text") return (o.text || "Chữ").slice(0, 28);
        if (o.kind === "shape") return ({ rect: "Hình chữ nhật", circle: "Hình tròn", ellipse: "Hình elip", line: "Đường thẳng" }[o.shape] || "Hình");
        const a = State.findAsset(o.assetId);
        return "PiP: " + (a ? a.name : "?");
    }

    // ─── Track A1: các đoạn âm thanh ĐỘC LẬP — kéo dời / trim được (v1.3.0) ───
    function drawFakeWave(x, y, wpx, hpx, color) {
        ctx.fillStyle = color;
        const step = 4;
        const mid = y + hpx / 2;
        for (let px = x + 6; px < x + wpx - 6; px += step) {
            const t = Math.sin(px * 0.31) * 0.5 + Math.sin(px * 0.13) * 0.5;
            const hh = Math.max(2, (hpx / 2 - 5) * Math.abs(t));
            ctx.fillRect(px, mid - hh / 2, 2, hh);
        }
    }

    function drawAudioTrack(w, h) {
        for (const ac of (State.doc.audioClips || [])) {
            const ly = audLaneY(audLaneOf.get(ac.id) || 0); // v1.3.1: lane riêng
            const x = xOf(ac.startMs);
            const dur = Math.max(100, (ac.outMs || 0) - (ac.inMs || 0));
            const wpx = Math.max(10, xOf(dur));
            const selected = State.selection === ac.id;
            const silent = ac.mute || State.doc.muteAll;
            const grad = ctx.createLinearGradient(0, ly, 0, ly + AUD_H);
            if (selected) { grad.addColorStop(0, "#1f7a4e"); grad.addColorStop(1, "#155c3a"); }
            else { grad.addColorStop(0, "#256b47"); grad.addColorStop(1, "#1a5034"); }
            ctx.fillStyle = silent ? "#33413a" : grad;
            roundRect(x, ly + 2, wpx, AUD_H - 4, 6);
            ctx.fill();
            ctx.strokeStyle = selected ? "#ff5c38" : (silent ? "#3f4f46" : "#3ea06b");
            ctx.lineWidth = selected ? 2 : 1;
            roundRect(x + 0.5, ly + 2.5, wpx - 1, AUD_H - 5, 6);
            ctx.stroke();
            ctx.lineWidth = 1;

            ctx.save();
            ctx.beginPath();
            ctx.rect(x, ly, wpx, AUD_H);
            ctx.clip();
            if (!silent) drawFakeWave(x, ly + 2, wpx, AUD_H - 4, "#7fe0ad");
            const asset = State.findAsset(ac.assetId);
            ctx.fillStyle = "rgba(6,20,12,.72)";
            ctx.fillRect(x, ly + 2, wpx, 18);
            ctx.fillStyle = selected ? "#d2ffe9" : "#cfe8db";
            ctx.font = "10.5px " + getComputedStyle(document.body).fontFamily;
            ctx.textBaseline = "middle";
            const name = (asset ? asset.name : "?") + (ac.volume && ac.volume !== 1 ? " · " + Math.round(ac.volume * 100) + "%" : "");
            ctx.fillText((silent ? "🔇 " : "🎵 ") + name, x + 6, ly + 11, Math.max(10, wpx - 12));
            ctx.fillStyle = "#9fcdb4";
            ctx.fillText(fmtMs(dur) + " @ " + fmtMs(ac.startMs), x + 6, ly + AUD_H - 10, Math.max(10, wpx - 12));
            ctx.restore();

            if (selected) {
                ctx.fillStyle = "#ff5c38";
                ctx.fillRect(x, ly + 2, 3, AUD_H - 4);
                ctx.fillRect(x + wpx - 3, ly + 2, 3, AUD_H - 4);
            }
        }
    }

    function drawTransitions() {
        const starts = State.clipStarts();
        State.doc.clips.forEach((c, i) => {
            const tr = i < State.doc.clips.length - 1
                ? State.transitionAfter(c.id) : null;
            if (!tr) return;
            const bx = xOf(starts[i + 1]);
            const cy = yV1 + MAIN_H / 2;
            const r = 11;
            ctx.save();
            ctx.translate(bx, cy);
            ctx.rotate(Math.PI / 4);
            ctx.fillStyle = "#ffb800";
            roundRect(-r / 1.42, -r / 1.42, r * 1.414, r * 1.414, 3);
            ctx.fill();
            ctx.strokeStyle = "#3a2f00";
            ctx.stroke();
            ctx.restore();
            ctx.fillStyle = "#3a2f00";
            ctx.font = "bold 9px sans-serif";
            ctx.textAlign = "center"; ctx.textBaseline = "middle";
            ctx.fillText("⇄", bx, cy + 0.5);
            ctx.textAlign = "left";
        });
    }

    function drawPlayhead(h) {
        const x = xOf(State.playheadMs);
        ctx.strokeStyle = "#ff5c38";
        ctx.lineWidth = 1.5;
        ctx.beginPath();
        ctx.moveTo(x, RULER_H - 6);
        ctx.lineTo(x, h);
        ctx.stroke();
        ctx.lineWidth = 1;
        ctx.fillStyle = "#ff5c38";
        ctx.beginPath();
        ctx.moveTo(x - 6, 2); ctx.lineTo(x + 6, 2); ctx.lineTo(x, RULER_H - 6);
        ctx.closePath();
        ctx.fill();
    }

    function drawDropIndicator() {
        if (dropX == null) return;
        let y0 = yV1, y1 = yV1 + MAIN_H;
        if (dropTrack === "v2") { y0 = yV2; y1 = yV2 + ovBandH(); }
        else if (dropTrack === "a1") { y0 = yAUD; y1 = yAUD + audBandH(); }
        ctx.strokeStyle = dropTrack === "v2" ? "#4aa3e0" : (dropTrack === "a1" ? "#3ea06b" : "#ffb800");
        ctx.setLineDash([6, 5]);
        ctx.beginPath();
        ctx.moveTo(dropX + 0.5, y0);
        ctx.lineTo(dropX + 0.5, y1);
        ctx.stroke();
        ctx.setLineDash([]);
    }

    // v1.3.1: đồng bộ chiều cao + nhãn track header HTML với số lane đang vẽ
    let headsSynced = "";
    function updateTrackHeads() {
        const sig = ovLaneCount + "/" + audLaneCount;
        const heads = document.querySelectorAll("#tl-heads .tl-head");
        if (heads.length >= 3) {
            heads[0].style.height = ovBandH() + "px";
            heads[0].querySelector(".th-name").textContent =
                ovLaneCount > 1 ? ("V2 · Lớp phủ ×" + ovLaneCount) : "V2 · Lớp phủ";
            heads[2].style.height = audBandH() + "px";
            heads[2].querySelector(".th-name").textContent =
                audLaneCount > 1 ? ("A1 · Âm thanh ×" + audLaneCount) : "A1 · Âm thanh";
        }
        headsSynced = sig;
    }

    function roundRect(x, y, w, h, r) {
        r = Math.min(r, w / 2, h / 2);
        ctx.beginPath();
        ctx.moveTo(x + r, y);
        ctx.arcTo(x + w, y, x + w, y + h, r);
        ctx.arcTo(x + w, y + h, x, y + h, r);
        ctx.arcTo(x, y + h, x, y, r);
        ctx.arcTo(x, y, x + w, y, r);
        ctx.closePath();
    }

    // ───────────────────────── Tương tác chuột ─────────────────────────
    function localX(e) {
        const rect = canvas.getBoundingClientRect();
        return e.clientX - rect.left;
    }
    function localY(e) {
        const rect = canvas.getBoundingClientRect();
        return e.clientY - rect.top;
    }

    // Hit-test lớp phủ V2 — trả {ov, edge: "l"|"r"|null} (v1.3.1: nhận biết lane)
    function hitOverlay(x, y) {
        if (y < yV2 || y >= yV1 - GAP) return null;
        for (const o of (State.doc.overlays || [])) {
            const ly = ovLaneY(ovLaneOf.get(o.id) || 0);
            if (y < ly || y > ly + OV_H) continue; // khối nằm lane khác — bỏ qua
            const x0 = xOf(o.startMs);
            const x1 = xOf(o.endMs);
            if (x >= x0 - 2 && x <= x1 + 2) {
                let edge = null;
                if (x <= x0 + OV_EDGE) edge = "l";
                else if (x >= x1 - OV_EDGE) edge = "r";
                return { ov: o, edge };
            }
        }
        return null;
    }

    // Hit-test clip V1 — trả {clip, index, startMs, x, wpx, edge}
    function hitClip(x, y) {
        if (y < yV1 || y > yV1 + MAIN_H + 8) return null;
        const starts = State.clipStarts();
        for (let i = 0; i < State.doc.clips.length; i++) {
            const c = State.doc.clips[i];
            const dur = Math.max(6, State.effMs(c));
            const cx = xOf(starts[i]);
            const cw = Math.max(6, xOf(dur));
            if (x >= cx - 2 && x <= cx + cw + 2) {
                let edge = null;
                if (x <= cx + EDGE) edge = "l";
                else if (x >= cx + cw - EDGE) edge = "r";
                return { clip: c, index: i, startMs: starts[i], x: cx, wpx: cw, edge };
            }
        }
        return null;
    }

    // Hit-test đoạn âm thanh A1 — trả {ac, edge} (v1.3.1: nhận biết lane)
    function hitAudioClip(x, y) {
        if (y < yAUD || y >= CONTENT_H - 4) return null;
        for (const ac of (State.doc.audioClips || [])) {
            const ly = audLaneY(audLaneOf.get(ac.id) || 0);
            if (y < ly || y > ly + AUD_H) continue; // khối nằm lane khác — bỏ qua
            const x0 = xOf(ac.startMs);
            const x1 = x0 + xOf(Math.max(100, ac.outMs - ac.inMs)); // +x0: toạ độ TUYỚI ĐỐI (bug v1.3.0-dev)
            if (x >= x0 - 2 && x <= x1 + 2) {
                let edge = null;
                if (x <= x0 + OV_EDGE) edge = "l";
                else if (x >= x1 - OV_EDGE) edge = "r";
                return { ac, edge };
            }
        }
        return null;
    }

    function msAtX(x) { return Math.round((x / State.pxPerSec) * 1000); }

    canvas.addEventListener("pointerdown", e => {
        // v1.2.6: chặn lỗi hiếm (bút cảm/chạm lạ) làm chết cả hàm chọn clip
        try { canvas.setPointerCapture(e.pointerId); } catch (_) { /* bỏ qua */ }
        const x = localX(e), y = localY(e);
        const tool = (window.VKSTool && window.VKSTool.current) || "select";

        // ── Vùng V2: lớp phủ ──
        const ov = hitOverlay(x, y);
        if (ov) {
            State.select(ov.ov.id);
            if (window.VKSTool && window.VKSTool.locked) {
                drag = { mode: "scrub" };
                return;
            }
            if (ov.edge) {
                drag = { mode: "ov-trim", ov: ov.ov, edge: ov.edge, startX: x,
                    origStart: ov.ov.startMs, origEnd: ov.ov.endMs };
            } else {
                drag = { mode: "ov-move", ov: ov.ov, startX: x, moved: false,
                    origStart: ov.ov.startMs, origEnd: ov.ov.endMs };
            }
            return;
        }

        // ── Vùng V1: clips ──
        const hit = hitClip(x, y);

        if (tool === "razor" && hit && y >= yV1 && y <= yV1 + MAIN_H) {
            const atMs = msAtX(x);
            State.select(hit.clip.id);
            splitAt(hit.clip, atMs);
            return;
        }

        if (window.VKSTool && window.VKSTool.locked && hit) {
            State.select(hit.clip.id);
            drag = { mode: "scrub" };
            return;
        }

        if (hit) {
            State.select(hit.clip.id);
            if (hit.edge && y <= yV1 + MAIN_H) {
                drag = { mode: "trim", hit, edge: hit.edge, startX: x,
                    origIn: hit.clip.inMs, origOut: hit.clip.outMs };
            } else if (e.altKey) {
                drag = { mode: "slip", hit, startX: x,
                    origIn: hit.clip.inMs, origOut: hit.clip.outMs };
            } else {
                drag = { mode: "move", hit, startX: x, moved: false };
            }
            return;
        }

        // ── Vùng A1: đoạn âm thanh (v1.3.0 — TRƯỚC scrub) ──
        const aud = hitAudioClip(x, y);
        if (aud) {
            State.select(aud.ac.id);
            if (window.VKSTool && window.VKSTool.locked) {
                drag = { mode: "scrub" };
                return;
            }
            if (aud.edge) {
                drag = { mode: "aud-trim", ac: aud.ac, edge: aud.edge, startX: x,
                    origStart: aud.ac.startMs, origIn: aud.ac.inMs, origOut: aud.ac.outMs };
            } else {
                drag = { mode: "aud-move", ac: aud.ac, startX: x, moved: false,
                    origStart: aud.ac.startMs };
            }
            return;
        }

        // ── Con trỏ trên thước / khoảng trống / track A1 ──
        drag = { mode: "scrub" };
        State.setPlayhead(msAtX(x));
    });

    async function splitAt(clip, timelineMs) {
        try {
            await call("SplitClip", clip.id, timelineMs);
            refreshDoc();
            window.VKSApp.toast("🗡 Đã cắt «" + (State.findAsset(clip.assetId) || {}).name + "»", "ok");
        } catch (err) {
            window.VKSApp.toast(err.message, "err");
        }
    }
    window.VKSSplitAt = splitAt;

    // 🧲 Nam châm: dính viền clip khác / con trỏ / ranh giây — ngưỡng 8px.
    function snapEdgeMs(ms, skipId) {
        const T = window.VKSTool;
        if (!T || !T.snapping) return ms;
        const tol = 8 / State.pxPerSec * 1000;
        let best = null, bestDist = tol;
        const consider = v => {
            if (v == null || !isFinite(v) || v < 0) return;
            const d = Math.abs(v - ms);
            if (d < bestDist) { bestDist = d; best = v; }
        };
        for (const c of State.doc.clips) {
            if (c.id === skipId) continue;
            consider(c.inMs);
            consider(c.outMs);
        }
        for (const o of (State.doc.overlays || [])) {
            if (o.id === skipId) continue;
            consider(o.startMs);
            consider(o.endMs);
        }
        consider(State.playheadMs);
        consider(Math.round(ms / 1000) * 1000);
        return best == null ? ms : best;
    }

    canvas.addEventListener("pointermove", e => {
        const x = localX(e), y = localY(e);

        if (!drag) {
            if ((window.VKSTool && window.VKSTool.current) === "razor") {
                const h1 = hitClip(x, y);
                canvas.style.cursor = (h1 && y >= yV1 && y <= yV1 + MAIN_H) ? "crosshair" : "default";
                return;
            }
            const ov = hitOverlay(x, y);
            if (ov && ov.edge) { canvas.style.cursor = "ew-resize"; return; }
            if (ov) { canvas.style.cursor = "grab"; return; }
            const aud = hitAudioClip(x, y);
            if (aud && aud.edge) { canvas.style.cursor = "ew-resize"; return; }
            if (aud) { canvas.style.cursor = "grab"; return; }
            const hit = hitClip(x, y);
            if (hit && hit.edge) canvas.style.cursor = "ew-resize";
            else if (hit) canvas.style.cursor = "grab";
            else canvas.style.cursor = "default";
            return;
        }

        if (drag.mode === "scrub") {
            State.setPlayhead(msAtX(x));
            return;
        }

        if (drag.mode === "trim") {
            const deltaMs = ((x - drag.startX) / State.pxPerSec) * 1000;
            const c = drag.hit.clip;
            if (drag.edge === "l") {
                let nin = Math.max(0, Math.min(drag.origIn + deltaMs, drag.origOut - 100));
                nin = snapEdgeMs(nin, c.id);
                c.inMs = Math.round(nin);
            } else {
                const asset = State.findAsset(c.assetId);
                const max = asset && asset.kind !== "image" ? asset.durationMs : 10 * 60 * 1000;
                let nout = Math.min(max, Math.max(drag.origOut + deltaMs, drag.origIn + 100));
                nout = snapEdgeMs(nout, c.id);
                c.outMs = Math.round(nout);
            }
            draw();
            return;
        }

        if (drag.mode === "slip") {
            const deltaMs = ((x - drag.startX) / State.pxPerSec) * 1000;
            const c = drag.hit.clip;
            const dur = drag.origOut - drag.origIn;
            const asset = State.findAsset(c.assetId);
            const max = asset && asset.kind !== "image" ? asset.durationMs : drag.origOut + 600000;
            let nin = drag.origIn + deltaMs;
            if (nin < 0) nin = 0;
            if (nin + dur > max) nin = max - dur;
            if (nin >= 0) {
                c.inMs = Math.round(nin);
                c.outMs = Math.round(nin + dur);
            }
            draw();
            return;
        }

        // v1.2.4: kéo lớp phủ V2 — dời nguyên khối, giữ độ dài
        if (drag.mode === "ov-move") {
            if (Math.abs(x - drag.startX) > 5) drag.moved = true;
            const deltaMs = ((x - drag.startX) / State.pxPerSec) * 1000;
            const o = drag.ov;
            const len = drag.origEnd - drag.origStart;
            let ns = drag.origStart + deltaMs;
            ns = snapEdgeMs(ns, o.id);
            if (ns < 0) ns = 0;
            if (ns + len > State.totalMs()) ns = Math.max(0, State.totalMs() - len);
            o.startMs = Math.round(ns);
            o.endMs = Math.round(ns + len);
            draw();
            return;
        }

        // v1.2.4: trim viền lớp phủ V2
        if (drag.mode === "ov-trim") {
            const deltaMs = ((x - drag.startX) / State.pxPerSec) * 1000;
            const o = drag.ov;
            if (drag.edge === "l") {
                let ns = Math.min(drag.origStart + deltaMs, drag.origEnd - 300);
                ns = snapEdgeMs(ns, o.id);
                o.startMs = Math.max(0, Math.round(ns));
            } else {
                let ne = Math.max(drag.origEnd + deltaMs, drag.origStart + 300);
                ne = snapEdgeMs(ne, o.id);
                o.endMs = Math.round(ne);
            }
            draw();
            return;
        }

        // v1.3.0: kéo dời đoạn âm thanh A1 — dời nguyên khối, giữ độ dài
        if (drag.mode === "aud-move") {
            if (Math.abs(x - drag.startX) > 5) drag.moved = true;
            const deltaMs = ((x - drag.startX) / State.pxPerSec) * 1000;
            const ac = drag.ac;
            let ns = drag.origStart + deltaMs;
            ns = snapEdgeMs(ns, ac.id);
            if (ns < 0) ns = 0;
            const dur = ac.outMs - ac.inMs;
            const limit = Math.max(0, Math.max(State.totalMs(), State.audioEndMs()) - dur);
            if (ns > limit) ns = limit;
            ac.startMs = Math.round(ns);
            draw();
            return;
        }

        // v1.3.0: trim viền đoạn âm thanh A1 (cắt trong tệp nguồn)
        if (drag.mode === "aud-trim") {
            const deltaMs = ((x - drag.startX) / State.pxPerSec) * 1000;
            const ac = drag.ac;
            const asset = State.findAsset(ac.assetId);
            const max = (asset && asset.durationMs) || ac.outMs;
            if (drag.edge === "l") {
                let nin = Math.min(drag.origIn + deltaMs, drag.origOut - 100);
                nin = snapEdgeMs(nin, ac.id);
                const shift = Math.round(nin) - drag.origIn; // cắt đầu = dời theo
                ac.inMs = Math.round(nin);
                ac.startMs = Math.max(0, drag.origStart + shift);
            } else {
                let nout = Math.max(drag.origOut + deltaMs, drag.origIn + 100);
                nout = snapEdgeMs(nout, ac.id);
                ac.outMs = Math.min(Math.round(nout), max);
            }
            draw();
            return;
        }

        if (drag.mode === "move") {
            if (Math.abs(x - drag.startX) > 5) drag.moved = true;
            drag.targetIndex = indexForInsert(x, drag.hit.index);
            drag.curX = x;
            draw();
            drawGhost(drag);
        }
    });

    function drawGhost(drag) {
        if (drag.targetIndex == null || drag.curX == null) return;
        ctx.strokeStyle = "#ffb800";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.moveTo(drag.curX + 0.5, yV1 + 4);
        ctx.lineTo(drag.curX + 0.5, yV1 + MAIN_H + 4);
        ctx.stroke();
        ctx.lineWidth = 1;
    }

    async function endDrag() {
        if (!drag) return;
        const d = drag;
        drag = null;
        canvas.style.cursor = "default";
        try {
            if (d.mode === "trim") {
                const c = d.hit.clip;
                await call("UpdateClip", c.id, { inMs: c.inMs, outMs: c.outMs });
                refreshDoc();
            } else if (d.mode === "slip") {
                const c = d.hit.clip;
                await call("UpdateClip", c.id, { inMs: c.inMs, outMs: c.outMs });
                refreshDoc();
            } else if (d.mode === "move" && d.moved && d.targetIndex != null) {
                await call("MoveClip", d.hit.clip.id, d.targetIndex);
                refreshDoc();
            } else if (d.mode === "ov-move" || d.mode === "ov-trim") {
                const o = d.ov;
                // Click đơn (không kéo) trên overlay → mở modal chỉnh sửa nhanh
                if (d.mode === "ov-move" && !d.moved) {
                    State.setPlayhead(o.startMs);
                    document.dispatchEvent(new CustomEvent("vks:open-overlay", { detail: o.id }));
                    return;
                }
                if (o.endMs - o.startMs < 300) { o.endMs = o.startMs + 300; }
                await call("UpdateOverlay", o.id, { startMs: o.startMs, endMs: o.endMs });
                refreshDoc();
            } else if (d.mode === "aud-move" || d.mode === "aud-trim") {
                // v1.3.0: lưu vị trí/cắt đoạn âm thanh A1
                const ac = d.ac;
                if (ac.outMs - ac.inMs < 100) { ac.outMs = ac.inMs + 100; }
                await call("UpdateAudioClip", ac.id, { startMs: ac.startMs, inMs: ac.inMs, outMs: ac.outMs });
                refreshDoc();
            }
        } catch (err) {
            window.VKSApp.toast(err.message, "err");
            refreshDoc();
        }
    }

    canvas.addEventListener("pointerup", endDrag);
    canvas.addEventListener("pointercancel", () => { drag = null; });

    // Vị trí chèn trong mảng ĐÃ GỠ clip đang kéo (khớp MoveClip backend):
    // trả k = số clip còn lại đứng trước điểm thả (0..n-1).
    function indexForInsert(x, excludeIndex) {
        const starts = State.clipStarts();
        let k = 0;
        for (let i = 0; i < State.doc.clips.length; i++) {
            if (i === excludeIndex) continue;
            const dur = State.effMs(State.doc.clips[i]);
            const x0 = xOf(starts[i]);
            const x1 = x0 + xOf(dur);
            if (x < x0 + (x1 - x0) / 2) return k;
            k++;
        }
        return k;
    }

    // ───────────────────────── Kéo-thả từ thư viện (v1.2.4: phân biệt V1/V2) ───
    canvas.addEventListener("dragover", e => {
        e.preventDefault();
        e.dataTransfer.dropEffect = "copy";
        dropX = localX(e);
        const y = localY(e);
        dropTrack = (y >= yV2 && y < yV1) ? "v2" : (y >= yAUD ? "a1" : "v1");
        canvas.classList.add("drop-ok");
        draw();
    });
    canvas.addEventListener("dragleave", () => {
        dropX = null; dropTrack = null;
        canvas.classList.remove("drop-ok");
        draw();
    });
    canvas.addEventListener("drop", async e => {
        e.preventDefault();
        const x = localX(e), y = localY(e);
        dropX = null; dropTrack = null;
        canvas.classList.remove("drop-ok");
        const assetId = e.dataTransfer.getData("text/vks-asset");
        const asset = assetId ? State.findAsset(assetId) : null;
        const intoV2 = y >= yV2 && y < yV1;
        try {
            if (assetId) {
                if (asset && asset.kind === "audio") {
                    // v1.3.0: tệp ÂM THANH → track A1 tại đúng chỗ thả (bất kể thả vào track nào)
                    const start = Math.max(0, msAtX(x));
                    const ac = await call("AddAudioClip", assetId, start);
                    State.select(ac.id);
                    refreshDoc();
                    window.VKSApp.toast("🎵 Đã thêm «" + asset.name + "» vào track A1 — kéo dời/trim tự do, tắt tiếng video gốc nếu muốn", "ok");
                } else if (intoV2) {
                    // Thả vào V2 → lớp phủ PiP tại đúng chỗ thả
                    const start = Math.max(0, msAtX(x));
                    const o = await call("AddMediaOverlay", assetId, start, 0);
                    State.select(o.id);
                    refreshDoc();
                    window.VKSApp.toast("▣ Đã thêm lớp phủ PiP vào V2 — kéo viền để chỉnh dài ngắn", "ok");
                } else {
                    const idx = indexForInsert(x, -1);
                    const c = await call("AddClip", assetId, idx);
                    State.select(c.id);
                    refreshDoc();
                }
            } else if (e.dataTransfer.files && e.dataTransfer.files.length) {
                window.VKSApp.toast("Kéo tệp trực tiếp vào cửa sổ hoặc dùng nút Nhập tư liệu");
            }
        } catch (err) {
            window.VKSApp.toast(err.message, "err");
        }
        draw();
    });

    // ───────────────────────── Thu phóng & cuộn (v1.2.4) ────────────────────
    // Lăn thường = cuộn ngang (CapCut style); Ctrl+lăn = zoom neo tại con trỏ.
    canvas.addEventListener("wheel", e => {
        e.preventDefault();
        if (e.ctrlKey || e.metaKey) {
            const x = localX(e);
            const anchorMs = (scroll.scrollLeft + x) / State.pxPerSec * 1000;
            const factor = e.deltaY < 0 ? 1.18 : 1 / 1.18;
            setZoom(State.pxPerSec * factor);
            scroll.scrollLeft = Math.max(0, anchorMs / 1000 * State.pxPerSec - x);
        } else {
            scroll.scrollLeft += (Math.abs(e.deltaY) >= Math.abs(e.deltaX) ? e.deltaY : e.deltaX);
        }
    }, { passive: false });

    function setZoom(px) {
        State.pxPerSec = Math.round(Math.max(State.ZOOM_MIN, Math.min(State.ZOOM_MAX, px)));
        document.dispatchEvent(new CustomEvent("vks:zoom-changed"));
        resize();
    }

    function zoomBy(f) {
        const center = scroll.scrollLeft + scroll.clientWidth / 2;
        const anchorMs = center / State.pxPerSec * 1000;
        setZoom(State.pxPerSec * f);
        scroll.scrollLeft = Math.max(0, anchorMs / 1000 * State.pxPerSec - scroll.clientWidth / 2);
    }

    function zoomFit() {
        const total = State.totalMs();
        if (total <= 0) { setZoom(48); return; }
        const fit = (scroll.clientWidth - 30) / (total / 1000);
        setZoom(fit);
        scroll.scrollLeft = 0;
    }

    // Auto-scroll theo con trỏ khi phát / scrub ra ngoài màn hình
    function ensurePlayheadVisible() {
        const px = xOf(State.playheadMs);
        const viewL = scroll.scrollLeft, viewR = viewL + scroll.clientWidth;
        if (px < viewL || px > viewR - 48) {
            scroll.scrollLeft = Math.max(0, px - scroll.clientWidth / 3);
        }
    }

    // ───────────────────────── Sự kiện hệ thống ─────────────────────────
    document.addEventListener("vks:doc-changed", () => { resize(); });
    document.addEventListener("vks:playhead-changed", () => {
        document.getElementById("tl-playhead-time").textContent = fmtMs(State.playheadMs);
        ensurePlayheadVisible();
        draw();
    });
    document.addEventListener("vks:selection-changed", draw);
    window.addEventListener("resize", resize);

    function refreshDoc() {
        call("GetState").then(s => { State.setDoc(s.doc); });
    }

    window.VKSTimeline = { resize, draw, refreshDoc, zoomBy, zoomFit, setZoom };
})();
