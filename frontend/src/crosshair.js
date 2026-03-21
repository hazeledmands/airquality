const syncedCharts = [];

export function registerChart(chart) {
    syncedCharts.push(chart);
}

export function unregisterChart(chart) {
    const i = syncedCharts.indexOf(chart);
    if (i >= 0) syncedCharts.splice(i, 1);
}

let hoverTimestamp = null;
let hoverSourceId = null;

export const crosshairPlugin = {
    id: 'crosshairSync',

    afterEvent(chart, args) {
        const { event, inChartArea } = args;
        if (!syncedCharts.includes(chart)) return;

        if (event.type === 'mousemove') {
            if (inChartArea) {
                const xScale = chart.scales.x;
                if (!xScale) return;
                hoverTimestamp = xScale.getValueForPixel(event.x);
                hoverSourceId = chart.id;
                for (const c of syncedCharts) {
                    if (c.id !== chart.id) c.draw();
                }
            } else {
                if (hoverSourceId === chart.id) {
                    hoverTimestamp = null;
                    hoverSourceId = null;
                    for (const c of syncedCharts) {
                        if (c.id !== chart.id) c.draw();
                    }
                }
            }
        } else if (event.type === 'mouseout') {
            if (hoverSourceId === chart.id) {
                hoverTimestamp = null;
                hoverSourceId = null;
                for (const c of syncedCharts) {
                    if (c.id !== chart.id) c.draw();
                }
            }
        }
    },

    afterDatasetsDraw(chart) {
        if (hoverTimestamp == null) return;
        if (!syncedCharts.includes(chart)) return;

        const xScale = chart.scales.x;
        if (!xScale) return;
        const x = xScale.getPixelForValue(hoverTimestamp);
        const { left, right, top, bottom } = chart.chartArea;

        if (x < left || x > right) return;

        const ctx = chart.ctx;
        ctx.save();
        ctx.beginPath();
        ctx.moveTo(x, top);
        ctx.lineTo(x, bottom);
        ctx.lineWidth = 1;
        ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
        ctx.stroke();
        ctx.restore();
    },
};
