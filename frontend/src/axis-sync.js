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

// Returns the global y-axis range {min, max} for all datasets assigned to the
// given axisId across every chart. Returns null when there is no matching data.
export function computeYRange(charts, axisId) {
    let min = Infinity;
    let max = -Infinity;
    for (const chart of charts) {
        for (const dataset of chart.data.datasets) {
            if (dataset.yAxisID !== axisId) continue;
            for (const point of dataset.data) {
                const v = typeof point === 'number' ? point : point.y;
                if (typeof v !== 'number') continue;
                if (v < min) min = v;
                if (v > max) max = v;
            }
        }
    }
    return min <= max ? { min, max } : null;
}

// Applies the same min and max to every chart's named y-axis so they are
// visually aligned. Charts that lack the axis are skipped silently.
export function syncYAxes(charts, axisId, range) {
    if (!range) return;
    for (const chart of charts) {
        if (!chart.options.scales[axisId]) continue;
        chart.options.scales[axisId].min = range.min;
        chart.options.scales[axisId].max = range.max;
    }
}
