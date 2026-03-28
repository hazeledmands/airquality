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

const DAY_MS = 24 * 60 * 60 * 1000;
const WEEK_MS = 7 * DAY_MS;

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

        // Draw sibling lines at the same time-of-day (or same time-of-week) for
        // other periods visible in the chart range.
        const rangeMs = xScale.max - xScale.min;
        const periodMs = rangeMs > WEEK_MS ? WEEK_MS : DAY_MS;
        if (rangeMs > periodMs) {
            ctx.save();
            ctx.lineWidth = 1;
            ctx.strokeStyle = 'rgba(255, 255, 255, 0.1)';
            ctx.setLineDash([3, 4]);
            for (let t = hoverTimestamp - periodMs; t >= xScale.min; t -= periodMs) {
                const sx = xScale.getPixelForValue(t);
                if (sx >= left && sx <= right) {
                    ctx.beginPath();
                    ctx.moveTo(sx, top);
                    ctx.lineTo(sx, bottom);
                    ctx.stroke();
                }
            }
            for (let t = hoverTimestamp + periodMs; t <= xScale.max; t += periodMs) {
                const sx = xScale.getPixelForValue(t);
                if (sx >= left && sx <= right) {
                    ctx.beginPath();
                    ctx.moveTo(sx, top);
                    ctx.lineTo(sx, bottom);
                    ctx.stroke();
                }
            }
            ctx.restore();
        }

        // Draw the main crosshair line.
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
