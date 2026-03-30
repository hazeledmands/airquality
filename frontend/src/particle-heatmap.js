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

        ctx.fillStyle = '#555';
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
