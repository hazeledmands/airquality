import { describe, it, expect, vi } from 'vitest';
import { renderHeatmap, TOTAL_H, findClosestPoint } from '../particle-heatmap.js';
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

    it('does not call fillRect when there is only one data point (no cells)', () => {
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
