import { describe, it, expect, vi, beforeEach } from 'vitest';
import { humidityHighlightPlugin, setHumidityHighlightEnabled } from '../humidity-highlight.js';

function makeCtx() {
    return {
        save: vi.fn(),
        restore: vi.fn(),
        fillRect: vi.fn(),
        beginPath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        stroke: vi.fn(),
        setLineDash: vi.fn(),
        strokeStyle: '',
        lineWidth: 0,
        fillStyle: '',
    };
}

function makeChart({ humidityData, hidden = false, enabled = true, threshold = 60 }) {
    const datasets = [
        {
            label: 'Temperature',
            yAxisID: 'y',
            data: humidityData.map((_, i) => ({ y: 70 + i })),
            borderColor: '#fb923c',
        },
        {
            label: 'Humidity',
            yAxisID: 'y1',
            data: humidityData.map(y => ({ y })),
            borderColor: '#34d399',
        },
    ];

    const points = humidityData.map((_, i) => ({ x: 50 + i * 100, y: 200 }));
    const metas = [
        { hidden: false, data: datasets[0].data.map((_, i) => ({ x: 50 + i * 100, y: 100 })) },
        { hidden, data: points },
    ];

    const ctx = makeCtx();
    return {
        chart: {
            data: { datasets },
            getDatasetMeta: (i) => metas[i],
            chartArea: { top: 0, bottom: 400, left: 0, right: 1200 },
            scales: {
                y1: { getPixelForValue: (v) => 400 - v * 4 },
            },
            ctx,
            options: {
                plugins: {
                    humidityHighlight: { enabled, threshold },
                },
            },
        },
        ctx,
    };
}

describe('humidityHighlightPlugin', () => {
    beforeEach(() => {
        setHumidityHighlightEnabled(true);
    });

    it('has id "humidityHighlight"', () => {
        expect(humidityHighlightPlugin.id).toBe('humidityHighlight');
    });

    it('draws shaded regions where humidity > threshold', () => {
        // 50, 65, 70, 55 — above 60 at indices 1 and 2
        const { chart, ctx } = makeChart({ humidityData: [50, 65, 70, 55] });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.fillRect).toHaveBeenCalled();
        // Should draw at least for the segment from index 1 to 2 (both above)
        const calls = ctx.fillRect.mock.calls;
        expect(calls.length).toBeGreaterThanOrEqual(1);
    });

    it('draws threshold reference line', () => {
        const { chart, ctx } = makeChart({ humidityData: [50, 65, 55] });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.setLineDash).toHaveBeenCalledWith([4, 4]);
        expect(ctx.stroke).toHaveBeenCalled();
    });

    it('does nothing when not enabled', () => {
        const { chart, ctx } = makeChart({ humidityData: [50, 65, 70, 55], enabled: false });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.fillRect).not.toHaveBeenCalled();
        expect(ctx.stroke).not.toHaveBeenCalled();
    });

    it('does nothing when humidity dataset is hidden', () => {
        const { chart, ctx } = makeChart({ humidityData: [50, 65, 70, 55], hidden: true });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.fillRect).not.toHaveBeenCalled();
    });

    it('does not shade when all values are below threshold', () => {
        const { chart, ctx } = makeChart({ humidityData: [40, 50, 55, 45] });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.fillRect).not.toHaveBeenCalled();
    });

    it('interpolates crossing points for partial segments', () => {
        // 50 -> 70: crosses 60 at midpoint. Should shade from midpoint to end.
        const { chart, ctx } = makeChart({ humidityData: [50, 70, 50] });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        const calls = ctx.fillRect.mock.calls;
        // Two shaded segments: rising crossing (50->70) and falling crossing (70->50)
        expect(calls.length).toBe(2);
        // First segment: crossing up around x=100 (midpoint of 50 and 150)
        expect(calls[0][0]).toBeCloseTo(100, 0);
        // Second segment: starts at x=150, crossing down around x=200
        expect(calls[1][0]).toBeCloseTo(150, 0);
    });

    it('respects custom threshold', () => {
        // With threshold=80, only index 2 (85) is above
        const { chart, ctx } = makeChart({ humidityData: [50, 65, 85, 55], threshold: 80 });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.fillRect).toHaveBeenCalled();
    });

    it('draws nothing when globally disabled', () => {
        setHumidityHighlightEnabled(false);
        const { chart, ctx } = makeChart({ humidityData: [50, 65, 70, 55] });

        humidityHighlightPlugin.beforeDatasetsDraw(chart);

        expect(ctx.fillRect).not.toHaveBeenCalled();
        expect(ctx.stroke).not.toHaveBeenCalled();
    });
});
