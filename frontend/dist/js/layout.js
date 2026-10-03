/* layout.js — v1.2.5 "Giao diện linh hoạt":
   · Kéo 3 vạch divider: Thư viện ⇔ Xem trước ⇔ Thuộc tính (ngang)
     và vạch đáy ⇕ chiều cao timeline (dọc).
   · Nhấp đúp vạch = về kích thước mặc định.
   · Nút « / » thu gọn / mở lại panel hai bên (tab dọc để mở lại).
   · Mọi thiết đặt lưu localStorage "vks.layout" — lần sau mở lại giữ nguyên.
   Không phụ thuộc backend — thuần giao diện. */
(function () {
    "use strict";

    const KEY = "vks.layout";
    const appEl = document.getElementById("app");

    const DEFAULTS = { lib: 272, props: 304, tl: 296, libCollapsed: false, propsCollapsed: false };
    // Giới hạn kéo (px) — chặn kéo để panel "biến mất" ngoài ý muốn.
    const LIMITS = {
        lib:   { min: 170, max: 520 },
        props: { min: 230, max: 520 },
        tl:    { min: 150, max: 0 }, // max tính động = 70% chiều cao cửa sổ
    };

    let saved = { ...DEFAULTS };
    try {
        const raw = localStorage.getItem(KEY);
        if (raw) {
            const o = JSON.parse(raw);
            if (o && typeof o === "object") saved = { ...DEFAULTS, ...o };
        }
    } catch (_) { /* dữ liệu hỏng — dùng mặc định */ }

    function apply() {
        const clamp = (v, lim) => Math.max(lim.min, Math.min(lim.max || Infinity, Math.round(v)));
        saved.lib = clamp(saved.lib, LIMITS.lib);
        saved.props = clamp(saved.props, LIMITS.props);
        saved.tl = clamp(saved.tl, { min: LIMITS.tl.min, max: Math.round(window.innerHeight * 0.7) });
        appEl.style.setProperty("--lib-w", saved.lib + "px");
        appEl.style.setProperty("--props-w", saved.props + "px");
        appEl.style.setProperty("--tl-h", saved.tl + "px");
        document.getElementById("library").classList.toggle("collapsed", !!saved.libCollapsed);
        document.getElementById("props").classList.toggle("collapsed", !!saved.propsCollapsed);
        updateTabs();
        updateCollapseBtns();
        if (window.VKSTimeline) { try { window.VKSTimeline.resize(); } catch (_) {} }
    }

    function save() {
        try { localStorage.setItem(KEY, JSON.stringify(saved)); } catch (_) {}
    }

    // ───── Tab mở lại khi panel thu gọn ─────
    function updateTabs() {
        ensureTab("tab-lib", "Thư viện", false, () => { saved.libCollapsed = false; saved.lib = Math.max(saved.lib, DEFAULTS.lib); apply(); save(); });
        ensureTab("tab-props", "Thuộc tính", true, () => { saved.propsCollapsed = false; saved.props = Math.max(saved.props, DEFAULTS.props); apply(); save(); });
        const tl = document.getElementById("tab-lib");
        const tp = document.getElementById("tab-props");
        tl.style.display = saved.libCollapsed ? "" : "none";
        tp.style.display = saved.propsCollapsed ? "" : "none";
    }

    function ensureTab(id, label, isRight, onClick) {
        let el = document.getElementById(id);
        if (!el) {
            el = document.createElement("button");
            el.id = id;
            el.className = "reopen-tab" + (isRight ? " right" : "");
            el.textContent = label;
            el.style.left = isRight ? "auto" : "0";
            if (isRight) el.style.right = "0";
            el.title = "Mở lại panel " + label;
            el.addEventListener("click", onClick);
            document.body.appendChild(el);
        }
        return el;
    }

    function updateCollapseBtns() {
        const bl = document.getElementById("lib-collapse");
        const bp = document.getElementById("props-collapse");
        if (bl) { bl.textContent = saved.libCollapsed ? "»" : "«"; bl.classList.toggle("active", !saved.libCollapsed); }
        if (bp) { bp.textContent = saved.propsCollapsed ? "«" : "»"; bp.classList.toggle("active", !saved.propsCollapsed); }
    }

    // ───── Kéo divider dùng chung ─────
    function drag(el, onMove, onReset, baseFn) {
        let startX = 0, startY = 0, base = 0, dragging = false;
        el.addEventListener("pointerdown", e => {
            if (e.button !== 0) return;
            dragging = true;
            startX = e.clientX; startY = e.clientY;
            base = baseFn ? baseFn() : 0;
            el.classList.add("active");
            document.body.classList.add("resizing");
            try { el.setPointerCapture(e.pointerId); } catch (_) {}
            e.preventDefault();
        });
        el.addEventListener("pointermove", e => {
            if (!dragging) return;
            onMove(e.clientX - startX, e.clientY - startY, base);
        });
        const stop = () => {
            if (!dragging) return;
            dragging = false;
            el.classList.remove("active");
            document.body.classList.remove("resizing");
            save();
        };
        el.addEventListener("pointerup", stop);
        el.addEventListener("pointercancel", stop);
        el.addEventListener("dblclick", () => { onReset(); apply(); save(); });
    }

    // ───── Gắn 3 vạch ─────
    const dLib = document.getElementById("div-lib");
    const dProps = document.getElementById("div-props");
    const dTl = document.getElementById("div-tl");

    // base() trả kích thước xuất phát; new = base ± delta (KHÔNG cộng dồn).
    if (dLib) drag(dLib,
        (dx, dy, base) => {
            saved.libCollapsed = false;
            saved.lib = Math.min(LIMITS.lib.max, Math.max(LIMITS.lib.min, base + dx));
            apply();
        },
        () => { saved.lib = DEFAULTS.lib; saved.libCollapsed = false; },
        () => (saved.libCollapsed ? LIMITS.lib.min : saved.lib));

    if (dProps) drag(dProps,
        (dx, dy, base) => {
            saved.propsCollapsed = false;
            saved.props = Math.min(LIMITS.props.max, Math.max(LIMITS.props.min, base - dx));
            apply();
        },
        () => { saved.props = DEFAULTS.props; saved.propsCollapsed = false; },
        () => (saved.propsCollapsed ? LIMITS.props.min : saved.props));

    if (dTl) drag(dTl,
        (_dx, dy, base) => {
            saved.tl = Math.max(LIMITS.tl.min, Math.min(Math.round(window.innerHeight * 0.7), base - dy));
            apply();
        },
        () => { saved.tl = DEFAULTS.tl; },
        () => saved.tl);

    // ───── Nút thu gọn ─────
    const bLib = document.getElementById("lib-collapse");
    const bProps = document.getElementById("props-collapse");
    if (bLib) bLib.addEventListener("click", () => {
        saved.libCollapsed = !saved.libCollapsed;
        if (!saved.libCollapsed && saved.lib < LIMITS.lib.min) saved.lib = DEFAULTS.lib;
        apply(); save();
    });
    if (bProps) bProps.addEventListener("click", () => {
        saved.propsCollapsed = !saved.propsCollapsed;
        if (!saved.propsCollapsed && saved.props < LIMITS.props.min) saved.props = DEFAULTS.props;
        apply(); save();
    });

    // Cửa sổ đổi cỡ → kẹp lại chiều cao timeline trong giới hạn 70%.
    window.addEventListener("resize", () => {
        const max = Math.round(window.innerHeight * 0.7);
        if (saved.tl > max) { saved.tl = max; apply(); }
    });

    // ───── Áp dụng lần đầu (layout.js được nạp trước timeline.js) ─────
    apply();

    window.VKSLayout = {
        reset: () => { saved = { ...DEFAULTS }; apply(); save(); },
        get: () => ({ ...saved }),
    };
})();
