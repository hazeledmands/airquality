import { COLORS, REFRESH_MS, cToF, filterByRange, buildDatasets, computeStatus, getAspectRatio } from './data.js';
import { chartOpts } from './charts.js';
import { crosshairPlugin, registerChart } from './crosshair.js';
import { maximaLabelsPlugin, setMaximaLabelsEnabled } from './maxima-labels.js';
import { setupDateRange } from './daterange.js';
import { setupReorder } from './reorder.js';

Chart.register(crosshairPlugin);
Chart.register(maximaLabelsPlugin);

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

    for (const chart of Object.values(charts)) {
        registerChart(chart);
    }
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
    ds.temp.forEach(d => d.peakUnit = '\u00B0F');
    ds.humidity.forEach(d => d.peakUnit = '%');
    ds.dewpoint.forEach(d => d.peakUnit = '\u00B0F');
    ds.pm.forEach(d => d.peakUnit = '\u00B5g/m\u00B3');
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

const { applyFilter, setRangeDefaults } = setupDateRange({
    getDataFn: () => allData,
    onFilter: (from, to) => {
        if (allData) updateCharts(filterByRange(allData, from, to));
    },
});

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

// Peaks checkbox — restore from URL, persist changes
const peaksCheckbox = document.getElementById('showPeaks');
const peaksParam = new URLSearchParams(window.location.search).get('peaks');
if (peaksParam !== null) {
    peaksCheckbox.checked = peaksParam !== '0';
}
setMaximaLabelsEnabled(peaksCheckbox.checked);
peaksCheckbox.addEventListener('change', () => {
    setMaximaLabelsEnabled(peaksCheckbox.checked);
    const url = new URL(window.location);
    if (peaksCheckbox.checked) url.searchParams.delete('peaks');
    else url.searchParams.set('peaks', '0');
    history.replaceState(null, '', url);
    for (const chart of Object.values(charts)) chart.update();
});

initCharts();
setupReorder(document.querySelector('.charts'));
fetchData();
setInterval(fetchData, REFRESH_MS);
