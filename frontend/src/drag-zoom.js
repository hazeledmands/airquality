const registeredCharts = [];
let onZoomCallback = null;
let dragState = null; // { startT, currentT, sourceChart } | null
const cleanupFns = new WeakMap();

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
    for (const c of registeredCharts) c.draw();
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
        for (const c of registeredCharts) c.draw();
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
        for (const c of registeredCharts) c.draw();
    }
}

export function setZoomCallback(fn) {
    onZoomCallback = fn;
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
