// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('../charts.js', () => ({ xTickCallback: vi.fn() }));

import { buildTempestSection, updateTempestSection, tooltipLabel, secondaryAspect, buildEmptyTempestSection, tempestPanelOpts } from '../tempest-card.js';
import { CHART_HEIGHTS } from '../layout.js';

// Minimal Chart stand-in: records config, supports visibility like Chart.js.
class FakeChart {
    constructor(canvas, cfg) {
        this.canvas = canvas;
        this.data = cfg.data;
        this.options = cfg.options;
    }
    isDatasetVisible(i) { return !(this.data.datasets[i]?.hidden); }
    setDatasetVisibility(i, v) { this.data.datasets[i].hidden = !v; }
    update() {}
}

const iso = (min) => new Date(Date.parse('2026-11-20T12:00:00Z') + min * 60_000).toISOString();
function ob(min, f = {}) {
    return { device_id: 2002, timestamp: iso(min), report_interval_min: 1, ...f };
}

function render(tempest, stepMs = 60_000) {
    document.body.innerHTML = '';
    const { section, panels } = buildTempestSection(tempest.station, 4);
    document.body.appendChild(section);
    updateTempestSection(panels, tempest, stepMs);
    const panel = (name) => panels.find(p => p.room === `__tempest__/${name}`);
    return { section, panels, panel };
}

beforeEach(() => {
    globalThis.Chart = FakeChart;
    history.replaceState(null, '', '/');
});

describe('Tempest section', () => {
    const obs = [
        ob(0, { air_temp_c: 10, relative_humidity: 70, wind_avg_ms: 1, wind_gust_ms: 2, wind_direction_deg: 90 }),
        ob(1, { air_temp_c: 11, relative_humidity: 71, wind_avg_ms: 0, wind_gust_ms: 1, wind_direction_deg: 180 }),
        ob(9, { air_temp_c: 12, relative_humidity: 72, wind_avg_ms: 3, wind_gust_ms: 5, wind_direction_deg: 270 }),
    ];
    const tempest = { enabled: true, station: { name: 'Home' }, observations: obs, latest: obs[2], events: [] };

    it('titles the section with the station name', () => {
        const { section } = render(tempest);
        expect(section.querySelector('h2').textContent).toContain('Tempest · Home');
    });

    // With index-mode tooltips, hovering point i of any wind dataset shows
    // the direction of that same minute.
    it('wind direction tooltip matches the hovered minute', () => {
        const { panel } = render(tempest);
        const dir = panel('direction').chart.data.datasets[0];
        const speed = panel('wind').chart.data.datasets[0];
        expect(dir.data.length).toBe(speed.data.length);
        const last = dir.data.length - 1;
        expect(dir.data[last].x).toBe(speed.data[last].x);
        const text = tooltipLabel({ dataset: dir, raw: dir.data[last], parsed: { y: dir.data[last].y } });
        expect(text).toContain('270');
        expect(text).toContain('W');
    });

    it('collapses the lightning panel to a note when there are no strikes', () => {
        const { section } = render(tempest);
        const card = section.querySelector('.tempest-panel[data-panel="lightning"]');
        expect(card.classList.contains('collapsed')).toBe(true);
        expect(card.textContent).toContain('no strikes');
    });

    it('shows the lightning panel when there are strikes', () => {
        const withStrikes = { ...tempest, events: [{ timestamp: iso(9), type: 'lightning_strike', distance_km: 8 }] };
        const { section } = render(withStrikes);
        const card = section.querySelector('.tempest-panel[data-panel="lightning"]');
        expect(card.classList.contains('collapsed')).toBe(false);
    });

    it('hides illuminance by default and restores legend state from the URL', () => {
        let { panel } = render(tempest);
        const lux = () => panel('sun').chart.data.datasets.find(d => d.key === 'lux');
        expect(lux().hidden).toBe(true);
        history.replaceState(null, '', '/?tempestHidden=gust');
        ({ panel } = render(tempest));
        expect(lux().hidden).toBe(false);
        expect(panel('wind').chart.data.datasets.find(d => d.key === 'gust').hidden).toBe(true);
    });

    it('a legend click persists the hidden set in the URL', () => {
        const { panel } = render(tempest);
        const chart = panel('wind').chart;
        const idx = chart.data.datasets.findIndex(d => d.key === 'gust');
        chart.options.plugins.legend.onClick(null, { datasetIndex: idx }, { chart });
        expect(new URLSearchParams(location.search).get('tempestHidden')).toBe('gust,lux');
    });

    it('builds the strip from the raw latest observation', () => {
        const { section } = render({ ...tempest, latest: ob(9, { air_temp_c: 30 }) });
        expect(section.querySelector('#tempest-current').textContent).toContain('86.0°F');
    });
});

describe('empty range', () => {
    it('explains why there is nothing to show', () => {
        const el = buildEmptyTempestSection({ name: 'Home' });
        expect(el.textContent).toContain('No Tempest observations in this range');
    });
});

describe('secondaryAspect', () => {
    it('is shorter on desktop and unchanged on phones', () => {
        expect(secondaryAspect(4)).toBeCloseTo(6.4);
        expect(secondaryAspect(2.4)).toBe(2.4);
    });
});

// The direction strip sits directly under the wind panel, which already
// labels the time axis; repeating the labels in a strip this thin crowds its
// compass labels, especially on phones.
describe('wind direction strip', () => {
    it('leaves time labels to the wind panel above', () => {
        expect(tempestPanelOpts('direction', 2.4).scales.x.ticks.display).toBe(false);
        expect(tempestPanelOpts('wind', 2.4).scales.x.ticks.display).not.toBe(false);
    });
    it('is a fixed pixel height on a phone', () => {
        expect(tempestPanelOpts('direction', 2.4, true).maintainAspectRatio).toBe(false);
    });
});

// Regression: the strip used stepSize 180, so Chart.js drew ticks only at
// 0/180/360 (N, S, N). An easterly (e.g. 70°) sat in the same unlabelled band
// a westerly would, a fifth of the way up from "N", which read as northerly
// or mirrored. Every cardinal point must be labelled.
describe('wind direction axis', () => {
    const yDir = () => tempestPanelOpts('direction', 4).scales.yDir;

    it('labels N, E, S, W and N at 0, 90, 180, 270, 360', () => {
        const scale = { ticks: [{ value: 0 }, { value: 180 }, { value: 360 }] };
        yDir().afterBuildTicks(scale);
        const cb = yDir().ticks.callback;
        expect(scale.ticks.map(t => [t.value, cb(t.value)])).toEqual([
            [0, 'N'], [90, 'E'], [180, 'S'], [270, 'W'], [360, 'N'],
        ]);
    });

    it('increases upward, so east sits below south and west above it', () => {
        const opts = yDir();
        expect(opts.reverse).toBeFalsy();
        expect(opts.min).toBe(0);
        expect(opts.max).toBe(360);
    });

    it('says the direction is where the wind blows from', () => {
        const { section } = render({ enabled: true, station: { name: 'Home' }, observations: [ob(0, { wind_avg_ms: 1, wind_direction_deg: 70 })], latest: null, events: [] });
        const label = section.querySelector('.tempest-panel[data-panel="direction"] .panel-label').textContent;
        expect(label).toMatch(/from/i);
    });

    it('has room for five labels at every width', () => {
        // Five 10 px labels need ~85 px. Regression: at 601-820 px the strip
        // was an aspect ratio of an aspect ratio and collapsed to 30-49 px.
        expect(CHART_HEIGHTS.direction).toBeGreaterThanOrEqual(90);
        for (const [w, h] of [[375, 812], [700, 900], [820, 1180], [1280, 800]]) {
            globalThis.innerWidth = w; globalThis.innerHeight = h;
            const { panel } = render({ enabled: true, station: { name: 'Home' }, observations: [ob(0, { wind_avg_ms: 1, wind_direction_deg: 70 })], latest: null, events: [] });
            expect(panel('direction').chart.options.maintainAspectRatio, `width ${w}`).toBe(false);
            expect(panel('direction').box.style.height, `width ${w}`).toBe(`${CHART_HEIGHTS.direction}px`);
        }
    });

    it('tooltips name the compass point the wind comes from', () => {
        const { panel } = render({ enabled: true, station: { name: 'Home' }, observations: [ob(0, { wind_avg_ms: 1, wind_direction_deg: 70 })], latest: null, events: [] });
        const ds = panel('direction').chart.data.datasets[0];
        expect(tooltipLabel({ dataset: ds, raw: ds.data[0], parsed: { y: 70 } })).toBe('From: 70° ENE');
    });
});

describe('phone layout', () => {
    const obs = [ob(0, { air_temp_c: 10, wind_avg_ms: 1, wind_gust_ms: 2 }), ob(1, { air_temp_c: 11, wind_avg_ms: 2, wind_gust_ms: 3 })];
    const tempest = { enabled: true, station: { name: 'Home' }, observations: obs, latest: obs[1], events: [] };

    it('gives every panel a fixed height and an HTML key row instead of the canvas legend', () => {
        globalThis.innerWidth = 390; globalThis.innerHeight = 664;
        const { panel } = render(tempest);
        expect(panel('wind').box.style.height).toBe(`${CHART_HEIGHTS.secondary}px`);
        expect(panel('outdoor').box.style.height).toBe(`${CHART_HEIGHTS.main}px`);
        expect(panel('wind').chart.options.plugins.legend.display).toBe(false);
        expect(panel('wind').chart.options.plugins.tooltip.enabled).toBe(false);
        const keys = [...panel('wind').keyRow.querySelectorAll('button')].map(b => b.textContent);
        expect(keys).toEqual(['Average', 'Gust', 'Lull']);
        expect(panel('direction').keyRow.children).toHaveLength(0);
    });

    it('a key-row tap toggles the dataset and persists ?tempestHidden', () => {
        globalThis.innerWidth = 390; globalThis.innerHeight = 664;
        const { panel } = render(tempest);
        const gust = [...panel('wind').keyRow.querySelectorAll('button')].find(b => b.textContent === 'Gust');
        gust.click();
        expect(new URLSearchParams(location.search).get('tempestHidden')).toBe('gust,lux');
        expect(panel('wind').chart.data.datasets.find(d => d.key === 'gust').hidden).toBe(true);
    });

    it('keeps the desktop layout on wide screens', () => {
        globalThis.innerWidth = 1280; globalThis.innerHeight = 800;
        const { panel } = render(tempest);
        expect(panel('wind').box.style.height).toBe('');
        expect(panel('wind').chart.options.maintainAspectRatio).toBe(true);
        expect(panel('wind').chart.options.plugins.legend.display).toBe(true);
    });
});

describe('latest-values strip expand', () => {
    it('has a control that reveals the secondary values and survives a refresh', () => {
        const latest = ob(0, { air_temp_c: 10, relative_humidity: 70, uv_index: 2, battery_v: 2.6 });
        const tempest = { enabled: true, station: { name: 'Home' }, observations: [latest], latest, events: [] };
        const { section, panels } = render(tempest);
        const strip = section.querySelector('#tempest-current');
        const more = strip.querySelector('.strip-more');
        expect(more.getAttribute('aria-expanded')).toBe('false');
        more.click();
        expect(strip.classList.contains('expanded')).toBe(true);
        updateTempestSection(panels, tempest, 60_000);
        expect(strip.classList.contains('expanded')).toBe(true);
        expect(strip.querySelector('.strip-more').getAttribute('aria-expanded')).toBe('true');
    });
});
