import { PARTICLE_BINS } from './room-charts.js';
import { onCrosshairChange, setCrosshairTimestamp } from './crosshair.js';
import { registerHeatmap, unregisterHeatmap } from './drag-zoom.js';

const ROW_H = 11;   // px per bin row
const LABEL_W = 50; // px for left-side labels
const GAP = 1;      // px gap between rows
const DAY_MS = 86400000;
const WEEK_MS = 7 * DAY_MS;

export const TOTAL_H = PARTICLE_BINS.length * (ROW_H + GAP);

export function renderHeatmap(canvas, particles, xRange, crosshairT = null, dragState = null, stepMs = 0) {
    const dpr = (typeof window !== 'undefined' && window.devicePixelRatio) || 1;
    const cssW = canvas.offsetWidth || 600;
    canvas.width  = cssW * dpr;
    canvas.height = TOTAL_H * dpr;
    canvas.style.height = `${TOTAL_H}px`;

    const ctx = canvas.getContext('2d');
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, cssW, TOTAL_H);

    const { min: xMin, max: xMax } = xRange;
    const plotW = cssW - LABEL_W;

    PARTICLE_BINS.forEach((bin, row) => {
        const seriesArr = particles?.[bin.field] || [];
        const allPoints = seriesArr
            .flatMap(s => s.data)
            .filter(p => {
                const t = +new Date(p.x);
                return t >= xMin && t <= xMax;
            })
            .sort((a, b) => +new Date(a.x) - +new Date(b.x));

        const y = row * (ROW_H + GAP);

        ctx.fillStyle = '#888';
        ctx.font = '9px monospace';
        ctx.textAlign = 'right';
        ctx.textBaseline = 'middle';
        ctx.fillText(bin.rangeLabel, LABEL_W - 3, y + ROW_H / 2);

        if (allPoints.length === 0) return;
        if (!stepMs && allPoints.length < 2) return;

        const maxVal = Math.max(...allPoints.map(p => p.y));
        if (maxVal === 0) return;

        if (stepMs > 0) {
            const cellW = Math.max(1, (stepMs / (xMax - xMin)) * plotW);
            for (const pt of allPoints) {
                const t0 = +new Date(pt.x);
                const x0 = LABEL_W + ((t0 - xMin) / (xMax - xMin)) * plotW;
                const intensity = pt.y / maxVal;
                ctx.fillStyle = colorForIntensity(intensity);
                ctx.fillRect(x0, y, cellW, ROW_H);
            }
        } else {
            for (let i = 0; i < allPoints.length - 1; i++) {
                const t0 = +new Date(allPoints[i].x);
                const t1 = +new Date(allPoints[i + 1].x);
                const x0 = LABEL_W + ((t0 - xMin) / (xMax - xMin)) * plotW;
                const x1 = LABEL_W + ((t1 - xMin) / (xMax - xMin)) * plotW;
                const intensity = allPoints[i].y / maxVal;
                ctx.fillStyle = colorForIntensity(intensity);
                ctx.fillRect(x0, y, Math.max(1, x1 - x0), ROW_H);
            }
        }
    });

    if (crosshairT != null && xMax > xMin && crosshairT >= xMin && crosshairT <= xMax) {
        const cx = LABEL_W + ((crosshairT - xMin) / (xMax - xMin)) * plotW;
        const rangeMs = xMax - xMin;
        const periodMs = rangeMs > WEEK_MS ? WEEK_MS : DAY_MS;
        if (rangeMs > periodMs) {
            ctx.save();
            ctx.lineWidth = 1;
            ctx.strokeStyle = 'rgba(255,255,255,0.1)';
            ctx.setLineDash([3, 4]);
            for (let t = crosshairT - periodMs; t >= xMin; t -= periodMs) {
                const sx = LABEL_W + ((t - xMin) / (xMax - xMin)) * plotW;
                ctx.beginPath(); ctx.moveTo(sx, 0); ctx.lineTo(sx, TOTAL_H); ctx.stroke();
            }
            for (let t = crosshairT + periodMs; t <= xMax; t += periodMs) {
                const sx = LABEL_W + ((t - xMin) / (xMax - xMin)) * plotW;
                ctx.beginPath(); ctx.moveTo(sx, 0); ctx.lineTo(sx, TOTAL_H); ctx.stroke();
            }
            ctx.restore();
        }
        ctx.save();
        ctx.beginPath();
        ctx.moveTo(cx, 0);
        ctx.lineTo(cx, TOTAL_H);
        ctx.lineWidth = 1;
        ctx.strokeStyle = 'rgba(255,255,255,0.3)';
        ctx.stroke();
        ctx.restore();
    }

    if (dragState != null && xMax > xMin) {
        const x1 = Math.max(LABEL_W, Math.min(cssW, LABEL_W + ((dragState.startT - xMin) / (xMax - xMin)) * plotW));
        const x2 = Math.max(LABEL_W, Math.min(cssW, LABEL_W + ((dragState.currentT - xMin) / (xMax - xMin)) * plotW));
        const xl = Math.min(x1, x2);
        const xr = Math.max(x1, x2);
        const w = xr - xl;
        if (w >= 1) {
            ctx.save();
            ctx.fillStyle = 'rgba(56, 189, 248, 0.15)';
            ctx.fillRect(xl, 0, w, TOTAL_H);
            ctx.strokeStyle = 'rgba(56, 189, 248, 0.5)';
            ctx.lineWidth = 1;
            ctx.strokeRect(xl, 0, w, TOTAL_H);
            ctx.restore();
        }
    }
}

export function findClosestPoint(points, t) {
    if (!points || points.length === 0) return null;
    return points.reduce((best, pt) => {
        return Math.abs(+new Date(pt.x) - t) < Math.abs(+new Date(best.x) - t) ? pt : best;
    });
}

let _tooltip = null;

function getTooltip() {
    if (_tooltip) return _tooltip;
    _tooltip = document.createElement('div');
    _tooltip.style.cssText = [
        'position:fixed', 'z-index:9999', 'pointer-events:none', 'display:none',
        'background:#1e1e30', 'border:1px solid #333', 'border-radius:4px',
        'padding:6px 10px', 'font-size:11px', 'line-height:1.6',
        'color:#aaa', 'white-space:nowrap',
    ].join(';');
    document.body.appendChild(_tooltip);
    return _tooltip;
}

export function setupHeatmapInteraction(canvas, getState) {
    const tooltip = getTooltip();
    const myId = 'heatmap-' + Math.random();
    let currentCrosshairT = null;

    function redraw() {
        const { particles, xRange, stepMs } = getState();
        if (particles && xRange && xRange.min != null) {
            renderHeatmap(canvas, particles, xRange, currentCrosshairT, null, stepMs);
        }
    }

    const unsubscribeCrosshair = onCrosshairChange((t, sourceId) => {
        if (sourceId === myId) return;
        currentCrosshairT = t;
        redraw();
    });

    const toTime = (cssX) => {
        const { xRange } = getState();
        if (!xRange || xRange.min == null) return null;
        const rect = canvas.getBoundingClientRect();
        const plotW = rect.width - LABEL_W;
        if (cssX < LABEL_W || cssX > rect.width || plotW <= 0) return null;
        return xRange.min + ((cssX - LABEL_W) / plotW) * (xRange.max - xRange.min);
    };
    const toPx = (t) => {
        const { xRange } = getState();
        const rect = canvas.getBoundingClientRect();
        return LABEL_W + ((t - xRange.min) / (xRange.max - xRange.min)) * (rect.width - LABEL_W);
    };
    registerHeatmap(canvas, toTime, toPx, redraw);

    canvas.addEventListener('mousemove', (e) => {
        const { particles, xRange } = getState();
        if (!particles || !xRange || xRange.min == null) return;

        const rect = canvas.getBoundingClientRect();
        const cssX = e.clientX - rect.left;
        const plotW = rect.width - LABEL_W;
        if (cssX < LABEL_W) { tooltip.style.display = 'none'; return; }

        const ratio = (cssX - LABEL_W) / plotW;
        const t = xRange.min + ratio * (xRange.max - xRange.min);

        currentCrosshairT = t;
        setCrosshairTimestamp(t, myId);
        redraw();

        // Find the closest timestamp using p03 as reference
        const refPoints = (particles.p03 || []).flatMap(s => s.data);
        const closest = findClosestPoint(refPoints, t);
        if (!closest) { tooltip.style.display = 'none'; return; }
        const closestT = +new Date(closest.x);

        // Look up each bin's value at that timestamp
        const rows = PARTICLE_BINS.map(bin => {
            const pts = (particles[bin.field] || []).flatMap(s => s.data);
            const pt = pts.find(p => +new Date(p.x) === closestT);
            const val = pt != null ? pt.y.toLocaleString('en', { maximumFractionDigits: 0 }) : '—';
            return `<div><span style="color:hsl(${HEATMAP_HUE},70%,55%)">\u25a0</span> ${bin.rangeLabel} <span style="color:#ccc;float:right;margin-left:12px">${val}</span></div>`;
        }).join('');

        const title = new Date(closestT).toLocaleString('en', {
            month: 'short', day: 'numeric',
            hour: 'numeric', minute: '2-digit',
        });

        tooltip.innerHTML = `<div style="color:#ccc;margin-bottom:3px;font-weight:600">${title}</div>${rows}<div style="color:#555;margin-top:3px;font-size:10px">#/dL (per-row max\u00a0=\u00a0100%)</div>`;
        tooltip.style.display = 'block';

        // Keep tooltip inside viewport
        const vw = window.innerWidth, vh = window.innerHeight;
        const tw = tooltip.offsetWidth || 180, th = tooltip.offsetHeight || 120;
        const left = e.clientX + 14 + tw > vw ? e.clientX - tw - 8 : e.clientX + 14;
        const top  = e.clientY - 10 + th > vh ? vh - th - 8 : e.clientY - 10;
        tooltip.style.left = `${left}px`;
        tooltip.style.top  = `${top}px`;
    });

    canvas.addEventListener('mouseleave', () => {
        tooltip.style.display = 'none';
        currentCrosshairT = null;
        setCrosshairTimestamp(null, myId);
        redraw();
    });

    return () => {
        unsubscribeCrosshair();
        unregisterHeatmap(canvas);
    };
}

export const HEATMAP_HUE = 200; // sky blue

export function colorForIntensity(t) {
    const s = Math.round(t * 85);
    const l = Math.round(7 + t * 48);
    return `hsl(${HEATMAP_HUE},${s}%,${l}%)`;
}
