import { describe, it, expect, vi } from 'vitest';
import { renderHeatmap, TOTAL_H } from '../particle-heatmap.js';
import { PARTICLE_BINS } from '../room-charts.js';

vi.mock('../charts.js', () => ({ xTickCallback: vi.fn() }));
vi.mock('../data.js', () => ({ makeWeatherTempSeries: vi.fn() }));

function makeCtx() {
    return {
        scale: vi.fn(),
        clearRect: vi.fn(),
        fillRect: vi.fn(),
        fillText: vi.fn(),
        fillStyle: '',
        font: '',
        textAlign: '',
        textBaseline: '',
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
