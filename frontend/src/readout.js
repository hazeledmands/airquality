// Touch readout: on phones, tapping or press-and-dragging a chart moves the
// synced crosshair and shows the values in one readout pinned under the
// header, instead of a tooltip covering the chart.

const toMs = (x) => (typeof x === 'number' ? x : new Date(x).getTime());

// The non-null point nearest t, or null if none is within 1.5 typical steps.
export function nearestPoint(data, t) {
    if (!data?.length) return null;
    const first = toMs(data[0].x);
    const last = toMs(data[data.length - 1].x);
    const step = data.length > 1 ? (last - first) / (data.length - 1) : 60_000;
    const tolerance = Math.max(2 * 60_000, step * 1.5);
    let best = null;
    let bestDist = Infinity;
    for (const p of data) {
        if (p.y == null) continue;
        const d = Math.abs(toMs(p.x) - t);
        if (d < bestDist) { best = p; bestDist = d; }
    }
    return bestDist <= tolerance ? best : null;
}

function formatValue(ds, p) {
    if (ds.stepped === true && ds.label === 'AC') return p.y ? 'on' : 'off';
    if (ds.tooltipFormat) return ds.tooltipFormat(p.y, p);
    const unit = ds.unit ?? ds.peakUnit ?? '';
    const spacer = unit && !unit.startsWith('°') && unit !== '%' ? ' ' : '';
    return p.y.toFixed(ds.tooltipDigits ?? 1) + spacer + unit;
}

// { title, lines: [{ text, color }] } for every visible dataset at time t.
export function readoutFor(chart, t) {
    const lines = [];
    chart.data.datasets.forEach((ds, i) => {
        if (!chart.isDatasetVisible(i)) return;
        const p = nearestPoint(ds.data, t);
        if (!p) return;
        lines.push({ text: `${ds.label} ${formatValue(ds, p)}`, color: ds.borderColor });
    });
    const title = new Date(t).toLocaleString([], { weekday: 'short', month: 'numeric', day: 'numeric', hour: 'numeric', minute: '2-digit' });
    return { title, lines };
}

// Gesture state machine for one chart. A quick tap shows the readout; a
// swipe scrolls the page as usual; press-and-hold, or a tap followed by a
// horizontal drag, scrubs.
export function createGesture({ slop = 10, tapMs = 300, retapMs = 500 } = {}) {
    let state = 'idle';
    let x0 = 0, y0 = 0, t0 = 0;
    let lastTap = -Infinity;
    return {
        start(x, y, t) {
            x0 = x; y0 = y; t0 = t;
            state = t - lastTap < retapMs ? 'armed' : 'pending';
        },
        move(x, y) {
            const dx = Math.abs(x - x0), dy = Math.abs(y - y0);
            if (state === 'scrub') return 'scrub';
            if (state === 'armed') {
                if (dx > slop && dx >= dy) state = 'scrub';
                else if (dy > slop) state = 'scroll';
                return state === 'armed' ? 'pending' : state;
            }
            if (state === 'pending' && (dx > slop || dy > slop)) state = 'scroll';
            return state;
        },
        longPress() {
            if (state !== 'pending' && state !== 'armed') return false;
            state = 'scrub';
            return true;
        },
        end(t) {
            const was = state;
            state = 'idle';
            if (was === 'scrub') return 'scrub-end';
            if ((was === 'pending' || was === 'armed') && t - t0 < tapMs) {
                lastTap = t;
                return 'tap';
            }
            return 'none';
        },
        get scrubbing() { return state === 'scrub'; },
    };
}
