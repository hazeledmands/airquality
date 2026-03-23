import { HUMIDITY_THRESHOLD } from './config.js';

let globalEnabled = true;

export function setHumidityHighlightEnabled(on) {
    globalEnabled = on;
}

export function isHumidityHighlightEnabled() {
    return globalEnabled;
}

export const humidityHighlightPlugin = {
    id: 'humidityHighlight',

    beforeDatasetsDraw(chart) {
        if (!globalEnabled) return;
        const opts = chart.options.plugins?.humidityHighlight;
        if (!opts?.enabled) return;

        const threshold = opts.threshold || HUMIDITY_THRESHOLD;

        // Find the humidity dataset
        let humIdx = -1;
        for (let i = 0; i < chart.data.datasets.length; i++) {
            if (chart.data.datasets[i].label === 'Humidity') {
                humIdx = i;
                break;
            }
        }
        if (humIdx === -1) return;

        const dataset = chart.data.datasets[humIdx];
        const meta = chart.getDatasetMeta(humIdx);
        if (meta.hidden) return;

        const points = meta.data;
        if (!points || points.length < 2) return;

        const { top, bottom, left, right } = chart.chartArea;
        const ctx = chart.ctx;

        ctx.save();

        // Draw threshold reference line
        const yScale = chart.scales[dataset.yAxisID || 'y1'];
        if (yScale) {
            const thresholdY = yScale.getPixelForValue(threshold);
            ctx.setLineDash([4, 4]);
            ctx.strokeStyle = 'rgba(239, 68, 68, 0.25)';
            ctx.lineWidth = 1;
            ctx.beginPath();
            ctx.moveTo(left, thresholdY);
            ctx.lineTo(right, thresholdY);
            ctx.stroke();
            ctx.setLineDash([]);
        }

        // Shade regions where humidity > threshold
        ctx.fillStyle = 'rgba(239, 68, 68, 0.07)';

        for (let i = 0; i < points.length - 1; i++) {
            const currVal = dataset.data[i]?.y;
            const nextVal = dataset.data[i + 1]?.y;
            if (currVal == null || nextVal == null) continue;

            const currX = points[i].x;
            const nextX = points[i + 1].x;
            if (nextX < left || currX > right) continue;

            const currAbove = currVal > threshold;
            const nextAbove = nextVal > threshold;

            let x0, x1;

            if (currAbove && nextAbove) {
                x0 = currX;
                x1 = nextX;
            } else if (currAbove && !nextAbove) {
                const t = (threshold - currVal) / (nextVal - currVal);
                x0 = currX;
                x1 = currX + t * (nextX - currX);
            } else if (!currAbove && nextAbove) {
                const t = (threshold - currVal) / (nextVal - currVal);
                x0 = currX + t * (nextX - currX);
                x1 = nextX;
            } else {
                continue;
            }

            // Clamp to chart area
            x0 = Math.max(x0, left);
            x1 = Math.min(x1, right);
            if (x1 > x0) {
                ctx.fillRect(x0, top, x1 - x0, bottom - top);
            }
        }

        ctx.restore();
    },
};
