// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest';
import { legendEntries, renderKeyRow } from '../key-row.js';

function fakeChart(datasets, legendOpts = {}) {
    const chart = {
        data: { datasets },
        options: { plugins: { legend: { labels: {}, ...legendOpts } } },
        isDatasetVisible: (i) => !datasets[i].hidden,
        setDatasetVisibility: (i, v) => { datasets[i].hidden = !v; },
        update: vi.fn(),
    };
    return chart;
}

describe('legendEntries', () => {
    it('lists one entry per dataset with colour and visibility', () => {
        const c = fakeChart([{ label: 'Gust', borderColor: '#aaa' }, { label: 'Lull', borderColor: '#bbb', hidden: true }]);
        expect(legendEntries(c)).toEqual([
            { text: 'Gust', datasetIndex: 0, color: '#aaa', hidden: false },
            { text: 'Lull', datasetIndex: 1, color: '#bbb', hidden: true },
        ]);
    });
    it("honours the chart legend's generateLabels and filter (one entry per metric)", () => {
        const ds = [{ label: 'Temperature · A', metric: 'Temperature', borderColor: '#f00' }, { label: 'Temperature · B', metric: 'Temperature', borderColor: '#f00' }];
        const c = fakeChart(ds, {
            labels: {
                generateLabels: (chart) => chart.data.datasets.map((d, i) => ({ text: d.metric, datasetIndex: i, strokeStyle: d.borderColor })),
                filter: (item, data) => data.datasets.findIndex(d => d.metric === data.datasets[item.datasetIndex].metric) === item.datasetIndex,
            },
        });
        expect(legendEntries(c).map(e => e.text)).toEqual(['Temperature']);
    });
    it('is empty when the legend is turned off for the chart', () => {
        expect(legendEntries(fakeChart([{ label: 'x' }], { keyRow: false }))).toEqual([]);
    });
});

describe('renderKeyRow', () => {
    it('renders 44 px tap targets that toggle through the legend onClick', () => {
        const onClick = vi.fn((_e, item, legend) => legend.chart.setDatasetVisibility(item.datasetIndex, false));
        const c = fakeChart([{ label: 'Gust', borderColor: '#aaa' }], { onClick });
        const el = document.createElement('div');
        renderKeyRow(el, c);
        const btn = el.querySelector('button');
        expect(btn.textContent).toContain('Gust');
        expect(btn.classList.contains('key-btn')).toBe(true);
        expect(btn.getAttribute('aria-pressed')).toBe('true');
        btn.click();
        expect(onClick).toHaveBeenCalledWith(null, { datasetIndex: 0 }, { chart: c });
        // re-rendered with the new state
        expect(el.querySelector('button').getAttribute('aria-pressed')).toBe('false');
    });
    it('toggles visibility itself when the chart has no legend onClick', () => {
        const c = fakeChart([{ label: 'UV', borderColor: '#ff0' }]);
        const el = document.createElement('div');
        renderKeyRow(el, c);
        el.querySelector('button').click();
        expect(c.data.datasets[0].hidden).toBe(true);
        expect(c.update).toHaveBeenCalled();
    });
});
