// Sizing and small view helpers shared by every chart. Phones (narrow, or
// short in landscape) get fixed pixel heights instead of aspect ratios, so a
// chart's plot area doesn't shrink to a sliver as the screen narrows.

// Matches the CSS media query `(max-width: 600px), (max-height: 500px)`.
export function isCompact(width = globalThis.innerWidth, height = globalThis.innerHeight) {
    return width <= 600 || height <= 500;
}

export const COMPACT_QUERY = '(max-width: 600px), (max-height: 500px)';

// Chart heights in px on phones; the wind direction strip uses its height at
// every width (an aspect ratio would flatten it on mid-width screens).
export const CHART_HEIGHTS = { main: 240, secondary: 200, direction: 110 };

// kind: 'main' (room, weather, outdoor charts), 'secondary' (Tempest panels)
// or 'direction'. ar is the desktop aspect ratio from getAspectRatio().
export function sizingFor(kind, compact, ar) {
    if (kind === 'direction') return { maintainAspectRatio: false, height: CHART_HEIGHTS.direction };
    if (compact) return { maintainAspectRatio: false, height: CHART_HEIGHTS[kind] ?? CHART_HEIGHTS.main };
    return { maintainAspectRatio: true, aspectRatio: kind === 'secondary' ? ar * 1.6 : ar, height: null };
}

const MIN_SPAN_MS = 10 * 60_000;

// Zooms [fromMs, toMs] by factor (0.5 in, 2 out) around its centre, never
// past now and never narrower than 10 minutes.
export function zoomRange(fromMs, toMs, factor, nowMs = Date.now()) {
    const span = Math.max(MIN_SPAN_MS, Math.round((toMs - fromMs) * factor));
    const centre = (fromMs + toMs) / 2;
    let from = Math.round(centre - span / 2);
    let to = from + span;
    if (to > nowMs) {
        to = nowMs;
        from = nowMs - span;
    }
    return { fromMs: from, toMs: to };
}

// Wraps a resize handler: debounced, and run only when the width changed.
// iOS fires resize as the address bar shows and hides (height only); redrawing
// a dozen charts for that is wasted work.
export function resizeGate(fn, ms = 150, getWidth = () => globalThis.innerWidth) {
    let lastWidth = getWidth();
    let timer = null;
    return () => {
        clearTimeout(timer);
        timer = setTimeout(() => {
            const w = getWidth();
            if (w === lastWidth) return;
            lastWidth = w;
            fn();
        }, ms);
    };
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function hourLabel(d) {
    const h = d.getHours() % 12 || 12;
    const m = d.getMinutes();
    const ampm = d.getHours() < 12 ? 'AM' : 'PM';
    return m ? `${h}:${String(m).padStart(2, '0')} ${ampm}` : `${h} ${ampm}`;
}

// One-line x tick for phones: the date on the first tick of each day, else
// the time.
export function compactTick(ticks, index) {
    const d = new Date(ticks[index].value);
    const first = new Date(ticks[0].value);
    const last = new Date(ticks[ticks.length - 1].value);
    const sameDay = first.toDateString() === last.toDateString();
    const prev = index > 0 ? new Date(ticks[index - 1].value) : null;
    const newDay = !prev || prev.toDateString() !== d.toDateString();
    if (!sameDay && newDay) return `${DAYS[d.getDay()]} ${d.getMonth() + 1}/${d.getDate()}`;
    return hourLabel(d);
}

// Y tick label rounded to what the axis can show: the padded min/max
// (e.g. 80.07, 36.3) otherwise print with stray decimals.
export function roundTick(value) {
    const v = Number(value);
    const a = Math.abs(v);
    if (a >= 10) return String(Math.round(v));
    if (a >= 1) return String(Number(v.toFixed(1)));
    return String(Number(v.toFixed(2)));
}

// Applies sizingFor() to a chart's options. On phones the canvas tooltip and
// legend give way to the shared touch readout and the HTML key row. A legend
// marked keyRow: false is off at every width.
export function sizeOptions(opts, kind, compact, ar) {
    const s = sizingFor(kind, compact, ar);
    opts.maintainAspectRatio = s.maintainAspectRatio;
    if (s.aspectRatio) opts.aspectRatio = s.aspectRatio;
    // Mutated in place: on a live chart these are Chart.js option proxies,
    // and replacing them would drop resolved settings such as callbacks.
    if (!opts.plugins) opts.plugins = {};
    if (!opts.plugins.tooltip) opts.plugins.tooltip = {};
    opts.plugins.tooltip.enabled = !compact;
    if (!opts.plugins.legend) opts.plugins.legend = {};
    if (opts.plugins.legend.keyRow !== false) opts.plugins.legend.display = !compact;
    return opts;
}

// Rounds a shared axis range outward to whole steps (and up to floor), so
// the end labels are round numbers like the ticks between them.
export function niceRange(range, step, floor = -Infinity) {
    if (!range) return null;
    return {
        min: Math.max(floor, Math.floor(range.min / step) * step),
        max: Math.ceil(range.max / step) * step,
    };
}

// The range button's label on phones: "Last 3 days" becomes "3 days" so it
// fits beside five 44 px buttons on a 320 px screen.
export function compactRangeLabel(label) {
    return label.startsWith('Last ') ? label.slice(5) : label;
}
