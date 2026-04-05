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
            legend: { position: 'bottom', labels: { color: '#555', boxWidth: 16, boxHeight: 1, padding: 10, font: { size: 10 } } },
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

    for (const ds of room.temp) {
        datasets.push({
            ...ds,
            label: 'Temperature',
            yAxisID: 'y',
            borderColor: METRIC_COLORS.temp,
            fill: false,
            peakUnit: '\u00B0F',
        });
    }

    for (const ds of room.dewpoint) {
        datasets.push({
            ...ds,
            label: 'Dew Point',
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
            label: 'Humidity',
            yAxisID: 'y1',
            borderColor: METRIC_COLORS.humidity,
            fill: false,
            peakUnit: '%',
        });
    }

    for (const ds of room.pm) {
        datasets.push({
            ...ds,
            label: 'PM2.5',
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
            ...makeWeatherTempSeries(currentWeather, {}, stepMs),
            yAxisID: 'y',
        });
    }

    return datasets;
}
