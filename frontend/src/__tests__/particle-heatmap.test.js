import { describe, it, expect, vi } from 'vitest';
import { renderHeatmap, TOTAL_H, findClosestPoint, colorForIntensity, HEATMAP_HUE } from '../particle-heatmap.js';
import { PARTICLE_BINS } from '../room-charts.js';

vi.mock('../charts.js', () => ({ xTickCallback: vi.fn() }));
vi.mock('../data.js', () => ({ makeWeatherTempSeries: vi.fn() }));

function makeCtx() {
    return {
        scale: vi.fn(),
        clearRect: vi.fn(),
        fillRect: vi.fn(),
        fillText: vi.fn(),
        save: vi.fn(),
        restore: vi.fn(),
        beginPath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        stroke: vi.fn(),
        setLineDash: vi.fn(),
        strokeRect: vi.fn(),
        fillStyle: '',
        font: '',
        textAlign: '',
        textBaseline: '',
        strokeStyle: '',
        lineWidth: 1,
    };
}

function makeCanvas(offsetWidth = 400) {
    const ctx = makeCtx();
    const canvas = { getContext: () => ctx, offsetWidth, style: {}, _ctx: ctx };
    return canvas;
}

function makeParticles(pointsByField) {
    const base = Object.fromEntries(PARTICLE_BINS.map(b => [b.field, []]));
    for (const [field, points] of Object.entries(pointsByField)) {
        base[field] = [{ data: points }];
    }
    return base;
}

const xRange = { min: 0, max: 10000 };

describe('TOTAL_H', () => {
    it('equals PARTICLE_BINS.length * 12', () => {
        expect(TOTAL_H).toBe(PARTICLE_BINS.length * 12);
    });
});

describe('renderHeatmap', () => {
    it('does not throw on null particles', () => {
        const canvas = makeCanvas();
        expect(() => renderHeatmap(canvas, null, xRange)).not.toThrow();
    });

    it('does not throw on empty particles object', () => {
        const canvas = makeCanvas();
        const particles = Object.fromEntries(PARTICLE_BINS.map(b => [b.field, []]));
        expect(() => renderHeatmap(canvas, particles, xRange)).not.toThrow();
    });

    it('does not throw when xRange min === max', () => {
        const canvas = makeCanvas();
        const particles = makeParticles({
            p03: [{ x: 5000, y: 500 }, { x: 6000, y: 400 }],
        });
        expect(() => renderHeatmap(canvas, particles, { min: 5000, max: 5000 })).not.toThrow();
    });

    it('sets canvas.height to TOTAL_H * dpr', () => {
        const canvas = makeCanvas();
        renderHeatmap(canvas, null, xRange);
        const dpr = (typeof window !== 'undefined' && window.devicePixelRatio) || 1;
        expect(canvas.height).toBe(TOTAL_H * dpr);
    });

    it('calls fillRect for data points within xRange', () => {
        const canvas = makeCanvas(400);
        const particles = makeParticles({
            p03: [{ x: 1000, y: 500 }, { x: 2000, y: 400 }, { x: 3000, y: 300 }],
        });
        renderHeatmap(canvas, particles, xRange);
        expect(canvas._ctx.fillRect).toHaveBeenCalled();
    });

    it('does not call fillRect for points outside xRange', () => {
        const canvas = makeCanvas(400);
        const particles = makeParticles({
            p03: [{ x: 20000, y: 500 }, { x: 30000, y: 400 }],
        });
        renderHeatmap(canvas, particles, xRange); // xRange is 0–10000
        expect(canvas._ctx.fillRect).not.toHaveBeenCalled();
    });

    it('does not call fillRect when there is only one data point and no stepMs', () => {
        const canvas = makeCanvas(400);
        const particles = makeParticles({
            p03: [{ x: 5000, y: 500 }],
        });
        renderHeatmap(canvas, particles, xRange);
        expect(canvas._ctx.fillRect).not.toHaveBeenCalled();
    });

    it('calls fillText for each bin row label', () => {
        const canvas = makeCanvas(400);
        const particles = Object.fromEntries(PARTICLE_BINS.map(b => [b.field, []]));
        renderHeatmap(canvas, particles, xRange);
        // One fillText call per bin row
        expect(canvas._ctx.fillText).toHaveBeenCalledTimes(PARTICLE_BINS.length);
    });

    it('calls ctx.scale with dpr', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange);
        const dpr = (typeof window !== 'undefined' && window.devicePixelRatio) || 1;
        expect(canvas._ctx.scale).toHaveBeenCalledWith(dpr, dpr);
    });
});

// ---- renderHeatmap with stepMs (fixed-width cells) ----

describe('renderHeatmap with stepMs', () => {
    const STEP = 1000; // 1s; xRange is 0–10000

    it('calls fillRect for a single data point when stepMs is provided', () => {
        const canvas = makeCanvas(400);
        const particles = makeParticles({
            p03: [{ x: 5000, y: 500 }],
        });
        renderHeatmap(canvas, particles, xRange, null, null, STEP);
        expect(canvas._ctx.fillRect).toHaveBeenCalled();
    });

    it('draws one cell per data point (not N-1 cells)', () => {
        const canvas = makeCanvas(400);
        const particles = makeParticles({
            p03: [{ x: 1000, y: 500 }, { x: 2000, y: 400 }, { x: 5000, y: 300 }],
        });
        renderHeatmap(canvas, particles, xRange, null, null, STEP);
        // p03 has 3 points → 3 fillRect calls; all other bins are empty
        expect(canvas._ctx.fillRect).toHaveBeenCalledTimes(3);
    });

    it('uses the same cell width for all data points', () => {
        const canvas = makeCanvas(400);
        const particles = makeParticles({
            p03: [{ x: 1000, y: 500 }, { x: 2000, y: 400 }, { x: 5000, y: 300 }],
        });
        renderHeatmap(canvas, particles, xRange, null, null, STEP);
        const widths = canvas._ctx.fillRect.mock.calls.map(c => c[2]);
        expect(widths[0]).toBeCloseTo(widths[1]);
        expect(widths[0]).toBeCloseTo(widths[2]);
    });

    it('cell width equals stepMs / rangeMs * plotW', () => {
        const cssW = 400;
        const LABEL_W = 38;
        const plotW = cssW - LABEL_W;
        const rangeMs = xRange.max - xRange.min; // 10000
        const expectedCellW = (STEP / rangeMs) * plotW; // 36.2
        const canvas = makeCanvas(cssW);
        const particles = makeParticles({
            p03: [{ x: 1000, y: 500 }],
        });
        renderHeatmap(canvas, particles, xRange, null, null, STEP);
        const call = canvas._ctx.fillRect.mock.calls[0];
        expect(call[2]).toBeCloseTo(expectedCellW);
    });
});

// ---- colorForIntensity ----

function parseHsl(str) {
    const m = str.match(/hsl\((\d+(?:\.\d+)?),\s*(\d+(?:\.\d+)?)%,\s*(\d+(?:\.\d+)?)%\)/);
    return m ? { h: +m[1], s: +m[2], l: +m[3] } : null;
}

describe('colorForIntensity', () => {
    it('returns an hsl string', () => {
        expect(parseHsl(colorForIntensity(0.5))).not.toBeNull();
    });

    it('uses HEATMAP_HUE for all intensities', () => {
        const low  = parseHsl(colorForIntensity(0.1));
        const high = parseHsl(colorForIntensity(0.9));
        expect(low.h).toBe(HEATMAP_HUE);
        expect(high.h).toBe(HEATMAP_HUE);
    });

    it('saturation increases with intensity', () => {
        const s0 = parseHsl(colorForIntensity(0)).s;
        const s1 = parseHsl(colorForIntensity(1)).s;
        expect(s1).toBeGreaterThan(s0);
    });

    it('at t=0 saturation is near zero', () => {
        expect(parseHsl(colorForIntensity(0)).s).toBeLessThanOrEqual(5);
    });

    it('at t=1 saturation is high', () => {
        expect(parseHsl(colorForIntensity(1)).s).toBeGreaterThanOrEqual(70);
    });
});

// ---- renderHeatmap drag overlay ----

describe('renderHeatmap drag overlay', () => {
    it('draws a selection rect when dragState spans part of the xRange', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange, null, { startT: 2000, currentT: 8000 });
        expect(canvas._ctx.fillRect).toHaveBeenCalled();
        expect(canvas._ctx.strokeRect).toHaveBeenCalled();
    });

    it('does not draw a selection rect when dragState is null', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange, null, null);
        expect(canvas._ctx.strokeRect).not.toHaveBeenCalled();
    });

    it('does not draw when drag region has zero width', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange, null, { startT: 5000, currentT: 5000 });
        expect(canvas._ctx.strokeRect).not.toHaveBeenCalled();
    });
});

// ---- renderHeatmap crosshair ----

describe('renderHeatmap crosshair', () => {
    it('draws a vertical line when crosshairT is within xRange', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange, 5000);
        expect(canvas._ctx.moveTo).toHaveBeenCalled();
    });

    it('does not draw a line when crosshairT is null', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange, null);
        expect(canvas._ctx.moveTo).not.toHaveBeenCalled();
    });

    it('does not draw a line when crosshairT is outside xRange', () => {
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, xRange, 99999);
        expect(canvas._ctx.moveTo).not.toHaveBeenCalled();
    });

    it('draws sibling lines for a multi-day range', () => {
        const DAY_MS = 86400000;
        const multiDayRange = { min: 0, max: 3 * DAY_MS };
        const canvas = makeCanvas(400);
        renderHeatmap(canvas, null, multiDayRange, DAY_MS * 1.5);
        expect(canvas._ctx.setLineDash).toHaveBeenCalledWith([3, 4]);
    });
});

// ---- findClosestPoint ----

describe('findClosestPoint', () => {
    it('returns null for null input', () => {
        expect(findClosestPoint(null, 5000)).toBeNull();
    });

    it('returns null for empty array', () => {
        expect(findClosestPoint([], 5000)).toBeNull();
    });

    it('returns the only point for a single-element array', () => {
        const pt = { x: '1970-01-01T00:00:01.000Z', y: 100 };
        expect(findClosestPoint([pt], 1000)).toBe(pt);
    });

    it('returns the closer of two points', () => {
        const a = { x: new Date(1000).toISOString(), y: 1 };
        const b = { x: new Date(9000).toISOString(), y: 2 };
        expect(findClosestPoint([a, b], 2000)).toBe(a);
        expect(findClosestPoint([a, b], 7000)).toBe(b);
    });

    it('returns exact match when available', () => {
        const a = { x: new Date(1000).toISOString(), y: 1 };
        const b = { x: new Date(5000).toISOString(), y: 2 };
        expect(findClosestPoint([a, b], 5000)).toBe(b);
    });

    it('works with numeric millisecond timestamps', () => {
        const a = { x: 1000, y: 1 };
        const b = { x: 9000, y: 2 };
        expect(findClosestPoint([a, b], 8000)).toBe(b);
    });
});
