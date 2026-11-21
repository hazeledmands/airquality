// Shared configuration constants.
// Change a value here to affect all consumers consistently.

// Minimum horizontal pixel gap between two peak labels on the same chart.
export const PEAK_MIN_PX_DIST = 200;

// Humidity % level above which the humidity-highlight plugin shades the chart.
export const HUMIDITY_THRESHOLD = 60;

// Chart.js animation duration in milliseconds.
export const CHART_ANIMATION_MS = 300;

// Fraction of the data span added as padding above and below each y-axis.
export const Y_AXIS_PADDING = 0.05;

// Width in px every chart's y axes are pinned to, so plot areas (and with
// them the shared time axis) line up vertically across all charts.
export const AXIS_WIDTH = 50;

export function pinAxisWidth(scale) {
    scale.width = AXIS_WIDTH;
}

// Default date range preset shown on first load (milliseconds).
export const DEFAULT_DATE_RANGE_MS = 259200000; // 3 days
