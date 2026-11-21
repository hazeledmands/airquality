// Open-Meteo extras on the dashboard: outdoor air quality (to compare with
// indoor PM2.5 and decide when to open windows), cloud cover, and a compact
// forecast strip. Pure functions; rooms.js draws them.
import { cToF } from './data.js';

export const AIR_COLORS = {
    outdoorPM: '#c4b5fd', // lighter than the indoor PM2.5 purple
    aqi: '#f472b6',
    pm: '#a78bfa',
    ozone: '#5eead4',
    no2: '#fdba74',
    pollen: '#bef264',
    cloud: '#94a3b8',
};

const POLLEN = [
    ['alder_pollen', 'Alder pollen'], ['birch_pollen', 'Birch pollen'], ['grass_pollen', 'Grass pollen'],
    ['mugwort_pollen', 'Mugwort pollen'], ['olive_pollen', 'Olive pollen'], ['ragweed_pollen', 'Ragweed pollen'],
];

const series = (rows, field) => rows.map(r => ({ x: r.timestamp, y: r[field] ?? null }));

// Outdoor PM2.5 as a dashed line on the indoor charts' PM axis (y2), so
// indoor and outdoor share one scale.
export function outdoorPMSeries(rows) {
    return {
        key: 'outdoorPM',
        label: 'Outdoor PM2.5',
        metric: 'Outdoor PM2.5',
        data: series(rows, 'pm2_5'),
        yAxisID: 'y2',
        borderColor: AIR_COLORS.outdoorPM,
        borderDash: [4, 3],
        borderWidth: 1.5,
        fill: false,
        spanGaps: false,
        noPeaks: true,
        unit: 'µg/m³',
    };
}

// US EPA AQI bands.
export function aqiCategory(aqi) {
    if (aqi == null) return '';
    if (aqi <= 50) return 'good';
    if (aqi <= 100) return 'moderate';
    if (aqi <= 150) return 'unhealthy for sensitive groups';
    if (aqi <= 200) return 'unhealthy';
    if (aqi <= 300) return 'very unhealthy';
    return 'hazardous';
}

// Datasets for the outdoor air chart: AQI on yAQI, gases and particles in
// µg/m³ on yGas, pollen (grains/m³, Europe only) on yPollen when present.
export function airDatasets(rows) {
    const line = (label, field, axis, color, extra = {}) => ({
        label, data: series(rows, field), yAxisID: axis, borderColor: color, fill: false, borderWidth: 1.5, ...extra,
    });
    const ds = [
        line('US AQI', 'us_aqi', 'yAQI', AIR_COLORS.aqi, { borderWidth: 2, peakUnit: '', tooltipDigits: 0 }),
        line('PM2.5', 'pm2_5', 'yGas', AIR_COLORS.pm, { noPeaks: true, unit: 'µg/m³' }),
        line('Ozone', 'ozone', 'yGas', AIR_COLORS.ozone, { noPeaks: true, unit: 'µg/m³' }),
        line('NO₂', 'nitrogen_dioxide', 'yGas', AIR_COLORS.no2, { noPeaks: true, unit: 'µg/m³' }),
    ];
    for (const [field, label] of POLLEN) {
        if (rows.some(r => r[field] != null)) {
            ds.push(line(label, field, 'yPollen', AIR_COLORS.pollen, { noPeaks: true, borderDash: [2, 2], tooltipDigits: 0, unit: 'grains/m³' }));
        }
    }
    return ds;
}

export function hasPollen(rows) {
    return POLLEN.some(([field]) => rows.some(r => r[field] != null));
}

const v = (text) => `<span class="value">${text}</span>`;

// The latest-values strip for outdoor air.
export function formatAirLatest(r) {
    if (!r) return '';
    const parts = [];
    if (r.us_aqi != null) parts.push(`<span>AQI ${v(Math.round(r.us_aqi))} ${aqiCategory(r.us_aqi)}</span>`);
    if (r.pm2_5 != null) parts.push(`<span>PM2.5 ${v(r.pm2_5.toFixed(1))}</span>`);
    if (r.ozone != null) parts.push(`<span class="secondary">O₃ ${v(Math.round(r.ozone))}</span>`);
    if (r.nitrogen_dioxide != null) parts.push(`<span class="secondary">NO₂ ${v(Math.round(r.nitrogen_dioxide))} µg/m³</span>`);
    for (const [field, label] of POLLEN) {
        if (r[field] != null) parts.push(`<span class="secondary">${label.toLowerCase()} ${v(Math.round(r[field]))}</span>`);
    }
    return parts.join('');
}

// Cloud cover as a faint fill behind the weather chart's lines.
export function cloudSeries(weather) {
    return {
        key: 'cloud',
        label: 'Cloud cover',
        data: weather.map(r => ({ x: r.timestamp, y: r.cloud_cover ?? null })),
        yAxisID: 'yCloud',
        borderColor: AIR_COLORS.cloud + '40',
        backgroundColor: AIR_COLORS.cloud + '18',
        borderWidth: 1,
        fill: 'origin',
        noPeaks: true,
        tooltipDigits: 0,
        unit: '%',
    };
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

// The next three local days from now: high/low (°F), highest chance of
// precipitation and mean cloud cover.
export function forecastDays(hours, nowMs = Date.now()) {
    const today = new Date(nowMs).toDateString();
    const byDay = new Map();
    for (const h of hours) {
        const t = new Date(h.timestamp);
        if (t.getTime() < nowMs - 3600_000) continue;
        const key = t.toDateString();
        if (!byDay.has(key)) byDay.set(key, { date: t, hours: [] });
        byDay.get(key).hours.push(h);
    }
    const days = [];
    for (const [key, { date, hours: hs }] of byDay) {
        const temps = hs.map(h => h.temp_c).filter(x => x != null);
        const probs = hs.map(h => h.precip_probability).filter(x => x != null);
        const clouds = hs.map(h => h.cloud_cover).filter(x => x != null);
        days.push({
            label: key === today ? 'Today' : DAYS[date.getDay()],
            hiF: temps.length ? cToF(Math.max(...temps)) : null,
            loF: temps.length ? cToF(Math.min(...temps)) : null,
            rainPct: probs.length ? Math.max(...probs) : null,
            cloudPct: clouds.length ? clouds.reduce((a, b) => a + b, 0) / clouds.length : null,
        });
        if (days.length === 3) break;
    }
    return days;
}

// One-line forecast strip: three days, then the next six hours' rain chance.
export function formatForecastStrip(hours, nowMs = Date.now()) {
    const days = forecastDays(hours, nowMs);
    if (!days.length) return '';
    const parts = days.map(d => {
        const temps = d.hiF != null ? `${v(Math.round(d.hiF))}/${Math.round(d.loF)}°F` : '';
        const rain = d.rainPct != null ? ` rain ${Math.round(d.rainPct)}%` : '';
        return `<span>${d.label} ${temps}${rain}</span>`;
    });
    const next6 = hours.filter(h => {
        const t = new Date(h.timestamp).getTime();
        return t >= nowMs - 3600_000 && t < nowMs + 6 * 3600_000 && h.precip_probability != null;
    });
    if (next6.length) {
        const max = Math.max(...next6.map(h => h.precip_probability));
        parts.push(`<span class="devices">next 6 h: rain up to ${Math.round(max)}%</span>`);
    }
    return parts.join('');
}
