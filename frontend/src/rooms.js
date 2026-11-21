import { REFRESH_MS, cToF, filterByRange, buildRoomDatasets, computeStatus, getAspectRatio, makeWeatherTempSeries, makeUVSeries, extractDaylightPairs, applyNewAnnotation, removeAnnotation } from './data.js';
import { CHART_ANIMATION_MS, Y_AXIS_PADDING, pinAxisWidth, MUTED_TEXT } from './config.js';
import { xTickCallback } from './charts.js';
import { combinedChartOpts, buildCombinedDatasets, PARTICLE_BINS, formatLatestValues } from './room-charts.js';
import { renderHeatmap, setupHeatmapInteraction } from './particle-heatmap.js';
import { crosshairPlugin, registerChart, unregisterChart, getHoverTimestamp, setCrosshairTimestamp } from './crosshair.js';
import { dragZoomPlugin, registerChart as registerZoomChart, unregisterChart as unregisterZoomChart, setZoomCallback, getDragState } from './drag-zoom.js';
import { maximaLabelsPlugin, setMaximaLabelsEnabled } from './maxima-labels.js';
import { humidityHighlightPlugin, setHumidityHighlightEnabled } from './humidity-highlight.js';
import { daylightPlugin, setDaylightPairs } from './daylight-plugin.js';
import { setupTimeRangePicker } from './time-range-picker.js';
import { createDragHandle, createChartBox, sizeChartBox } from './chart-card.js';
import { isCompact, sizeOptions, sizingFor, zoomRange, resizeGate, roundTick, niceRange } from './layout.js';
import { renderKeyRow } from './key-row.js';
import { readoutFor, createGesture } from './readout.js';
import { setupReorder } from './reorder.js';
import { computeXRange, syncXAxes, computeYRange, syncYAxes } from './axis-sync.js';
import { annotationsPlugin, setAnnotations, setupAnnotationUI, setAnnotationsEditable, promptAddAnnotation, isAnnotationsEditable } from './annotations.js';
import { fetchAuth, loginUrl } from './auth.js';
import { getParam, setParam } from './url-state.js';
import { buildTempestSection, updateTempestSection, buildEmptyTempestSection } from './tempest-card.js';
import { tempestAsWeather, tempestPanelIds } from './tempest.js';

Chart.register(crosshairPlugin);
Chart.register(dragZoomPlugin);
Chart.register(maximaLabelsPlugin);
Chart.register(humidityHighlightPlugin);
Chart.register(daylightPlugin);
Chart.register(annotationsPlugin);

let allData = null;
let currentStepSize = 0;
let roomCharts = []; // { room, chart, kind, box, keyRow, ... }
let cleanupAnnotationUI = null;
let reorder = null;
let outdoorTempEnabled = false;
let currentWeather = [];
let currentTempest = null; // { enabled, station, observations, latest, events } from /api/data
let renderedLayout = []; // section/chart ids the DOM was last built for
let skipAnimation = false; // set true before zoom-triggered updateCharts to avoid filler plugin errors


function weatherChartOpts(ar) {
    const opts = {
        responsive: true,
        animation: { duration: CHART_ANIMATION_MS },
        interaction: { mode: 'index', intersect: false },
        scales: {
            x: {
                type: 'time',
                time: { tooltipFormat: 'ff' },
                ticks: { color: MUTED_TEXT, autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback, major: { enabled: true } },
                grid: { color: '#ffffff06' },
            },
            y: {
                type: 'linear',
                position: 'left',
                title: { display: true, text: '\u00B0F', color: '#888', font: { size: 11 } },
                ticks: { color: '#888', callback: roundTick },
                grid: { color: '#ffffff06' },
                afterFit: pinAxisWidth,
            },
            yUV: {
                type: 'linear',
                position: 'right',
                title: { display: true, text: 'UV', color: '#888', font: { size: 11 } },
                ticks: { color: '#888' },
                grid: { drawOnChartArea: false },
                min: 0,
                afterFit: pinAxisWidth,
            },
        },
        plugins: {
            legend: { position: 'bottom', labels: { color: MUTED_TEXT, boxWidth: 16, boxHeight: 1, padding: 10, font: { size: 10 } } },
            tooltip: { backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1 },
            humidityHighlight: { enabled: false },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 2, tension: 0 },
        },
    };
    return sizeOptions(opts, 'main', isCompact(), ar);
}

// The chart box and key row for a 'main' chart, sized for this screen.
function mainChartBox(ar) {
    const parts = createChartBox('main');
    sizeChartBox(parts.box, sizingFor('main', isCompact(), ar).height);
    return parts;
}

function buildWeatherCard(weather, ar) {
    const section = document.createElement('div');
    section.className = 'room-section';
    section.dataset.chartId = '__weather__';

    const heading = document.createElement('h2');
    heading.style.color = '#94a3b8';
    heading.appendChild(createDragHandle());
    // No coordinates: the page is public.
    heading.appendChild(document.createTextNode('Open-Meteo'));
    section.appendChild(heading);

    const strip = document.createElement('div');
    strip.className = 'room-latest';
    strip.id = 'weather-current';
    section.appendChild(strip);

    const card = document.createElement('div');
    card.className = 'chart-card';
    const { box, canvas, keyRow } = mainChartBox(ar);
    card.appendChild(box);
    card.appendChild(keyRow);
    section.appendChild(card);

    const chart = new Chart(canvas, {
        type: 'line',
        data: { datasets: [] },
        options: weatherChartOpts(ar),
    });

    registerChart(chart);
    registerZoomChart(chart);
    attachTouchReadout(chart);
    roomCharts.push({ room: '__weather__', chart, kind: 'main', box, keyRow });
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
            // mph, like the Tempest.
            parts.push(`<span><span class="value">${(latest.wind_speed_kmh / 1.609344).toFixed(1)}</span> mph wind</span>`);
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

    // Teardown old annotation listeners and charts
    if (cleanupAnnotationUI) { cleanupAnnotationUI(); cleanupAnnotationUI = null; }
    for (const rc of roomCharts) {
        unregisterChart(rc.chart);
        unregisterZoomChart(rc.chart);
        if (rc.crosshairUnsub) rc.crosshairUnsub();
    }
    roomCharts = [];

    const ar = getAspectRatio(window.innerWidth);

    if (currentWeather.length > 0) {
        container.appendChild(buildWeatherCard(currentWeather, ar));
    }

    if (tempestEmpty(currentTempest)) {
        container.appendChild(buildEmptyTempestSection(currentTempest.station));
    } else if (hasTempest(currentTempest)) {
        const { section, panels } = buildTempestSection(currentTempest.station, ar);
        container.appendChild(section);
        for (const p of panels) {
            registerChart(p.chart);
            registerZoomChart(p.chart);
            attachTouchReadout(p.chart);
            roomCharts.push(p);
        }
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
        const { box, canvas, keyRow } = mainChartBox(ar);
        card.appendChild(box);
        card.appendChild(keyRow);
        section.appendChild(card);

        const opts = sizeOptions(combinedChartOpts(ar, room), 'main', isCompact(), ar);
        opts.plugins.annotations = { room: room.name };
        const chart = new Chart(canvas, {
            type: 'line',
            data: { datasets: [] },
            options: opts,
        });

        registerChart(chart);
        registerZoomChart(chart);
        attachTouchReadout(chart);

        let heatmapCanvas = null;
        const hasParticles = PARTICLE_BINS.some(b => (room.particles?.[b.field]?.length ?? 0) > 0);
        if (hasParticles) {
            heatmapCanvas = document.createElement('canvas');
            heatmapCanvas.className = 'heatmap-canvas';
            section.appendChild(heatmapCanvas);
        }

        container.appendChild(section);
        roomCharts.push({ room: room.name, chart, kind: 'main', box, keyRow, heatmapCanvas, roomData: null });

        if (heatmapCanvas) {
            const rc = roomCharts[roomCharts.length - 1];
            rc.crosshairUnsub = setupHeatmapInteraction(heatmapCanvas, () => ({
                particles: rc.roomData?.particles,
                xRange: { min: rc.chart.options.scales.x.min, max: rc.chart.options.scales.x.max },
                stepMs: currentStepSize,
            }));
        }
    }

    if (reorder) reorder.bindItems();
    cleanupAnnotationUI = setupAnnotationUI(roomCharts, { onSave: saveAnnotation, onDelete: deleteAnnotation });
}

function updateLatestValues(rooms) {
    for (const room of rooms) {
        const el = document.querySelector(`.room-latest[data-room="${room.name}"]`);
        if (el) el.innerHTML = formatLatestValues(room);
    }
}

function hasTempest(tempest) {
    return (tempest?.observations?.length ?? 0) > 0;
}

// Recording is on but the selected range has no observations.
function tempestEmpty(tempest) {
    return !!tempest?.enabled && !hasTempest(tempest);
}

function updateCharts(data) {
    const sensors = data.sensors || [];
    const acStates = data.ac_states || [];
    const weather = data.weather || [];
    currentWeather = weather;
    currentTempest = data.tempest || null;
    const tempestShown = hasTempest(currentTempest);
    setDaylightPairs(extractDaylightPairs(weather));

    if (!sensors.length && !acStates.length && !tempestShown && !tempestEmpty(currentTempest)) {
        document.getElementById('status').textContent = 'No data yet.';
        const container = document.getElementById('rooms-container');
        container.innerHTML = '<div class="no-data">Waiting for first sensor reading&hellip;</div>';
        for (const rc of roomCharts) { unregisterChart(rc.chart); unregisterZoomChart(rc.chart); }
        roomCharts = [];
        renderedLayout = [];
        return;
    }

    const rooms = buildRoomDatasets(sensors, acStates, currentStepSize);

    // Rebuild DOM if room set or weather presence changed
    const currentRooms = renderedLayout;
    const newRooms = (weather.length > 0 ? ['__weather__'] : [])
        .concat(tempestShown ? tempestPanelIds() : [])
        .concat(tempestEmpty(currentTempest) ? ['__tempest_empty__'] : [])
        .concat(rooms.map(r => r.name));
    if (currentRooms.length !== newRooms.length || currentRooms.some((r, i) => r !== newRooms[i])) {
        for (const rc of roomCharts) rc.chart.destroy();
        buildRoomDOM(rooms);
        renderedLayout = newRooms;
    }

    updateWeatherCard(weather);
    if (tempestShown) {
        updateTempestSection(roomCharts.filter(rc => rc.room.startsWith('__tempest__/')), currentTempest, currentStepSize);
    }
    // The outdoor overlay prefers the on-site Tempest over Open-Meteo.
    const outdoorTemp = tempestShown ? tempestAsWeather(currentTempest.observations) : currentWeather;

    // Set chart data (no update yet)
    for (const room of rooms) {
        const rc = roomCharts.find(r => r.room === room.name);
        if (!rc) continue;
        rc.chart.data.datasets = buildCombinedDatasets(room, { outdoorTempEnabled, currentWeather: outdoorTemp, stepMs: currentStepSize });
        rc.roomData = room;
    }

    // Align all axes to the same ranges, then update
    const allCharts = roomCharts.map(rc => rc.chart);
    const xRange = computeXRange(allCharts);
    syncXAxes(allCharts, xRange);
    // Shared y ranges, rounded outward to whole 5s so the end labels are
    // round numbers. PM2.5 can't be negative.
    syncYAxes(allCharts, 'y',  niceRange(computeYRange(allCharts, 'y',  Y_AXIS_PADDING), 5));
    syncYAxes(allCharts, 'y2', niceRange(computeYRange(allCharts, 'y2', Y_AXIS_PADDING), 5, 0));
    // UV appears on both the Open-Meteo and Tempest charts; keep one scale.
    const uvRange = computeYRange(allCharts, 'yUV', 0);
    if (uvRange) syncYAxes(allCharts, 'yUV', { min: 0, max: Math.max(uvRange.max, 3) });
    const updateMode = skipAnimation ? 'none' : undefined;
    skipAnimation = false;
    for (const rc of roomCharts) {
        rc.chart.update(updateMode);
        // After update(): Chart.js builds legend items from dataset metadata,
        // which is stale until the chart has seen the new datasets.
        if (rc.keyRow) renderKeyRow(rc.keyRow, rc.chart);
        if (rc.heatmapCanvas && rc.roomData?.particles && xRange) {
            renderHeatmap(rc.heatmapCanvas, rc.roomData.particles, xRange, getHoverTimestamp(), getDragState(), currentStepSize);
        }
    }

    setAnnotations(data.annotations || []);
    updateLatestValues(rooms);
    const tempestStatus = tempestShown ? ` \u00B7 ${currentTempest.observations.length} Tempest obs` : '';
    document.getElementById('status').textContent =
        computeStatus(sensors, acStates) + tempestStatus + ` \u00B7 updated ${new Date().toLocaleTimeString()}`;
}

// Re-size for a new width (rotation, window resize). Debounced, skipped for
// height-only changes (the iOS address bar), and not animated: animating a
// dozen charts on every resize cost three ~170 ms long tasks.
window.addEventListener('resize', resizeGate(() => {
    const ar = getAspectRatio(window.innerWidth);
    const compact = isCompact();
    for (const rc of roomCharts) {
        const kind = rc.kind || 'main';
        sizeOptions(rc.chart.options, kind, compact, ar);
        if (rc.box) sizeChartBox(rc.box, sizingFor(kind, compact, ar).height);
        rc.chart.update('none');
        if (rc.keyRow) renderKeyRow(rc.keyRow, rc.chart);
        if (rc.heatmapCanvas && rc.roomData?.particles) {
            const xRange = { min: rc.chart.options.scales.x.min, max: rc.chart.options.scales.x.max };
            renderHeatmap(rc.heatmapCanvas, rc.roomData.particles, xRange, getHoverTimestamp(), getDragState(), currentStepSize);
        }
    }
}));

// --- Touch readout (phones) -------------------------------------------------
// A tap, press-and-hold or tap-then-drag on any chart moves the synced
// crosshair and shows that chart's values in one readout pinned under the
// header. A tap elsewhere or a scroll dismisses it. Mouse behaviour is
// unchanged. The readout is transient, so it is not kept in the URL.
const readoutEl = document.getElementById('readout');
let scrubbing = false;

function chartTitle(chart) {
    const section = chart.canvas.closest('.room-section');
    const panel = chart.canvas.closest('.tempest-panel')?.querySelector('.panel-label .label-short, .panel-label')?.textContent;
    const name = section?.querySelector('h2')?.textContent.replace('\u2630', '').trim() || '';
    return panel ? `${name} \u00B7 ${panel.split(' \u00B7 ')[0]}` : name;
}

function escapeHtml(s) {
    return String(s).replace(/[&<>"]/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c]));
}

function showReadout(chart, t) {
    if (t == null || !Number.isFinite(t)) return;
    setCrosshairTimestamp(t, chart.id);
    const r = readoutFor(chart, t);
    readoutEl.innerHTML = `<div class="readout-title">${escapeHtml(chartTitle(chart))} \u00B7 ${escapeHtml(r.title)}</div>`
        + r.lines.map(l => `<span class="readout-item"><i style="background:${escapeHtml(l.color || '#888')}"></i>${escapeHtml(l.text)}</span>`).join('');
    if (isAnnotationsEditable()) {
        const add = document.createElement('button');
        add.type = 'button';
        add.className = 'readout-add';
        add.textContent = 'Add note here';
        add.addEventListener('click', () => {
            const rect = readoutEl.getBoundingClientRect();
            hideReadout();
            promptAddAnnotation(chart, t, window.innerWidth / 2, rect.bottom + 8);
        });
        readoutEl.appendChild(add);
    }
    readoutEl.hidden = false;
}

function hideReadout() {
    if (readoutEl.hidden) return;
    readoutEl.hidden = true;
    setCrosshairTimestamp(null, null);
}

function attachTouchReadout(chart) {
    const canvas = chart.canvas;
    const gesture = createGesture();
    let timer = null;
    const timeAt = (touch) => {
        const rect = canvas.getBoundingClientRect();
        const { left, right } = chart.chartArea || { left: 0, right: rect.width };
        const x = Math.min(Math.max(touch.clientX - rect.left, left), right);
        return chart.scales.x?.getValueForPixel(x);
    };
    // Passive throughout: canvases are touch-action: pan-y, so vertical
    // swipes scroll natively and horizontal moves reach these handlers.
    canvas.addEventListener('touchstart', (e) => {
        if (e.touches.length !== 1) return;
        const touch = e.touches[0];
        gesture.start(touch.clientX, touch.clientY, e.timeStamp);
        clearTimeout(timer);
        timer = setTimeout(() => {
            if (gesture.longPress()) { scrubbing = true; showReadout(chart, timeAt(touch)); }
        }, 350);
    }, { passive: true });
    canvas.addEventListener('touchmove', (e) => {
        const touch = e.touches[0];
        const state = gesture.move(touch.clientX, touch.clientY);
        if (state === 'scroll') clearTimeout(timer);
        if (state === 'scrub') { scrubbing = true; showReadout(chart, timeAt(touch)); }
    }, { passive: true });
    const end = (e) => {
        clearTimeout(timer);
        const touch = e.changedTouches[0];
        const result = gesture.end(e.timeStamp);
        scrubbing = false;
        if (result === 'tap' && touch) showReadout(chart, timeAt(touch));
    };
    canvas.addEventListener('touchend', end, { passive: true });
    canvas.addEventListener('touchcancel', end, { passive: true });
}

document.addEventListener('touchstart', (e) => {
    if (!e.target.closest?.('.chart-box canvas, #readout')) hideReadout();
}, { passive: true });
window.addEventListener('scroll', () => { if (!scrubbing) hideReadout(); }, { passive: true });

// --- Header -------------------------------------------------------------------
// The header is sticky, so nothing is hidden under it; its measured height
// positions the readout just below it.
const header = document.querySelector('.date-range');
new ResizeObserver(() => {
    document.documentElement.style.setProperty('--header-h', `${header.getBoundingClientRect().height}px`);
}).observe(header);

// Phones: the toggles live in a sheet behind the "more" button. Whether the
// sheet is open is transient, so it is not kept in the URL.
const moreBtn = document.getElementById('moreBtn');
const moreSheet = document.getElementById('moreSheet');
function setSheet(open) {
    moreSheet.classList.toggle('open', open);
    moreBtn.setAttribute('aria-expanded', String(open));
}
moreBtn.addEventListener('click', (e) => { e.stopPropagation(); setSheet(!moreSheet.classList.contains('open')); });
document.addEventListener('click', (e) => {
    if (moreSheet.classList.contains('open') && !e.target.closest('#moreSheet, #moreBtn')) setSheet(false);
});
document.addEventListener('keydown', (e) => { if (e.key === 'Escape') setSheet(false); });

const picker = setupTimeRangePicker({ onApply: fetchData });

// Zoom buttons (phones): drag-to-zoom is mouse-only, so the page still
// scrolls under a finger.
function zoomBy(factor) {
    const { fromMs, toMs } = picker.getCurrentRange();
    const r = zoomRange(fromMs, toMs, factor);
    skipAnimation = true;
    picker.applyRange(String(r.fromMs), String(r.toMs));
}
document.getElementById('zoomIn').addEventListener('click', () => zoomBy(0.5));
document.getElementById('zoomOut').addEventListener('click', () => zoomBy(2));

setZoomCallback(({ min, max }) => {
    // Skipping animation avoids filler plugin errors during zoom redraws
    // (see comment in drag-zoom.js for detail).
    // Round to integer ms — xScale.getValueForPixel returns floats.
    skipAnimation = true;
    picker.applyRange(String(Math.round(min)), String(Math.round(max)));
});

// Viewing is public; annotating needs a signed-in user. Until /api/me says
// otherwise the dashboard is read-only, so a signed-out viewer never opens an
// add popover whose Save would fail.
const signInLink = document.getElementById('signIn');
// Resolve at click time: the query string changes as the view does.
signInLink.addEventListener('click', () => { signInLink.href = loginUrl(window.location); });

function applyAuth({ canEdit }) {
    setAnnotationsEditable(canEdit);
    signInLink.hidden = canEdit;
}
applyAuth({ canEdit: false });
fetchAuth().then(applyAuth);

// A write that bounces off sign-in (session expired) comes back as 401 or,
// via redirect: 'manual', an opaque redirect. Either way, drop to read-only.
function handleWriteRejected(resp) {
    if (resp.status === 401 || resp.type === 'opaqueredirect') {
        applyAuth({ canEdit: false });
    }
}

async function saveAnnotation(timestamp, note, room) {
    try {
        const resp = await fetch('/api/annotations', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ timestamp, note, room }),
            redirect: 'manual',
        });
        handleWriteRejected(resp);
        if (resp.ok && allData) {
            // Immediately reflect in the UI so the user sees the marker right away.
            const newAnnotation = await resp.json();
            allData = applyNewAnnotation(allData, newAnnotation);
            updateCharts(allData);
        }
        fetchData();
    } catch (err) {
        console.error('Failed to save annotation:', err);
    }
}

async function deleteAnnotation(id) {
    try {
        const resp = await fetch(`/api/annotations/${id}`, { method: 'DELETE', redirect: 'manual' });
        handleWriteRejected(resp);
        if (resp.ok && allData) {
            // Immediately remove from the UI.
            allData = removeAnnotation(allData, id);
            updateCharts(allData);
        }
        fetchData();
    } catch (err) {
        console.error('Failed to delete annotation:', err);
    }
}

async function fetchData() {
    try {
        const { fromMs, toMs } = picker.getCurrentRange();
        const stepSize = Math.max(60_000, Math.ceil((toMs - fromMs) / 500 / 60_000) * 60_000);
        const params = new URLSearchParams({ from: String(fromMs), to: String(toMs), stepSize: String(stepSize) });
        const resp = await fetch('/api/data?' + params);
        allData = await resp.json();
        currentStepSize = stepSize;
        updateCharts(allData);
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
    // The overlay is a dataset: rebuild them now rather than at the next
    // refresh. Not animated: changing datasets mid-animation breaks the
    // filler plugin (as with zoom).
    if (allData) {
        skipAnimation = true;
        updateCharts(allData);
    }
});

reorder = setupReorder(document.getElementById('rooms-container'), {
    itemSelector: '.room-section',
    urlParam: 'roomOrder',
});

// Refresh every minute while the page is visible; a hidden tab doesn't
// fetch, and catches up as soon as it is shown again.
let lastFetch = 0;
function refresh() {
    lastFetch = Date.now();
    fetchData();
}
refresh();
setInterval(() => { if (!document.hidden) refresh(); }, REFRESH_MS);
document.addEventListener('visibilitychange', () => {
    if (!document.hidden && Date.now() - lastFetch >= REFRESH_MS) refresh();
});
