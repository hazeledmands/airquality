import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { setupDateRange } from '../daterange.js';

vi.mock('../data.js', () => ({
    toLocalInput: (d) => d.toISOString().slice(0, 16),
    getDataRange: (data) => data ? { min: new Date(data.min), max: new Date(data.max) } : null,
}));

function makeInput(value = '') {
    return {
        value,
        addEventListener: vi.fn(),
    };
}

function makeBtn(ms) {
    return {
        dataset: { ms: String(ms) },
        classList: { toggle: vi.fn(), remove: vi.fn(), add: vi.fn() },
        addEventListener: vi.fn(),
    };
}

function makeDOM({ search = '', presets = [86400000, 259200000] } = {}) {
    const fromEl = makeInput();
    const toEl = makeInput();
    const prevBtn = { addEventListener: vi.fn() };
    const nextBtn = { addEventListener: vi.fn() };
    const resetBtn = { addEventListener: vi.fn() };
    const presetBtns = presets.map(ms => makeBtn(ms));

    const replaceState = vi.fn();
    vi.stubGlobal('history', { replaceState });
    vi.stubGlobal('location', { search, href: `http://localhost/${search}` });
    vi.stubGlobal('URL', class {
        constructor() {
            this._params = new URLSearchParams(search);
            this.searchParams = {
                delete: (k) => this._params.delete(k),
                set: (k, v) => this._params.set(k, v),
                get: (k) => this._params.get(k),
            };
        }
        toString() { return `http://localhost/?${this._params}`; }
    });
    vi.stubGlobal('document', {
        getElementById: vi.fn((id) => {
            if (id === 'rangeFrom') return fromEl;
            if (id === 'rangeTo') return toEl;
            if (id === 'rangePrev') return prevBtn;
            if (id === 'rangeNext') return nextBtn;
            if (id === 'rangeReset') return resetBtn;
            return null;
        }),
        querySelectorAll: vi.fn(() => presetBtns),
    });

    return { fromEl, toEl, presetBtns, replaceState };
}

beforeEach(() => {});
afterEach(() => { vi.unstubAllGlobals(); });

describe('setupDateRange URL persistence — restoreFromURL', () => {
    it('activates the preset from ?preset= on load', () => {
        const { presetBtns } = makeDOM({ search: '?preset=86400000' });
        setupDateRange({ getDataFn: () => null, onFilter: vi.fn() });
        // The 1d preset button should be toggled active
        expect(presetBtns[0].classList.toggle).toHaveBeenCalledWith('active', true);
    });

    it('restores from/to inputs from ?from=&to= on load', () => {
        const { fromEl, toEl } = makeDOM({ search: '?from=2025-01-01T00%3A00&to=2025-01-04T00%3A00' });
        setupDateRange({ getDataFn: () => null, onFilter: vi.fn() });
        expect(fromEl.value).toBe('2025-01-01T00:00');
        expect(toEl.value).toBe('2025-01-04T00:00');
    });

    it('defaults to DEFAULT_DATE_RANGE_MS preset when no URL params present', () => {
        const { presetBtns } = makeDOM({ search: '', presets: [86400000, 259200000] });
        setupDateRange({ getDataFn: () => null, onFilter: vi.fn() });
        // 259200000 is the default (3 days) — its toggle should be called active=true
        expect(presetBtns[1].classList.toggle).toHaveBeenCalledWith('active', true);
    });

    it('ignores invalid preset value in URL', () => {
        const { fromEl, toEl } = makeDOM({ search: '?preset=notanumber' });
        setupDateRange({ getDataFn: () => null, onFilter: vi.fn() });
        // Should not set inputs to anything meaningful
        expect(fromEl.value).toBe('');
        expect(toEl.value).toBe('');
    });
});

describe('setupDateRange URL persistence — syncURL', () => {
    it('writes preset param when applyFilter is called with an active preset', () => {
        const data = { min: Date.now() - 86400000, max: Date.now() };
        const { replaceState } = makeDOM({ search: '?preset=86400000' });
        const { applyFilter } = setupDateRange({ getDataFn: () => data, onFilter: vi.fn() });
        applyFilter();
        expect(replaceState).toHaveBeenCalled();
        const url = replaceState.mock.calls.at(-1)[2].toString();
        expect(url).toContain('preset=86400000');
    });

    it('writes from/to params when applyFilter is called without a preset', () => {
        const { fromEl, toEl, replaceState } = makeDOM({ search: '?from=2025-01-01T00%3A00&to=2025-01-04T00%3A00' });
        fromEl.value = '2025-01-01T00:00';
        toEl.value = '2025-01-04T00:00';
        const { applyFilter } = setupDateRange({ getDataFn: () => null, onFilter: vi.fn() });
        applyFilter();
        const url = replaceState.mock.calls.at(-1)[2].toString();
        expect(url).toContain('from=');
        expect(url).toContain('to=');
        expect(url).not.toContain('preset=');
    });

    it('preserves non-date params when updating URL', () => {
        const { fromEl, toEl, replaceState } = makeDOM({ search: '?roomOrder=A%2CB&from=2025-01-01T00%3A00&to=2025-01-04T00%3A00' });
        fromEl.value = '2025-01-01T00:00';
        toEl.value = '2025-01-04T00:00';
        const { applyFilter } = setupDateRange({ getDataFn: () => null, onFilter: vi.fn() });
        applyFilter();
        const url = replaceState.mock.calls.at(-1)[2].toString();
        expect(url).toContain('roomOrder=');
    });
});
