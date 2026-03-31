const registeredCharts = [];
const heatmapEntries = []; // { canvas, toTime, toPx, onRedraw, cleanup }
let onZoomCallback = null;
let dragState = null; // { startT, currentT, sourceChart? | sourceCanvas? } | null
const cleanupFns = new WeakMap();

export function getDragState() { return dragState; }

function redrawAll() {
    for (const c of registeredCharts) c.draw();
    for (const h of heatmapEntries) h.onRedraw();
}

const DRAG_THRESHOLD_PX = 5;

function startDrag(chart, clientX) {
    const canvas = chart.canvas;
    const rect = canvas.getBoundingClientRect();
    const x = clientX - rect.left;
    const { left, right } = chart.chartArea;
    if (x < left || x > right) return;
    const xScale = chart.scales.x;
    if (!xScale) return;
    const t = xScale.getValueForPixel(x);
    dragState = { startT: t, currentT: t, sourceChart: chart };
    canvas.style.cursor = 'ew-resize';
}

function updateDrag(chart, clientX) {
    if (!dragState || dragState.sourceChart !== chart) return;
    const canvas = chart.canvas;
    const rect = canvas.getBoundingClientRect();
    const x = clientX - rect.left;
    const xScale = chart.scales.x;
    if (!xScale) return;
    dragState = { ...dragState, currentT: xScale.getValueForPixel(x) };
    redrawAll();
}

function endDrag(chart, clientX) {
    if (!dragState || dragState.sourceChart !== chart) return;
    const canvas = chart.canvas;
    const rect = canvas.getBoundingClientRect();
    const x = clientX - rect.left;
    const xScale = chart.scales.x;

    const finalT = xScale ? xScale.getValueForPixel(x) : dragState.currentT;
    dragState = { ...dragState, currentT: finalT };

    const startPx = xScale ? xScale.getPixelForValue(dragState.startT) : 0;
    const endPx = xScale ? xScale.getPixelForValue(finalT) : 0;
    const dx = Math.abs(endPx - startPx);

    const zoomed = dx >= DRAG_THRESHOLD_PX && onZoomCallback;
    if (zoomed) {
        // Suppress the click event the browser fires after mouseup so that the
        // annotation popover doesn't open at the end of a drag-zoom gesture.
        canvas.addEventListener('click', (e) => e.stopImmediatePropagation(), { capture: true, once: true });
        onZoomCallback({
            min: Math.min(dragState.startT, dragState.currentT),
            max: Math.max(dragState.startT, dragState.currentT),
        });
    }

    dragState = null;
    canvas.style.cursor = '';
    // When a zoom fires, the callback triggers a full chart update that redraws everything.
    // Only redraw manually for short non-zoom releases (to clear the lingering overlay).
    if (!zoomed) {
        redrawAll();
    }
}

export function registerChart(chart) {
    registeredCharts.push(chart);
    const canvas = chart.canvas;
    if (!canvas) return;

    const onDown = (e) => startDrag(chart, e.clientX);
    const onMove = (e) => updateDrag(chart, e.clientX);
    const onUp   = (e) => endDrag(chart, e.clientX);

    canvas.addEventListener('mousedown', onDown);
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);

    cleanupFns.set(chart, () => {
        canvas.removeEventListener('mousedown', onDown);
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
    });
}

export function unregisterChart(chart) {
    const i = registeredCharts.indexOf(chart);
    if (i >= 0) registeredCharts.splice(i, 1);
    const cleanup = cleanupFns.get(chart);
    if (cleanup) { cleanup(); cleanupFns.delete(chart); }
    if (dragState?.sourceChart === chart) {
        dragState = null;
        redrawAll();
    }
}

export function setZoomCallback(fn) {
    onZoomCallback = fn;
}

export function registerHeatmap(canvas, toTime, toPx, onRedraw) {
    const onDown = (e) => {
        const rect = canvas.getBoundingClientRect();
        const cssX = e.clientX - rect.left;
        const t = toTime(cssX);
        if (t == null) return;
        dragState = { startT: t, currentT: t, sourceCanvas: canvas };
        canvas.style.cursor = 'ew-resize';
    };

    const onMove = (e) => {
        if (!dragState || dragState.sourceCanvas !== canvas) return;
        const rect = canvas.getBoundingClientRect();
        const cssX = e.clientX - rect.left;
        const t = toTime(cssX);
        if (t == null) return;
        dragState = { ...dragState, currentT: t };
        redrawAll();
    };

    const onUp = (e) => {
        if (!dragState || dragState.sourceCanvas !== canvas) return;
        const rect = canvas.getBoundingClientRect();
        const cssX = e.clientX - rect.left;
        const finalT = toTime(cssX) ?? dragState.currentT;
        dragState = { ...dragState, currentT: finalT };

        const startPx = toPx(dragState.startT);
        const endPx = toPx(finalT);
        const dx = Math.abs(endPx - startPx);

        const zoomed = dx >= DRAG_THRESHOLD_PX && onZoomCallback;
        if (zoomed) {
            canvas.addEventListener('click', (e) => e.stopImmediatePropagation(), { capture: true, once: true });
            onZoomCallback({
                min: Math.min(dragState.startT, dragState.currentT),
                max: Math.max(dragState.startT, dragState.currentT),
            });
        }

        dragState = null;
        canvas.style.cursor = '';
        if (!zoomed) redrawAll();
    };

    canvas.addEventListener('mousedown', onDown);
    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);

    const entry = { canvas, toTime, toPx, onRedraw, cleanup: () => {
        canvas.removeEventListener('mousedown', onDown);
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup', onUp);
    }};
    heatmapEntries.push(entry);
}

export function unregisterHeatmap(canvas) {
    const i = heatmapEntries.findIndex(h => h.canvas === canvas);
    if (i < 0) return;
    heatmapEntries[i].cleanup();
    heatmapEntries.splice(i, 1);
    if (dragState?.sourceCanvas === canvas) {
        dragState = null;
        redrawAll();
    }
}

export const dragZoomPlugin = {
    id: 'dragZoom',

    afterDatasetsDraw(chart) {
        if (!dragState) return;
        if (!registeredCharts.includes(chart)) return;

        const xScale = chart.scales.x;
        if (!xScale) return;

        const { left, right, top, bottom } = chart.chartArea;

        const x1 = Math.max(left, Math.min(right, xScale.getPixelForValue(dragState.startT)));
        const x2 = Math.max(left, Math.min(right, xScale.getPixelForValue(dragState.currentT)));

        const xl = Math.min(x1, x2);
        const xr = Math.max(x1, x2);
        const w = xr - xl;

        if (w < 1) return;

        const ctx = chart.ctx;
        ctx.save();
        ctx.fillStyle = 'rgba(56, 189, 248, 0.15)';
        ctx.fillRect(xl, top, w, bottom - top);
        ctx.strokeStyle = 'rgba(56, 189, 248, 0.5)';
        ctx.lineWidth = 1;
        ctx.strokeRect(xl, top, w, bottom - top);
        ctx.restore();
    },
};
