import { describe, it, expect, vi, beforeEach } from 'vitest';
import { maximaLabelsPlugin, setMaximaLabelsEnabled } from '../maxima-labels.js';

function makeChart(datasets, metaOverrides = {}, pointSpacing = 250) {
    const metas = datasets.map((ds, i) => {
        const points = ds.data.map((d, j) => ({ x: 50 + j * pointSpacing, y: 200 - d.y * 2 }));
        return { hidden: false, data: points, ...metaOverrides[i] };
    });

    const ctx = {
        save: vi.fn(),
        restore: vi.fn(),
        fillText: vi.fn(),
        font: '',
        textAlign: '',
        textBaseline: '',
        fillStyle: '',
    };

    return {
        data: { datasets },
        getDatasetMeta: (i) => metas[i],
        chartArea: { top: 0, bottom: 400, left: 0, right: 2000 },
        ctx,
    };
}

describe('maximaLabelsPlugin', () => {
    beforeEach(() => {
        setMaximaLabelsEnabled(true);
    });

    it('has id "maximaLabels"', () => {
        expect(maximaLabelsPlugin.id).toBe('maximaLabels');
    });

    it('draws labels at local maxima', () => {
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }, { y: 8 }, { y: 2 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        const calls = chart.ctx.fillText.mock.calls;
        // points at index 1 (y=5) and index 3 (y=8) are local maxima
        expect(calls.length).toBe(2);
        expect(calls.some(c => c[0] === '5')).toBe(true);
        expect(calls.some(c => c[0] === '8')).toBe(true);
    });

    it('uses dataset borderColor for label color', () => {
        const ds = {
            data: [{ y: 1 }, { y: 10 }, { y: 3 }],
            borderColor: '#38bdf8',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillStyle).toBe('#38bdf8');
    });

    it('skips stepped datasets (AC state)', () => {
        const ds = {
            data: [{ y: 0 }, { y: 1 }, { y: 0 }],
            borderColor: '#555',
            stepped: true,
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText).not.toHaveBeenCalled();
    });

    it('skips hidden datasets', () => {
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds], { 0: { hidden: true } });

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText).not.toHaveBeenCalled();
    });

    it('does not label datasets with fewer than 3 points', () => {
        const ds = {
            data: [{ y: 1 }, { y: 5 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText).not.toHaveBeenCalled();
    });

    it('formats integer values without decimals', () => {
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText.mock.calls[0][0]).toBe('5');
    });

    it('formats decimal values with one decimal place', () => {
        const ds = {
            data: [{ y: 1.2 }, { y: 5.67 }, { y: 3.1 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText.mock.calls[0][0]).toBe('5.7');
    });

    it('enforces 200px minimum distance between labels', () => {
        // Points spaced 50px apart — two maxima at indices 1 and 3 are only 100px apart
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }, { y: 8 }, { y: 2 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds], {}, 50);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        const calls = chart.ctx.fillText.mock.calls;
        // Only the higher peak (8) should be drawn; 5 is too close
        expect(calls.length).toBe(1);
        expect(calls[0][0]).toBe('8');
    });

    it('draws both labels when maxima are far enough apart', () => {
        // Points spaced 250px apart — two maxima at indices 1 and 3 are 500px apart
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }, { y: 8 }, { y: 2 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds], {}, 250);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        const calls = chart.ctx.fillText.mock.calls;
        expect(calls.length).toBe(2);
        expect(calls.some(c => c[0] === '5')).toBe(true);
        expect(calls.some(c => c[0] === '8')).toBe(true);
    });

    it('labels each dataset independently', () => {
        const ds1 = {
            data: [{ y: 1 }, { y: 10 }, { y: 3 }, { y: 8 }, { y: 2 }],
            borderColor: '#ff0000',
        };
        const ds2 = {
            data: [{ y: 2 }, { y: 9 }, { y: 4 }, { y: 7 }, { y: 1 }],
            borderColor: '#00ff00',
        };
        const chart = makeChart([ds1, ds2]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        const calls = chart.ctx.fillText.mock.calls;
        // Both datasets get their peaks labeled independently
        expect(calls.some(c => c[0] === '10')).toBe(true);
        expect(calls.some(c => c[0] === '9')).toBe(true);
    });

    it('draws nothing when disabled', () => {
        setMaximaLabelsEnabled(false);
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText).not.toHaveBeenCalled();
    });

    it('appends peakUnit to label text', () => {
        const ds = {
            data: [{ y: 1.2 }, { y: 72.5 }, { y: 3.1 }],
            borderColor: '#fb923c',
            peakUnit: '\u00B0F',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText.mock.calls[0][0]).toBe('72.5\u00B0F');
    });

    it('shows no unit suffix when peakUnit is not set', () => {
        const ds = {
            data: [{ y: 1 }, { y: 5 }, { y: 3 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds]);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        expect(chart.ctx.fillText.mock.calls[0][0]).toBe('5');
    });

    it('detects plateau peaks (equal consecutive values at top)', () => {
        // 78.8 appears twice at the top — should label it (at least once)
        // With tight spacing, only one edge of the plateau survives the distance check
        const ds = {
            data: [{ y: 77 }, { y: 78.5 }, { y: 78.8 }, { y: 78.8 }, { y: 78.2 }, { y: 77 }],
            borderColor: '#ff0000',
        };
        const chart = makeChart([ds], {}, 10);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        const calls = chart.ctx.fillText.mock.calls;
        expect(calls.length).toBe(1);
        expect(calls[0][0]).toBe('78.8');
    });

    it('suppresses lesser peak when dominated by nearby higher value', () => {
        // Simulates: high peak at 78.8 then a smaller bump at 76.6, close together
        const ds = {
            data: [{ y: 77 }, { y: 78.8 }, { y: 78 }, { y: 76.6 }, { y: 75 }],
            borderColor: '#ff0000',
        };
        // 10px spacing so all points are within 200px of each other
        const chart = makeChart([ds], {}, 10);

        maximaLabelsPlugin.afterDatasetsDraw(chart);

        const calls = chart.ctx.fillText.mock.calls;
        expect(calls.length).toBe(1);
        expect(calls[0][0]).toBe('78.8');
    });
});
