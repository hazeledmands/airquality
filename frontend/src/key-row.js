// HTML legend ("key row") shown on phones instead of the canvas legend: real
// buttons with 44 px tap targets. It goes through the chart's legend
// onClick, so per-metric grouping and ?tempestHidden keep working.

export function legendEntries(chart) {
    const legend = chart.options?.plugins?.legend || {};
    if (legend.keyRow === false) return [];
    const labels = legend.labels || {};
    const gen = labels.generateLabels;
    let items = gen
        ? gen(chart)
        : chart.data.datasets.map((d, i) => ({ text: d.label, datasetIndex: i, strokeStyle: d.borderColor }));
    if (labels.filter) items = items.filter(item => labels.filter(item, chart.data));
    return items.map(item => ({
        text: item.text,
        datasetIndex: item.datasetIndex,
        color: item.strokeStyle || chart.data.datasets[item.datasetIndex]?.borderColor,
        hidden: !chart.isDatasetVisible(item.datasetIndex),
    }));
}

function toggle(chart, datasetIndex) {
    const onClick = chart.options?.plugins?.legend?.onClick;
    if (typeof onClick === 'function') {
        onClick(null, { datasetIndex }, { chart });
        return;
    }
    chart.setDatasetVisibility(datasetIndex, !chart.isDatasetVisible(datasetIndex));
    chart.update();
}

export function renderKeyRow(el, chart) {
    el.innerHTML = '';
    for (const entry of legendEntries(chart)) {
        const btn = document.createElement('button');
        btn.type = 'button';
        btn.className = 'key-btn';
        btn.setAttribute('aria-pressed', String(!entry.hidden));
        const swatch = document.createElement('span');
        swatch.className = 'key-swatch';
        swatch.style.background = entry.color || '#888';
        btn.appendChild(swatch);
        btn.appendChild(document.createTextNode(entry.text));
        btn.addEventListener('click', () => {
            toggle(chart, entry.datasetIndex);
            renderKeyRow(el, chart);
        });
        el.appendChild(btn);
    }
}
