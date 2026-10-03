/* panels.js — thư viện tư liệu, thuộc tính clip và màn hình xem trước. */
(function () {
    "use strict";

    const { call, fmtMs, fmtBytes, fmtFps } = window.VKSApi;
    const State = window.VKSState;

    // ═════════════════════════ THƯ VIỆN ═════════════════════════
    const libList = document.getElementById("library-list");
    const libEmpty = document.getElementById("library-empty");
    const libCount = document.getElementById("library-count");

    async function renderLibrary() {
        const assets = State.doc.assets;
        libCount.textContent = assets.length + " tư liệu";
        libEmpty.hidden = assets.length > 0;
        // Xoá card cũ (giữ empty hint)
        libList.querySelectorAll(".lib-card").forEach(n => n.remove());
        for (const a of assets) {
            const card = document.createElement("div");
            card.className = "lib-card";
            card.draggable = true;
            card.dataset.assetId = a.id;

            const kindLabel = a.kind === "video" ? "Video" : a.kind === "image" ? "Ảnh" : "Âm thanh";
            let sub = kindLabel;
            if (a.kind !== "audio") sub += " · " + a.width + "×" + a.height;
            if (a.kind === "video") sub += " · " + fmtFps(a.rate) + "fps";
            if (a.kind !== "image") sub += " · " + fmtMs(a.durationMs);
            const isAudio = a.kind === "audio";

            card.innerHTML = `
                <div class="lib-thumb">
                    <img alt="" draggable="false" ${isAudio ? 'hidden' : ''}>
                    <span class="lib-badge">${kindLabel}</span>
                </div>
                <div class="lib-meta">
                    <div class="lib-name" title="${a.path}">${a.name}</div>
                    <div class="lib-sub">${sub} · ${fmtBytes(a.sizeBytes)}</div>
                    ${a.missing ? '<div class="lib-missing">⚠ Tệp nguồn không còn trên đĩa</div>' : ""}
                </div>
                <div class="lib-actions">
                    <button class="mini-btn wide add-btn" ${a.missing ? "disabled" : ""} title="${isAudio ? "Thêm vào track âm thanh A1 (đặt sau đoạn hiện có)" : "Thêm vào cuối timeline"}">${isAudio ? "＋ Track A1" : "＋ Timeline"}</button>
                    <button class="mini-btn wide del-btn danger" title="Gỡ khỏi thư viện">Gỡ</button>
                </div>`;

            const img = card.querySelector("img");
            if (a.kind !== "audio" && !a.missing) {
                const t = a.kind === "image" ? 0 : Math.min(1000, Math.max(0, Math.round(a.durationMs / 3)));
                img.src = thumbURL(a.id, t);
                img.onerror = async () => {
                    try {
                        const u = await call("GetAssetThumb", a.id, t);
                        img.src = u;
                    } catch (_) { img.hidden = true; }
                };
            }

            card.addEventListener("dragstart", e => {
                e.dataTransfer.setData("text/vks-asset", a.id);
                e.dataTransfer.effectAllowed = "copy";
            });
            card.querySelector(".add-btn").addEventListener("click", async () => {
                try {
                    if (a.kind === "audio") {
                        // v1.3.0: âm thanh → track A1
                        const ac = await call("AddAudioClip", a.id, -1);
                        State.select(ac.id);
                        refresh();
                        toast("🎵 Đã thêm «" + a.name + "» vào track A1 (sau đoạn hiện có)", "ok");
                        return;
                    }
                    const c = await call("AddClip", a.id, -1);
                    State.select(c.id);
                    refresh();
                    toast("Đã thêm «" + a.name + "» vào timeline", "ok");
                } catch (err) { toast(err.message, "err"); }
            });
            card.querySelector(".del-btn").addEventListener("click", async () => {
                if (!confirm('Gỡ «' + a.name + '» khỏi thư viện? (các clip liên quan cũng bị xoá)')) return;
                try { await call("RemoveAsset", a.id); refresh(); }
                catch (err) { toast(err.message, "err"); }
            });
            // v1.2.3: CLICK ĐÔI = xem tư liệu gốc (Source Monitor)
            card.title = "Kéo vào timeline · click đôi để xem trước tư liệu gốc";
            card.addEventListener("dblclick", () => {
                if (window.VKSSourceMonitor) window.VKSSourceMonitor.open(a.id);
            });

            libList.appendChild(card);
        }
    }

    function thumbURL(assetId, tMs) {
        return "/local/thumbs/" + encodeURIComponent(assetId) + "_" + tMs + ".jpg";
    }

    // ═════════════════════════ THUỘC TÍNH ═════════════════════════
    const propsBody = document.getElementById("props-body");

    // v1.2.3: đổi % âm lượng → dB (100% = 0 dB).
    function volDb(v) {
        const x = Number(v) || 1;
        if (x <= 0) return "−∞ dB";
        const db = 20 * Math.log10(x);
        return (db >= 0 ? "+" : "−") + Math.abs(db).toFixed(1) + " dB";
    }
    let propSync = false; // chống vòng lặp khi đang điền giá trị

    function renderProps() {
        // v1.3.0: lựa chọn là ĐOẠN ÂM THANH A1 → panel âm thanh riêng
        const aud = State.selection ? State.findAudioClip(State.selection) : null;
        if (aud) { renderAudioProps(aud); return; }
        const c = State.selection ? State.findClip(State.selection) : null;
        if (!c) {
            // v1.2.6: lựa chọn là LỚP PHỦ (V2) → panel sửa nhanh thay vì chỗ trống
            const ov = State.selection ? State.findOverlay(State.selection) : null;
            if (ov) {
                const kindName = { text: "Chữ / tiêu đề", shape: "Hình vẽ", media: "Video/ảnh PiP" }[ov.kind] || ov.kind;
                const isMedia = ov.kind === "media";
                const op = (ov.opacity != null && ov.opacity > 0 && ov.opacity <= 1) ? Math.round(ov.opacity * 100) : 100;
                propsBody.innerHTML = `
                    <div class="prop-group">
                        <div class="group-title">Lớp phủ V2 — ${kindName}</div>
                        <div class="tl-dim" style="margin-bottom:8px">${fmtMs(ov.startMs)} → ${fmtMs(ov.endMs)} · khối trên track V2</div>
                        <div class="btn-row">
                            <button class="mini-btn wide" id="pr-ov-edit" title="Mở trình sửa lớp phủ đầy đủ">✏️ Sửa lớp phủ</button>
                            <button class="mini-btn wide danger" id="pr-ov-del" title="Xoá lớp phủ này (phím Del cũng được)">🗑 Xoá</button>
                        </div>
                        <div class="tl-dim" style="margin-top:8px">✨ Mẹo v1.3.1: kéo trực tiếp lớp trên <b>khung xem trước</b> để dời, kéo tay nắm góc để phóng to/thu nhỏ.</div>
                    </div>

                    <div class="prop-group">
                        <div class="group-title">Chế độ lớp <span class="tl-dim">(v1.3.1 — kiểu KineMaster)</span></div>
                        ${isMedia ? `
                        <div class="field">
                            <label>Kích thước (% canvas) — <b id="pr-ovsclabel">${Math.round(ov.scalePct || 30)}%</b></label>
                            <input type="range" id="pr-ovscale" min="2" max="100" step="1" value="${Math.round(ov.scalePct || 30)}">
                        </div>` : ""}
                        <div class="field">
                            <label>Hòa trộn với lớp dưới (blend)</label>
                            <select id="pr-ovblend">
                                <option value="">Bình thường</option>
                                <option value="screen" ${ov.blend === "screen" ? "selected" : ""}>Screen — hòa sáng (lấp lánh/ánh sáng)</option>
                                <option value="multiply" ${ov.blend === "multiply" ? "selected" : ""}>Multiply — nhân (bóng đổ/texture)</option>
                                <option value="overlay" ${ov.blend === "overlay" ? "selected" : ""}>Overlay — chồng tương phản</option>
                                <option value="darken" ${ov.blend === "darken" ? "selected" : ""}>Darken — lấy màu tối</option>
                                <option value="lighten" ${ov.blend === "lighten" ? "selected" : ""}>Lighten — lấy màu sáng</option>
                                <option value="add" ${ov.blend === "add" ? "selected" : ""}>Add — cộng sáng</option>
                            </select>
                        </div>
                        <div class="field">
                            <label>Độ mờ đục — <b id="pr-ovoplabel">${op}%</b></label>
                            <input type="range" id="pr-ovop" min="5" max="100" step="5" value="${op}">
                        </div>
                        <div class="row">
                            <div class="field grow">
                                <label>Hiện dần vào (s)</label>
                                <input type="number" id="pr-ovfin" step="0.1" min="0" max="10" value="${((ov.fadeInMs || 0) / 1000).toFixed(1)}">
                            </div>
                            <div class="field grow">
                                <label>Biến mất dần (s)</label>
                                <input type="number" id="pr-ovfout" step="0.1" min="0" max="10" value="${((ov.fadeOutMs || 0) / 1000).toFixed(1)}">
                            </div>
                        </div>
                        <div class="field">
                            <label>Preset hiệu ứng nghệ thuật</label>
                            <select id="pr-ovfilter">
                                <option value="">— Không —</option>
                                <option value="bw" ${ov.filterPreset === "bw" ? "selected" : ""}>Trắng đen</option>
                                <option value="negative" ${ov.filterPreset === "negative" ? "selected" : ""}>Âm bản</option>
                                <option value="sepia" ${ov.filterPreset === "sepia" ? "selected" : ""}>Sepia cổ điển</option>
                                <option value="vintage" ${ov.filterPreset === "vintage" ? "selected" : ""}>Vintage retro</option>
                                <option value="vivid" ${ov.filterPreset === "vivid" ? "selected" : ""}>Rực rỡ</option>
                                <option value="contrast" ${ov.filterPreset === "contrast" ? "selected" : ""}>Tương phản đậm</option>
                                <option value="dream" ${ov.filterPreset === "dream" ? "selected" : ""}>Mơ màng (mềm + sáng)</option>
                            </select>
                        </div>
                        ${isMedia ? `
                        <label class="check-row"><input type="checkbox" id="pr-ovck" ${ov.chromaKey && ov.chromaKey.enabled ? "checked" : ""}> 🟩 Tách nền xanh (chroma key)</label>
                        <div class="tl-dim" style="margin-top:4px">Tách nền chi tiết (màu, độ giống, mềm viền) trong «Sửa lớp phủ».</div>` : ""}
                    </div>`;
                propsBody.querySelector("#pr-ov-edit").addEventListener("click", () => {
                    document.dispatchEvent(new CustomEvent("vks:open-overlay", { detail: ov.id }));
                });
                propsBody.querySelector("#pr-ov-del").addEventListener("click", async () => {
                    try { await call("RemoveOverlay", ov.id); State.select(null); refresh(); }
                    catch (err) { toast(err.message, "err"); }
                });
                // v1.3.1: xử lý nhóm "Chế độ lớp"
                const ovPatch = async patch => {
                    try { await call("UpdateOverlay", ov.id, patch); refresh(); }
                    catch (err) { toast(err.message, "err"); refresh(); }
                };
                const scl = propsBody.querySelector("#pr-ovscale");
                if (scl) {
                    propsBody.querySelector("#pr-ovsclabel").textContent = scl.value + "%";
                    scl.addEventListener("input", e => { propsBody.querySelector("#pr-ovsclabel").textContent = e.target.value + "%"; });
                    scl.addEventListener("change", e => ovPatch({ scalePct: Number(e.target.value) }));
                }
                propsBody.querySelector("#pr-ovblend").addEventListener("change", e => ovPatch({ blend: e.target.value }));
                propsBody.querySelector("#pr-ovop").addEventListener("input", e => { propsBody.querySelector("#pr-ovoplabel").textContent = e.target.value + "%"; });
                propsBody.querySelector("#pr-ovop").addEventListener("change", e => ovPatch({ opacity: Number(e.target.value) / 100 }));
                propsBody.querySelector("#pr-ovfin").addEventListener("change", e => ovPatch({ fadeInMs: Math.round(Number(e.target.value) * 1000) }));
                propsBody.querySelector("#pr-ovfout").addEventListener("change", e => ovPatch({ fadeOutMs: Math.round(Number(e.target.value) * 1000) }));
                propsBody.querySelector("#pr-ovfilter").addEventListener("change", e => ovPatch({ filterPreset: e.target.value }));
                const ck = propsBody.querySelector("#pr-ovck");
                if (ck) ck.addEventListener("change", e => {
                    const base = ov.chromaKey || { enabled: false, hex: "00ff00", similarity: 0.1, blend: 0.08 };
                    ovPatch({ chromaKey: { ...base, enabled: e.target.checked } });
                });
                return;
            }
            propsBody.innerHTML = `
                <div class="empty-hint">
                    <div class="empty-icon">🎚️</div>
                    <p>Chọn một clip trên timeline<br>để chỉnh sửa</p>
                </div>`;
            return;
        }
        const asset = State.findAsset(c.assetId);
        const tr = State.transitionAfter(c.id);
        const isLast = State.doc.clips[State.doc.clips.length - 1]?.id === c.id;
        const isImage = asset && asset.kind === "image";

        const fx = c.effects || {};
        const hasFx = fx.brightness || (fx.contrast && fx.contrast !== 1) || (fx.saturation && fx.saturation !== 1) ||
            (fx.gamma && fx.gamma !== 1) || fx.blur || fx.sharpen || fx.vignette || fx.grayscale || fx.invert || fx.denoise;

        propsBody.innerHTML = `
            <div class="prop-group">
                <div class="group-title">Clip — ${asset ? asset.name : "?"}</div>
                <div class="field">
                    <label>Cắt (giây) — đầu / cuối</label>
                    <div class="row">
                        <input type="number" id="pr-in" class="grow" step="0.05" min="0" value="${(c.inMs / 1000).toFixed(2)}">
                        <input type="number" id="pr-out" class="grow" step="0.05" min="0" value="${(c.outMs / 1000).toFixed(2)}">
                    </div>
                    <div class="btn-row" style="margin-top:6px">
                        <button class="mini-btn" id="pr-setin" title="Đặt đầu = con trỏ">⇤ Đầu</button>
                        <button class="mini-btn" id="pr-setout" title="Đặt cuối = con trỏ">Cuột ⇥</button>
                        <span class="tl-dim" style="margin-left:auto">trên timeline: ${fmtMs(State.effMs(c))}</span>
                    </div>
                </div>
            </div>

            <div class="prop-group">
                <div class="group-title">Âm thanh</div>
                <label class="check-row"><input type="checkbox" id="pr-mute" ${c.mute ? "checked" : ""}> Tắt tiếng clip</label>
                <div class="field" style="margin-top:8px">
                    <label>Âm lượng — <b id="pr-vollabel">${Math.round(c.volume * 100)}%</b></label>
                    <input type="range" id="pr-vol" min="0" max="200" step="5" value="${Math.round(c.volume * 100)}">
                </div>
                <div class="row">
                    <div class="field grow">
                        <label>Fade vào (s)</label>
                        <input type="number" id="pr-fadein" step="0.1" min="0" max="60" value="${((c.audioFadeInMs || 0) / 1000).toFixed(1)}">
                    </div>
                    <div class="field grow">
                        <label>Fade ra (s)</label>
                        <input type="number" id="pr-fadeout" step="0.1" min="0" max="60" value="${((c.audioFadeOutMs || 0) / 1000).toFixed(1)}">
                    </div>
                </div>
                <div class="field" style="margin-top:6px">
                    <label>Tương đương <b id="pr-dblabel">${volDb(c.volume)}</b></label>
                </div>
                <label class="check-row"><input type="checkbox" id="pr-aden" ${fx.audioDenoise ? "checked" : ""}> ✨ Lọc tạp âm tiếng (quạt/gió/ồn)</label>
                <label class="check-row"><input type="checkbox" id="pr-vocal" ${fx.vocalEnhance ? "checked" : ""}> 🎙 Làm sạch giọng nói (studio)</label>
                <div class="btn-row" style="margin-top:8px">
                    <button class="mini-btn wide" id="pr-detach" title="Tách âm thanh thành tư liệu riêng + tắt tiếng clip gốc (Ctrl+L)">🔗 Tách âm thanh</button>
                </div>
            </div>

            <div class="prop-group">
                <div class="group-title">Hình ảnh</div>
                <div class="field">
                    <label>Xoay</label>
                    <div class="btn-row">
                        <button class="rot-btn rot ${c.transform.rotateDeg === 0 ? "active" : ""}" data-deg="0">0°</button>
                        <button class="rot-btn rot ${c.transform.rotateDeg === 90 ? "active" : ""}" data-deg="90">90°</button>
                        <button class="rot-btn rot ${c.transform.rotateDeg === 180 ? "active" : ""}" data-deg="180">180°</button>
                        <button class="rot-btn rot ${c.transform.rotateDeg === 270 ? "active" : ""}" data-deg="270">270°</button>
                    </div>
                </div>
                <div class="btn-row" style="margin-bottom:12px">
                    <button class="rot-btn ${c.transform.flipH ? "active" : ""}" id="pr-fliph">⇋ Lật ngang</button>
                    <button class="rot-btn ${c.transform.flipV ? "active" : ""}" id="pr-flipv">⇅ Lật dọc</button>
                </div>
                <div class="field">
                    <label>Thu phóng — <b id="pr-zoomlabel">${c.transform.zoom.toFixed(2)}×</b></label>
                    <input type="range" id="pr-zoom" min="1" max="4" step="0.05" value="${c.transform.zoom}">
                </div>
                <div class="field">
                    <label>Vị trí ngang — <b id="pr-posxlabel">${Math.round((c.transform.posX ?? 0.5) * 100)}%</b> <span class="tl-dim">(hiệu lực khi zoom &gt; 1×)</span></label>
                    <input type="range" id="pr-posx" min="2" max="98" step="2" value="${Math.round((c.transform.posX ?? 0.5) * 100)}">
                </div>
                <div class="field">
                    <label>Vị trí dọc — <b id="pr-posylabel">${Math.round((c.transform.posY ?? 0.5) * 100)}%</b></label>
                    <input type="range" id="pr-posy" min="2" max="98" step="2" value="${Math.round((c.transform.posY ?? 0.5) * 100)}">
                </div>
                <div class="field">
                    <label>Độ mờ đục (Opacity) — <b id="pr-oplabel">${Math.round((c.opacity ?? 1) * 100)}%</b></label>
                    <input type="range" id="pr-opacity" min="5" max="100" step="5" value="${Math.round((c.opacity ?? 1) * 100)}">
                </div>
                <div class="btn-row" style="margin-top:6px">
                    <button class="mini-btn wide" id="pr-reframe-c" title="Đặt pan về giữa khung (Auto Reframe thủ công)">⌖ Căn giữa</button>
                </div>
            </div>

            <div class="prop-group">
                <div class="group-title">Tốc độ &amp; thời gian <span class="tl-dim">(v1.1)</span></div>
                <div class="field">
                    <label>Tốc độ phát — <b id="pr-speedlabel">${(c.speed || 1).toFixed(2)}×</b></label>
                    <input type="range" id="pr-speed" min="0.25" max="4" step="0.05" value="${c.speed || 1}">
                    <div class="range-marks"><span>0.25× chậm</span><span>1×</span><span>4× nhanh</span></div>
                </div>
                <div class="btn-row" style="margin-bottom:8px">
                    <button class="rot-btn ${c.reverse ? "active" : ""}" id="pr-reverse">◀◀ Phát ngược</button>
                    <button class="rot-btn ${c.boomerang ? "active" : ""}" id="pr-boom">⇄ Boomerang</button>
                </div>
                <div class="row">
                    <div class="field grow">
                        <label>Lặp lại (lần)</label>
                        <input type="number" id="pr-loop" min="1" max="20" value="${c.loopN || 1}">
                    </div>
                    <div class="field grow">
                        <label>Đóng khung đầu (s)</label>
                        <input type="number" id="pr-fzstart" step="0.1" min="0" max="30" value="${((c.freezeStartMs || 0) / 1000).toFixed(1)}">
                    </div>
                    <div class="field grow">
                        <label>Đóng khung cuối (s)</label>
                        <input type="number" id="pr-fzend" step="0.1" min="0" max="30" value="${((c.freezeEndMs || 0) / 1000).toFixed(1)}">
                    </div>
                </div>
                <div class="field">
                    <label>Gợn sóng nước — <b id="pr-wdlabel">${(((c.waterDropMs || 0)) / 1000).toFixed(1)}s</b> (0 = tắt)</label>
                    <input type="range" id="pr-wd" min="0" max="6" step="0.5" value="${((c.waterDropMs || 0) / 1000)}">
                </div>
            </div>

            <div class="prop-group">
                <div class="group-title">Màu sắc &amp; hiệu ứng <span class="tl-dim">(v1.1)</span></div>
                <div class="field">
                    <label>Preset phong cách</label>
                    <select id="pr-look">
                        <option value="">— Tự chỉnh —</option>
                        <option value="cine">Điện ảnh (teal &amp; orange)</option>
                        <option value="warm">Ấm áp (vàng nắng)</option>
                        <option value="cold">Lạnh (xanh thép)</option>
                        <option value="vivid">Rực rỡ</option>
                        <option value="mono">Đen trắng</option>
                        <option value="dream">Mơ mộng (mờ + sáng)</option>
                        <option value="noir">Noir (tương phản cao)</option>
                    </select>
                </div>
                <div class="field"><label>Độ sáng — <b>${((fx.brightness || 0) * 100).toFixed(0)}%</b></label>
                    <input type="range" class="fx" data-k="brightness" min="-50" max="50" step="5" value="${Math.round((fx.brightness || 0) * 100)}"></div>
                <div class="field"><label>Tương phản — <b>${(fx.contrast || 1).toFixed(2)}</b></label>
                    <input type="range" class="fx" data-k="contrast" min="50" max="200" step="5" value="${Math.round((fx.contrast || 1) * 100)}"></div>
                <div class="field"><label>Bão hòa màu — <b>${(fx.saturation || 1).toFixed(2)}</b></label>
                    <input type="range" class="fx" data-k="saturation" min="0" max="300" step="5" value="${Math.round((fx.saturation || 1) * 100)}"></div>
                <div class="field"><label>Gamma — <b>${(fx.gamma || 1).toFixed(2)}</b></label>
                    <input type="range" class="fx" data-k="gamma" min="20" max="300" step="5" value="${Math.round((fx.gamma || 1) * 100)}"></div>
                <div class="field"><label>Làm mờ (Gaussian) — <b>${(fx.blur || 0).toFixed(1)}</b></label>
                    <input type="range" class="fx" data-k="blur" min="0" max="20" step="0.5" value="${fx.blur || 0}"></div>
                <div class="field"><label>Làm sắc nét — <b>${(fx.sharpen || 0).toFixed(1)}</b></label>
                    <input type="range" class="fx" data-k="sharpen" min="0" max="3" step="0.1" value="${fx.sharpen || 0}"></div>
                <div class="field"><label>Vignette (tối góc) — <b>${(fx.vignette || 0).toFixed(2)}</b></label>
                    <input type="range" class="fx" data-k="vignette" min="0" max="100" step="5" value="${Math.round((fx.vignette || 0) * 100)}"></div>
                <div class="btn-row">
                    <button class="rot-btn ${fx.grayscale ? "active" : ""}" id="pr-gray">⬜ Đen trắng</button>
                    <button class="rot-btn ${fx.invert ? "active" : ""}" id="pr-inv">◐ Âm bản</button>
                    <button class="rot-btn ${fx.denoise ? "active" : ""}" id="pr-dn" title="Giảm nhiễu hqdn3d (pre-pass ffmpeg)">✨ Giảm nhiễu</button>
                </div>
                <div class="field" style="margin-top:10px">
                    <label>Tông màu theo vùng sáng (bánh xe rút gọn v1.2.3)</label>
                    <div class="row">
                        <select id="pr-cb-s" class="grow" title="Tông vùng TỐI (Shadows)">
                            <option value="">Tối: nguyên bản</option>
                            <option value="warm">Tối: ấm (thêm đỏ+vàng)</option>
                            <option value="cool">Tối: lạnh (thêm xanh dương)</option>
                            <option value="teal">Tối: teal điện ảnh</option>
                        </select>
                        <select id="pr-cb-m" class="grow" title="Tông vùng TRUNG (Midtones)">
                            <option value="">Trung: nguyên bản</option>
                            <option value="warm">Trung: ấm</option>
                            <option value="cool">Trung: lạnh</option>
                            <option value="teal">Trung: teal điện ảnh</option>
                        </select>
                        <select id="pr-cb-h" class="grow" title="Tông vùng SÁNG (Highlights)">
                            <option value="">Sáng: nguyên bản</option>
                            <option value="warm">Sáng: ấm</option>
                            <option value="cool">Sáng: lạnh</option>
                            <option value="teal">Sáng: teal điện ảnh</option>
                        </select>
                    </div>
                </div>
                <div class="field" style="margin-top:10px">
                    <label>LUT 3D (.cube) — ${c.lutPath ? "✅ " + c.lutPath.split(/[\\\\/]/).pop() : "chưa có"}</label>
                    <div class="btn-row">
                        <button class="mini-btn" id="pr-lut-load">Chọn LUT…</button>
                        <button class="mini-btn danger" id="pr-lut-clear" ${c.lutPath ? "" : "disabled"}>Gỡ LUT</button>
                    </div>
                </div>
                <div class="btn-row" style="margin-top:10px">
                    <button class="mini-btn" id="pr-match" title="Khớp màu clip này theo một clip chuẩn (phân tích độ sáng + độ bão hoà)">🎨 Khớp màu…</button>
                    <button class="mini-btn" id="pr-applyall" title="Áp toàn bộ hiệu ứng/màu của clip này cho TẤT CẢ clip (như lớp điều chỉnh)">⧉ Áp cho tất cả</button>
                </div>
            </div>

            <div class="prop-group">
                <div class="group-title">Hoạt hình keyframe <span class="tl-dim">(v1.1)</span></div>
                <div class="field">
                    <label>Mẫu dựng sẵn</label>
                    <select id="pr-kf">
                        <option value="">— Không —</option>
                        <option value="fadein">Mờ dần vào (fade in)</option>
                        <option value="fadeout">Mờ dần ra (fade out)</option>
                        <option value="zoomin">Phóng to dần 1→1.25</option>
                        <option value="zoomout">Thu nhỏ dần 1.25→1</option>
                    </select>
                </div>
                <div class="tl-dim" style="font-size:10.5px">Mẫu áp lên toàn bộ chiều dài clip (opacity hoặc scale).</div>
            </div>

            <div class="prop-group">
                <div class="group-title">Chuyển cảnh sau clip này ${isLast ? "(clip cuối — không áp dụng)" : ""}</div>
                <div class="field">
                    <select id="pr-trans" ${isLast ? "disabled" : ""}></select>
                </div>
                <div class="field" ${!tr || isLast ? "hidden" : ""} id="pr-transdur-field">
                    <label>Thời lượng — <b id="pr-transdur-label">${fmtMs(tr ? tr.durationMs : 800)}</b></label>
                    <input type="range" id="pr-transdur" min="200" max="3000" step="100" value="${tr ? tr.durationMs : 800}">
                </div>
            </div>

            <div class="prop-group">
                <div class="btn-row">
                    <button class="mini-btn wide" id="pr-move-l">◀ Dời trái</button>
                    <button class="mini-btn wide" id="pr-move-r">Dời phải ▶</button>
                    <button class="mini-btn wide danger" id="pr-del">🗑 Xoá clip</button>
                </div>
            </div>`;

        // Danh sách chuyển cảnh
        const sel = propsBody.querySelector("#pr-trans");
        call("ListTransitions").then(list => {
            for (const t of list) {
                const opt = document.createElement("option");
                opt.value = t.type;
                opt.textContent = t.name;
                if ((tr && tr.type === t.type) || (!tr && t.type === "none")) opt.selected = true;
                sel.appendChild(opt);
            }
        });

        // ── Gắn sự kiện ──
        const numIn = propsBody.querySelector("#pr-in");
        const numOut = propsBody.querySelector("#pr-out");
        const applyTrim = async () => {
            if (propSync) return;
            try {
                const inMs = Math.round(Number(numIn.value) * 1000);
                const outMs = Math.round(Number(numOut.value) * 1000);
                await call("UpdateClip", c.id, { inMs, outMs });
                refresh();
            } catch (err) { toast(err.message, "err"); }
        };
        numIn.addEventListener("change", applyTrim);
        numOut.addEventListener("change", applyTrim);
        propsBody.querySelector("#pr-setin").addEventListener("click", () => {
            const local = State.playheadMs - State.clipStarts()[State.doc.clips.findIndex(x => x.id === c.id)];
            numIn.value = ((Math.max(0, local) + c.inMs) / 1000).toFixed(2);
            applyTrim();
        });
        propsBody.querySelector("#pr-setout").addEventListener("click", () => {
            const idx = State.doc.clips.findIndex(x => x.id === c.id);
            const local = State.playheadMs - State.clipStarts()[idx];
            numOut.value = ((c.inMs + Math.max(0, local)) / 1000).toFixed(2);
            applyTrim();
        });

        propsBody.querySelector("#pr-mute").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { mute: e.target.checked }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-vol").addEventListener("input", e => {
            propsBody.querySelector("#pr-vollabel").textContent = e.target.value + "%";
        });
        propsBody.querySelector("#pr-vol").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { volume: Number(e.target.value) / 100 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        // Cập nhật dB khi kéo (v1.2.3)
        propsBody.querySelector("#pr-vol").addEventListener("input", e => {
            const lbl = propsBody.querySelector("#pr-dblabel");
            if (lbl) lbl.textContent = volDb(Number(e.target.value) / 100);
        });

        // ── v1.2.3: lọc tạp âm tiếng + làm sạch giọng nói ──
        propsBody.querySelector("#pr-aden").addEventListener("change", async e => {
            const fx2 = currentFx(); fx2.audioDenoise = e.target.checked;
            try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); toast(e.target.checked ? "✨ Sẽ lọc tạp âm tiếng khi dựng (render lần đầu hơi chậm hơn)" : "Đã tắt lọc tạp âm tiếng", "ok"); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-vocal").addEventListener("change", async e => {
            const fx2 = currentFx(); fx2.vocalEnhance = e.target.checked;
            try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); toast(e.target.checked ? "🎙 Giọng nói sẽ được làm sạch khi dựng" : "Đã tắt làm sạch giọng nói", "ok"); }
            catch (err) { toast(err.message, "err"); }
        });

        // ── v1.2.3: 🔗 Tách âm thanh (Link/Unlink) ──
        propsBody.querySelector("#pr-detach").addEventListener("click", async () => {
            try {
                const as = await call("DetachAudio", c.id);
                refresh();
                toast("🔗 Đã tách «" + (as && as.name ? as.name : "âm thanh") + "» vào thư viện — clip gốc đã tắt tiếng", "ok");
            } catch (err) { toast(err.message, "err"); }
        });

        propsBody.querySelectorAll(".rot").forEach(b => b.addEventListener("click", async () => {
            try { await call("UpdateClip", c.id, { rotateDeg: Number(b.dataset.deg) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        }));
        propsBody.querySelector("#pr-fliph").addEventListener("click", async () => {
            try { await call("UpdateClip", c.id, { flipH: !c.transform.flipH }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-flipv").addEventListener("click", async () => {
            try { await call("UpdateClip", c.id, { flipV: !c.transform.flipV }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-zoom").addEventListener("input", e => {
            propsBody.querySelector("#pr-zoomlabel").textContent = Number(e.target.value).toFixed(2) + "×";
        });
        propsBody.querySelector("#pr-zoom").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { zoom: Number(e.target.value) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        // ── v1.2.3: pan vị trí + opacity ──
        const bindLabelRange = (rid, lid, suffix) => {
            const r = propsBody.querySelector(rid), l = propsBody.querySelector(lid);
            if (r && l) r.addEventListener("input", e => { l.textContent = e.target.value + suffix; });
        };
        bindLabelRange("#pr-posx", "#pr-posxlabel", "%");
        bindLabelRange("#pr-posy", "#pr-posylabel", "%");
        bindLabelRange("#pr-opacity", "#pr-oplabel", "%");
        propsBody.querySelector("#pr-posx").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { posX: Number(e.target.value) / 100 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-posy").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { posY: Number(e.target.value) / 100 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-opacity").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { opacity: Number(e.target.value) / 100 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-reframe-c").addEventListener("click", async () => {
            try {
                await call("UpdateClip", c.id, { posX: 0.5, posY: 0.5 });
                refresh(); toast("⌖ Đã căn pan về giữa khung", "ok");
            } catch (err) { toast(err.message, "err"); }
        });

        // ── v1.1: fade âm thanh ──
        propsBody.querySelector("#pr-fadein").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { audioFadeInMs: Math.round(Number(e.target.value) * 1000) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-fadeout").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { audioFadeOutMs: Math.round(Number(e.target.value) * 1000) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });

        // ── v1.1: tốc độ / ngược / boomerang / lặp / freeze / waterdrop ──
        const speedSlider = propsBody.querySelector("#pr-speed");
        speedSlider.addEventListener("input", e => {
            propsBody.querySelector("#pr-speedlabel").textContent = Number(e.target.value).toFixed(2) + "×";
        });
        speedSlider.addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { speed: Number(e.target.value) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-reverse").addEventListener("click", async () => {
            try { await call("UpdateClip", c.id, { reverse: !c.reverse }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-boom").addEventListener("click", async () => {
            try { await call("UpdateClip", c.id, { boomerang: !c.boomerang }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-loop").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { loopN: Math.max(1, Math.round(Number(e.target.value) || 1)) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-fzstart").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { freezeStartMs: Math.round(Number(e.target.value) * 1000) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-fzend").addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { freezeEndMs: Math.round(Number(e.target.value) * 1000) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        const wd = propsBody.querySelector("#pr-wd");
        wd.addEventListener("input", e => {
            propsBody.querySelector("#pr-wdlabel").textContent = Number(e.target.value).toFixed(1) + "s";
        });
        wd.addEventListener("change", async e => {
            try { await call("UpdateClip", c.id, { waterDropMs: Math.round(Number(e.target.value) * 1000) }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });

        // ── v1.1: hiệu ứng màu + preset looks ──
        const currentFx = () => JSON.parse(JSON.stringify(c.effects || {}));
        const looks = {
            cine:  { brightness: 0, contrast: 1.12, saturation: 1.1, gamma: 1.05, vignette: 0.25 },
            warm:  { brightness: 0.03, saturation: 1.15, gamma: 1.08 },
            cold:  { saturation: 0.9, contrast: 1.08, gamma: 0.95 },
            vivid: { saturation: 1.5, contrast: 1.15, sharpen: 0.5 },
            mono:  { grayscale: true },
            dream: { blur: 1.5, brightness: 0.08, saturation: 1.1, gamma: 1.1 },
            noir:  { grayscale: true, contrast: 1.5, vignette: 0.5 },
        };
        propsBody.querySelector("#pr-look").addEventListener("change", async e => {
            const p = looks[e.target.value];
            if (!p) return;
            try { await call("UpdateClip", c.id, { effects: p }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelectorAll("input.fx").forEach(sl => {
            sl.addEventListener("change", async e => {
                const fx2 = currentFx();
                const k = e.target.dataset.k;
                let v = Number(e.target.value);
                if (k === "brightness") fx2.brightness = v / 100;
                else if (k === "contrast" || k === "saturation" || k === "gamma") fx2[k] = v / 100;
                else fx2[k] = v;
                try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); }
                catch (err) { toast(err.message, "err"); }
            });
        });
        propsBody.querySelector("#pr-gray").addEventListener("click", async () => {
            const fx2 = currentFx(); fx2.grayscale = !fx2.grayscale;
            try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-inv").addEventListener("click", async () => {
            const fx2 = currentFx(); fx2.invert = !fx2.invert;
            try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-dn").addEventListener("click", async () => {
            const fx2 = currentFx(); fx2.denoise = !fx2.denoise;
            try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });

        // ── v1.2.3: tông màu theo vùng sáng (Shadows/Midtones/Highlights) ──
        const CB_TINTS = {
            warm: { r: 0.15, g: 0.06, b: -0.18 },  // ấm: thêm đỏ + vàng, bớt xanh dương
            cool: { r: -0.10, g: 0.0, b: 0.18 },   // lạnh: thêm xanh dương
            teal: { r: -0.18, g: 0.05, b: 0.15 },  // teal điện ảnh
        };
        const applyCB = async () => {
            const fx2 = currentFx();
            const pick = (el) => el.value ? CB_TINTS[el.value] : null;
            const setRange = (el, R, G, B) => { if (el) { el.R = R; el.G = G; el.B = B; } };
            const s = pick(propsBody.querySelector("#pr-cb-s"));
            const m = pick(propsBody.querySelector("#pr-cb-m"));
            const h = pick(propsBody.querySelector("#pr-cb-h"));
            const zero = { r: 0, g: 0, b: 0 };
            const S = s || zero, M = m || zero, H = h || zero;
            fx2.CBShadowR = S.r; fx2.CBShadowG = S.g; fx2.CBShadowB = S.b;
            fx2.CBMidR = M.r; fx2.CBMidG = M.g; fx2.CBMidB = M.b;
            fx2.CBHighR = H.r; fx2.CBHighG = H.g; fx2.CBHighB = H.b;
            try { await call("UpdateClip", c.id, { effects: fx2 }); refresh(); toast("Đã áp tông màu vùng sáng (xem trước bằng khung/xuất)", "ok"); }
            catch (err) { toast(err.message, "err"); }
        };
        ["#pr-cb-s", "#pr-cb-m", "#pr-cb-h"].forEach(id => {
            const el = propsBody.querySelector(id);
            if (el) el.addEventListener("change", applyCB);
        });

        // ── v1.2.3: 🎨 Khớp màu với clip chuẩn ──
        propsBody.querySelector("#pr-match").addEventListener("click", async () => {
            const others = State.doc.clips.filter(x => x.id !== c.id);
            if (!others.length) { toast("Cần ít nhất 2 clip trên timeline để khớp màu"); return; }
            const names = others.map((x, i) => {
                const a = State.findAsset(x.assetId);
                return (i + 1) + ". " + (a ? a.name : "?") + " [" + fmtMs(x.inMs) + "→" + fmtMs(x.outMs) + "]";
            }).join("\n");
            const pick = prompt("Chọn clip CHUẨN (gõ số) — màu của clip này sẽ được chỉnh theo clip chuẩn:\n" + names, "1");
            if (!pick) return;
            const idx = parseInt(pick, 10) - 1;
            if (isNaN(idx) || idx < 0 || idx >= others.length) { toast("Số không hợp lệ"); return; }
            toast("Đang phân tích màu hai clip…");
            try {
                await call("ColorMatch", others[idx].id, c.id);
                refresh();
                toast("🎨 Đã khớp màu theo clip chuẩn — tinh chỉnh thêm bằng thanh trượt nếu muốn", "ok");
            } catch (err) { toast(err.message, "err"); }
        });

        // ── v1.2.3: ⧉ Áp hiệu ứng cho tất cả clip (lớp điều chỉnh nhanh) ──
        propsBody.querySelector("#pr-applyall").addEventListener("click", async () => {
            const fx2 = JSON.parse(JSON.stringify(c.effects || {}));
            const n = State.doc.clips.length;
            if (!confirm("Áp TOÀN BỘ hiệu ứng/màu của clip này cho " + n + " clip trên timeline?")) return;
            let ok = 0;
            for (const x of State.doc.clips) {
                try { await call("UpdateClip", x.id, { effects: fx2 }); ok++; }
                catch (err) { /* bỏ qua clip lỗi */ }
            }
            refresh();
            toast("⧉ Đã áp cho " + ok + "/" + n + " clip", "ok");
        });
        propsBody.querySelector("#pr-lut-load").addEventListener("click", async () => {
            try {
                const p = await call("ChooseLUTFile");
                if (p) { await call("UpdateClip", c.id, { lutPath: p }); refresh(); }
            } catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#pr-lut-clear").addEventListener("click", async () => {
            try { await call("UpdateClip", c.id, { lutPath: "" }); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });

        // ── v1.1: keyframe presets ──
        propsBody.querySelector("#pr-kf").addEventListener("change", async e => {
            const kind = e.target.value;
            if (!kind) { await call("UpdateClip", c.id, { keyframes: [] }); refresh(); return; }
            const dur = Math.max(500, State.effMs(c));
            let keys = [];
            if (kind === "fadein") keys = [{ prop: "opacity", atMs: 0, val: 0 }, { prop: "opacity", atMs: Math.min(800, dur / 2), val: 1 }];
            else if (kind === "fadeout") keys = [{ prop: "opacity", atMs: Math.max(0, dur - 800), val: 1 }, { prop: "opacity", atMs: dur, val: 0 }];
            else if (kind === "zoomin") keys = [{ prop: "scale", atMs: 0, val: 1 }, { prop: "scale", atMs: dur, val: 1.25 }];
            else if (kind === "zoomout") keys = [{ prop: "scale", atMs: 0, val: 1.25 }, { prop: "scale", atMs: dur, val: 1 }];
            try { await call("UpdateClip", c.id, { keyframes: keys }); refresh(); toast("Đã áp mẫu hoạt hình", "ok"); }
            catch (err) { toast(err.message, "err"); }
        });

        sel.addEventListener("change", async () => {
            const durField = propsBody.querySelector("#pr-transdur-field");
            try {
                await call("SetTransition", c.id, sel.value, 800);
                durField.hidden = sel.value === "none";
                refresh();
            } catch (err) { toast(err.message, "err"); }
        });
        const durSlider = propsBody.querySelector("#pr-transdur");
        if (durSlider) {
            durSlider.addEventListener("input", e => {
                propsBody.querySelector("#pr-transdur-label").textContent = fmtMs(Number(e.target.value));
            });
            durSlider.addEventListener("change", async e => {
                try { await call("SetTransition", c.id, sel.value, Number(e.target.value)); refresh(); }
                catch (err) { toast(err.message, "err"); }
            });
        }

        propsBody.querySelector("#pr-move-l").addEventListener("click", () => moveClip(-1));
        propsBody.querySelector("#pr-move-r").addEventListener("click", () => moveClip(1));
        propsBody.querySelector("#pr-del").addEventListener("click", async () => {
            try { await call("RemoveClip", c.id); State.select(null); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
    }

    // ═══════ v1.3.0: panel thuộc tính ĐOẠN ÂM THANH A1 ═══════
    function renderAudioProps(ac) {
        const asset = State.findAsset(ac.assetId);
        const dur = Math.max(0, ac.outMs - ac.inMs);
        propsBody.innerHTML = `
            <div class="prop-group">
                <div class="group-title">🎵 Âm thanh A1 — ${asset ? asset.name : "?"}</div>
                <div class="tl-dim" style="margin-bottom:8px">Track độc lập: song song video, không phá cắt ghép. Vị trí ${fmtMs(ac.startMs)} → ${fmtMs(ac.startMs + dur)} · nguồn ${fmtMs(ac.inMs)} → ${fmtMs(ac.outMs)}</div>
                <label class="check-row"><input type="checkbox" id="au-mute" ${ac.mute ? "checked" : ""}> Tắt tiếng đoạn này</label>
                <div class="field" style="margin-top:8px">
                    <label>Âm lượng — <b id="au-vollabel">${Math.round((ac.volume || 1) * 100)}%</b> (<b id="au-dblabel">${volDb(ac.volume || 1)}</b>)</label>
                    <input type="range" id="au-vol" min="0" max="200" step="5" value="${Math.round((ac.volume || 1) * 100)}">
                </div>
                <div class="row">
                    <div class="field grow">
                        <label>Fade vào (s)</label>
                        <input type="number" id="au-fadein" step="0.1" min="0" max="60" value="${((ac.fadeInMs || 0) / 1000).toFixed(1)}">
                    </div>
                    <div class="field grow">
                        <label>Fade ra (s)</label>
                        <input type="number" id="au-fadeout" step="0.1" min="0" max="60" value="${((ac.fadeOutMs || 0) / 1000).toFixed(1)}">
                    </div>
                </div>
                <div class="field" style="margin-top:8px">
                    <label>Cắt nguồn (giây) — đầu / cuối</label>
                    <div class="row">
                        <input type="number" id="au-in" class="grow" step="0.1" min="0" value="${(ac.inMs / 1000).toFixed(2)}">
                        <input type="number" id="au-out" class="grow" step="0.1" min="0" value="${(ac.outMs / 1000).toFixed(2)}">
                    </div>
                    <div class="field" style="margin-top:6px">
                        <label>Vị trí trên timeline (giây)</label>
                        <input type="number" id="au-start" step="0.1" min="0" value="${(ac.startMs / 1000).toFixed(2)}">
                    </div>
                </div>
                <div class="btn-row" style="margin-top:10px">
                    <button class="mini-btn" id="au-atcursor" title="Đặt vị trí = con trỏ">⌖ Tại con trỏ</button>
                    <button class="mini-btn" id="au-dup" title="Nhân bản — bản sao đặt ngay sau">⧉ Nhân bản</button>
                    <button class="mini-btn wide danger" id="au-del">🗑 Xoá</button>
                </div>
                <div class="tl-dim" style="margin-top:10px">💡 Muốn chỉ nghe nhạc chèn? Chọn clip video → tick «Tắt tiếng clip» (hoặc Ctrl+L tách âm thanh rồi gỡ).</div>
            </div>`;

        const save = async (patch, msg) => {
            try { await call("UpdateAudioClip", ac.id, patch); if (msg) toast(msg, "ok"); refresh(); }
            catch (err) { toast(err.message, "err"); }
        };
        propsBody.querySelector("#au-mute").addEventListener("change", e =>
            save({ mute: e.target.checked }, e.target.checked ? "🔇 Đã tắt tiếng đoạn A1" : "🔊 Đã bật tiếng"));
        const vol = propsBody.querySelector("#au-vol");
        vol.addEventListener("input", e => {
            propsBody.querySelector("#au-vollabel").textContent = e.target.value + "%";
            propsBody.querySelector("#au-dblabel").textContent = volDb(Number(e.target.value) / 100);
        });
        vol.addEventListener("change", e => save({ volume: Number(e.target.value) / 100 }));
        propsBody.querySelector("#au-fadein").addEventListener("change", e =>
            save({ fadeInMs: Math.round(Number(e.target.value) * 1000) }));
        propsBody.querySelector("#au-fadeout").addEventListener("change", e =>
            save({ fadeOutMs: Math.round(Number(e.target.value) * 1000) }));
        propsBody.querySelector("#au-in").addEventListener("change", e =>
            save({ inMs: Math.round(Number(e.target.value) * 1000) }));
        propsBody.querySelector("#au-out").addEventListener("change", e =>
            save({ outMs: Math.round(Number(e.target.value) * 1000) }));
        propsBody.querySelector("#au-start").addEventListener("change", e =>
            save({ startMs: Math.round(Number(e.target.value) * 1000) }));
        propsBody.querySelector("#au-atcursor").addEventListener("click", () =>
            save({ startMs: Math.round(State.playheadMs) }, "⌖ Đã đặt đoạn A1 tại con trỏ"));
        propsBody.querySelector("#au-dup").addEventListener("click", async () => {
            try {
                const nc = await call("DuplicateAudioClip", ac.id);
                State.select(nc.id); refresh();
                toast("⧉ Đã nhân bản đoạn âm thanh", "ok");
            } catch (err) { toast(err.message, "err"); }
        });
        propsBody.querySelector("#au-del").addEventListener("click", async () => {
            try { await call("RemoveAudioClip", ac.id); State.select(null); refresh(); }
            catch (err) { toast(err.message, "err"); }
        });
    }

    async function moveClip(dir) {
        const i = State.doc.clips.findIndex(c => c.id === State.selection);
        if (i < 0) return;
        try {
            await call("MoveClip", State.selection, i + dir);
            refresh();
        } catch (err) { toast(err.message, "err"); }
    }

    // ═════════════════════════ XEM TRƯỚC ═════════════════════════
    const frameImg = document.getElementById("frame-img");
    const frameLoader = document.getElementById("frame-loader");
    const frameEmpty = document.getElementById("frame-empty");
    const previewVideo = document.getElementById("preview-video");
    const previewInfo = document.getElementById("preview-info");
    let lastFrameKey = "";

    // v1.2.7 SỬA LỖI "CMD NHẤP NHÁY LIÊN TỤC": trước đây mỗi lần đổi con trỏ
    // (kể cả khi đang phát khung, ~8 lần/giây) đều gọi GetClipFrameURL — mỗi
    // lần spawn một tiến trình ffmpeg + cửa sổ cmd, dồn tới khi app quá tải
    // tự tắt và để lại cmd mồ côi. Giờ: giới hạn nhịp 650ms + không dồn yêu
    // cầu khi 1 render đang chạy + luôn render KHUNG CUỐI sau khi ngừng tua.
    let frameBusy = false;
    let lastFrameAt = 0;

    async function updateFrameView(force) {
        if (State.previewMode !== "frame") return;
        const now = Date.now();
        if (!force && now - lastFrameAt < 650) return; // v1.2.7: giới hạn nhịp
        const at = State.clipAtTimeline(State.playheadMs);
        if (!at || !at.clip) {
            frameEmpty.hidden = false;
            frameImg.removeAttribute("src");
            previewInfo.textContent = "—";
            return;
        }
        frameEmpty.hidden = true;
        const localMs = Math.min(Math.max(0, at.localMs - at.clip.inMs), Math.max(0, at.clip.outMs - at.clip.inMs - 1));
        const key = at.clip.id + "@" + Math.round(localMs / 200);
        if (key === lastFrameKey && frameImg.getAttribute("src")) return;
        if (frameBusy) return; // v1.2.7: không dồn yêu cầu render
        lastFrameKey = key;
        lastFrameAt = now;
        frameBusy = true;
        frameLoader.hidden = false;
        try {
            const url = await call("GetClipFrameURL", at.clip.id, Math.round(localMs));
            frameImg.src = url + "?v=" + Date.now();
            // v1.3.1: ảnh khung nạp xong phải vẽ LẠI lớp phủ — trước đây lớp
            // chỉ hiện sau khi người dùng tương tác (bug hiển thị lần đầu)
            frameImg.addEventListener("load", () => renderOverlayPreview(), { once: true });
            const asset = State.findAsset(at.clip.assetId);
            previewInfo.textContent = (asset ? asset.name : "?") +
                " · " + fmtMs(at.clip.inMs + localMs) + " / " + fmtMs(at.clip.outMs) +
                " · canvas " + State.doc.canvas.width + "×" + State.doc.canvas.height +
                " @ " + fmtFps(State.doc.canvas.rate) + "fps";
        } catch (err) {
            previewInfo.textContent = "Không dựng được khung: " + err.message;
        } finally {
            frameLoader.hidden = true;
            frameBusy = false;
        }
    }

    function updatePreviewVideo() {
        if (State.previewURL) {
            previewVideo.src = State.previewURL + "?v=" + Date.now();
        }
    }

    // ═══ v1.3.0: LỚP PHỦ XEM TRƯỚC (DOM phủ lên khung) ═══
    // Trước đây lớp phủ (chữ/hình/PiP) KHÔNG BAO GIỜ hiện ở khung xem trước
    // vì GetClipFrameURL chỉ dựng clip trơn. Giờ vẽ trực tiếp các lớp phủ
    // đang hoạt động tại mốc con trỏ lên trên khung (xem gần đúng — render
    // thật khi «Xem trước»/«Xuất video»). Neo góc trên-trái khớp moviego
    // RelPos: x = relX*(canvas−child), y = relY*(canvas−child).
    const ovLayer = document.getElementById("ov-preview");
    const clamp01v = v => { const n = Number(v); return (isNaN(n) || n < 0) ? 0 : (n > 1 ? 1 : n); };

    function activeOverlaysAt(ms) {
        if (State.doc.hideOverlays) return [];
        return (State.doc.overlays || []).filter(o => ms >= o.startMs && ms < o.endMs);
    }

    function renderOverlayPreview() {
        if (!ovLayer) return;
        if (window.VKSOvDrag && window.VKSOvDrag.active) return; // đang kéo trực tiếp — giữ DOM
        ovLayer.innerHTML = "";
        if (State.previewMode !== "frame") return;
        const list = activeOverlaysAt(State.playheadMs);
        if (!list.length) return;
        if (!frameImg.getAttribute("src") || !frameEmpty.hidden) return;
        let iw = frameImg.naturalWidth, ih = frameImg.naturalHeight;
        let box = frameImg.getBoundingClientRect();
        let ew = box.width, eh = box.height;
        if (!iw || !ih) {
            // Ảnh chưa nạp — chờ load; nếu ảnh LỖI (không bao giờ load) thì dùng
            // kích thước khung chứa để lớp phủ vẫn hiển thị gần đúng.
            if (frameImg.dataset.ovErr === "1" || !ew || !eh) {
                ew = frameView.clientWidth - 28; eh = frameView.clientHeight - 28;
                if (ew <= 0 || eh <= 0) return;
                iw = iw || ew; ih = ih || eh; // ảnh lỗi → box = trọn khung (s=1)
                box = null;
            } else {
                frameImg.addEventListener("load", renderOverlayPreview, { once: true });
                frameImg.addEventListener("error", () => { frameImg.dataset.ovErr = "1"; renderOverlayPreview(); }, { once: true });
                return;
            }
        }
        if (box && (!ew || !eh)) return;
        const s = Math.min(ew / iw, eh / ih);
        const boxW = iw * s, boxH = ih * s;
        const offX = box ? (ew - boxW) / 2 : 0, offY = box ? (eh - boxH) / 2 : 0;
        const cvH = State.doc.canvas.height, cvW = State.doc.canvas.width;
        OVBOX = { boxW, boxH, offX, offY, cvW, cvH }; // v1.3.1: toạ độ cho kéo/thu

        for (const o of list) {
            let el = null;
            if (o.kind === "text") {
                el = document.createElement("div");
                el.className = "ovp-text";
                el.textContent = o.text || "";
                el.style.fontSize = Math.max(8, (o.fontSize || 64) * (boxH / cvH)) + "px";
                el.style.color = "#" + (o.colorHex || "FFFFFF").replace("#", "");
                if (o.outline) el.classList.add("outline");
                else if (o.bold) el.style.fontWeight = "700";
                if (o.anim) el.classList.add("anim-" + o.anim);
            } else if (o.kind === "shape") {
                el = document.createElement("div");
                el.className = "ovp-shape";
                el.style.width = (clampPctv(o.widthPct) / 100 * boxW) + "px";
                el.style.height = (clampPctv(o.heightPct) / 100 * boxH) + "px";
                el.style.background = "#" + (o.colorHex || "FFFFFF").replace("#", "");
                if (o.shape === "circle") el.style.borderRadius = "50%";
                else if (o.shape === "ellipse") el.style.borderRadius = "50% / 50%";
                else if (o.shape === "line") { el.style.height = Math.max(2, boxH * 0.004) + "px"; }
            } else if (o.kind === "media") {
                const a = State.findAsset(o.assetId);
                if (!a || a.missing || a.kind === "audio") continue;
                el = document.createElement("div");
                el.className = "ovp-pip";
                const w = clampPctv(o.scalePct) / 100 * boxW;
                el.style.width = w + "px";
                const img = document.createElement("img");
                img.draggable = false;
                // v1.3.1: cache-bust chỉ khi cần — dùng URL cache chuẩn, lỗi thì
                // rơi về GetAssetThumb (backend sinh thumbnail thật)
                img.src = thumbURL(a.id, Math.round((o.startMs + o.endMs) / 2));
                img.onerror = async () => {
                    try {
                        const u = await call("GetAssetThumb", a.id, Math.round((o.startMs + o.endMs) / 2));
                        if (u && !img.dataset.fb) { img.dataset.fb = "1"; img.src = u + "?v=" + Date.now(); }
                        else img.style.display = "none";
                    } catch (_) { img.style.display = "none"; }
                };
                el.appendChild(img);
                const tag = document.createElement("span");
                tag.className = "ovp-pip-tag";
                tag.textContent = "PiP · " + a.name;
                el.appendChild(tag);
                if (o.chromaKey && o.chromaKey.enabled) tag.textContent += " · chroma";
            }
            if (!el) continue;
            el.dataset.ovid = o.id; // v1.3.1: nhận diện để kéo/thu trực tiếp
            applyOvFxStyle(el, o);  // v1.3.1: chế độ lớp hiện cả ở khung xem trước
            ovLayer.appendChild(el);
            // hai-pass: đo kích thước thật rồi neo góc trên-trái
            const cw = el.offsetWidth, ch = el.offsetHeight;
            el.style.left = (offX + clamp01v(o.posX) * Math.max(0, boxW - cw)) + "px";
            el.style.top = (offY + clamp01v(o.posY) * Math.max(0, boxH - ch)) + "px";
            el.style.visibility = "visible";
            if (State.selection === o.id) {
                el.classList.add("ovp-selected"); // v1.3.1: viền + tay nắm khi đang chọn
                addOvHandles(el, o);
            }
        }
    }

    // v1.3.1: mở khung toạ độ + trạng thái kéo cho app.js dùng (phím mũi tên)
    let OVBOX = null;
    window.VKSOvDrag = { active: false, box: () => OVBOX };

    // v1.3.1: thể hiện chế độ lớp (hòa trộn/độ mờ/preset) ngay trên khung xem trước
    function applyOvFxStyle(el, o) {
        const FILTER_CSS = {
            bw: "grayscale(1)", negative: "invert(1)", sepia: "sepia(0.95) saturate(1.4)",
            vintage: "sepia(0.45) saturate(0.75) contrast(1.05)", vivid: "saturate(1.6) contrast(1.08)",
            contrast: "contrast(1.38) saturate(0.92)", dream: "blur(2px) brightness(1.07)",
        };
        if (o.filterPreset && FILTER_CSS[o.filterPreset]) el.style.filter = FILTER_CSS[o.filterPreset];
        const BLEND_CSS = { screen: "screen", multiply: "multiply", overlay: "overlay", darken: "darken", lighten: "lighten", add: "plus-lighter" };
        if (o.blend && BLEND_CSS[o.blend]) el.style.mixBlendMode = BLEND_CSS[o.blend];
        const op = Number(o.opacity);
        if (op > 0 && op < 1) el.style.opacity = String(op);
        else el.style.opacity = "";
    }

    // v1.3.1: tay nắm góc phải-dưới để PHÓNG TO/THU NHỎ bằng chuột
    // (listeners gắn lên window — không phụ thuộc pointer capture)
    function addOvHandles(el, o) {
        const h = document.createElement("div");
        h.className = "ovp-handle";
        h.title = "Kéo để phóng to / thu nhỏ lớp";
        el.appendChild(h);
        h.addEventListener("pointerdown", e => {
            e.preventDefault(); e.stopPropagation();
            const startX = e.clientX, startY = e.clientY;
            const startW = el.offsetWidth, startH = el.offsetHeight;
            window.VKSOvDrag.active = true;
            const onMove = ev => {
                const f = Math.max(0.15, Math.min(6, (ev.clientX - startX + startW) / Math.max(8, startW)));
                resizeOvElement(el, o, startW, startH, f);
            };
            const onUp = async () => {
                window.removeEventListener("pointermove", onMove);
                window.removeEventListener("pointerup", onUp);
                window.VKSOvDrag.active = false;
                await persistOvResize(o, el);
            };
            window.addEventListener("pointermove", onMove);
            window.addEventListener("pointerup", onUp);
        });
    }

    // v1.3.1: đổi kích thước TRỰC TIẾP theo loại lớp (giống KineMaster)
    function resizeOvElement(el, o, startW, startH, f) {
        const boxH = (OVBOX && OVBOX.boxH) || el.parentElement.clientHeight || 480;
        const cvH = (OVBOX && OVBOX.cvH) || State.doc.canvas.height || 1080;
        if (o.kind === "media") {
            el.style.width = Math.round(clampPctv(o.scalePct) * f) / 100 * (OVBOX ? OVBOX.boxW : startW / (clampPctv(o.scalePct) / 100)) + "px";
        } else if (o.kind === "text") {
            el.style.fontSize = Math.max(8, (o.fontSize || 64) * f * (boxH / cvH)) + "px";
        } else if (o.kind === "shape") {
            el.style.width = Math.max(4, startW * f) + "px";
            el.style.height = Math.max(2, startH * f) + "px";
        }
    }

    // v1.3.1: chốt kích thước mới vào model (gọi backend 1 lần khi thả chuột)
    async function persistOvResize(o, el) {
        const boxW = (OVBOX && OVBOX.boxW) || 1, boxH = (OVBOX && OVBOX.boxH) || 1;
        try {
            if (o.kind === "media") {
                const pct = Math.round(el.offsetWidth / boxW * 100);
                o.scalePct = Math.max(2, Math.min(100, pct));
                await call("UpdateOverlay", o.id, { scalePct: o.scalePct });
            } else if (o.kind === "text") {
                const cvH = (OVBOX && OVBOX.cvH) || State.doc.canvas.height || 1080;
                const fs = Math.round(el.offsetHeight ? (parseFloat(el.style.fontSize) || 8) * (cvH / boxH) : o.fontSize);
                o.fontSize = Math.max(8, Math.min(300, fs));
                await call("UpdateOverlay", o.id, { fontSize: o.fontSize });
            } else if (o.kind === "shape") {
                o.widthPct = Math.max(1, Math.min(100, Math.round(el.offsetWidth / boxW * 100)));
                o.heightPct = Math.max(1, Math.min(100, Math.round(el.offsetHeight / boxH * 100)));
                await call("UpdateOverlay", o.id, { widthPct: o.widthPct, heightPct: o.heightPct });
            }
            toast("⤢ Kích thước lớp: " + (o.kind === "media" ? o.scalePct + "% canvas" : "đã cập nhật"), "ok");
            refresh();
        } catch (err) { toast(err.message, "err"); refresh(); }
    }
    const clampPctv = v => { const n = Number(v); return (isNaN(n) || n < 1) ? 1 : (n > 100 ? 100 : n); };

    const frameView = document.getElementById("frame-view");
    // Nạp lại cờ lỗi khi ảnh khung được thay (src mới)
    if (frameImg) {
        new MutationObserver(() => { delete frameImg.dataset.ovErr; }).observe(frameImg, { attributes: true, attributeFilter: ["src"] });
    }

    // Vẽ lại lớp phủ khi ảnh khung đổi kích thước / cửa sổ đổi cỡ
    window.addEventListener("resize", renderOverlayPreview);

    // ═══ v1.3.1: KÉO DI CHUYỂN LỚP PHỦ TRỰC TIẾP TRÊN KHUNG XEM TRƯỚC ═══
    // Bắt pointerdown toàn cục trên lớp DOM phủ: bấm vào lớp (không phải tay
    // nắm) → chọn + kéo dời; thả chuột → chốt posX/posY qua UpdateOverlay.
    if (ovLayer) {
        ovLayer.addEventListener("pointerdown", e => {
            const el = e.target.closest(".ovp-text, .ovp-shape, .ovp-pip");
            if (!el || !el.dataset.ovid) return;
            if (e.target.classList.contains("ovp-handle")) return; // tay nắm tự xử lý
            e.preventDefault();
            const o = State.findOverlay(el.dataset.ovid);
            if (!o) return;
            // v1.3.1 SỬA BUG: bật cờ chặn re-render TRƯỚC khi select —
            // selection-changed chạy đồng bộ → renderOverlayPreview xoá el
            // giữa lúc kéo nếu không chặn trước.
            window.VKSOvDrag.active = true;
            if (State.selection !== o.id) State.select(o.id); // panel đổi theo
            const startX = e.clientX, startY = e.clientY;
            const relX0 = clamp01v(o.posX), relY0 = clamp01v(o.posY);
            const freeW = Math.max(1, (OVBOX ? OVBOX.boxW : el.parentElement.offsetWidth) - el.offsetWidth);
            const freeH = Math.max(1, (OVBOX ? OVBOX.boxH : el.parentElement.offsetHeight) - el.offsetHeight);
            let moved = false;
            window.VKSOvDrag.active = true;
            el.classList.add("ovp-dragging");
            const onMove = ev => {
                if (Math.abs(ev.clientX - startX) + Math.abs(ev.clientY - startY) > 3) moved = true;
                if (!moved) return;
                const nx = Math.max(0, Math.min(1, relX0 + (ev.clientX - startX) / freeW));
                const ny = Math.max(0, Math.min(1, relY0 + (ev.clientY - startY) / freeH));
                o.posX = nx; o.posY = ny; // cập nhật model tạm (chốt khi thả)
                el.style.left = ((OVBOX ? OVBOX.offX : 0) + nx * freeW) + "px";
                el.style.top = ((OVBOX ? OVBOX.offY : 0) + ny * freeH) + "px";
            };
            const onUp = async () => {
                window.removeEventListener("pointermove", onMove);
                window.removeEventListener("pointerup", onUp);
                el.classList.remove("ovp-dragging");
                window.VKSOvDrag.active = false;
                if (!moved) return; // chỉ bấm → đã chọn ở trên
                try {
                    await call("UpdateOverlay", o.id, { posX: clamp01v(o.posX), posY: clamp01v(o.posY) });
                    refresh(); // doc-changed → vẽ lại đúng vị trí theo model
                } catch (err) { toast(err.message, "err"); refresh(); }
            };
            window.addEventListener("pointermove", onMove);
            window.addEventListener("pointerup", onUp);
        });
    }

    document.getElementById("preview-mode").addEventListener("click", e => {
        const btn = e.target.closest("button");
        if (!btn) return;
        State.previewMode = btn.dataset.mode;
        document.querySelectorAll("#preview-mode button").forEach(b => b.classList.toggle("active", b === btn));
        const isVideo = State.previewMode === "video";
        previewVideo.hidden = !isVideo;
        document.getElementById("frame-view").hidden = isVideo;
        if (ovLayer) ovLayer.innerHTML = ""; // v1.3.0: rời chế độ Khung → xoá lớp phủ DOM
        if (isVideo) {
            if (!State.previewURL) {
                toast("Chưa có bản render — bấm «Xem trước» trên thanh công cụ để tạo");
                State.previewMode = "frame";
                document.querySelector('#preview-mode [data-mode="frame"]').classList.add("active");
                btn.classList.remove("active");
                document.getElementById("frame-view").hidden = false;
                previewVideo.hidden = true;
            } else {
                updatePreviewVideo();
            }
        } else {
            updateFrameView();
        }
    });

    // Con trỏ thay đổi → cập nhật khung (v1.2.7: đủ nhịp mới render; luôn
    // render 1 khung CUỐI sau 700ms ngừng tua/phát để không kẹt khung cũ)
    let frameTimer = null;
    document.addEventListener("vks:playhead-changed", () => {
        updateFrameView();
        renderOverlayPreview(); // v1.3.0: lớp phủ theo con trỏ (nhẹ, không render)
        clearTimeout(frameTimer);
        frameTimer = setTimeout(() => updateFrameView(true), 700);
    });
    // v1.2.6 SỬA LỖI NGHIÊM TRỌNG: panel THUỘC TÍNH phải vẽ lại khi ĐỔI LỰA CHỌN.
    // Trước đây renderProps chỉ chạy khi doc đổi → chọn clip xong panel vẫn trống,
    // người dùng tưởng selection hỏng + không tìm thấy công cụ (kể cả chuyển cảnh).
    // v1.2.7: đổi chọn/doc → render NGAY (force), không bị chặn giới hạn nhịp.
    document.addEventListener("vks:selection-changed", () => { lastFrameKey = ""; updateFrameView(true); renderProps(); renderOverlayPreview(); });
    document.addEventListener("vks:doc-changed", () => { lastFrameKey = ""; renderLibrary(); renderProps(); updateFrameView(true); renderOverlayPreview(); });

    function refresh() {
        call("GetState").then(s => State.setDoc(s.doc));
    }

    function toast(msg, kind) {
        if (window.VKSApp) window.VKSApp.toast(msg, kind);
    }

    // Export cho app.js
    window.VKSPanels = {
        renderLibrary, renderProps, updateFrameView, updatePreviewVideo,
        setPreviewURL(url) { State.previewURL = url; updatePreviewVideo(); },
        renderOverlayPreview, // v1.3.0
    };
})();
