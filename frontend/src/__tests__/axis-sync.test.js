import { describe, it, expect } from 'vitest';
import { computeXRange, syncXAxes, computeYRange, syncYAxes } from '../axis-sync.js';

describe('computeXRange', () => {
    it('returns null for empty charts array', () => {
        expect(computeXRange([])).toBeNull();
    });

    it('returns null for charts with no datasets', () => {
        expect(computeXRange([{ data: { datasets: [] } }])).toBeNull();
    });

    it('returns null for charts with empty datasets', () => {
        expect(computeXRange([{ data: { datasets: [{ data: [] }] } }])).toBeNull();
    });

    it('finds min and max across a single chart with ISO string timestamps', () => {
        const charts = [{
            data: { datasets: [{ data: [
                { x: '2026-03-01T00:00:00Z', y: 1 },
                { x: '2026-03-03T00:00:00Z', y: 2 },
                { x: '2026-03-02T00:00:00Z', y: 3 },
            ] }] },
        }];
        const range = computeXRange(charts);
        expect(range.min).toBe(new Date('2026-03-01T00:00:00Z').getTime());
        expect(range.max).toBe(new Date('2026-03-03T00:00:00Z').getTime());
    });

    it('handles numeric millisecond timestamps', () => {
        const t1 = 1_000_000;
        const t2 = 2_000_000;
        const charts = [{
            data: { datasets: [{ data: [{ x: t1, y: 0 }, { x: t2, y: 0 }] }] },
        }];
        const range = computeXRange(charts);
        expect(range.min).toBe(t1);
        expect(range.max).toBe(t2);
    });

    it('spans min/max across multiple charts', () => {
        const charts = [
            { data: { datasets: [{ data: [{ x: '2026-03-02T00:00:00Z', y: 1 }] }] } },
            { data: { datasets: [{ data: [
                { x: '2026-03-01T00:00:00Z', y: 2 },
                { x: '2026-03-05T00:00:00Z', y: 3 },
            ] }] } },
        ];
        const range = computeXRange(charts);
        expect(range.min).toBe(new Date('2026-03-01T00:00:00Z').getTime());
        expect(range.max).toBe(new Date('2026-03-05T00:00:00Z').getTime());
    });

    it('spans min/max across multiple datasets within a chart', () => {
        const charts = [{
            data: { datasets: [
                { data: [{ x: '2026-03-03T00:00:00Z', y: 1 }] },
                { data: [{ x: '2026-03-01T00:00:00Z', y: 2 }, { x: '2026-03-07T00:00:00Z', y: 3 }] },
            ] },
        }];
        const range = computeXRange(charts);
        expect(range.min).toBe(new Date('2026-03-01T00:00:00Z').getTime());
        expect(range.max).toBe(new Date('2026-03-07T00:00:00Z').getTime());
    });

    it('returns equal min and max for a single data point', () => {
        const t = new Date('2026-03-01T12:00:00Z').getTime();
        const charts = [{
            data: { datasets: [{ data: [{ x: t, y: 5 }] }] },
        }];
        const range = computeXRange(charts);
        expect(range.min).toBe(t);
        expect(range.max).toBe(t);
    });
});

describe('syncXAxes', () => {
    it('sets the same min and max on every chart x scale', () => {
        const chart1 = { options: { scales: { x: {} } } };
        const chart2 = { options: { scales: { x: {} } } };
        syncXAxes([chart1, chart2], { min: 1000, max: 9000 });
        expect(chart1.options.scales.x.min).toBe(1000);
        expect(chart1.options.scales.x.max).toBe(9000);
        expect(chart2.options.scales.x.min).toBe(1000);
        expect(chart2.options.scales.x.max).toBe(9000);
    });

    it('does nothing when range is null', () => {
        const chart = { options: { scales: { x: { min: 0, max: 100 } } } };
        syncXAxes([chart], null);
        expect(chart.options.scales.x.min).toBe(0);
        expect(chart.options.scales.x.max).toBe(100);
    });

    it('handles an empty charts array without error', () => {
        expect(() => syncXAxes([], { min: 0, max: 100 })).not.toThrow();
    });
});

describe('computeYRange', () => {
    it('returns null for empty charts array', () => {
        expect(computeYRange([], 'y')).toBeNull();
    });

    it('returns null for charts with no datasets', () => {
        expect(computeYRange([{ data: { datasets: [] } }], 'y')).toBeNull();
    });

    it('returns null when no datasets match the axis id', () => {
        const charts = [{ data: { datasets: [
            { yAxisID: 'y1', data: [{ x: 0, y: 50 }] },
        ] } }];
        expect(computeYRange(charts, 'y')).toBeNull();
    });

    it('returns null for matching datasets with no data points', () => {
        const charts = [{ data: { datasets: [{ yAxisID: 'y', data: [] }] } }];
        expect(computeYRange(charts, 'y')).toBeNull();
    });

    it('finds min and max across a single dataset', () => {
        const charts = [{ data: { datasets: [
            { yAxisID: 'y', data: [{ x: 1, y: 72 }, { x: 2, y: 65 }, { x: 3, y: 80 }] },
        ] } }];
        const range = computeYRange(charts, 'y');
        expect(range.min).toBe(65);
        expect(range.max).toBe(80);
    });

    it('spans min/max across multiple charts', () => {
        const charts = [
            { data: { datasets: [{ yAxisID: 'y', data: [{ x: 1, y: 70 }, { x: 2, y: 75 }] }] } },
            { data: { datasets: [{ yAxisID: 'y', data: [{ x: 1, y: 60 }, { x: 2, y: 85 }] }] } },
        ];
        const range = computeYRange(charts, 'y');
        expect(range.min).toBe(60);
        expect(range.max).toBe(85);
    });

    it('ignores datasets with a different axis id', () => {
        const charts = [{ data: { datasets: [
            { yAxisID: 'y',  data: [{ x: 1, y: 70 }, { x: 2, y: 75 }] },
            { yAxisID: 'y1', data: [{ x: 1, y: 0  }, { x: 2, y: 100 }] },
        ] } }];
        const range = computeYRange(charts, 'y');
        expect(range.min).toBe(70);
        expect(range.max).toBe(75);
    });

    it('spans min/max across multiple datasets within a chart', () => {
        const charts = [{ data: { datasets: [
            { yAxisID: 'y', data: [{ x: 1, y: 72 }] },
            { yAxisID: 'y', data: [{ x: 1, y: 65 }, { x: 2, y: 80 }] },
        ] } }];
        const range = computeYRange(charts, 'y');
        expect(range.min).toBe(65);
        expect(range.max).toBe(80);
    });

    it('returns equal min and max for a single data point', () => {
        const charts = [{ data: { datasets: [{ yAxisID: 'y', data: [{ x: 1, y: 73 }] }] } }];
        const range = computeYRange(charts, 'y');
        expect(range.min).toBe(73);
        expect(range.max).toBe(73);
    });

    it('expands the range by the given padding fraction on each side', () => {
        // span = 80 - 60 = 20; 5% padding = 1 on each side
        const charts = [{ data: { datasets: [
            { yAxisID: 'y', data: [{ x: 1, y: 60 }, { x: 2, y: 80 }] },
        ] } }];
        const range = computeYRange(charts, 'y', 0.05);
        expect(range.min).toBeCloseTo(59);
        expect(range.max).toBeCloseTo(81);
    });

    it('uses a span of 1 when all values are equal, to avoid zero padding', () => {
        const charts = [{ data: { datasets: [{ yAxisID: 'y', data: [{ x: 1, y: 73 }] }] } }];
        const range = computeYRange(charts, 'y', 0.05);
        expect(range.min).toBeCloseTo(72.95);
        expect(range.max).toBeCloseTo(73.05);
    });
});

describe('syncYAxes', () => {
    it('sets the same min and max on every chart that has the axis', () => {
        const chart1 = { options: { scales: { y: {} } } };
        const chart2 = { options: { scales: { y: {} } } };
        syncYAxes([chart1, chart2], 'y', { min: 60, max: 85 });
        expect(chart1.options.scales.y.min).toBe(60);
        expect(chart1.options.scales.y.max).toBe(85);
        expect(chart2.options.scales.y.min).toBe(60);
        expect(chart2.options.scales.y.max).toBe(85);
    });

    it('skips charts that do not have the axis', () => {
        const chart1 = { options: { scales: { y: {} } } };
        const chart2 = { options: { scales: { yUV: {} } } };
        syncYAxes([chart1, chart2], 'y', { min: 60, max: 85 });
        expect(chart1.options.scales.y.min).toBe(60);
        expect(chart2.options.scales.yUV.min).toBeUndefined();
    });

    it('does nothing when range is null', () => {
        const chart = { options: { scales: { y: { min: 10, max: 90 } } } };
        syncYAxes([chart], 'y', null);
        expect(chart.options.scales.y.min).toBe(10);
        expect(chart.options.scales.y.max).toBe(90);
    });

    it('handles an empty charts array without error', () => {
        expect(() => syncYAxes([], 'y', { min: 0, max: 100 })).not.toThrow();
    });
});
