import { describe, it, expect } from 'vitest';
import { computeXRange, syncXAxes } from '../axis-sync.js';

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
