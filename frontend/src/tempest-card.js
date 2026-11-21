// The Tempest section: a latest-values strip over small multiples (outdoor,
// wind, direction, rain, sun, lightning, station) that share the dashboard's
// time axis.
import { CHART_ANIMATION_MS, pinAxisWidth, MUTED_TEXT } from './config.js';
import { xTickCallback } from './charts.js';
import { METRIC_COLORS } from './room-charts.js';
import { createDragHandle, createChartBox, sizeChartBox } from './chart-card.js';
import { isCompact, sizeOptions, sizingFor } from './layout.js';
import { renderKeyRow } from './key-row.js';
import { getParam, setParam } from './url-state.js';
import {
    tempestPanelIds, tempestSeries, tempestEventPoints, formatTempestLatest, cardinal,
    parseHidden, serializeHidden,
} from './tempest.js';

export const TEMPEST_COLORS = {
    windAvg: '#e2e8f0',
    windGust: '#94a3b8',
    windDir: '#94a3b8',
    rain: '#38bdf8',
    rainPeak: '#0ea5e9',
    rainDay: '#7dd3fc',
    rainChecked: '#f1f5f9',
    hail: '#f8fafc',
    uv: '#a78bfa',
    solar: '#fbbf24',
    lux: '#fde68a',
    lightning: '#c084fc',
    pressure: '#cbd5e1',
    battery: '#4ade80',
};

const LABELS = {
    outdoor: 'Outdoor · °F, %',
    wind: 'Wind · mph',
    direction: 'Wind direction · where it blows from',
    rain: 'Rain · in/h (area: bucket average, line: wettest minute), in since local midnight',
    rainShort: 'Rain · in/h, in today',
    sun: 'Sun · UV index, W/m²',
    lightning: 'Lightning · distance in miles',
    station: 'Station · sea-level pressure inHg, battery V',
};

const DIRS = { 0: 'N', 90: 'E', 180: 'S', 270: 'W', 360: 'N' };

const HIDDEN_PARAM = 'tempestHidden';

// Secondary panels are shorter on wide screens; on phones they keep the
// normal height or they flatten into unreadable strips.
export function secondaryAspect(ar) {
    return ar >= 4 ? ar * 1.6 : ar;
}

function axis(position, title, extra = {}) {
    return {
        type: 'linear',
        position,
        title: { display: !!title, text: title, color: '#888', font: { size: 11 } },
        ticks: { color: '#888', maxTicksLimit: 4 },
        grid: position === 'left' ? { color: '#ffffff06' } : { drawOnChartArea: false },
        afterFit: pinAxisWidth,
        ...extra,
    };
}

// Legend clicks toggle a dataset and record the hidden set in the URL.
function legendClick(_e, item, legend) {
    const chart = legend.chart;
    const ds = chart.data.datasets[item.datasetIndex];
    chart.setDatasetVisibility(item.datasetIndex, !chart.isDatasetVisible(item.datasetIndex));
    if (ds?.key) {
        const hidden = parseHidden(getParam(HIDDEN_PARAM));
        if (chart.isDatasetVisible(item.datasetIndex)) hidden.delete(ds.key); else hidden.add(ds.key);
        setParam(HIDDEN_PARAM, serializeHidden(hidden));
    }
    chart.update();
}

// Size class of a panel: the outdoor chart matches the room charts, the
// direction strip is a fixed-height strip, the rest are shorter panels.
export function panelKind(panel) {
    if (panel === 'outdoor') return 'main';
    if (panel === 'direction') return 'direction';
    return 'secondary';
}

export function tempestPanelOpts(panel, ar, compact = isCompact()) {
    const scales = {
        x: {
            type: 'time',
            time: { tooltipFormat: 'ff' },
            ticks: { color: MUTED_TEXT, autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback, major: { enabled: true } },
            grid: { color: '#ffffff06' },
        },
    };
    switch (panel) {
    case 'outdoor':
        scales.y = axis('left', '°F');
        scales.y1 = axis('right', '%', { min: 0, max: 100 });
        break;
    case 'wind':
        scales.yWind = axis('left', 'mph', { min: 0 });
        scales.yWindR = axis('right', '', { display: true, ticks: { display: false }, grid: { drawOnChartArea: false } });
        break;
    case 'direction':
        // A thin strip: direction needs a compass axis, not the speed axis.
        // The wind panel right above labels the shared time axis, so the
        // strip hides its own time labels and gives the height to the plot.
        // Its height is fixed in pixels at every width (CHART_HEIGHTS), tall
        // enough for all five compass labels.
        scales.x.ticks = { ...scales.x.ticks, display: false };
        // Degrees increase upward (N at the bottom, E, S, W, N at the top).
        // The ticks are pinned: with a step size Chart.js would label only
        // some cardinal points, and without E and W an easterly and a
        // westerly look alike.
        scales.yDir = axis('left', '', {
            min: 0, max: 360,
            ticks: { color: MUTED_TEXT, autoSkip: false, font: { size: 10 }, callback: (val) => DIRS[val] ?? '' },
            afterBuildTicks: (scale) => { scale.ticks = [0, 90, 180, 270, 360].map(value => ({ value })); },
        });
        scales.yDirR = axis('right', '', { display: true, ticks: { display: false }, grid: { drawOnChartArea: false } });
        break;
    case 'rain':
        scales.yRain = axis('left', 'in/h', { min: 0, suggestedMax: 0.1 });
        scales.yRainDay = axis('right', 'in', { min: 0, suggestedMax: 0.1 });
        break;
    case 'sun':
        scales.yUV = axis('left', 'UV', { min: 0, suggestedMax: 3 });
        scales.ySolar = axis('right', 'W/m²', { min: 0 });
        scales.yLux = { type: 'linear', display: false, min: 0 };
        break;
    case 'lightning':
        scales.yStrike = axis('left', 'mi', { min: 0, suggestedMax: 25 });
        scales.yStrikeR = axis('right', '', { display: true, ticks: { display: false }, grid: { drawOnChartArea: false } });
        break;
    case 'station':
        scales.yPress = axis('left', 'inHg', { ticks: { color: '#888', maxTicksLimit: 4, callback: (val) => Number(val).toFixed(2) } });
        scales.yBatt = axis('right', 'V', { suggestedMin: 2.3, suggestedMax: 2.8 });
        break;
    }
    const nearest = panel === 'lightning' || panel === 'rain';
    const opts = {
        responsive: true,
        animation: { duration: CHART_ANIMATION_MS },
        interaction: nearest ? { mode: 'nearest', axis: 'x', intersect: false } : { mode: 'index', intersect: false },
        scales,
        plugins: {
            legend: {
                display: panel !== 'direction',
                // The direction strip has one dataset: no legend at any width.
                keyRow: panel === 'direction' ? false : undefined,
                position: 'bottom',
                onClick: legendClick,
                labels: { color: MUTED_TEXT, boxWidth: 16, boxHeight: 1, padding: 10, font: { size: 10 } },
            },
            tooltip: {
                backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1,
                callbacks: { label: tooltipLabel },
            },
            humidityHighlight: { enabled: false },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 1.5, tension: 0 },
        },
    };
    return sizeOptions(opts, panelKind(panel), compact, ar);
}

export function tooltipLabel(item) {
    const ds = item.dataset;
    const raw = item.raw || {};
    const y = item.parsed?.y;
    if (y == null) return null;
    const fmt = ds.tooltipFormat || ((val) => val.toFixed(ds.tooltipDigits ?? 1));
    let text = `${ds.label}: ${fmt(y, raw)}${ds.unit ? ' ' + ds.unit : ''}`;
    if (raw.count) text += ` (${raw.count} strike${raw.count === 1 ? '' : 's'})`;
    if (raw.energy != null) text += ` (energy ${raw.energy})`;
    return text;
}

function radiusForCount(pt) {
    return 2 + Math.sqrt(pt.count || 0) * 2;
}

// Datasets for one panel from tempestSeries() and tempestEventPoints().
// key names a dataset in ?tempestHidden.
export function buildTempestDatasets(panel, s, ev) {
    switch (panel) {
    case 'outdoor':
        return [
            { key: 'temp', label: 'Temperature', data: s.temp, yAxisID: 'y', borderColor: METRIC_COLORS.temp, borderWidth: 2, fill: false, peakUnit: '°F', unit: '°F' },
            { key: 'dewpoint', label: 'Dew Point', data: s.dewpoint, yAxisID: 'y', borderColor: METRIC_COLORS.dewpoint, borderWidth: 2, borderDash: [5, 3], fill: false, noPeaks: true, unit: '°F' },
            { key: 'humidity', label: 'Humidity', data: s.humidity, yAxisID: 'y1', borderColor: METRIC_COLORS.humidity, borderWidth: 2, fill: false, peakUnit: '%', tooltipDigits: 0, unit: '%' },
        ];
    case 'wind':
        return [
            { key: 'avg', label: 'Average', data: s.windAvg, yAxisID: 'yWind', borderColor: TEMPEST_COLORS.windAvg, borderWidth: 2, fill: false, noPeaks: true, unit: 'mph' },
            { key: 'gust', label: 'Gust', data: s.windGust, yAxisID: 'yWind', borderColor: TEMPEST_COLORS.windGust, borderWidth: 1, fill: false, peakUnit: '', unit: 'mph' },
            { key: 'lull', label: 'Lull', data: s.windLull, yAxisID: 'yWind', borderColor: TEMPEST_COLORS.windGust + '60', borderWidth: 1, fill: 1, backgroundColor: TEMPEST_COLORS.windGust + '22', noPeaks: true, unit: 'mph' },
        ];
    case 'direction':
        return [{
            key: 'direction', label: 'From', data: s.windDir, yAxisID: 'yDir', borderColor: TEMPEST_COLORS.windDir, backgroundColor: TEMPEST_COLORS.windDir,
            showLine: false, pointRadius: 1.2, pointHoverRadius: 3, noPeaks: true,
            tooltipFormat: (val) => `${Math.round(val)}° ${cardinal(val)}`,
        }];
    case 'rain':
        return [
            { key: 'rate', label: 'Rate', data: s.rainRate, yAxisID: 'yRain', borderColor: TEMPEST_COLORS.rain, backgroundColor: TEMPEST_COLORS.rain + '40', fill: 'origin', stepped: 'before', borderWidth: 1, tooltipDigits: 2, unit: 'in/h' },
            { key: 'peak', label: 'Peak rate', data: s.rainPeak, yAxisID: 'yRain', borderColor: TEMPEST_COLORS.rainPeak, borderWidth: 1, fill: false, noPeaks: true, tooltipDigits: 2, unit: 'in/h' },
            { key: 'today', label: 'Today', data: s.rainDay, yAxisID: 'yRainDay', borderColor: TEMPEST_COLORS.rainDay, borderWidth: 2, fill: false, noPeaks: true, tooltipDigits: 2, unit: 'in' },
            { key: 'checked', label: 'Today (Rain Check)', data: s.rainDayChecked, yAxisID: 'yRainDay', borderColor: TEMPEST_COLORS.rainChecked, borderDash: [2, 3], borderWidth: 1, fill: false, noPeaks: true, tooltipDigits: 2, unit: 'in' },
            {
                key: 'hail', label: 'Hail', data: s.hail, yAxisID: 'yRain', showLine: false, pointStyle: 'circle', pointRadius: 3, borderColor: TEMPEST_COLORS.hail,
                backgroundColor: TEMPEST_COLORS.hail, noPeaks: true, tooltipFormat: (_val, raw) => (raw.precipType === 3 ? 'rain and hail' : 'hail'),
            },
            {
                key: 'start', label: 'Rain start', data: ev.rainStarts, yAxisID: 'yRain', showLine: false, pointStyle: 'triangle', pointRadius: 5,
                borderColor: TEMPEST_COLORS.rain, backgroundColor: TEMPEST_COLORS.rain, noPeaks: true, tooltipFormat: () => 'started',
            },
        ];
    case 'sun':
        return [
            { key: 'uv', label: 'UV', data: s.uv, yAxisID: 'yUV', borderColor: TEMPEST_COLORS.uv, fill: false, peakUnit: '' },
            { key: 'solar', label: 'Solar', data: s.solar, yAxisID: 'ySolar', borderColor: TEMPEST_COLORS.solar, fill: false, noPeaks: true, tooltipDigits: 0, unit: 'W/m²' },
            { key: 'lux', label: 'Illuminance', data: s.lux, yAxisID: 'yLux', borderColor: TEMPEST_COLORS.lux, borderWidth: 1, fill: false, noPeaks: true, tooltipDigits: 0, unit: 'lux' },
        ];
    case 'lightning':
        return [
            {
                key: 'strikes', label: 'Strikes', data: s.strikes, yAxisID: 'yStrike', showLine: false, borderColor: TEMPEST_COLORS.lightning,
                backgroundColor: TEMPEST_COLORS.lightning + '55', pointRadius: s.strikes.map(radiusForCount), noPeaks: true, tooltipDigits: 0, unit: 'mi avg',
            },
            {
                key: 'strike', label: 'Single strike', data: ev.strikes, yAxisID: 'yStrike', showLine: false, pointStyle: 'crossRot', pointRadius: 3,
                borderColor: TEMPEST_COLORS.lightning, backgroundColor: TEMPEST_COLORS.lightning, noPeaks: true, tooltipDigits: 0, unit: 'mi',
            },
        ];
    case 'station':
        return [
            { key: 'pressure', label: 'Sea-level pressure', data: s.pressure, yAxisID: 'yPress', borderColor: TEMPEST_COLORS.pressure, fill: false, noPeaks: true, tooltipDigits: 2, unit: 'inHg' },
            { key: 'battery', label: 'Battery', data: s.battery, yAxisID: 'yBatt', borderColor: TEMPEST_COLORS.battery, borderWidth: 1, fill: false, noPeaks: true, tooltipDigits: 2, unit: 'V' },
        ];
    }
    return [];
}

const panelOf = (id) => id.split('/')[1];

function heading(station) {
    const h = document.createElement('h2');
    h.style.color = '#cbd5e1';
    h.appendChild(createDragHandle());
    const name = station?.room || station?.name;
    h.appendChild(document.createTextNode(name ? `Tempest · ${name}` : 'Tempest'));
    return h;
}

// Shown instead of the charts when recording is on but the range is empty.
export function buildEmptyTempestSection(station) {
    const section = document.createElement('div');
    section.className = 'room-section tempest-section';
    section.dataset.chartId = '__tempest__';
    section.appendChild(heading(station));
    const note = document.createElement('div');
    note.className = 'panel-label';
    note.textContent = 'No Tempest observations in this range. Older history needs `airquality tempest-backfill`; a station that has not reported yet shows up within a minute.';
    section.appendChild(note);
    return section;
}

// Builds the section and its charts. Returns { section, panels: [{ room, chart }] }.
export function buildTempestSection(station, ar) {
    const section = document.createElement('div');
    section.className = 'room-section tempest-section';
    section.dataset.chartId = '__tempest__';
    section.appendChild(heading(station));

    const strip = document.createElement('div');
    strip.className = 'room-latest';
    strip.id = 'tempest-current';
    section.appendChild(strip);

    const panels = [];
    for (const id of tempestPanelIds()) {
        const panel = panelOf(id);
        const card = document.createElement('div');
        card.className = 'chart-card tempest-panel';
        card.dataset.panel = panel;
        const label = document.createElement('div');
        label.className = 'panel-label';
        if (panel === 'rain') {
            // Phones show the short title (CSS picks one).
            label.innerHTML = `<span class="label-full"></span><span class="label-short"></span>`;
            label.querySelector('.label-full').textContent = LABELS.rain;
            label.querySelector('.label-short').textContent = LABELS.rainShort;
        } else {
            label.textContent = LABELS[panel];
        }
        card.appendChild(label);
        const kind = panelKind(panel);
        const { box, canvas, keyRow } = createChartBox(kind);
        sizeChartBox(box, sizingFor(kind, isCompact(), ar).height);
        card.appendChild(box);
        card.appendChild(keyRow);
        section.appendChild(card);
        const chart = new Chart(canvas, { type: 'line', data: { datasets: [] }, options: tempestPanelOpts(panel, ar) });
        panels.push({ room: id, chart, kind, box, keyRow, card, label });
    }
    return { section, panels };
}

// Sets every panel's datasets (no chart.update; the caller syncs axes first).
// Hidden datasets come from ?tempestHidden, so reloads keep legend state.
export function updateTempestSection(panels, tempest, stepMs) {
    const obs = tempest?.observations || [];
    const events = tempest?.events || [];
    const series = tempestSeries(obs, stepMs);
    const ev = tempestEventPoints(events);
    const hidden = parseHidden(getParam(HIDDEN_PARAM));
    for (const p of panels) {
        const panel = panelOf(p.room);
        const datasets = buildTempestDatasets(panel, series, ev);
        for (const d of datasets) d.hidden = hidden.has(d.key);
        p.chart.data.datasets = datasets;
        // Panels keep the same datasets, so the key row can be drawn before
        // the caller's update(); rooms.js redraws it after update() too.
        if (p.keyRow) renderKeyRow(p.keyRow, p.chart);
        if (panel === 'lightning' && p.card) {
            const none = series.strikes.length === 0 && ev.strikes.length === 0;
            p.card.classList.toggle('collapsed', none);
            p.label.textContent = none ? 'Lightning · no strikes in this range' : LABELS.lightning;
        }
    }
    const strip = document.getElementById('tempest-current');
    if (strip) {
        strip.innerHTML = formatTempestLatest(tempest?.latest, obs, events);
        // Phones show the key values; this reveals the rest. The state is
        // transient (not in the URL) but survives the minute refresh.
        if (strip.querySelector('.secondary')) {
            const more = document.createElement('button');
            more.type = 'button';
            more.className = 'strip-more';
            const sync = () => {
                const open = strip.classList.contains('expanded');
                more.textContent = open ? 'less' : 'more';
                more.setAttribute('aria-expanded', String(open));
            };
            more.addEventListener('click', () => { strip.classList.toggle('expanded'); sync(); });
            sync();
            strip.appendChild(more);
        }
    }
}
