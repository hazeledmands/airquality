import { toLocalInput, getDataRange } from './data.js';

export function setupDateRange({ getDataFn, onFilter }) {
    const fromEl = document.getElementById('rangeFrom');
    const toEl = document.getElementById('rangeTo');
    const presetBtns = document.querySelectorAll('.preset-btn');
    const prevBtn = document.getElementById('rangePrev');
    const nextBtn = document.getElementById('rangeNext');
    const resetBtn = document.getElementById('rangeReset');

    function applyFilter() {
        onFilter(fromEl.value, toEl.value);
    }

    function setRangeDefaults(data) {
        if (fromEl.value || toEl.value) return;
        const range = getDataRange(data);
        if (range) {
            fromEl.value = toLocalInput(range.min);
            toEl.value = toLocalInput(range.max);
        }
    }

    function clearActivePreset() {
        presetBtns.forEach(b => b.classList.remove('active'));
    }

    fromEl.addEventListener('change', () => { clearActivePreset(); applyFilter(); });
    toEl.addEventListener('change', () => { clearActivePreset(); applyFilter(); });

    resetBtn.addEventListener('click', () => {
        fromEl.value = '';
        toEl.value = '';
        clearActivePreset();
        const data = getDataFn();
        if (data) {
            setRangeDefaults(data);
            applyFilter();
        }
    });

    for (const btn of presetBtns) {
        btn.addEventListener('click', () => {
            const data = getDataFn();
            if (!data) return;
            const range = getDataRange(data);
            if (!range) return;

            const ms = parseInt(btn.dataset.ms);
            const to = range.max;
            const from = new Date(to.getTime() - ms);
            fromEl.value = toLocalInput(from);
            toEl.value = toLocalInput(to);

            clearActivePreset();
            btn.classList.add('active');
            applyFilter();
        });
    }

    prevBtn.addEventListener('click', () => {
        if (!fromEl.value || !toEl.value) return;
        const from = new Date(fromEl.value);
        const to = new Date(toEl.value);
        const span = to.getTime() - from.getTime();
        fromEl.value = toLocalInput(new Date(from.getTime() - span));
        toEl.value = toLocalInput(new Date(to.getTime() - span));
        clearActivePreset();
        applyFilter();
    });

    nextBtn.addEventListener('click', () => {
        if (!fromEl.value || !toEl.value) return;
        const from = new Date(fromEl.value);
        const to = new Date(toEl.value);
        const span = to.getTime() - from.getTime();
        fromEl.value = toLocalInput(new Date(from.getTime() + span));
        toEl.value = toLocalInput(new Date(to.getTime() + span));
        clearActivePreset();
        applyFilter();
    });

    return { applyFilter, setRangeDefaults };
}
