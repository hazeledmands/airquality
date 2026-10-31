import { CHART_ANIMATION_MS } from './config.js';
import { xTickCallback } from './charts.js';
import { makeWeatherTempSeries } from './data.js';

export const METRIC_COLORS = {
    temp:     '#fb923c', // orange
    dewpoint: '#38bdf8', // blue
    humidity: '#34d399', // green
    pm:       '#a78bfa', // purple
    acState:  '#6b7280', // gray
};

export const PARTICLE_BINS = [
    { field: 'p03',  label: 'P\u22650.3\u00b5m', rangeLabel: '0.3\u20130.5\u00b5m',  color: '#67e8f9' },
    { field: 'p05',  label: 'P\u22650.5\u00b5m', rangeLabel: '0.5\u20131.0\u00b5m',  color: '#86efac' },
    { field: 'p10',  label: 'P\u22651.0\u00b5m', rangeLabel: '1.0\u20132.5\u00b5m',  color: '#fde68a' },
    { field: 'p25',  label: 'P\u22652.5\u00b5m', rangeLabel: '2.5\u20135.0\u00b5m',  color: '#fca5a5' },
    { field: 'p50',  label: 'P\u22655.0\u00b5m', rangeLabel: '5.0\u201310\u00b5m',   color: '#f87171' },
    { field: 'p100', label: 'P\u226510\u00b5m',  rangeLabel: '\u226510\u00b5m',       color: '#dc2626' },
];

export function computeExclusiveBins(particles) {
    const fields = PARTICLE_BINS.map(b => b.field);
    const result = {};
    for (let bi = 0; bi < fields.length; bi++) {
        const field = fields[bi];
        const nextField = fields[bi + 1];
        result[field] = (particles[field] || []).map((series, si) => {
            const nextSeries = nextField ? (particles[nextField] || [])[si] : null;
            const exclusiveData = series.data.map((pt, pi) => {
                const nextY = nextSeries ? (nextSeries.data[pi]?.y ?? 0) : 0;
                return { x: pt.x, y: Math.max(0, pt.y - nextY) };
            });
            return { ...series, data: exclusiveData };
        });
    }
    return result;
}

export function isIndoorRoom(room) {
    return !/outside/i.test(room.name);
}

// Legend shows one entry per metric even when several devices contribute a
// line; clicking it toggles every device's line for that metric.
function metricOf(chart, i) {
    const ds = chart.data.datasets[i];
    return ds?.metric ?? ds?.label;
}

const metricLegend = {
    filter(item, chartData) {
        const metric = chartData.datasets[item.datasetIndex]?.metric;
        if (!metric) return true;
        return chartData.datasets.findIndex(d => d.metric === metric) === item.datasetIndex;
    },
    generateLabels(chart) {
        const base = globalThis.Chart?.defaults?.plugins?.legend?.labels?.generateLabels;
        const items = base
            ? base(chart)
            : chart.data.datasets.map((d, i) => ({ text: d.label, datasetIndex: i }));
        return items.map(item => ({ ...item, text: metricOf(chart, item.datasetIndex) ?? item.text }));
    },
    onClick(_e, item, legend) {
        const chart = legend.chart;
        const metric = metricOf(chart, item.datasetIndex);
        const visible = !chart.isDatasetVisible(item.datasetIndex);
        chart.data.datasets.forEach((_, i) => {
            if (metricOf(chart, i) === metric) chart.setDatasetVisibility(i, visible);
        });
        chart.update();
    },
};

export function combinedChartOpts(ar, room) {
    const hasPM = room.pm.length > 0;
    const hasAC = room.acState.length > 0;
    const indoor = isIndoorRoom(room);

    const opts = {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio: ar,
        animation: { duration: CHART_ANIMATION_MS },
        interaction: { mode: 'index', intersect: false },
        scales: {
            x: {
                type: 'time',
                time: { tooltipFormat: 'ff' },
                ticks: { color: '#555', autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback, major: { enabled: true } },
                grid: { color: '#ffffff06' },
            },
            y: {
                type: 'linear',
                position: 'left',
                title: { display: true, text: '\u00B0F', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { color: '#ffffff06' },
            },
            y1: {
                type: 'linear',
                position: 'right',
                title: { display: true, text: '%', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { drawOnChartArea: false },
                min: 0,
                max: 100,
            },
        },
        plugins: {
            legend: {
                position: 'bottom',
                onClick: metricLegend.onClick,
                labels: {
                    color: '#555', boxWidth: 16, boxHeight: 1, padding: 10, font: { size: 10 },
                    filter: metricLegend.filter,
                    generateLabels: metricLegend.generateLabels,
                },
            },
            tooltip: { backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1 },
            humidityHighlight: { enabled: indoor, threshold: 60 },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 2, tension: 0 },
        },
    };

    if (hasPM) {
        opts.scales.y2 = {
            type: 'linear',
            position: 'right',
            title: { display: true, text: '\u00B5g/m\u00B3', color: '#888', font: { size: 11 } },
            ticks: { color: '#888' },
            grid: { drawOnChartArea: false },
        };
    }

    if (hasAC) {
        opts.scales.y3 = {
            type: 'linear',
            position: 'right',
            display: false,
            min: -0.05,
            max: 1.05,
        };
    }

    return opts;
}

// `opts` is `{ outdoorTempEnabled, currentWeather }` — passed in to avoid
// module-level state coupling and to keep this function pure/testable.
export function buildCombinedDatasets(room, { outdoorTempEnabled = false, currentWeather = [], stepMs = 0 } = {}) {
    const datasets = [];
    // With several devices in one room, all lines of a metric share a color;
    // the device name in the label tells them apart in the tooltip.
    const multiDevice = (room.devices?.length ?? 0) > 1;
    const label = (metric, ds) => (multiDevice && ds.device ? `${metric} \u00B7 ${ds.device}` : metric);

    for (const ds of room.temp) {
        datasets.push({
            ...ds,
            label: label('Temperature', ds),
            metric: 'Temperature',
            yAxisID: 'y',
            borderColor: METRIC_COLORS.temp,
            fill: false,
            peakUnit: '\u00B0F',
        });
    }

    for (const ds of room.dewpoint) {
        datasets.push({
            ...ds,
            label: label('Dew Point', ds),
            metric: 'Dew Point',
            yAxisID: 'y',
            borderColor: METRIC_COLORS.dewpoint,
            fill: false,
            borderDash: [5, 3],
            peakUnit: '\u00B0F',
        });
    }

    for (const ds of room.humidity) {
        datasets.push({
            ...ds,
            label: label('Humidity', ds),
            metric: 'Humidity',
            yAxisID: 'y1',
            borderColor: METRIC_COLORS.humidity,
            fill: false,
            peakUnit: '%',
        });
    }

    for (const ds of room.pm) {
        datasets.push({
            ...ds,
            label: label('PM2.5', ds),
            metric: 'PM2.5',
            yAxisID: 'y2',
            borderColor: METRIC_COLORS.pm,
            fill: false,
            peakUnit: '\u00B5g/m\u00B3',
        });
    }

    for (const ds of room.acState) {
        datasets.push({
            ...ds,
            label: 'AC',
            yAxisID: 'y3',
            borderColor: METRIC_COLORS.acState,
            backgroundColor: METRIC_COLORS.acState + '18',
            fill: true,
            borderWidth: 1,
            stepped: true,
            tension: 0,
        });
    }

    if (outdoorTempEnabled && currentWeather.length > 0 && isIndoorRoom(room)) {
        datasets.push({
            ...makeWeatherTempSeries(currentWeather),
            yAxisID: 'y',
        });
    }

    return datasets;
}

function lastY(series) {
    const data = series?.data;
    for (let i = (data?.length ?? 0) - 1; i >= 0; i--) {
        if (data[i].y != null) return data[i].y;
    }
    return null;
}

// HTML for the latest-values strip above a room chart. With several devices,
// each metric shows every device's value ("70.1 / 71.5°F") followed by a key
// naming the devices in the same order.
export function formatLatestValues(room) {
    if (!room.temp?.length || lastY(room.temp[0]) == null) return '';
    const join = (list, digits) => list.map(lastY).filter(v => v != null).map(v => v.toFixed(digits)).join(' / ');
    const parts = [`<span><span class="value">${join(room.temp, 1)}\u00B0F</span></span>`];
    if (room.humidity?.length) parts.push(`<span><span class="value">${join(room.humidity, 0)}%</span> humidity</span>`);
    if (room.dewpoint?.length) parts.push(`<span><span class="value">${join(room.dewpoint, 1)}\u00B0F</span> dew point</span>`);
    if (room.pm?.length) parts.push(`<span>PM2.5 <span class="value">${join(room.pm, 1)}</span></span>`);
    if ((room.devices?.length ?? 0) > 1) parts.push(`<span class="devices">${room.devices.join(' / ')}</span>`);
    return parts.join('');
}
