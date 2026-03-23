import { CHART_ANIMATION_MS } from './config.js';

export function xTickCallback(value, index, ticks) {
    const dt = luxon.DateTime.fromMillis(ticks[index].value);
    const time = dt.toFormat('h:mm a');
    const first = luxon.DateTime.fromMillis(ticks[0].value);
    const last = luxon.DateTime.fromMillis(ticks[ticks.length - 1].value);
    if (first.hasSame(last, 'day')) return time;
    if (index === 0 || ticks[index].major) return [dt.toFormat('ccc M/d'), time];
    return time;
}

export function chartOpts(aspectRatio) {
    return {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio,
        animation: { duration: CHART_ANIMATION_MS },
        interaction: { mode: 'index', intersect: false },
        scales: {
            x: {
                type: 'time',
                time: { tooltipFormat: 'ff' },
                ticks: { color: '#555', autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback, major: { enabled: true } },
                grid: { color: '#ffffff06' },
            },
            y: {
                ticks: { color: '#555' },
                grid: { color: '#ffffff06' },
            },
        },
        plugins: {
            legend: { labels: { color: '#999', boxWidth: 12, padding: 12 } },
            tooltip: { backgroundColor: '#1e1e30', titleColor: '#ccc', bodyColor: '#aaa', borderColor: '#333', borderWidth: 1 },
        },
        elements: {
            point: { radius: 0, hoverRadius: 4 },
            line: { borderWidth: 2, tension: 0 },
        },
    };
}
