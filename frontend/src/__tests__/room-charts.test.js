import { describe, it, expect, vi } from 'vitest';
import { METRIC_COLORS, isIndoorRoom, combinedChartOpts, buildCombinedDatasets } from '../room-charts.js';

// room-charts.js imports xTickCallback from charts.js which accesses Chart global
vi.mock('../charts.js', () => ({ xTickCallback: vi.fn() }));
vi.mock('../data.js', () => ({
    makeWeatherTempSeries: (weather) => ({
        label: 'Outside Temp',
        data: weather.map(w => ({ x: w.ts, y: w.temp_f })),
        borderColor: '#94a3b8',
        fill: false,
    }),
}));

function makeRoom(overrides = {}) {
    return {
        name: 'Bedroom',
        temp: [],
        dewpoint: [],
        humidity: [],
        pm: [],
        acState: [],
        ...overrides,
    };
}

// ---- isIndoorRoom ----

describe('isIndoorRoom', () => {
    it('returns true for a regular room name', () => {
        expect(isIndoorRoom({ name: 'Bedroom' })).toBe(true);
    });

    it('returns false for outside room', () => {
        expect(isIndoorRoom({ name: 'Outside' })).toBe(false);
    });

    it('is case-insensitive', () => {
        expect(isIndoorRoom({ name: 'OUTSIDE SENSOR' })).toBe(false);
    });
});

// ---- combinedChartOpts ----

describe('combinedChartOpts', () => {
    it('always includes x, y, y1 scales', () => {
        const opts = combinedChartOpts(2, makeRoom());
        expect(opts.scales.x).toBeDefined();
        expect(opts.scales.y).toBeDefined();
        expect(opts.scales.y1).toBeDefined();
    });

    it('adds y2 scale when room has PM data', () => {
        const room = makeRoom({ pm: [{ data: [] }] });
        const opts = combinedChartOpts(2, room);
        expect(opts.scales.y2).toBeDefined();
    });

    it('omits y2 scale when room has no PM data', () => {
        const opts = combinedChartOpts(2, makeRoom());
        expect(opts.scales.y2).toBeUndefined();
    });

    it('adds y3 scale when room has AC data', () => {
        const room = makeRoom({ acState: [{ data: [] }] });
        const opts = combinedChartOpts(2, room);
        expect(opts.scales.y3).toBeDefined();
    });

    it('omits y3 scale when room has no AC data', () => {
        const opts = combinedChartOpts(2, makeRoom());
        expect(opts.scales.y3).toBeUndefined();
    });

    it('enables humidityHighlight for indoor rooms', () => {
        const opts = combinedChartOpts(2, makeRoom({ name: 'Bedroom' }));
        expect(opts.plugins.humidityHighlight.enabled).toBe(true);
    });

    it('disables humidityHighlight for outside rooms', () => {
        const opts = combinedChartOpts(2, makeRoom({ name: 'Outside' }));
        expect(opts.plugins.humidityHighlight.enabled).toBe(false);
    });

    it('sets aspectRatio from the ar parameter', () => {
        const opts = combinedChartOpts(3.5, makeRoom());
        expect(opts.aspectRatio).toBe(3.5);
    });

    it('uses tension: 0 (no interpolation)', () => {
        const opts = combinedChartOpts(2, makeRoom());
        expect(opts.elements.line.tension).toBe(0);
    });
});

// ---- buildCombinedDatasets ----

describe('buildCombinedDatasets', () => {
    it('returns empty array when room has no data', () => {
        expect(buildCombinedDatasets(makeRoom())).toEqual([]);
    });

    it('maps temp data to y axis with correct color', () => {
        const room = makeRoom({ temp: [{ data: [{ x: 1, y: 72 }] }] });
        const ds = buildCombinedDatasets(room);
        expect(ds).toHaveLength(1);
        expect(ds[0].yAxisID).toBe('y');
        expect(ds[0].borderColor).toBe(METRIC_COLORS.temp);
        expect(ds[0].label).toBe('Temperature');
    });

    it('maps dewpoint data to y axis with dashed line', () => {
        const room = makeRoom({ dewpoint: [{ data: [] }] });
        const ds = buildCombinedDatasets(room);
        expect(ds[0].yAxisID).toBe('y');
        expect(ds[0].borderDash).toBeDefined();
    });

    it('maps humidity data to y1 axis', () => {
        const room = makeRoom({ humidity: [{ data: [] }] });
        const ds = buildCombinedDatasets(room);
        expect(ds[0].yAxisID).toBe('y1');
        expect(ds[0].label).toBe('Humidity');
    });

    it('maps PM data to y2 axis', () => {
        const room = makeRoom({ pm: [{ data: [] }] });
        const ds = buildCombinedDatasets(room);
        expect(ds[0].yAxisID).toBe('y2');
        expect(ds[0].label).toBe('PM2.5');
    });

    it('maps acState to y3 with fill and stepped', () => {
        const room = makeRoom({ acState: [{ data: [] }] });
        const ds = buildCombinedDatasets(room);
        expect(ds[0].yAxisID).toBe('y3');
        expect(ds[0].fill).toBe(true);
        expect(ds[0].stepped).toBe(true);
    });

    it('does not add outdoor temp series when outdoorTempEnabled is false', () => {
        const room = makeRoom({ name: 'Bedroom', temp: [{ data: [] }] });
        const ds = buildCombinedDatasets(room, { outdoorTempEnabled: false, currentWeather: [{ ts: 1, temp_f: 65 }] });
        expect(ds.every(d => d.label !== 'Outside Temp')).toBe(true);
    });

    it('adds outdoor temp series for indoor rooms when enabled', () => {
        const room = makeRoom({ name: 'Bedroom' });
        const weather = [{ ts: 1, temp_f: 65 }];
        const ds = buildCombinedDatasets(room, { outdoorTempEnabled: true, currentWeather: weather });
        expect(ds.some(d => d.label === 'Outside Temp')).toBe(true);
        expect(ds.find(d => d.label === 'Outside Temp').yAxisID).toBe('y');
    });

    it('does not add outdoor temp series for outside rooms', () => {
        const room = makeRoom({ name: 'Outside' });
        const weather = [{ ts: 1, temp_f: 65 }];
        const ds = buildCombinedDatasets(room, { outdoorTempEnabled: true, currentWeather: weather });
        expect(ds.some(d => d.label === 'Outside Temp')).toBe(false);
    });

    it('does not add outdoor temp series when weather is empty', () => {
        const room = makeRoom({ name: 'Bedroom' });
        const ds = buildCombinedDatasets(room, { outdoorTempEnabled: true, currentWeather: [] });
        expect(ds.some(d => d.label === 'Outside Temp')).toBe(false);
    });
});
