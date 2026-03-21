export function xTickCallback(value, index, ticks) {
    const dt = luxon.DateTime.fromMillis(ticks[index].value);
    const time = dt.toFormat('h:mm a');
    if (index === 0) {
        return [dt.toFormat('ccc M/d'), time];
    }
    const prev = luxon.DateTime.fromMillis(ticks[index - 1].value);
    if (dt.day !== prev.day || dt.month !== prev.month) {
        return [dt.toFormat('ccc M/d'), time];
    }
    return time;
}

export function chartOpts(aspectRatio) {
    return {
        responsive: true,
        maintainAspectRatio: true,
        aspectRatio,
        animation: { duration: 300 },
        interaction: { mode: 'index', intersect: false },
        scales: {
            x: {
                type: 'time',
                time: { tooltipFormat: 'ff' },
                ticks: { color: '#555', autoSkip: true, maxTicksLimit: 8, maxRotation: 0, callback: xTickCallback },
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
            line: { borderWidth: 2, tension: 0.3 },
        },
    };
}
