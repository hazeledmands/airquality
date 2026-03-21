/** Create a drag-handle element (☰ icon). */
export function createDragHandle() {
    const handle = document.createElement('span');
    handle.className = 'drag-handle';
    handle.title = 'Drag to reorder';
    handle.textContent = '\u2630';
    return handle;
}

/**
 * Create a chart-card element with an optional title and drag handle.
 * Returns { card, canvas } so callers can attach a Chart.js instance to the canvas.
 */
export function createChartCard({ title, canvasId, chartId } = {}) {
    const card = document.createElement('div');
    card.className = 'chart-card';
    if (chartId) card.dataset.chartId = chartId;

    if (title) {
        const h2 = document.createElement('h2');
        h2.appendChild(createDragHandle());
        h2.appendChild(document.createTextNode(title));
        card.appendChild(h2);
    }

    const canvas = document.createElement('canvas');
    if (canvasId) canvas.id = canvasId;
    card.appendChild(canvas);

    return { card, canvas };
}
