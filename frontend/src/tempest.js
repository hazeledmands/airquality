// Pure helpers for the WeatherFlow Tempest section: unit conversion, chart
// series, URL state and the latest-values strip. Storage and the API stay
// metric; the dashboard shows °F like the indoor charts, with wind in mph,
// rain in inches and pressure in inHg to match.
import { cToF, insertGaps } from './data.js';

export const msToMph = (v) => (v == null ? null : v * 2.236936);
export const mmToIn = (v) => (v == null ? null : v / 25.4);
export const kmToMi = (v) => (v == null ? null : v / 1.609344);
export const hpaToInHg = (v) => (v == null ? null : v / 33.8639);

const POINTS = ['N', 'NNE', 'NE', 'ENE', 'E', 'ESE', 'SE', 'SSE', 'S', 'SSW', 'SW', 'WSW', 'W', 'WNW', 'NW', 'NNW'];

export function cardinal(deg) {
    if (deg == null) return '';
    return POINTS[Math.round((((deg % 360) + 360) % 360) / 22.5) % 16];
}

const PANELS = ['outdoor', 'wind', 'direction', 'rain', 'sun', 'lightning', 'station'];

// Chart ids the Tempest section registers, in display order.
export function tempestPanelIds() {
    return PANELS.map(p => `__tempest__/${p}`);
}

// Legend toggles persist in ?tempestHidden=key,key. Absent means the
// default (illuminance hidden); empty means nothing hidden.
export const DEFAULT_HIDDEN = ['lux'];

export function parseHidden(param) {
    if (param == null) return new Set(DEFAULT_HIDDEN);
    return new Set(param.split(',').map(s => s.trim()).filter(Boolean));
}

// Returns the query value for a hidden set, or null when it is the default.
export function serializeHidden(hidden) {
    const keys = [...hidden].sort();
    const def = [...DEFAULT_HIDDEN].sort();
    if (keys.length === def.length && keys.every((k, i) => k === def[i])) return null;
    return keys.join(',');
}

const ms = (o) => new Date(o.timestamp).getTime();

// Chart series for every Tempest field. stepMs is the server's bucket size
// (0 for raw one-minute rows); it sets the rain-rate divisor and gap width.
// Every series built with line() has one point per observation plus the same
// gap markers, so index-mode tooltips pair the right values.
export function tempestSeries(obs, stepMs = 0) {
    const line = (fn) => {
        const raw = obs.map(o => ({ x: o.timestamp, y: fn(o) ?? null }));
        return stepMs > 0 ? insertGaps(raw, stepMs) : raw;
    };
    const minutes = (o) => (stepMs > 0 ? stepMs / 60_000 : (o.report_interval_min || 1));
    const ratePerHour = (mm, o) => (mm == null ? null : mmToIn(mm) * 60 / minutes(o));
    const perMinuteRate = (mm, o) => (mm == null ? null : mmToIn(mm) * 60 / (o.report_interval_min || 1));
    return {
        temp: line(o => (o.air_temp_c == null ? null : cToF(o.air_temp_c))),
        dewpoint: line(o => (o.dewpoint_c == null ? null : cToF(o.dewpoint_c))),
        humidity: line(o => o.relative_humidity),
        windAvg: line(o => msToMph(o.wind_avg_ms)),
        windGust: line(o => msToMph(o.wind_gust_ms)),
        windLull: line(o => msToMph(o.wind_lull_ms)),
        // No direction in calm air: the vane reading is meaningless then.
        windDir: line(o => (o.wind_avg_ms === 0 ? null : o.wind_direction_deg)),
        rainRate: line(o => ratePerHour(o.rain_mm, o)),
        // The wettest single minute in the bucket, as a rate.
        rainPeak: line(o => perMinuteRate(o.rain_max_mm ?? o.rain_mm, o)),
        rainDay: line(o => mmToIn(o.local_day_rain_mm)),
        rainDayChecked: line(o => mmToIn(o.local_day_rain_final_mm)),
        hail: obs.filter(o => o.precip_type >= 2).map(o => ({ x: o.timestamp, y: 0, precipType: o.precip_type })),
        uv: line(o => o.uv_index),
        solar: line(o => o.solar_radiation_wm2),
        lux: line(o => o.illuminance_lux),
        strikes: obs.filter(o => o.lightning_strike_count > 0)
            .map(o => ({ x: o.timestamp, y: kmToMi(o.lightning_avg_distance_km), count: o.lightning_strike_count })),
        pressure: line(o => hpaToInHg(o.sea_level_pressure_mb)),
        battery: line(o => o.battery_v),
    };
}

// Lightning strike and rain start events as chart points.
export function tempestEventPoints(events) {
    return {
        strikes: events.filter(e => e.type === 'lightning_strike')
            .map(e => ({ x: e.timestamp, y: kmToMi(e.distance_km), energy: e.energy })),
        rainStarts: events.filter(e => e.type === 'rain_start').map(e => ({ x: e.timestamp, y: 0 })),
    };
}

const HOURS3 = 3 * 3600_000;

// WeatherFlow's 3-hour pressure trend: ±1 hPa or more is rising/falling.
export function pressureTrend(obs) {
    const withP = obs.filter(o => o.station_pressure_mb != null);
    if (withP.length < 2) return null;
    const latest = withP[withP.length - 1];
    const target = ms(latest) - HOURS3;
    let before = null;
    for (const o of withP) {
        if (ms(o) <= target + 5 * 60_000) before = o; else break;
    }
    if (!before || ms(before) < target - 30 * 60_000) return null;
    const delta = latest.station_pressure_mb - before.station_pressure_mb;
    const label = delta >= 1 ? 'rising' : delta <= -1 ? 'falling' : 'steady';
    return { delta, label };
}

// Rain since station-local midnight, preferring the Rain Check total.
export function rainToday(obs) {
    for (let i = obs.length - 1; i >= 0; i--) {
        const o = obs[i];
        if (o.local_day_rain_final_mm != null) return { mm: o.local_day_rain_final_mm, checked: true };
        if (o.local_day_rain_mm != null) return { mm: o.local_day_rain_mm, checked: false };
    }
    return null;
}

// Strikes in the 3 hours before the latest observation, and the latest
// strike's distance (from a WebSocket event when there is a newer one).
export function lightningSummary(obs, events = []) {
    if (!obs.length) return { count3h: 0, lastDistanceKm: null, lastTime: null };
    const end = ms(obs[obs.length - 1]);
    let count3h = 0;
    let lastTime = null;
    let lastDistanceKm = null;
    for (const o of obs) {
        if (!(o.lightning_strike_count > 0)) continue;
        if (ms(o) > end - HOURS3) count3h += o.lightning_strike_count;
        lastTime = ms(o);
        lastDistanceKm = o.lightning_avg_distance_km;
    }
    for (const e of events) {
        if (e.type !== 'lightning_strike') continue;
        const t = new Date(e.timestamp).getTime();
        if (lastTime == null || t >= lastTime) {
            lastTime = t;
            lastDistanceKm = e.distance_km;
        }
    }
    return { count3h, lastDistanceKm, lastTime };
}

// Tempest air temperature shaped like Open-Meteo rows, for the outdoor
// overlay on indoor charts.
export function tempestAsWeather(obs) {
    return obs.filter(o => o.air_temp_c != null).map(o => ({ timestamp: o.timestamp, temp_c: o.air_temp_c }));
}

const v = (text) => `<span class="value">${text}</span>`;
const STALE_MS = 5 * 60_000;

// HTML for the strip above the Tempest charts. latest is the newest raw
// observation (never a downsampled bucket); obs are the chart rows, used only
// for the 3-hour pressure trend and strike count.
export function formatTempestLatest(latest, obs = [], events = [], nowMs = Date.now()) {
    if (!latest) return '';
    const parts = [];
    const t = ms(latest);
    if (nowMs - t > STALE_MS) {
        const when = new Date(t).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' });
        parts.push(`<span class="devices">as of ${when}</span>`);
    }
    if (latest.air_temp_c != null) parts.push(`<span>${v(cToF(latest.air_temp_c).toFixed(1) + '°F')}</span>`);
    if (latest.relative_humidity != null) parts.push(`<span>${v(latest.relative_humidity.toFixed(0) + '%')} humidity</span>`);
    if (latest.dewpoint_c != null) parts.push(`<span>${v(cToF(latest.dewpoint_c).toFixed(1) + '°F')} dew point</span>`);

    if (latest.wind_avg_ms != null) {
        const dir = latest.wind_avg_ms > 0 ? ' ' + cardinal(latest.wind_direction_deg) : '';
        const gust = latest.wind_gust_ms != null ? `, gusts ${v(msToMph(latest.wind_gust_ms).toFixed(1))}` : '';
        parts.push(`<span>wind ${v(msToMph(latest.wind_avg_ms).toFixed(1))} mph${dir}${gust}</span>`);
    }

    const today = rainToday([latest]);
    if (today) parts.push(`<span>${v(mmToIn(today.mm).toFixed(2) + ' in')} rain today${today.checked ? ' (Rain Check)' : ''}</span>`);
    if (latest.rain_mm > 0) {
        const rate = mmToIn(latest.rain_mm) * 60 / (latest.report_interval_min || 1);
        parts.push(`<span>${v(rate.toFixed(2))} in/h now</span>`);
    }

    if (latest.uv_index != null) parts.push(`<span>UV ${v(latest.uv_index.toFixed(1))}</span>`);
    if (latest.solar_radiation_wm2 != null) parts.push(`<span>${v(Math.round(latest.solar_radiation_wm2))} W/m²</span>`);
    if (latest.illuminance_lux != null) parts.push(`<span>${v(Math.round(latest.illuminance_lux).toLocaleString('en-US'))} lux</span>`);

    const ls = lightningSummary(obs, events);
    if (ls.count3h > 0) {
        const dist = ls.lastDistanceKm != null ? `, last ${v(kmToMi(ls.lastDistanceKm).toFixed(0))} mi away` : '';
        parts.push(`<span>${v(ls.count3h)} strike${ls.count3h === 1 ? '' : 's'} in 3 h${dist}</span>`);
    }

    if (latest.sea_level_pressure_mb != null) {
        const trend = pressureTrend(obs);
        const tr = trend ? ` ${trend.label} (${trend.delta >= 0 ? '+' : ''}${hpaToInHg(trend.delta).toFixed(2)} in 3 h)` : '';
        parts.push(`<span>${v(hpaToInHg(latest.sea_level_pressure_mb).toFixed(2))} inHg${tr}</span>`);
    }
    if (latest.battery_v != null) parts.push(`<span>battery ${v(latest.battery_v.toFixed(2) + ' V')}</span>`);
    return parts.join('');
}
