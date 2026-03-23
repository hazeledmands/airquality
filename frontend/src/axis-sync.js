// Returns the global x-axis range {min, max} (milliseconds) that spans all
// data points across every chart. Returns null when there is no data.
export function computeXRange(charts) {
    let min = Infinity;
    let max = -Infinity;
    for (const chart of charts) {
        for (const dataset of chart.data.datasets) {
            for (const point of dataset.data) {
                const t = typeof point.x === 'number' ? point.x : new Date(point.x).getTime();
                if (t < min) min = t;
                if (t > max) max = t;
            }
        }
    }
    return min <= max ? { min, max } : null;
}

// Applies the same x-axis min and max to every chart so they are visually aligned.
export function syncXAxes(charts, range) {
    if (!range) return;
    for (const chart of charts) {
        chart.options.scales.x.min = range.min;
        chart.options.scales.x.max = range.max;
    }
}
