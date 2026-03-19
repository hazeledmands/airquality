import { COLORS, REFRESH_MS, cToF, filterByRange, toLocalInput, buildDatasets, computeStatus, getDataRange, getAspectRatio } from './data.js';
import { chartOpts } from './charts.js';

const charts = {};
let allData = null;

function initCharts() {
    const ar = getAspectRatio(window.innerWidth);
    charts.temp = new Chart(document.getElementById('tempChart'), { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });
    charts.humidity = new Chart(document.getElementById('humidityChart'), { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });
    charts.dewpoint = new Chart(document.getElementById('dewpointChart'), { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });
    charts.pm = new Chart(document.getElementById('pmChart'), { type: 'line', data: { datasets: [] }, options: chartOpts(ar) });
    const acStateOpts = chartOpts(ar);
    acStateOpts.scales.y.min = -0.05;
    acStateOpts.scales.y.max = 1.05;
    acStateOpts.scales.y.ticks.stepSize = 1;
    acStateOpts.scales.y.ticks.callback = (value) => value === 1 ? 'On' : value === 0 ? 'Off' : '';
    charts.acState = new Chart(document.getElementById('acStateChart'), { type: 'line', data: { datasets: [] }, options: acStateOpts });
}

window.addEventListener('resize', () => {
    const ar = getAspectRatio(window.innerWidth);
    for (const chart of Object.values(charts)) {
        chart.options.aspectRatio = ar;
        chart.update();
    }
});

function updateLatest(sensors) {
    const el = document.getElementById('latest');
    el.innerHTML = '';
    sensors.forEach((s, i) => {
        const last = s.readings[s.readings.length - 1];
        if (!last) return;
        const color = COLORS[i % COLORS.length];
        const name = s.location || s.sensor_id;
        let details = `${last.humidity.toFixed(0)}% humidity · ${cToF(last.dewpoint_c).toFixed(1)}°F dew`;
        if (last.pm25) details += ` · PM2.5 ${last.pm25.toFixed(1)}`;
        const card = document.createElement('div');
        card.className = 'sensor-card';
        card.innerHTML = `
            <div class="name" style="color:${color}">${name}</div>
            <div class="temp">${cToF(last.temp_c).toFixed(1)}&deg;</div>
            <div class="details">${details}</div>
        `;
        el.appendChild(card);
    });
}

function updateCharts(data) {
    const sensors = data.sensors || [];
    const acStates = data.ac_states || [];
    if (!sensors.length && !acStates.length) {
        document.getElementById('status').textContent = 'No data yet.';
        document.getElementById('latest').innerHTML = '<div class="no-data">Waiting for first sensor reading&hellip;</div>';
        charts.acState.data.datasets = [];
        charts.acState.update();
        return;
    }

    const ds = buildDatasets(sensors, acStates);
    charts.temp.data.datasets = ds.temp;
    charts.humidity.data.datasets = ds.humidity;
    charts.dewpoint.data.datasets = ds.dewpoint;
    charts.pm.data.datasets = ds.pm;
    charts.acState.data.datasets = ds.acState;
    charts.temp.update();
    charts.humidity.update();
    charts.dewpoint.update();
    charts.pm.update();
    charts.acState.update();

    if (sensors.length) {
        updateLatest(sensors);
    } else {
        document.getElementById('latest').innerHTML = '<div class="no-data">Waiting for first sensor reading&hellip;</div>';
    }

    document.getElementById('status').textContent =
        computeStatus(sensors, acStates) + ` · updated ${new Date().toLocaleTimeString()}`;
}

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

initCharts();
fetchData();
setInterval(fetchData, REFRESH_MS);
