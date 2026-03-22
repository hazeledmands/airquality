let globalEnabled = true;
let daylightPairs = []; // [{sunrise: Date, sunset: Date}]

export function setDaylightEnabled(on) {
    globalEnabled = on;
}

export function setDaylightPairs(pairs) {
    daylightPairs = pairs;
}

export const daylightPlugin = {
    id: 'daylightShading',

    beforeDatasetsDraw(chart) {
        if (!globalEnabled) return;
        if (!daylightPairs.length) return;

        const xScale = chart.scales.x;
        if (!xScale) return;

        const { top, bottom, left, right } = chart.chartArea;
        const ctx = chart.ctx;

        ctx.save();
        ctx.fillStyle = 'rgba(0,0,20,0.35)';

        for (const { sunrise, sunset } of daylightPairs) {
            const srX = Math.min(Math.max(xScale.getPixelForValue(sunrise.getTime()), left), right);
            const ssX = Math.min(Math.max(xScale.getPixelForValue(sunset.getTime()), left), right);

            // Pre-dawn: left edge to sunrise
            if (srX > left) {
                ctx.fillRect(left, top, srX - left, bottom - top);
            }

            // Post-dusk: sunset to right edge
            if (ssX < right) {
                ctx.fillRect(ssX, top, right - ssX, bottom - top);
            }
        }

        ctx.restore();
    },
};
