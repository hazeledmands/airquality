export const COLORS = ['#38bdf8','#f472b6','#34d399','#fb923c','#a78bfa','#fbbf24','#22d3ee','#e879f9'];
export const REFRESH_MS = 60000;
export const MOBILE_BREAKPOINT = 600;

export function getAspectRatio(width) {
    return width <= MOBILE_BREAKPOINT ? 5 : 8;
}

export function cToF(c) {
    return c * 9 / 5 + 32;
}

export function makeSeries(sensor, field, colorIdx, transform) {
    const color = COLORS[colorIdx % COLORS.length];
    const label = sensor.location || `${sensor.source}/${sensor.sensor_id}`;
    const fn = transform || (v => v);
    const data = sensor.readings.map(r => ({ x: r.timestamp, y: fn(r[field]) }));
    return { label, data, borderColor: color, backgroundColor: color + '18', fill: false };
}

export function makeACStateSeries(state, colorIdx) {
    const color = COLORS[colorIdx % COLORS.length];
    const label = state.room || state.device_id;
    const data = (state.changes || [])
        .map(change => ({ x: change.timestamp, y: change.on ? 1 : 0 }))
        .sort((a, b) => new Date(a.x).getTime() - new Date(b.x).getTime());
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
    };
}

export function toLocalInput(d) {
    const pad = (n) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function buildDatasets(sensors, acStates = []) {
    const tempDS = [], humDS = [], dewDS = [], pmDS = [], acStateDS = [];
    sensors.forEach((s, i) => {
        tempDS.push(makeSeries(s, 'temp_c', i, cToF));
        humDS.push(makeSeries(s, 'humidity', i));
        dewDS.push(makeSeries(s, 'dewpoint_c', i, cToF));
        if (s.source === 'purpleair' || s.readings.some(r => r.pm25 > 0)) {
            pmDS.push(makeSeries(s, 'pm25', i));
        }
    });
    acStates.forEach((state, i) => {
        acStateDS.push(makeACStateSeries(state, i));
    });
    return { temp: tempDS, humidity: humDS, dewpoint: dewDS, pm: pmDS, acState: acStateDS };
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
    return minT !== Infinity ? { min: new Date(minT), max: new Date(maxT) } : null;
}
