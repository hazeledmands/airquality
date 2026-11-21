// @vitest-environment jsdom
import { describe, it, expect } from 'vitest';
import { outdoorPMSeries, aqiCategory, airDatasets, formatAirLatest, forecastDays, formatForecastStrip, cloudSeries } from '../outdoor-air.js';

const at = (iso) => new Date(iso).toISOString();
const row = (iso, f = {}) => ({ timestamp: at(iso), pm2_5: null, pm10: null, ozone: null, nitrogen_dioxide: null, us_aqi: null, grass_pollen: null, birch_pollen: null, ragweed_pollen: null, ...f });

describe('outdoorPMSeries', () => {
    it('plots outdoor PM2.5 for the indoor PM charts, nulls as gaps', () => {
        const s = outdoorPMSeries([row('2026-11-21T10:00:00Z', { pm2_5: 12 }), row('2026-11-21T11:00:00Z')]);
        expect(s.label).toBe('Outdoor PM2.5');
        expect(s.yAxisID).toBe('y2');
        expect(s.data.map(p => p.y)).toEqual([12, null]);
        expect(s.borderDash).toBeTruthy();
    });
});

describe('aqiCategory', () => {
    it('uses the US AQI bands', () => {
        expect(aqiCategory(30)).toBe('good');
        expect(aqiCategory(86)).toBe('moderate');
        expect(aqiCategory(120)).toBe('unhealthy for sensitive groups');
        expect(aqiCategory(180)).toBe('unhealthy');
        expect(aqiCategory(250)).toBe('very unhealthy');
        expect(aqiCategory(400)).toBe('hazardous');
        expect(aqiCategory(null)).toBe('');
    });
});

describe('airDatasets', () => {
    const rows = [row('2026-11-21T10:00:00Z', { us_aqi: 86, pm2_5: 24, ozone: 13, nitrogen_dioxide: 67 })];
    it('puts the AQI on its own axis and gases in µg/m³ on the other', () => {
        const ds = airDatasets(rows);
        // Labelled as the rolling-average index it is, not an hourly reading.
        expect(ds.find(d => d.label === 'US AQI (rolling avg)').yAxisID).toBe('yAQI');
        for (const l of ['PM2.5', 'Ozone', 'NO₂']) expect(ds.find(d => d.label === l).yAxisID).toBe('yGas');
    });
    it('adds pollen only where the model covers it (Europe)', () => {
        expect(airDatasets(rows).some(d => /pollen/i.test(d.label))).toBe(false);
        const eu = [row('2026-05-01T10:00:00Z', { grass_pollen: 40 })];
        expect(airDatasets(eu).some(d => d.label === 'Grass pollen')).toBe(true);
    });
});

describe('formatAirLatest', () => {
    it('summarises the newest sample', () => {
        const html = formatAirLatest(row('2026-11-21T10:00:00Z', { us_aqi: 86, pm2_5: 24, ozone: 13, nitrogen_dioxide: 67.1 }));
        expect(html).toContain('AQI');
        expect(html).toContain('86');
        expect(html).toContain('moderate');
        expect(html).toContain('24');
        expect(html).toContain('67');
    });
    it('is empty without data', () => expect(formatAirLatest(null)).toBe(''));
});

describe('forecast strip', () => {
    // The API sends four local days from local midnight; "now" is midday.
    const now = new Date(2026, 10, 21, 12, 0).getTime();
    const hours = [];
    for (let h = 0; h < 4 * 24 - 5; h++) { // the fourth day is cut short
        const t = new Date(2026, 10, 21, h);
        // 06:00 is the coldest hour of each day; 15:00 the warmest.
        const temp = (h % 24) === 6 ? 2 : (h % 24) === 15 ? 20 : 10;
        hours.push({ timestamp: t.toISOString(), temp_c: temp, precip_probability: h < 18 ? 40 : 5, cloud_cover: 50 });
    }
    it('lists only complete local days, today included', () => {
        const days = forecastDays(hours, now);
        expect(days.map(d => d.label)).toEqual(['Today', 'Sun', 'Mon']);
    });
    it("uses the whole of today, including hours already past, for today's high and low", () => {
        const today = forecastDays(hours, now)[0];
        expect(today.loF).toBeCloseTo(35.6); // 2 °C at 06:00, before "now"
        expect(today.hiF).toBeCloseTo(68);
        expect(today.rainPct).toBe(40);
    });
    it('formats a one-line strip with the next 6 hours', () => {
        const html = formatForecastStrip(hours, now);
        expect(html).toContain('Today');
        expect(html).toContain('next 6 h');
        expect(html).toContain('40%');
        expect(formatForecastStrip([], now)).toBe('');
    });
});

describe('cloudSeries', () => {
    it('is a faint filled series on a hidden 0–100 axis', () => {
        const s = cloudSeries([{ timestamp: at('2026-11-21T10:00:00Z'), cloud_cover: 80 }, { timestamp: at('2026-11-21T10:15:00Z'), cloud_cover: null }]);
        expect(s.yAxisID).toBe('yCloud');
        expect(s.fill).toBeTruthy();
        expect(s.data.map(p => p.y)).toEqual([80, null]);
    });
});
