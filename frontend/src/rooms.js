import { REFRESH_MS, cToF, filterByRange, buildRoomDatasets, computeStatus, getAspectRatio } from './data.js';
import { xTickCallback } from './charts.js';
import { crosshairPlugin, registerChart, unregisterChart } from './crosshair.js';
import { setupDateRange } from './daterange.js';
import { createDragHandle } from './chart-card.js';
import { setupReorder } from './reorder.js';

Chart.register(crosshairPlugin);

const METRIC_COLORS = {
    temp:     '#fb923c', // orange
    dewpoint: '#38bdf8', // blue
    humidity: '#34d399', // green
    pm:       '#a78bfa', // purple
    acState:  '#555555', // gray
};

let allData = null;
let roomCharts = []; // { room, chart }
let reorder = null;

function combinedChartOpts(ar, room) {
    const hasPM = room.pm.length > 0;
    const hasAC = room.acState.length > 0;

    const opts = {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio: ar,
        animation: { duration: 300 },
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
            legend: { labels: { color: '#999', boxWidth: 12, padding: 12 } },
            tooltip: { backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1 },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 2, tension: 0.3 },
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
            backgroundColor: METRIC_COLORS.temp + '18',
        });
    }

    for (const ds of room.dewpoint) {
        datasets.push({
            ...ds,
            label: 'Dew Point',
            yAxisID: 'y',
            borderColor: METRIC_COLORS.dewpoint,
            backgroundColor: METRIC_COLORS.dewpoint + '18',
            borderDash: [5, 3],
        });
    }

    for (const ds of room.humidity) {
        datasets.push({
            ...ds,
            label: 'Humidity',
            yAxisID: 'y1',
            borderColor: METRIC_COLORS.humidity,
            backgroundColor: METRIC_COLORS.humidity + '18',
        });
    }

    for (const ds of room.pm) {
        datasets.push({
            ...ds,
            label: 'PM2.5',
            yAxisID: 'y2',
            borderColor: METRIC_COLORS.pm,
            backgroundColor: METRIC_COLORS.pm + '18',
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

    return datasets;
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

        const chart = new Chart(canvas, {
            type: 'line',
            data: { datasets: [] },
            options: combinedChartOpts(ar, room),
        });

        registerChart(chart);
        container.appendChild(section);
        roomCharts.push({ room: room.name, chart });
    }

    if (reorder) reorder.bindItems();
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

    if (!sensors.length && !acStates.length) {
        document.getElementById('status').textContent = 'No data yet.';
        const container = document.getElementById('rooms-container');
        container.innerHTML = '<div class="no-data">Waiting for first sensor reading&hellip;</div>';
        for (const rc of roomCharts) unregisterChart(rc.chart);
        roomCharts = [];
        return;
    }

    const rooms = buildRoomDatasets(sensors, acStates);

    // Rebuild DOM if room set changed
    const currentRooms = roomCharts.map(rc => rc.room);
    const newRooms = rooms.map(r => r.name);
    if (currentRooms.length !== newRooms.length || currentRooms.some((r, i) => r !== newRooms[i])) {
        for (const rc of roomCharts) rc.chart.destroy();
        buildRoomDOM(rooms);
    }

    // Update chart data
    for (const room of rooms) {
        const rc = roomCharts.find(r => r.room === room.name);
        if (!rc) continue;
        rc.chart.data.datasets = buildCombinedDatasets(room);
        rc.chart.update();
    }

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

reorder = setupReorder(document.getElementById('rooms-container'), {
    itemSelector: '.room-section',
    urlParam: 'roomOrder',
});

fetchData();
setInterval(fetchData, REFRESH_MS);
