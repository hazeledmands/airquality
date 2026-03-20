import { REFRESH_MS, cToF, filterByRange, toLocalInput, buildRoomDatasets, computeStatus, getDataRange, getAspectRatio } from './data.js';
import { chartOpts } from './charts.js';

let allData = null;
let roomCharts = []; // { room, charts: { temp, humidity, dewpoint, pm?, acState? } }

function makeAcStateOpts(ar) {
    const opts = chartOpts(ar);
    opts.scales.y.min = -0.05;
    opts.scales.y.max = 1.05;
    opts.scales.y.ticks.stepSize = 1;
    opts.scales.y.ticks.callback = (value) => value === 1 ? 'On' : value === 0 ? 'Off' : '';
    return opts;
}

function buildRoomDOM(rooms) {
    const container = document.getElementById('rooms-container');
    container.innerHTML = '';
    roomCharts = [];
    const ar = getAspectRatio(window.innerWidth);

    for (const room of rooms) {
        const section = document.createElement('div');
        section.className = 'room-section';

        const heading = document.createElement('h2');
        heading.style.color = room.color;
        heading.textContent = room.name;
        section.appendChild(heading);

        const latest = document.createElement('div');
        latest.className = 'room-latest';
        latest.dataset.room = room.name;
        section.appendChild(latest);

        const grid = document.createElement('div');
        grid.className = 'room-charts';

        const charts = {};

        // Temperature
        const tempCard = makeChartCard('Temperature (\u00B0F)');
        grid.appendChild(tempCard.card);
        charts.temp = new Chart(tempCard.canvas, { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });

        // Humidity
        const humCard = makeChartCard('Humidity (%)');
        grid.appendChild(humCard.card);
        charts.humidity = new Chart(humCard.canvas, { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });

        // Dew Point
        const dewCard = makeChartCard('Dew Point (\u00B0F)');
        grid.appendChild(dewCard.card);
        charts.dewpoint = new Chart(dewCard.canvas, { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });

        // PM2.5 (only if data exists)
        if (room.pm.length) {
            const pmCard = makeChartCard('PM2.5 (\u00B5g/m\u00B3)');
            grid.appendChild(pmCard.card);
            charts.pm = new Chart(pmCard.canvas, { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });
        }

        // AC State (only if data exists)
        if (room.acState.length) {
            const acCard = makeChartCard('AC State');
            grid.appendChild(acCard.card);
            charts.acState = new Chart(acCard.canvas, { type: 'line', data: { datasets: [] }, options: makeAcStateOpts(ar) });
        }

        section.appendChild(grid);
        container.appendChild(section);
        roomCharts.push({ room: room.name, charts });
    }
}

function makeChartCard(title) {
    const card = document.createElement('div');
    card.className = 'chart-card';
    const h3 = document.createElement('h3');
    h3.textContent = title;
    card.appendChild(h3);
    const canvas = document.createElement('canvas');
    card.appendChild(canvas);
    return { card, canvas };
}

function updateLatestValues(rooms) {
    for (const room of rooms) {
        const el = document.querySelector(`.room-latest[data-room="${room.name}"]`);
        if (!el) continue;
        // Get latest reading from the first temp dataset
        const tempData = room.temp[0]?.data;
        const humData = room.humidity[0]?.data;
        const dewData = room.dewpoint[0]?.data;
        const pmData = room.pm[0]?.data;
        if (!tempData?.length) { el.innerHTML = ''; continue; }

        const lastTemp = tempData[tempData.length - 1].y;
        const parts = [`<span><span class="value">${lastTemp.toFixed(1)}\u00B0F</span></span>`];
        if (humData?.length) {
            parts.push(`<span><span class="value">${humData[humData.length - 1].y.toFixed(0)}%</span> humidity</span>`);
        }
        if (dewData?.length) {
            parts.push(`<span><span class="value">${dewData[dewData.length - 1].y.toFixed(1)}\u00B0F</span> dew point</span>`);
        }
        if (pmData?.length) {
            parts.push(`<span>PM2.5 <span class="value">${pmData[pmData.length - 1].y.toFixed(1)}</span></span>`);
        }
        el.innerHTML = parts.join('');
    }
}

function updateCharts(data) {
    const sensors = data.sensors || [];
    const acStates = data.ac_states || [];

    if (!sensors.length && !acStates.length) {
        document.getElementById('status').textContent = 'No data yet.';
        const container = document.getElementById('rooms-container');
        container.innerHTML = '<div class="no-data">Waiting for first sensor reading&hellip;</div>';
        roomCharts = [];
        return;
    }

    const rooms = buildRoomDatasets(sensors, acStates);

    // Rebuild DOM if room set changed
    const currentRooms = roomCharts.map(rc => rc.room);
    const newRooms = rooms.map(r => r.name);
    if (currentRooms.length !== newRooms.length || currentRooms.some((r, i) => r !== newRooms[i])) {
        // Destroy old charts
        for (const rc of roomCharts) {
            for (const chart of Object.values(rc.charts)) chart.destroy();
        }
        buildRoomDOM(rooms);
    }

    // Update chart data
    for (const room of rooms) {
        const rc = roomCharts.find(r => r.room === room.name);
        if (!rc) continue;
        rc.charts.temp.data.datasets = room.temp;
        rc.charts.temp.update();
        rc.charts.humidity.data.datasets = room.humidity;
        rc.charts.humidity.update();
        rc.charts.dewpoint.data.datasets = room.dewpoint;
        rc.charts.dewpoint.update();
        if (rc.charts.pm) {
            rc.charts.pm.data.datasets = room.pm;
            rc.charts.pm.update();
        }
        if (rc.charts.acState) {
            rc.charts.acState.data.datasets = room.acState;
            rc.charts.acState.update();
        }
    }

    updateLatestValues(rooms);
    document.getElementById('status').textContent =
        computeStatus(sensors, acStates) + ` \u00B7 updated ${new Date().toLocaleTimeString()}`;
}

window.addEventListener('resize', () => {
    const ar = getAspectRatio(window.innerWidth);
    for (const rc of roomCharts) {
        for (const chart of Object.values(rc.charts)) {
            chart.options.aspectRatio = ar;
            chart.update();
        }
    }
});

function applyFilter() {
    if (allData) {
        const from = document.getElementById('rangeFrom').value;
        const to = document.getElementById('rangeTo').value;
        updateCharts(filterByRange(allData, from, to));
    }
}

function setRangeDefaults(data) {
    const fromEl = document.getElementById('rangeFrom');
    const toEl = document.getElementById('rangeTo');
    if (fromEl.value || toEl.value) return;
    const range = getDataRange(data);
    if (range) {
        fromEl.value = toLocalInput(range.min);
        toEl.value = toLocalInput(range.max);
    }
}

async function fetchData() {
    try {
        const resp = await fetch('/api/data');
        allData = await resp.json();
        setRangeDefaults(allData);
        const from = document.getElementById('rangeFrom').value;
        const to = document.getElementById('rangeTo').value;
        updateCharts(filterByRange(allData, from, to));
    } catch (err) {
        document.getElementById('status').textContent = 'Error: ' + err.message;
    }
}

document.getElementById('rangeFrom').addEventListener('change', applyFilter);
document.getElementById('rangeTo').addEventListener('change', applyFilter);
document.getElementById('rangeReset').addEventListener('click', () => {
    document.getElementById('rangeFrom').value = '';
    document.getElementById('rangeTo').value = '';
    if (allData) {
        setRangeDefaults(allData);
        applyFilter();
    }
});

fetchData();
setInterval(fetchData, REFRESH_MS);
