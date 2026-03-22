const MIN_PX_DIST = 200; // minimum horizontal pixel distance between labels

let enabled = true;

export function setMaximaLabelsEnabled(on) {
    enabled = on;
}

export function isMaximaLabelsEnabled() {
    return enabled;
}

export const maximaLabelsPlugin = {
    id: 'maximaLabels',

    afterDatasetsDraw(chart) {
        if (!enabled) return;

        const ctx = chart.ctx;
        const { top, bottom, left, right } = chart.chartArea;

        for (let di = 0; di < chart.data.datasets.length; di++) {
            const dataset = chart.data.datasets[di];
            if (dataset.stepped) continue; // skip AC state

            const meta = chart.getDatasetMeta(di);
            if (meta.hidden) continue;

            const points = meta.data;
            if (!points || points.length < 3) continue;

            // Find dominant peaks: local maxima that aren't dominated by any
            // higher value within MIN_PX_DIST pixels. This naturally enforces
            // spacing and picks the true peak of each hill.
            const maxima = [];
            for (let i = 1; i < points.length - 1; i++) {
                const prev = dataset.data[i - 1]?.y;
                const curr = dataset.data[i]?.y;
                const next = dataset.data[i + 1]?.y;
                if (prev == null || curr == null || next == null) continue;

                // Local max: >= both neighbors, strictly > at least one (handles plateaus)
                if (!(curr >= prev && curr >= next && (curr > prev || curr > next))) continue;

                const px = points[i].x;
                const py = points[i].y;
                if (px < left || px > right || py < top || py > bottom) continue;

                // Check if any point within MIN_PX_DIST has a higher value.
                // For equal values, the earlier (leftmost) point wins.
                let dominated = false;
                for (let j = i - 1; j >= 0 && !dominated; j--) {
                    if (points[j].x < px - MIN_PX_DIST) break;
                    const val = dataset.data[j]?.y;
                    if (val != null && val >= curr) dominated = true;
                }
                if (!dominated) {
                    for (let j = i + 1; j < points.length && !dominated; j++) {
                        if (points[j].x > px + MIN_PX_DIST) break;
                        const val = dataset.data[j]?.y;
                        if (val != null && val > curr) dominated = true;
                    }
                }

                if (!dominated) {
                    maxima.push({ value: curr, x: px, y: py });
                }
            }

            // Draw labels
            const color = dataset.borderColor;
            const unit = dataset.peakUnit || '';
            ctx.save();
            ctx.font = 'bold 10px system-ui, sans-serif';
            ctx.textAlign = 'center';
            ctx.textBaseline = 'bottom';
            ctx.fillStyle = color;

            for (const m of maxima) {
                const num = Number.isInteger(m.value) ? String(m.value) : m.value.toFixed(1);
                ctx.fillText(num + unit, m.x, m.y - 6);
            }
            ctx.restore();
        }
    },
};
