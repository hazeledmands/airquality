import { REFRESH_MS, cToF, filterByRange, buildRoomDatasets, computeStatus, getAspectRatio, makeWeatherTempSeries, makeUVSeries, extractDaylightPairs, applyNewAnnotation, removeAnnotation } from './data.js';
import { CHART_ANIMATION_MS, Y_AXIS_PADDING } from './config.js';
import { xTickCallback } from './charts.js';
import { crosshairPlugin, registerChart, unregisterChart } from './crosshair.js';
import { maximaLabelsPlugin, setMaximaLabelsEnabled } from './maxima-labels.js';
import { humidityHighlightPlugin, setHumidityHighlightEnabled } from './humidity-highlight.js';
import { daylightPlugin, setDaylightPairs } from './daylight-plugin.js';
import { setupDateRange } from './daterange.js';
import { createDragHandle } from './chart-card.js';
import { setupReorder } from './reorder.js';
import { computeXRange, syncXAxes, computeYRange, syncYAxes } from './axis-sync.js';
import { annotationsPlugin, setAnnotations, setupAnnotationUI } from './annotations.js';
import { getParam, setParam } from './url-state.js';

Chart.register(crosshairPlugin);
Chart.register(maximaLabelsPlugin);
Chart.register(humidityHighlightPlugin);
Chart.register(daylightPlugin);
Chart.register(annotationsPlugin);

const METRIC_COLORS = {
    temp:     '#fb923c', // orange
    dewpoint: '#38bdf8', // blue
    humidity: '#34d399', // green
    pm:       '#a78bfa', // purple
    acState:  '#6b7280', // gray
};

let allData = null;
let roomCharts = []; // { room, chart }
let reorder = null;
let outdoorTempEnabled = false;
let currentWeather = [];

function isIndoorRoom(room) {
    return !/outside/i.test(room.name);
}

function combinedChartOpts(ar, room) {
    const hasPM = room.pm.length > 0;
    const hasAC = room.acState.length > 0;
    const indoor = isIndoorRoom(room);

    const opts = {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio: ar,
        animation: { duration: CHART_ANIMATION_MS },
        interaction: { mode: 'index', intersect: false },
        scales: {
            x: {
                type: 'time',
                time: { tooltipFormat: 'ff' },
                ticks: { color: '#555', autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback, major: { enabled: true } },
                grid: { color: '#ffffff06' },
            },
            y: {
                type: 'linear',
                position: 'left',
                title: { display: true, text: '\u00B0F', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { color: '#ffffff06' },
            },
            y1: {
                type: 'linear',
                position: 'right',
                title: { display: true, text: '%', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { drawOnChartArea: false },
                min: 0,
                max: 100,
            },
        },
        plugins: {
            legend: { position: 'bottom', labels: { color: '#555', boxWidth: 16, boxHeight: 1, padding: 10, font: { size: 10 } } },
            tooltip: { backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1 },
            humidityHighlight: { enabled: indoor, threshold: 60 },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 2, tension: 0 },
        },
    };

    if (hasPM) {
        opts.scales.y2 = {
            type: 'linear',
            position: 'right',
            title: { display: true, text: '\u00B5g/m\u00B3', color: '#888', font: { size: 11 } },
            ticks: { color: '#888' },
            grid: { drawOnChartArea: false },
        };
    }

    if (hasAC) {
        opts.scales.y3 = {
            type: 'linear',
            position: 'right',
            display: false,
            min: -0.05,
            max: 1.05,
        };
    }

    return opts;
}

function buildCombinedDatasets(room) {
    const datasets = [];

    for (const ds of room.temp) {
        datasets.push({
            ...ds,
            label: 'Temperature',
            yAxisID: 'y',
            borderColor: METRIC_COLORS.temp,
            fill: false,
            peakUnit: '\u00B0F',
        });
    }

    for (const ds of room.dewpoint) {
        datasets.push({
            ...ds,
            label: 'Dew Point',
            yAxisID: 'y',
            borderColor: METRIC_COLORS.dewpoint,
            fill: false,
            borderDash: [5, 3],
            peakUnit: '\u00B0F',
        });
    }

    for (const ds of room.humidity) {
        datasets.push({
            ...ds,
            label: 'Humidity',
            yAxisID: 'y1',
            borderColor: METRIC_COLORS.humidity,
            fill: false,
            peakUnit: '%',
        });
    }

    for (const ds of room.pm) {
        datasets.push({
            ...ds,
            label: 'PM2.5',
            yAxisID: 'y2',
            borderColor: METRIC_COLORS.pm,
            fill: false,
            peakUnit: '\u00B5g/m\u00B3',
        });
    }

    for (const ds of room.acState) {
        datasets.push({
            ...ds,
            label: 'AC',
            yAxisID: 'y3',
            borderColor: METRIC_COLORS.acState,
            backgroundColor: METRIC_COLORS.acState + '18',
            fill: true,
            borderWidth: 1,
            stepped: true,
            tension: 0,
        });
    }

    if (outdoorTempEnabled && currentWeather.length > 0 && isIndoorRoom(room)) {
        datasets.push({
            ...makeWeatherTempSeries(currentWeather),
            yAxisID: 'y',
        });
    }

    return datasets;
}

function weatherChartOpts(ar) {
    return {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio: ar,
        animation: { duration: CHART_ANIMATION_MS },
        interaction: { mode: 'index', intersect: false },
        scales: {
            x: {
                type: 'time',
                time: { tooltipFormat: 'ff' },
                ticks: { color: '#555', autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback, major: { enabled: true } },
                grid: { color: '#ffffff06' },
            },
            y: {
                type: 'linear',
                position: 'left',
                title: { display: true, text: '\u00B0F', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { color: '#ffffff06' },
            },
            yUV: {
                type: 'linear',
                position: 'right',
                title: { display: true, text: 'UV', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { drawOnChartArea: false },
                min: 0,
            },
        },
        plugins: {
            legend: { position: 'bottom', labels: { color: '#555', boxWidth: 16, boxHeight: 1, padding: 10, font: { size: 10 } } },
            tooltip: { backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1 },
            humidityHighlight: { enabled: false },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 2, tension: 0 },
        },
    };
}

function buildWeatherCard(weather, ar) {
    const section = document.createElement('div');
    section.className = 'room-section';
    section.dataset.chartId = '__weather__';

    const heading = document.createElement('h2');
    heading.style.color = '#94a3b8';
    heading.appendChild(createDragHandle());
    const lat = weather[0]?.latitude?.toFixed(4) ?? '';
    const lon = weather[0]?.longitude?.toFixed(4) ?? '';
    const title = lat && lon ? `Open-Meteo (${lat}, ${lon})` : 'Open-Meteo';
    heading.appendChild(document.createTextNode(title));
    section.appendChild(heading);

    const strip = document.createElement('div');
    strip.className = 'room-latest';
    strip.id = 'weather-current';
    section.appendChild(strip);

    const card = document.createElement('div');
    card.className = 'chart-card';
    const canvas = document.createElement('canvas');
    card.appendChild(canvas);
    section.appendChild(card);

    const chart = new Chart(canvas, {
        type: 'line',
        data: { datasets: [] },
        options: weatherChartOpts(ar),
    });

    registerChart(chart);
    roomCharts.push({ room: '__weather__', chart });
    return section;
}

function updateWeatherCard(weather) {
    const strip = document.getElementById('weather-current');
    const rc = roomCharts.find(r => r.room === '__weather__');
    if (!rc) return;

    if (weather.length > 0) {
        const latest = weather[weather.length - 1];
        const parts = [
            `<span><span class="value">${cToF(latest.temp_c).toFixed(1)}\u00B0F</span></span>`,
            `<span><span class="value">${latest.humidity.toFixed(0)}%</span> humidity</span>`,
        ];
        if (latest.wind_speed_kmh) {
            parts.push(`<span><span class="value">${latest.wind_speed_kmh.toFixed(1)}</span> km/h wind</span>`);
        }
        if (latest.precipitation_mm) {
            parts.push(`<span><span class="value">${latest.precipitation_mm.toFixed(1)}</span> mm precip</span>`);
        }
        if (strip) strip.innerHTML = parts.join('');

        rc.chart.data.datasets = [
            { ...makeWeatherTempSeries(weather), yAxisID: 'y' },
            { ...makeUVSeries(weather), yAxisID: 'yUV' },
        ];
    }
}

function buildRoomDOM(rooms) {
    const container = document.getElementById('rooms-container');
    container.innerHTML = '';

    // Unregister old charts
    for (const rc of roomCharts) {
        unregisterChart(rc.chart);
    }
    roomCharts = [];

    const ar = getAspectRatio(window.innerWidth);

    if (currentWeather.length > 0) {
        container.appendChild(buildWeatherCard(currentWeather, ar));
    }

    for (const room of rooms) {
        const section = document.createElement('div');
        section.className = 'room-section';
        section.dataset.chartId = room.name;

        const heading = document.createElement('h2');
        heading.style.color = room.color;
        heading.appendChild(createDragHandle());
        heading.appendChild(document.createTextNode(room.name));
        section.appendChild(heading);

        const latest = document.createElement('div');
        latest.className = 'room-latest';
        latest.dataset.room = room.name;
        section.appendChild(latest);

        const card = document.createElement('div');
        card.className = 'chart-card';
        const canvas = document.createElement('canvas');
        card.appendChild(canvas);
        section.appendChild(card);

        const opts = combinedChartOpts(ar, room);
        opts.plugins.annotations = { room: room.name };
        const chart = new Chart(canvas, {
            type: 'line',
            data: { datasets: [] },
            options: opts,
        });

        registerChart(chart);
        container.appendChild(section);
        roomCharts.push({ room: room.name, chart });
    }

    if (reorder) reorder.bindItems();
    setupAnnotationUI(roomCharts, { onSave: saveAnnotation, onDelete: deleteAnnotation });
}

function updateLatestValues(rooms) {
    for (const room of rooms) {
        const el = document.querySelector(`.room-latest[data-room="${room.name}"]`);
        if (!el) continue;
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
    const weather = data.weather || [];
    currentWeather = weather;
    setDaylightPairs(extractDaylightPairs(weather));

    if (!sensors.length && !acStates.length) {
        document.getElementById('status').textContent = 'No data yet.';
        const container = document.getElementById('rooms-container');
        container.innerHTML = '<div class="no-data">Waiting for first sensor reading&hellip;</div>';
        for (const rc of roomCharts) unregisterChart(rc.chart);
        roomCharts = [];
        return;
    }

    const rooms = buildRoomDatasets(sensors, acStates);

    // Rebuild DOM if room set or weather presence changed
    const currentRooms = roomCharts.map(rc => rc.room);
    const newRooms = (weather.length > 0 ? ['__weather__'] : []).concat(rooms.map(r => r.name));
    if (currentRooms.length !== newRooms.length || currentRooms.some((r, i) => r !== newRooms[i])) {
        for (const rc of roomCharts) rc.chart.destroy();
        buildRoomDOM(rooms);
    }

    updateWeatherCard(weather);

    // Set chart data (no update yet)
    for (const room of rooms) {
        const rc = roomCharts.find(r => r.room === room.name);
        if (!rc) continue;
        rc.chart.data.datasets = buildCombinedDatasets(room);
    }

    // Align all axes to the same ranges, then update
    const allCharts = roomCharts.map(rc => rc.chart);
    syncXAxes(allCharts, computeXRange(allCharts));
    syncYAxes(allCharts, 'y',  computeYRange(allCharts, 'y',  Y_AXIS_PADDING));
    syncYAxes(allCharts, 'y2', computeYRange(allCharts, 'y2', Y_AXIS_PADDING));
    for (const rc of roomCharts) rc.chart.update();

    setAnnotations(data.annotations || []);
    updateLatestValues(rooms);
    document.getElementById('status').textContent =
        computeStatus(sensors, acStates) + ` \u00B7 updated ${new Date().toLocaleTimeString()}`;
}

window.addEventListener('resize', () => {
    const ar = getAspectRatio(window.innerWidth);
    for (const rc of roomCharts) {
        rc.chart.options.aspectRatio = ar;
        rc.chart.update();
    }
});

const { setRangeDefaults } = setupDateRange({
    getDataFn: () => allData,
    onFilter: (from, to) => {
        if (allData) updateCharts(filterByRange(allData, from, to));
    },
});

async function saveAnnotation(timestamp, note, room) {
    try {
        const resp = await fetch('/api/annotations', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ timestamp, note, room }),
        });
        if (resp.ok && allData) {
            // Immediately reflect in the UI so the user sees the marker right away.
            const newAnnotation = await resp.json();
            allData = applyNewAnnotation(allData, newAnnotation);
            const from = document.getElementById('rangeFrom').value;
            const to = document.getElementById('rangeTo').value;
            updateCharts(filterByRange(allData, from, to));
        }
        fetchData();
    } catch (err) {
        console.error('Failed to save annotation:', err);
    }
}

async function deleteAnnotation(id) {
    try {
        const resp = await fetch(`/api/annotations/${id}`, { method: 'DELETE' });
        if (resp.ok && allData) {
            // Immediately remove from the UI.
            allData = removeAnnotation(allData, id);
            const from = document.getElementById('rangeFrom').value;
            const to = document.getElementById('rangeTo').value;
            updateCharts(filterByRange(allData, from, to));
        }
        fetchData();
    } catch (err) {
        console.error('Failed to delete annotation:', err);
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

// Peaks checkbox — restore from URL, persist changes
const peaksCheckbox = document.getElementById('showPeaks');
if (getParam('peaks') !== null) peaksCheckbox.checked = getParam('peaks') !== '0';
setMaximaLabelsEnabled(peaksCheckbox.checked);
peaksCheckbox.addEventListener('change', () => {
    setMaximaLabelsEnabled(peaksCheckbox.checked);
    setParam('peaks', peaksCheckbox.checked ? null : '0');
    for (const rc of roomCharts) rc.chart.update();
});

// High humidity checkbox — restore from URL, persist changes
const humidityCheckbox = document.getElementById('showHumidity');
if (getParam('humidity') !== null) humidityCheckbox.checked = getParam('humidity') !== '0';
setHumidityHighlightEnabled(humidityCheckbox.checked);
humidityCheckbox.addEventListener('change', () => {
    setHumidityHighlightEnabled(humidityCheckbox.checked);
    setParam('humidity', humidityCheckbox.checked ? null : '0');
    for (const rc of roomCharts) rc.chart.update();
});

// Outdoor temp checkbox — restore from URL, persist changes
const outdoorTempCheckbox = document.getElementById('showOutdoorTemp');
if (getParam('outdoorTemp') !== null) outdoorTempCheckbox.checked = getParam('outdoorTemp') === '1';
outdoorTempEnabled = outdoorTempCheckbox.checked;
outdoorTempCheckbox.addEventListener('change', () => {
    outdoorTempEnabled = outdoorTempCheckbox.checked;
    setParam('outdoorTemp', outdoorTempCheckbox.checked ? '1' : null);
    for (const rc of roomCharts) rc.chart.update();
});

reorder = setupReorder(document.getElementById('rooms-container'), {
    itemSelector: '.room-section',
    urlParam: 'roomOrder',
});

fetchData();
setInterval(fetchData, REFRESH_MS);
