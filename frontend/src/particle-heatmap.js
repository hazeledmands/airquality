import { PARTICLE_BINS } from './room-charts.js';

const ROW_H = 11;   // px per bin row
const LABEL_W = 38; // px for left-side labels
const GAP = 1;      // px gap between rows

export const TOTAL_H = PARTICLE_BINS.length * (ROW_H + GAP);

export function renderHeatmap(canvas, particles, xRange) {
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

        if (allPoints.length < 2) return;

        const maxVal = Math.max(...allPoints.map(p => p.y));
        if (maxVal === 0) return;

        for (let i = 0; i < allPoints.length - 1; i++) {
            const t0 = +new Date(allPoints[i].x);
            const t1 = +new Date(allPoints[i + 1].x);
            const x0 = LABEL_W + ((t0 - xMin) / (xMax - xMin)) * plotW;
            const x1 = LABEL_W + ((t1 - xMin) / (xMax - xMin)) * plotW;
            const intensity = allPoints[i].y / maxVal;
            ctx.fillStyle = colorForIntensity(bin.color, intensity);
            ctx.fillRect(x0, y, Math.max(1, x1 - x0), ROW_H);
        }
    });
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

    canvas.addEventListener('mousemove', (e) => {
        const { particles, xRange } = getState();
        if (!particles || !xRange || xRange.min == null) return;

        const rect = canvas.getBoundingClientRect();
        const cssX = e.clientX - rect.left;
        const plotW = rect.width - LABEL_W;
        if (cssX < LABEL_W) { tooltip.style.display = 'none'; return; }

        const ratio = (cssX - LABEL_W) / plotW;
        const t = xRange.min + ratio * (xRange.max - xRange.min);

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
            return `<div><span style="color:${bin.color}">\u25a0</span> ${bin.rangeLabel} <span style="color:#ccc;float:right;margin-left:12px">${val}</span></div>`;
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
    });
}

function colorForIntensity(hex, t) {
    const bg = [15, 15, 26];
    const fg = hexToRgb(hex);
    const r = Math.round(bg[0] + t * (fg[0] - bg[0]));
    const g = Math.round(bg[1] + t * (fg[1] - bg[1]));
    const b = Math.round(bg[2] + t * (fg[2] - bg[2]));
    return `rgb(${r},${g},${b})`;
}

function hexToRgb(hex) {
    const n = parseInt(hex.slice(1), 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}
