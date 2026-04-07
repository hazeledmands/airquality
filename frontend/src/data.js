export const COLORS = ['#38bdf8','#f472b6','#34d399','#fb923c','#a78bfa','#fbbf24','#22d3ee','#e879f9'];
export const REFRESH_MS = 60000;
export const MOBILE_BREAKPOINT = 600;

export function getAspectRatio(width) {
    return width <= MOBILE_BREAKPOINT ? 2.4 : 4;
}

export function cToF(c) {
    return c * 9 / 5 + 32;
}

// Returns a per-point radius array: isolated or edge points (adjacent to a null)
// get a visible dot; interior points stay invisible.
export function computePointRadii(data, radius = 2) {
    return data.map((pt, i) => {
        if (pt.y === null) return 0;
        const prevNull = i > 0 && data[i - 1].y === null;
        const nextNull = i < data.length - 1 && data[i + 1].y === null;
        return (prevNull || nextNull) ? radius : 0;
    });
}

export function insertGaps(points, stepMs) {
    if (!stepMs || points.length < 2) return points;
    const threshold = stepMs * 1.5;
    const result = [];
    for (let i = 0; i < points.length; i++) {
        result.push(points[i]);
        if (i < points.length - 1) {
            const t0 = +new Date(points[i].x);
            const t1 = +new Date(points[i + 1].x);
            if (t1 - t0 > threshold) {
                result.push({ x: Math.round((t0 + t1) / 2), y: null });
            }
        }
    }
    return result;
}

export function makeSeries(sensor, field, colorIdx, transform, stepMs = 0) {
    const color = COLORS[colorIdx % COLORS.length];
    const label = sensor.location || `${sensor.source}/${sensor.sensor_id}`;
    const fn = transform || (v => v);
    const raw = sensor.readings.map(r => ({ x: r.timestamp, y: fn(r[field]) }));
    const data = stepMs > 0 ? insertGaps(raw, stepMs) : raw;
    const pointRadius = stepMs > 0 ? computePointRadii(data) : undefined;
    return { label, data, borderColor: color, backgroundColor: color + '18', fill: false, pointRadius };
}

// Build an AC state series resampled at `stepMs` bucket intervals so it has
// the same array length as the temperature dataset. Chart.js mode:'index'
// aligns datasets by array index, so mismatched lengths produce wrong tooltips.
export function makeACStateSeries(state, colorIdx, { fromMs = 0, toMs = 0, stepMs = 0 } = {}) {
    const color = COLORS[colorIdx % COLORS.length];
    const label = state.room || state.device_id;
    const changes = (state.changes || [])
        .map(change => ({ t: new Date(change.timestamp).getTime(), on: change.on }))
        .sort((a, b) => a.t - b.t);

    let data;
    if (stepMs > 0 && fromMs > 0 && toMs > 0) {
        // Resample: one point per step bucket, carrying the last known state forward.
        data = [];
        let currentY = 0;
        let ci = 0;
        for (let t = fromMs; t <= toMs; t += stepMs) {
            while (ci < changes.length && changes[ci].t <= t) {
                currentY = changes[ci].on ? 1 : 0;
                ci++;
            }
            data.push({ x: t, y: currentY });
        }
    } else {
        data = changes.map(c => ({ x: new Date(c.t).toISOString(), y: c.on ? 1 : 0 }));
    }

    return {
        label,
        data,
        borderColor: color,
        backgroundColor: color + '18',
        fill: false,
        stepped: true,
        tension: 0,
    };
}

export function filterByRange(data, from, to) {
    if (!from && !to) return data;

    const fromMs = from ? new Date(from).getTime() : -Infinity;
    const toMs = to ? new Date(to).getTime() : Infinity;

    return {
        sensors: data.sensors.map(s => ({
            ...s,
            readings: s.readings.filter(r => {
                const t = new Date(r.timestamp).getTime();
                return t >= fromMs && t <= toMs;
            }),
        })).filter(s => s.readings.length > 0),
        ac_states: (data.ac_states || []).map(state => ({
            ...state,
            changes: (state.changes || []).filter(change => {
                const t = new Date(change.timestamp).getTime();
                return t >= fromMs && t <= toMs;
            }),
        })).filter(state => state.changes.length > 0),
        weather: (data.weather || []).filter(r => {
            const t = new Date(r.timestamp).getTime();
            return t >= fromMs && t <= toMs;
        }),
        annotations: (data.annotations || []).filter(a => {
            const t = new Date(a.timestamp).getTime();
            return t >= fromMs && t <= toMs;
        }),
    };
}

export function toLocalInput(d) {
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}


const PARTICLE_FIELDS = ['p03', 'p05', 'p10', 'p25', 'p50', 'p100'];

export function buildRoomDatasets(sensors, acStates = [], stepMs = 0) {
    const roomMap = new Map();
    const makeRoom = (name, colorIdx) => ({
        name,
        color: COLORS[colorIdx % COLORS.length],
        temp: [],
        humidity: [],
        dewpoint: [],
        pm: [],
        acState: [],
        particles: Object.fromEntries(PARTICLE_FIELDS.map(f => [f, []])),
    });

    // sensor_id → room name, so AC states can match by device_id
    const sensorIdToRoom = new Map();
    let minSensorMs = Infinity;
    let maxSensorMs = 0;

    sensors.forEach((s, i) => {
        const name = s.location || s.sensor_id;
        if (!roomMap.has(name)) roomMap.set(name, makeRoom(name, i));
        sensorIdToRoom.set(s.sensor_id, name);
        for (const r of s.readings) {
            const t = new Date(r.timestamp).getTime();
            if (t < minSensorMs) minSensorMs = t;
            if (t > maxSensorMs) maxSensorMs = t;
        }
        const room = roomMap.get(name);
        room.temp.push(makeSeries(s, 'temp_c', i, cToF, stepMs));
        room.humidity.push(makeSeries(s, 'humidity', i, null, stepMs));
        room.dewpoint.push(makeSeries(s, 'dewpoint_c', i, cToF, stepMs));
        if (s.source === 'purpleair' || s.readings.some(r => r.pm25 > 0)) {
            room.pm.push(makeSeries(s, 'pm25', i, null, stepMs));
        }
        if (s.source === 'purpleair') {
            for (const field of PARTICLE_FIELDS) {
                room.particles[field].push(makeSeries(s, field, i, null, stepMs));
            }
        }
    });

    acStates.forEach((state, i) => {
        const name = sensorIdToRoom.get(state.device_id) || state.room || state.device_id;
        if (!roomMap.has(name)) roomMap.set(name, makeRoom(name, i));
        roomMap.get(name).acState.push(makeACStateSeries(state, i, { fromMs: minSensorMs, toMs: maxSensorMs, stepMs }));
    });

    return Array.from(roomMap.values());
}

export function computeStatus(sensors, acStates = []) {
    const total = sensors.reduce((n, s) => n + s.readings.length, 0);
    const changes = acStates.reduce((n, state) => n + (state.changes || []).length, 0);
    if (!changes) return `${sensors.length} sensor(s) · ${total} readings`;
    return `${sensors.length} sensor(s) · ${total} readings · ${changes} AC change(s)`;
}

export function getDataRange(data) {
    let minT = Infinity, maxT = -Infinity;
    for (const s of (data.sensors || [])) {
        for (const r of s.readings) {
            const t = new Date(r.timestamp).getTime();
            if (t < minT) minT = t;
            if (t > maxT) maxT = t;
        }
    }
    for (const state of (data.ac_states || [])) {
        for (const change of (state.changes || [])) {
            const t = new Date(change.timestamp).getTime();
            if (t < minT) minT = t;
            if (t > maxT) maxT = t;
        }
    }
    for (const r of (data.weather || [])) {
        const t = new Date(r.timestamp).getTime();
        if (t < minT) minT = t;
        if (t > maxT) maxT = t;
    }
    return minT !== Infinity ? { min: new Date(minT), max: new Date(maxT) } : null;
}

export function makeWeatherTempSeries(weather, opts = {}) {
    const data = weather.map(r => ({ x: r.timestamp, y: cToF(r.temp_c) }));
    return {
        label: 'Outdoor Temp',
        data,
        borderColor: opts.borderColor || '#94a3b8',
        backgroundColor: (opts.borderColor || '#94a3b8') + '18',
        borderDash: opts.borderDash || [6, 4],
        borderWidth: opts.borderWidth || 1.5,
        fill: false,
        ...opts,
    };
}

export function makeUVSeries(weather) {
    const data = weather.map(r => ({ x: r.timestamp, y: r.uv_index }));
    return {
        label: 'UV Index',
        data,
        borderColor: '#fbbf24',
        fill: false,
        borderWidth: 1.5,
    };
}

// Returns a new data object with `annotation` appended to the annotations list.
export function applyNewAnnotation(data, annotation) {
    return { ...data, annotations: [...(data.annotations || []), annotation] };
}

// Returns a new data object with the annotation matching `id` removed.
export function removeAnnotation(data, id) {
    return { ...data, annotations: (data.annotations || []).filter(a => a.id !== id) };
}

export function extractDaylightPairs(weather) {
    const seen = new Set();
    const pairs = [];
    for (const r of weather) {
        if (!r.sunrise || !r.sunset) continue;
        const dayKey = r.sunrise.slice(0, 10);
        if (seen.has(dayKey)) continue;
        seen.add(dayKey);
        pairs.push({ sunrise: new Date(r.sunrise), sunset: new Date(r.sunset) });
    }
    return pairs;
}
