import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
    filterAnnotations,
    findAnnotationAtPixel,
    annotationsPlugin,
    setAnnotations,
    setAnnotationsEnabled,
} from '../annotations.js';

// ---- helpers ----

function makeCtx() {
    return {
        save: vi.fn(),
        restore: vi.fn(),
        beginPath: vi.fn(),
        closePath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        stroke: vi.fn(),
        fill: vi.fn(),
        fillText: vi.fn(),
        setLineDash: vi.fn(),
        strokeStyle: '',
        lineWidth: 0,
        fillStyle: '',
        font: '',
    };
}

function makeChart(minMs, maxMs) {
    const ctx = makeCtx();
    const chart = {
        ctx,
        chartArea: { top: 0, bottom: 400, left: 0, right: 1000 },
        scales: {
            x: {
                getPixelForValue: (ms) => ((ms - minMs) / (maxMs - minMs)) * 1000,
                getValueForPixel: (px) => minMs + (px / 1000) * (maxMs - minMs),
                min: minMs,
                max: maxMs,
            },
        },
    };
    return { chart, ctx };
}

// ---- filterAnnotations ----

describe('filterAnnotations', () => {
    const annotations = [
        { id: 1, timestamp: '2026-03-01T10:00:00Z', note: 'early' },
        { id: 2, timestamp: '2026-03-01T12:00:00Z', note: 'mid' },
        { id: 3, timestamp: '2026-03-01T14:00:00Z', note: 'late' },
    ];
    const from = new Date('2026-03-01T11:00:00Z').getTime();
    const to   = new Date('2026-03-01T13:00:00Z').getTime();

    it('returns only annotations within [from, to)', () => {
        const result = filterAnnotations(annotations, from, to);
        expect(result).toHaveLength(1);
        expect(result[0].id).toBe(2);
    });

    it('includes the from boundary', () => {
        const fromExact = new Date('2026-03-01T10:00:00Z').getTime();
        const result = filterAnnotations(annotations, fromExact, to);
        expect(result.map(a => a.id)).toContain(1);
    });

    it('excludes the to boundary', () => {
        const toExact = new Date('2026-03-01T12:00:00Z').getTime();
        const result = filterAnnotations(annotations, from, toExact);
        expect(result.map(a => a.id)).not.toContain(2);
    });

    it('returns empty array when none match', () => {
        expect(filterAnnotations(annotations, 0, 1)).toHaveLength(0);
    });

    it('returns empty array for empty input', () => {
        expect(filterAnnotations([], from, to)).toHaveLength(0);
    });
});

// ---- findAnnotationAtPixel ----

describe('findAnnotationAtPixel', () => {
    const T  = new Date('2026-03-01T12:00:00Z').getTime();
    const T2 = new Date('2026-03-01T13:00:00Z').getTime();
    const annotations = [
        { id: 1, timestamp: new Date(T).toISOString(),  note: 'first'  },
        { id: 2, timestamp: new Date(T2).toISOString(), note: 'second' },
    ];

    it('returns null for empty annotations', () => {
        const { chart } = makeChart(T, T2);
        expect(findAnnotationAtPixel(chart, 500, [])).toBeNull();
    });

    it('returns null when no annotation is within threshold', () => {
        // T at x=0, T2 at x=1000 — clicking at x=500 is 500px from both
        const { chart } = makeChart(T, T2);
        expect(findAnnotationAtPixel(chart, 500, annotations, 8)).toBeNull();
    });

    it('returns annotation within threshold', () => {
        // T at x=0; click at x=3 (3px away, within default threshold 8)
        const { chart } = makeChart(T, T2);
        const result = findAnnotationAtPixel(chart, 3, annotations, 8);
        expect(result?.id).toBe(1);
    });

    it('returns null when click is outside threshold', () => {
        // T at x=0; click at x=20 (20px away, beyond threshold 8)
        const { chart } = makeChart(T, T2);
        expect(findAnnotationAtPixel(chart, 20, annotations, 8)).toBeNull();
    });

    it('returns the closest annotation when multiple are near', () => {
        // Both T at x=0 and T2 at x=1000, click at x=998 (2px from T2)
        const { chart } = makeChart(T, T2);
        const result = findAnnotationAtPixel(chart, 998, annotations, 8);
        expect(result?.id).toBe(2);
    });
});

// ---- annotationsPlugin ----

describe('annotationsPlugin', () => {
    beforeEach(() => {
        setAnnotations([]);
        setAnnotationsEnabled(true);
    });

    it('has id "annotations"', () => {
        expect(annotationsPlugin.id).toBe('annotations');
    });

    it('does not draw when disabled', () => {
        setAnnotationsEnabled(false);
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test' }]);
        const { chart, ctx } = makeChart(0, 1000000);
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('does not draw when annotations array is empty', () => {
        setAnnotations([]);
        const { chart, ctx } = makeChart(0, 1000000);
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('draws a vertical line for an annotation within chart bounds', () => {
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test' }]);
        const { chart, ctx } = makeChart(0, 1000000);
        annotationsPlugin.afterDraw(chart);
        // 1 moveTo for the vertical line + 1 moveTo for the triangle marker
        expect(ctx.moveTo).toHaveBeenCalledTimes(2);
        expect(ctx.lineTo).toHaveBeenCalled();
    });

    it('draws at the correct x pixel', () => {
        // T=500000 maps to x=500 on a 0..1000000 range with width 1000
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test' }]);
        const { chart, ctx } = makeChart(0, 1000000);
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).toHaveBeenCalledWith(500, 0);   // chartArea.top = 0
        expect(ctx.lineTo).toHaveBeenCalledWith(500, 400); // chartArea.bottom = 400
    });

    it('skips annotations outside chart bounds', () => {
        // timestamp maps to x < chartArea.left (0)
        setAnnotations([{ id: 1, timestamp: new Date(-100000).toISOString(), note: 'outside' }]);
        const { chart, ctx } = makeChart(0, 1000000);
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('draws multiple annotations', () => {
        setAnnotations([
            { id: 1, timestamp: new Date(300000).toISOString(), note: 'first'  },
            { id: 2, timestamp: new Date(700000).toISOString(), note: 'second' },
        ]);
        const { chart, ctx } = makeChart(0, 1000000);
        annotationsPlugin.afterDraw(chart);
        // 2 moveTo calls per annotation (line + triangle marker)
        expect(ctx.moveTo).toHaveBeenCalledTimes(4);
    });
});
