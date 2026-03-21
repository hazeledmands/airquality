import { toLocalInput, getDataRange } from './data.js';

function updateURL(params) {
    const url = new URL(window.location);
    url.search = '';
    for (const [k, v] of Object.entries(params)) {
        if (v) url.searchParams.set(k, v);
    }
    history.replaceState(null, '', url);
}

function readURL() {
    const params = new URLSearchParams(window.location.search);
    return {
        from: params.get('from'),
        to: params.get('to'),
        preset: params.get('preset'),
    };
}

export function setupDateRange({ getDataFn, onFilter }) {
    const fromEl = document.getElementById('rangeFrom');
    const toEl = document.getElementById('rangeTo');
    const presetBtns = document.querySelectorAll('.preset-btn');
    const prevBtn = document.getElementById('rangePrev');
    const nextBtn = document.getElementById('rangeNext');
    const resetBtn = document.getElementById('rangeReset');

    let activePresetMs = null;

    function syncURL() {
        if (activePresetMs) {
            updateURL({ preset: String(activePresetMs) });
        } else {
            updateURL({ from: fromEl.value, to: toEl.value });
        }
    }

    function applyFilter() {
        onFilter(fromEl.value, toEl.value);
        syncURL();
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
        activePresetMs = null;
        presetBtns.forEach(b => b.classList.remove('active'));
    }

    function activatePreset(ms) {
        const data = getDataFn();
        if (!data) return false;
        const range = getDataRange(data);
        if (!range) return false;

        const to = range.max;
        const from = new Date(to.getTime() - ms);
        fromEl.value = toLocalInput(from);
        toEl.value = toLocalInput(to);

        activePresetMs = ms;
        presetBtns.forEach(b => {
            b.classList.toggle('active', parseInt(b.dataset.ms) === ms);
        });
        return true;
    }

    // Restore state from URL on load
    function restoreFromURL() {
        const saved = readURL();
        if (saved.preset) {
            const ms = parseInt(saved.preset);
            if (ms > 0) {
                // Preset will be applied after first data fetch via applyPresetAfterLoad
                activePresetMs = ms;
                presetBtns.forEach(b => {
                    b.classList.toggle('active', parseInt(b.dataset.ms) === ms);
                });
                return;
            }
        }
        if (saved.from) fromEl.value = saved.from;
        if (saved.to) toEl.value = saved.to;
    }

    // Called after data loads — applies a pending preset if one was in the URL
    function setRangeDefaultsWithURL(data) {
        if (activePresetMs) {
            const range = getDataRange(data);
            if (range) {
                const to = range.max;
                const from = new Date(to.getTime() - activePresetMs);
                fromEl.value = toLocalInput(from);
                toEl.value = toLocalInput(to);
            }
            return;
        }
        setRangeDefaults(data);
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
            const ms = parseInt(btn.dataset.ms);
            if (activatePreset(ms)) applyFilter();
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

    restoreFromURL();

    return { applyFilter, setRangeDefaults: setRangeDefaultsWithURL };
}
