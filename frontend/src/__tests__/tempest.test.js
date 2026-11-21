// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import {
    msToMph, mmToIn, kmToMi, cardinal, tempestSeries, pressureTrend, rainToday,
    lightningSummary, tempestAsWeather, formatTempestLatest, tempestPanelIds,
    hpaToInHg, parseHidden, serializeHidden, DEFAULT_HIDDEN,
} from '../tempest.js';

const T0 = Date.parse('2026-11-20T12:00:00Z');
const iso = (min) => new Date(T0 + min * 60_000).toISOString();

function ob(min, fields = {}) {
    return {
        device_id: 2002, timestamp: iso(min),
        wind_lull_ms: null, wind_avg_ms: null, wind_gust_ms: null, wind_direction_deg: null,
        station_pressure_mb: null, air_temp_c: null, relative_humidity: null, dewpoint_c: null,
        illuminance_lux: null, uv_index: null, solar_radiation_wm2: null,
        rain_mm: null, precip_type: null, lightning_avg_distance_km: null, lightning_strike_count: null,
        battery_v: null, report_interval_min: 1, local_day_rain_mm: null, rain_final_mm: null,
        local_day_rain_final_mm: null, precip_analysis_type: null,
        ...fields,
    };
}

describe('unit conversions', () => {
    it('converts m/s to mph', () => expect(msToMph(10)).toBeCloseTo(22.369, 2));
    it('converts mm to inches', () => expect(mmToIn(25.4)).toBeCloseTo(1));
    it('converts km to miles', () => expect(kmToMi(1.609344)).toBeCloseTo(1));
    it('passes null through', () => {
        expect(msToMph(null)).toBeNull();
        expect(mmToIn(null)).toBeNull();
        expect(kmToMi(undefined)).toBeNull();
    });
});

describe('cardinal', () => {
    it('maps degrees to 16 compass points', () => {
        expect(cardinal(0)).toBe('N');
        expect(cardinal(359)).toBe('N');
        expect(cardinal(22.5)).toBe('NNE');
        expect(cardinal(90)).toBe('E');
        expect(cardinal(225)).toBe('SW');
        expect(cardinal(null)).toBe('');
    });
});

describe('tempestSeries', () => {
    const obs = [
        ob(0, { air_temp_c: 10, dewpoint_c: 5, relative_humidity: 70, wind_avg_ms: 1, wind_gust_ms: 2, wind_lull_ms: 0.5,
            wind_direction_deg: 90, rain_mm: 0.254, local_day_rain_mm: 1.27, uv_index: 2, solar_radiation_wm2: 300,
            illuminance_lux: 36000, station_pressure_mb: 1012, battery_v: 2.6, lightning_strike_count: 0 }),
        ob(1, { air_temp_c: 11, wind_gust_ms: 4, rain_mm: 0, lightning_strike_count: 3, lightning_avg_distance_km: 16.09 }),
    ];

    it('converts temperatures to °F and keeps humidity in %', () => {
        const s = tempestSeries(obs, 0);
        expect(s.temp.map(p => p.y)).toEqual([50, 51.8]);
        expect(s.dewpoint[0].y).toBeCloseTo(41);
        expect(s.dewpoint[1].y).toBeNull();
        expect(s.humidity[0].y).toBe(70);
    });

    it('converts wind to mph', () => {
        const s = tempestSeries(obs, 0);
        expect(s.windGust[1].y).toBeCloseTo(8.948, 2);
        expect(s.windAvg[0].y).toBeCloseTo(2.237, 2);
        expect(s.windDir[0].y).toBe(90);
    });

    // Chart.js index mode pairs datasets by array index, so direction must
    // line up with speed point for point, gaps included.
    it('keeps wind direction index-aligned with speed, gaps included', () => {
        const gappy = [
            ob(0, { wind_avg_ms: 1, wind_direction_deg: 90 }),
            ob(1, { wind_avg_ms: 0, wind_direction_deg: 180 }), // calm: no direction
            ob(9, { wind_avg_ms: 2, wind_direction_deg: 270 }),
        ];
        const s = tempestSeries(gappy, 60_000);
        expect(s.windDir.map(p => p.x)).toEqual(s.windAvg.map(p => p.x));
        expect(s.windDir.map(p => p.y)).toEqual([90, null, null, 270]);
    });

    it('converts sea-level pressure to inHg', () => {
        const s = tempestSeries([ob(0, { sea_level_pressure_mb: 1013.25 })], 0);
        expect(s.pressure[0].y).toBeCloseTo(29.92, 2);
    });

    it('plots the peak rain rate from the wettest minute in a bucket', () => {
        const s = tempestSeries([ob(0, { rain_mm: 0.5, rain_max_mm: 0.254 })], 300_000);
        expect(s.rainRate[0].y).toBeCloseTo(mmToIn(0.5) * 60 / 5);
        expect(s.rainPeak[0].y).toBeCloseTo(0.6);
    });

    it('derives rain rate in in/h from the per-interval amount', () => {
        // 0.254 mm in one minute = 15.24 mm/h = 0.6 in/h
        expect(tempestSeries(obs, 0).rainRate[0].y).toBeCloseTo(0.6);
        // In a 5-minute bucket the same amount is a fifth of the rate.
        expect(tempestSeries(obs, 300_000).rainRate[0].y).toBeCloseTo(0.12);
        expect(tempestSeries(obs, 0).rainDay[0].y).toBeCloseTo(0.05);
    });

    it('plots strike minutes only, sized by count, distance in miles', () => {
        const s = tempestSeries(obs, 0);
        expect(s.strikes).toHaveLength(1);
        expect(s.strikes[0].y).toBeCloseTo(10, 1);
        expect(s.strikes[0].count).toBe(3);
    });

    it('inserts gaps where minutes are missing', () => {
        const gappy = [ob(0, { air_temp_c: 10 }), ob(10, { air_temp_c: 12 })];
        const s = tempestSeries(gappy, 60_000);
        expect(s.temp).toHaveLength(3);
        expect(s.temp[1].y).toBeNull();
    });
});

describe('pressureTrend', () => {
    it('is rising when pressure climbs ≥1 hPa over 3 hours', () => {
        const obs = [ob(0, { station_pressure_mb: 1010 }), ob(90, { station_pressure_mb: 1010.5 }), ob(180, { station_pressure_mb: 1011.2 })];
        const t = pressureTrend(obs);
        expect(t.label).toBe('rising');
        expect(t.delta).toBeCloseTo(1.2);
    });
    it('is falling when it drops ≥1 hPa', () => {
        const obs = [ob(0, { station_pressure_mb: 1012 }), ob(185, { station_pressure_mb: 1010.9 })];
        expect(pressureTrend(obs).label).toBe('falling');
    });
    it('is steady within ±1 hPa', () => {
        const obs = [ob(0, { station_pressure_mb: 1012 }), ob(180, { station_pressure_mb: 1012.4 })];
        expect(pressureTrend(obs).label).toBe('steady');
    });
    it('is null with less than 3 hours of data', () => {
        expect(pressureTrend([ob(0, { station_pressure_mb: 1 }), ob(60, { station_pressure_mb: 2 })])).toBeNull();
    });
});

describe('rainToday', () => {
    it('prefers the Rain Check total when present', () => {
        expect(rainToday([ob(0, { local_day_rain_mm: 2, local_day_rain_final_mm: 2.5 })])).toEqual({ mm: 2.5, checked: true });
        expect(rainToday([ob(0, { local_day_rain_mm: 2 }), ob(1, {})])).toEqual({ mm: 2, checked: false });
        expect(rainToday([])).toBeNull();
    });
});

describe('lightningSummary', () => {
    it('counts strikes in the last 3 hours of observations and finds the latest', () => {
        const obs = [
            ob(-300, { lightning_strike_count: 9, lightning_avg_distance_km: 30 }),
            ob(-60, { lightning_strike_count: 2, lightning_avg_distance_km: 20 }),
            ob(0, { lightning_strike_count: 1, lightning_avg_distance_km: 8 }),
        ];
        const events = [{ timestamp: iso(0), type: 'lightning_strike', distance_km: 7 }];
        const s = lightningSummary(obs, events);
        expect(s.count3h).toBe(3);
        expect(s.lastDistanceKm).toBe(7);
        expect(s.lastTime).toBe(T0);
    });
    it('falls back to observations without events', () => {
        const s = lightningSummary([ob(0, { lightning_strike_count: 1, lightning_avg_distance_km: 8 })], []);
        expect(s.lastDistanceKm).toBe(8);
    });
});

describe('tempestAsWeather', () => {
    it('shapes observations like weather rows for the outdoor overlay', () => {
        expect(tempestAsWeather([ob(0, { air_temp_c: 10 }), ob(1, {})])).toEqual([{ timestamp: iso(0), temp_c: 10 }]);
    });
});

describe('formatTempestLatest', () => {
    const latest = ob(0, {
        air_temp_c: 20, relative_humidity: 50, dewpoint_c: 9.3, wind_avg_ms: 2, wind_gust_ms: 5, wind_direction_deg: 270,
        local_day_rain_mm: 2.54, rain_mm: 0.254, uv_index: 3.2, solar_radiation_wm2: 410, illuminance_lux: 51000,
        station_pressure_mb: 1011, sea_level_pressure_mb: 1013.25, battery_v: 2.61, report_interval_min: 1,
    });

    it('summarises the latest raw observation', () => {
        const html = formatTempestLatest(latest, [latest], [], T0 + 60_000);
        expect(html).toContain('68.0°F');
        expect(html).toContain('50%');
        expect(html).toContain('mph W');
        expect(html).toContain('11.2'); // gust mph
        expect(html).toContain('0.10'); // inches today
        expect(html).toContain('UV');
        expect(html).toContain('410');
        expect(html).toContain('51,000');
        expect(html).toContain('29.92</span> inHg');
        expect(html).toContain('2.61');
        expect(html).not.toContain('as of');
    });

    // The rate comes from the raw minute, whatever bucket size the charts use:
    // 0.254 mm in one minute is 0.60 in/h.
    it('shows the current rain rate from the raw minute, not a bucket sum', () => {
        const bucketed = [{ ...latest, rain_mm: 0.254 * 9 }]; // a 9-minute bucket sum
        const html = formatTempestLatest(latest, bucketed, [], T0);
        expect(html).toContain('0.60</span> in/h now');
    });

    it('says "as of" when the latest observation is stale', () => {
        const html = formatTempestLatest(latest, [latest], [], T0 + 20 * 60_000);
        expect(html).toMatch(/as of \d{1,2}:\d{2}/);
    });

    it('is empty without an observation', () => {
        expect(formatTempestLatest(null, [], [])).toBe('');
    });
});

describe('hidden-dataset URL state', () => {
    it('hides illuminance by default', () => {
        expect(parseHidden(null)).toEqual(new Set(DEFAULT_HIDDEN));
        expect(DEFAULT_HIDDEN).toEqual(['lux']);
    });
    it('round-trips through the query string', () => {
        expect(parseHidden('')).toEqual(new Set());
        expect(parseHidden('lux,gust')).toEqual(new Set(['lux', 'gust']));
        expect(serializeHidden(new Set(['gust', 'lux']))).toBe('gust,lux');
    });
    it('drops the parameter when the state is the default', () => {
        expect(serializeHidden(new Set(['lux']))).toBeNull();
        expect(serializeHidden(new Set())).toBe('');
    });
});

describe('hpaToInHg', () => {
    it('converts', () => expect(hpaToInHg(1013.25)).toBeCloseTo(29.921, 2));
});

describe('tempestPanelIds', () => {
    it('lists one chart id per panel', () => {
        expect(tempestPanelIds()).toEqual(['__tempest__/outdoor', '__tempest__/wind', '__tempest__/direction', '__tempest__/rain', '__tempest__/sun', '__tempest__/lightning', '__tempest__/station']);
    });
});

// Phones show the key values and hide the rest behind an expand control.
describe('formatTempestLatest on phones', () => {
    const latest = {
        device_id: 1, timestamp: new Date(T0).toISOString(), air_temp_c: 20, relative_humidity: 50, dewpoint_c: 9, wind_avg_ms: 2, wind_gust_ms: 5,
        wind_direction_deg: 270, local_day_rain_mm: 2.54, rain_mm: 0, uv_index: 3, solar_radiation_wm2: 400, illuminance_lux: 50000,
        sea_level_pressure_mb: 1013, battery_v: 2.6, report_interval_min: 1,
    };
    it('marks everything except temperature, humidity, wind and rain today as secondary', () => {
        const el = document.createElement('div');
        el.innerHTML = formatTempestLatest(latest, [latest], [], T0);
        const key = [...el.querySelectorAll(':scope > span:not(.secondary)')].map(s => s.textContent);
        expect(key).toHaveLength(4);
        expect(key[0]).toContain('°F');
        expect(key[1]).toContain('humidity');
        expect(key[2]).toContain('wind');
        expect(key[3]).toContain('rain today');
        const secondary = [...el.querySelectorAll('.secondary')].map(s => s.textContent).join(' ');
        for (const s of ['dew point', 'UV', 'W/m²', 'lux', 'inHg', 'battery']) expect(secondary).toContain(s);
    });
});
