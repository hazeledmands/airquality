import { describe, it, expect } from 'vitest';
import {
    COLORS,
    REFRESH_MS,
    MOBILE_BREAKPOINT,
    cToF,
    makeSeries,
    insertGaps,
    computePointRadii,
    filterByRange,
    toLocalInput,
    buildRoomDatasets,
    computeStatus,
    getDataRange,
    getAspectRatio,
    applyNewAnnotation,
    removeAnnotation,
} from '../data.js';

describe('computePointRadii', () => {
    const pt = (y) => ({ x: 0, y });

    it('returns 0 for all points in a contiguous series', () => {
        const data = [pt(1), pt(2), pt(3)];
        expect(computePointRadii(data)).toEqual([0, 0, 0]);
    });

    it('returns radius for an isolated point (null on both sides)', () => {
        const data = [pt(null), pt(5), pt(null)];
        const radii = computePointRadii(data);
        expect(radii[1]).toBeGreaterThan(0);
    });

    it('returns radius for a point at the start of a gap', () => {
        const data = [pt(1), pt(2), pt(null), pt(3)];
        const radii = computePointRadii(data);
        expect(radii[1]).toBeGreaterThan(0); // edge before gap
        expect(radii[0]).toBe(0);            // interior
    });

    it('returns radius for a point at the end of a gap', () => {
        const data = [pt(1), pt(null), pt(2), pt(3)];
        const radii = computePointRadii(data);
        expect(radii[2]).toBeGreaterThan(0); // edge after gap
        expect(radii[3]).toBe(0);            // interior
    });

    it('returns 0 for null points themselves', () => {
        const data = [pt(1), pt(null), pt(2)];
        expect(computePointRadii(data)[1]).toBe(0);
    });

    it('uses the provided radius value', () => {
        const data = [pt(null), pt(5), pt(null)];
        expect(computePointRadii(data, 4)[1]).toBe(4);
    });
});

describe('insertGaps', () => {
    const STEP = 60_000; // 1 minute
    const T1 = 1_000_000;
    const T2 = T1 + STEP;         // contiguous
    const T3 = T2 + STEP;         // contiguous
    const T4 = T3 + 5 * STEP;     // big gap after T3

    const pts = (ts) => ts.map(t => ({ x: new Date(t).toISOString(), y: 10 }));

    it('returns same reference when stepMs is 0', () => {
        const data = pts([T1, T2]);
        expect(insertGaps(data, 0)).toBe(data);
    });

    it('returns same reference for fewer than 2 points', () => {
        const data = pts([T1]);
        expect(insertGaps(data, STEP)).toBe(data);
    });

    it('does not insert a gap when points are contiguous', () => {
        const result = insertGaps(pts([T1, T2, T3]), STEP);
        expect(result).toHaveLength(3);
    });

    it('inserts a null point when gap exceeds 1.5× stepMs', () => {
        const result = insertGaps(pts([T1, T4]), STEP);
        expect(result).toHaveLength(3);
        expect(result[1].y).toBeNull();
    });

    it('null point x falls between the two surrounding timestamps', () => {
        const result = insertGaps(pts([T1, T4]), STEP);
        const nullX = result[1].x;
        expect(nullX).toBeGreaterThan(T1);
        expect(nullX).toBeLessThan(T4);
    });

    it('preserves original points on either side of the null', () => {
        const data = pts([T1, T4]);
        const result = insertGaps(data, STEP);
        expect(result[0]).toEqual(data[0]);
        expect(result[2]).toEqual(data[1]);
    });

    it('inserts a null for each gap in a multi-gap series', () => {
        // T1→T2 contiguous, T2→T4 big gap, T4→T4+STEP contiguous, T4+STEP→T4+6*STEP big gap
        const T5 = T4 + STEP;
        const T6 = T4 + 6 * STEP;
        const result = insertGaps(pts([T1, T2, T4, T5, T6]), STEP);
        const nullCount = result.filter(p => p.y === null).length;
        expect(nullCount).toBe(2);
    });
});

describe('cToF', () => {
    it('converts 0°C to 32°F', () => {
        expect(cToF(0)).toBe(32);
    });

    it('converts 100°C to 212°F', () => {
        expect(cToF(100)).toBe(212);
    });

    it('converts -40°C to -40°F', () => {
        expect(cToF(-40)).toBe(-40);
    });

    it('converts 22°C to 71.6°F', () => {
        expect(cToF(22)).toBeCloseTo(71.6);
    });
});

describe('makeSeries', () => {
    const sensor = {
        source: 'sensibo',
        sensor_id: 'abc123',
        location: 'Office',
        readings: [
            { timestamp: '2026-03-17T10:00:00Z', temp_c: 22, humidity: 45 },
            { timestamp: '2026-03-17T11:00:00Z', temp_c: 23, humidity: 50 },
        ],
    };

    it('uses location as label', () => {
        const series = makeSeries(sensor, 'temp_c', 0);
        expect(series.label).toBe('Office');
    });

    it('falls back to source/sensor_id when no location', () => {
        const noLoc = { ...sensor, location: '' };
        const series = makeSeries(noLoc, 'temp_c', 0);
        expect(series.label).toBe('sensibo/abc123');
    });

    it('maps readings to {x, y} data points', () => {
        const series = makeSeries(sensor, 'temp_c', 0);
        expect(series.data).toEqual([
            { x: '2026-03-17T10:00:00Z', y: 22 },
            { x: '2026-03-17T11:00:00Z', y: 23 },
        ]);
    });

    it('applies transform function to values', () => {
        const series = makeSeries(sensor, 'temp_c', 0, cToF);
        expect(series.data[0].y).toBeCloseTo(71.6);
        expect(series.data[1].y).toBeCloseTo(73.4);
    });

    it('assigns color from COLORS array by index', () => {
        const s0 = makeSeries(sensor, 'temp_c', 0);
        const s1 = makeSeries(sensor, 'temp_c', 1);
        expect(s0.borderColor).toBe(COLORS[0]);
        expect(s1.borderColor).toBe(COLORS[1]);
    });

    it('wraps color index around COLORS length', () => {
        const s = makeSeries(sensor, 'temp_c', COLORS.length);
        expect(s.borderColor).toBe(COLORS[0]);
    });

    it('sets backgroundColor with 18 alpha suffix', () => {
        const s = makeSeries(sensor, 'temp_c', 0);
        expect(s.backgroundColor).toBe(COLORS[0] + '18');
    });

    it('sets fill to false', () => {
        const s = makeSeries(sensor, 'temp_c', 0);
        expect(s.fill).toBe(false);
    });
});

describe('filterByRange', () => {
    const data = {
        sensors: [
            {
                source: 'sensibo',
                sensor_id: 'a',
                location: 'Office',
                readings: [
                    { timestamp: '2026-03-17T08:00:00Z', temp_c: 20 },
                    { timestamp: '2026-03-17T10:00:00Z', temp_c: 22 },
                    { timestamp: '2026-03-17T12:00:00Z', temp_c: 24 },
                ],
            },
            {
                source: 'sensibo',
                sensor_id: 'b',
                location: 'Bedroom',
                readings: [
                    { timestamp: '2026-03-17T09:00:00Z', temp_c: 19 },
                ],
            },
        ],
        ac_states: [
            {
                device_id: 'a',
                room: 'Office',
                changes: [
                    { timestamp: '2026-03-17T08:30:00Z', on: true, mode: 'heat' },
                    { timestamp: '2026-03-17T11:30:00Z', on: false, mode: 'heat' },
                ],
            },
        ],
    };

    it('returns original data when no range specified', () => {
        expect(filterByRange(data, '', '')).toBe(data);
    });

    it('returns original data when both are falsy', () => {
        expect(filterByRange(data, null, null)).toBe(data);
    });

    it('filters by from only', () => {
        const result = filterByRange(data, '2026-03-17T09:30:00Z', '');
        expect(result.sensors).toHaveLength(1);
        expect(result.sensors[0].location).toBe('Office');
        expect(result.sensors[0].readings).toHaveLength(2);
    });

    it('filters by to only', () => {
        const result = filterByRange(data, '', '2026-03-17T09:30:00Z');
        expect(result.sensors).toHaveLength(2);
        expect(result.sensors[0].readings).toHaveLength(1);
        expect(result.sensors[0].readings[0].temp_c).toBe(20);
    });

    it('filters by both from and to', () => {
        const result = filterByRange(data, '2026-03-17T09:00:00Z', '2026-03-17T11:00:00Z');
        expect(result.sensors).toHaveLength(2);
        expect(result.sensors[0].readings).toHaveLength(1);
        expect(result.sensors[0].readings[0].temp_c).toBe(22);
        expect(result.sensors[1].readings).toHaveLength(1);
    });

    it('removes sensors with no readings in range', () => {
        const result = filterByRange(data, '2026-03-17T14:00:00Z', '');
        expect(result.sensors).toHaveLength(0);
    });

    it('preserves sensor metadata', () => {
        const result = filterByRange(data, '2026-03-17T09:00:00Z', '2026-03-17T11:00:00Z');
        expect(result.sensors[0].source).toBe('sensibo');
        expect(result.sensors[0].sensor_id).toBe('a');
        expect(result.sensors[0].location).toBe('Office');
    });

    it('filters AC state changes by range', () => {
        const result = filterByRange(data, '2026-03-17T09:00:00Z', '2026-03-17T12:00:00Z');
        expect(result.ac_states).toHaveLength(1);
        expect(result.ac_states[0].changes).toHaveLength(1);
        expect(result.ac_states[0].changes[0].on).toBe(false);
    });
});

describe('toLocalInput', () => {
    it('formats a date for datetime-local input', () => {
        const d = new Date(2026, 2, 17, 14, 5); // March 17, 2026 14:05
        expect(toLocalInput(d)).toBe('2026-03-17T14:05');
    });

    it('pads single-digit month and day', () => {
        const d = new Date(2026, 0, 5, 9, 3); // Jan 5, 2026 09:03
        expect(toLocalInput(d)).toBe('2026-01-05T09:03');
    });

    it('handles midnight', () => {
        const d = new Date(2026, 11, 31, 0, 0); // Dec 31, 2026 00:00
        expect(toLocalInput(d)).toBe('2026-12-31T00:00');
    });
});


describe('computeStatus', () => {
    it('counts sensors and total readings', () => {
        const sensors = [
            { readings: [1, 2, 3] },
            { readings: [4, 5] },
        ];
        expect(computeStatus(sensors)).toBe('2 sensor(s) · 5 readings');
    });

    it('handles empty sensors array', () => {
        expect(computeStatus([])).toBe('0 sensor(s) · 0 readings');
    });

    it('handles single sensor', () => {
        const sensors = [{ readings: [1] }];
        expect(computeStatus(sensors)).toBe('1 sensor(s) · 1 readings');
    });
});

describe('getDataRange', () => {
    it('returns min and max dates from sensor data', () => {
        const data = {
            sensors: [
                {
                    readings: [
                        { timestamp: '2026-03-17T10:00:00Z' },
                        { timestamp: '2026-03-17T14:00:00Z' },
                    ],
                },
                {
                    readings: [
                        { timestamp: '2026-03-17T08:00:00Z' },
                    ],
                },
            ],
        };
        const range = getDataRange(data);
        expect(range.min.toISOString()).toBe('2026-03-17T08:00:00.000Z');
        expect(range.max.toISOString()).toBe('2026-03-17T14:00:00.000Z');
    });

    it('returns null for empty data', () => {
        expect(getDataRange({ sensors: [] })).toBeNull();
    });

    it('returns null when sensors is undefined', () => {
        expect(getDataRange({})).toBeNull();
    });

    it('returns same date for single reading', () => {
        const data = {
            sensors: [{ readings: [{ timestamp: '2026-03-17T12:00:00Z' }] }],
        };
        const range = getDataRange(data);
        expect(range.min.getTime()).toBe(range.max.getTime());
    });
});

describe('getAspectRatio', () => {
    it('returns wider ratio for desktop widths', () => {
        expect(getAspectRatio(1280)).toBe(4);
    });

    it('returns taller ratio for mobile widths', () => {
        expect(getAspectRatio(375)).toBe(2.4);
    });

    it('uses mobile ratio at exactly the breakpoint', () => {
        expect(getAspectRatio(MOBILE_BREAKPOINT)).toBe(2.4);
    });

    it('uses desktop ratio above the breakpoint', () => {
        expect(getAspectRatio(MOBILE_BREAKPOINT + 1)).toBe(4);
    });
});

describe('constants', () => {
    it('has 8 colors', () => {
        expect(COLORS).toHaveLength(8);
    });

    it('REFRESH_MS is 60 seconds', () => {
        expect(REFRESH_MS).toBe(60000);
    });
});

describe('buildRoomDatasets', () => {
    const sensors = [
        {
            source: 'sensibo',
            sensor_id: 'a',
            location: 'Office',
            readings: [
                { timestamp: '2026-03-17T10:00:00Z', temp_c: 22, humidity: 45, dewpoint_c: 10, pm25: 0 },
            ],
        },
        {
            source: 'sensibo',
            sensor_id: 'b',
            location: 'Bedroom',
            readings: [
                { timestamp: '2026-03-17T10:00:00Z', temp_c: 20, humidity: 55, dewpoint_c: 11, pm25: 0 },
            ],
        },
        {
            source: 'purpleair',
            sensor_id: 'c',
            location: 'Outside',
            readings: [
                { timestamp: '2026-03-17T10:00:00Z', temp_c: 18, humidity: 60, dewpoint_c: 8, pm25: 12.5 },
            ],
        },
    ];
    const acStates = [
        {
            device_id: 'a',
            room: 'Office',
            changes: [
                { timestamp: '2026-03-17T10:00:00Z', on: true },
                { timestamp: '2026-03-17T11:00:00Z', on: false },
            ],
        },
    ];

    it('returns an array with one entry per room', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        expect(rooms).toHaveLength(3);
    });

    it('each entry has name, color, and dataset arrays', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        const office = rooms.find(r => r.name === 'Office');
        expect(office).toBeDefined();
        expect(office.color).toBe(COLORS[0]);
        expect(office.temp).toHaveLength(1);
        expect(office.humidity).toHaveLength(1);
        expect(office.dewpoint).toHaveLength(1);
    });

    it('includes PM datasets only for sensors with PM data', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        const office = rooms.find(r => r.name === 'Office');
        const outside = rooms.find(r => r.name === 'Outside');
        expect(office.pm).toHaveLength(0);
        expect(outside.pm).toHaveLength(1);
    });

    it('includes particle bin series only for purpleair sensors', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        const office = rooms.find(r => r.name === 'Office');
        const outside = rooms.find(r => r.name === 'Outside');
        expect(office.particles.p03).toHaveLength(0);
        expect(outside.particles.p03).toHaveLength(1);
    });

    it('populates all 6 particle bin arrays for purpleair sensors', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        const outside = rooms.find(r => r.name === 'Outside');
        for (const field of ['p03', 'p05', 'p10', 'p25', 'p50', 'p100']) {
            expect(outside.particles[field]).toHaveLength(1);
        }
    });

    it('particle series data maps to correct field values', () => {
        const purpleairSensor = {
            source: 'purpleair',
            sensor_id: 'x',
            location: 'Yard',
            readings: [
                { timestamp: '2026-03-17T10:00:00Z', p03: 500, p05: 200, p10: 80, p25: 30, p50: 5, p100: 1 },
            ],
        };
        const rooms = buildRoomDatasets([purpleairSensor]);
        const yard = rooms.find(r => r.name === 'Yard');
        expect(yard.particles.p03[0].data[0].y).toBe(500);
        expect(yard.particles.p100[0].data[0].y).toBe(1);
    });

    it('attaches AC state to the matching room', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        const office = rooms.find(r => r.name === 'Office');
        const bedroom = rooms.find(r => r.name === 'Bedroom');
        expect(office.acState).toHaveLength(1);
        expect(office.acState[0].stepped).toBe(true);
        expect(bedroom.acState).toHaveLength(0);
    });

    it('matches AC state by device_id even when room name differs from sensor location', () => {
        const renamedAC = [{ device_id: 'a', room: 'OldOfficeName', changes: [{ timestamp: '2026-03-17T10:00:00Z', on: true }] }];
        const rooms = buildRoomDatasets(sensors, renamedAC);
        const office = rooms.find(r => r.name === 'Office');
        expect(office.acState).toHaveLength(1);
        // No orphan room created for 'OldOfficeName'
        expect(rooms.find(r => r.name === 'OldOfficeName')).toBeUndefined();
    });

    it('preserves sensor order', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        expect(rooms[0].name).toBe('Office');
        expect(rooms[1].name).toBe('Bedroom');
        expect(rooms[2].name).toBe('Outside');
    });

    it('converts temperature to Fahrenheit', () => {
        const rooms = buildRoomDatasets(sensors, acStates);
        const office = rooms.find(r => r.name === 'Office');
        expect(office.temp[0].data[0].y).toBeCloseTo(cToF(22));
    });
});

const A1 = { id: 1, timestamp: '2026-03-01T12:00:00Z', note: 'opened window' };
const A2 = { id: 2, timestamp: '2026-03-01T13:00:00Z', note: 'closed window' };

describe('applyNewAnnotation', () => {
    it('appends a new annotation to existing list', () => {
        const data = { annotations: [A1] };
        const result = applyNewAnnotation(data, A2);
        expect(result.annotations).toEqual([A1, A2]);
    });

    it('works when annotations array is empty', () => {
        const data = { annotations: [] };
        const result = applyNewAnnotation(data, A1);
        expect(result.annotations).toEqual([A1]);
    });

    it('works when annotations key is absent', () => {
        const result = applyNewAnnotation({}, A1);
        expect(result.annotations).toEqual([A1]);
    });

    it('does not mutate the original data object', () => {
        const data = { annotations: [A1] };
        applyNewAnnotation(data, A2);
        expect(data.annotations).toEqual([A1]);
    });

    it('preserves other data keys', () => {
        const data = { sensors: ['s'], annotations: [] };
        const result = applyNewAnnotation(data, A1);
        expect(result.sensors).toEqual(['s']);
    });
});

describe('removeAnnotation', () => {
    it('removes the annotation with the given id', () => {
        const data = { annotations: [A1, A2] };
        const result = removeAnnotation(data, A1.id);
        expect(result.annotations).toEqual([A2]);
    });

    it('returns empty array when last annotation is removed', () => {
        const data = { annotations: [A1] };
        const result = removeAnnotation(data, A1.id);
        expect(result.annotations).toEqual([]);
    });

    it('leaves list unchanged when id is not found', () => {
        const data = { annotations: [A1, A2] };
        const result = removeAnnotation(data, 999);
        expect(result.annotations).toEqual([A1, A2]);
    });

    it('works when annotations key is absent', () => {
        const result = removeAnnotation({}, 1);
        expect(result.annotations).toEqual([]);
    });

    it('does not mutate the original data object', () => {
        const data = { annotations: [A1, A2] };
        removeAnnotation(data, A1.id);
        expect(data.annotations).toEqual([A1, A2]);
    });

    it('preserves other data keys', () => {
        const data = { sensors: ['s'], annotations: [A1] };
        const result = removeAnnotation(data, A1.id);
        expect(result.sensors).toEqual(['s']);
    });
});
