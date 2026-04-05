import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { QUICK_RANGES, formatRangeLabel, setupTimeRangePicker } from '../time-range-picker.js';

const REF = new Date('2025-04-05T15:30:00.000Z');

// ---- QUICK_RANGES shape ----

describe('QUICK_RANGES', () => {
    it('is a non-empty array', () => {
        expect(Array.isArray(QUICK_RANGES)).toBe(true);
        expect(QUICK_RANGES.length).toBeGreaterThan(0);
    });

    it('every entry has label, from, to strings', () => {
        for (const r of QUICK_RANGES) {
            expect(typeof r.label).toBe('string');
            expect(typeof r.from).toBe('string');
            expect(typeof r.to).toBe('string');
        }
    });

    it('every from expression starts with "now"', () => {
        for (const r of QUICK_RANGES) {
            expect(r.from).toMatch(/^now/);
        }
    });

    it('every to expression is "now"', () => {
        for (const r of QUICK_RANGES) {
            expect(r.to).toBe('now');
        }
    });

    it('includes a "Last 12 hours" entry', () => {
        expect(QUICK_RANGES.some(r => r.label === 'Last 12 hours')).toBe(true);
    });

    it('includes a "Last 1 week" entry', () => {
        expect(QUICK_RANGES.some(r => r.label === 'Last 1 week')).toBe(true);
    });
});

// ---- formatRangeLabel ----

describe('formatRangeLabel', () => {
    it('returns the matching quick range label for an exact match', () => {
        const qr = QUICK_RANGES.find(r => r.label === 'Last 12 hours');
        expect(formatRangeLabel(qr.from, qr.to, REF)).toBe('Last 12 hours');
    });

    it('returns the matching quick range label for "Last 1 week"', () => {
        const qr = QUICK_RANGES.find(r => r.label === 'Last 1 week');
        expect(formatRangeLabel(qr.from, qr.to, REF)).toBe('Last 1 week');
    });

    it('returns a formatted date string for a non-quick-range relative pair', () => {
        // now-13h to now doesn't match any quick range
        const label = formatRangeLabel('now-13h', 'now', REF);
        expect(typeof label).toBe('string');
        expect(label.length).toBeGreaterThan(0);
        expect(label).not.toBe('');
    });

    it('returns a formatted date string for absolute expressions', () => {
        const label = formatRangeLabel('2025-01-01T00:00:00Z', '2025-01-02T00:00:00Z', REF);
        expect(typeof label).toBe('string');
        expect(label).toContain('Jan');
    });
});

// ---- setupTimeRangePicker ----

function makeElement(overrides = {}) {
    return {
        textContent: '',
        style: { display: '' },
        value: '',
        addEventListener: vi.fn(),
        appendChild: vi.fn(),
        contains: vi.fn(() => false),
        ...overrides,
    };
}

function makeDOM({ search = '' } = {}) {
    const trigger  = makeElement();
    const popover  = makeElement({ style: { display: 'none' } });
    const quickDiv = makeElement();
    const fromInput = makeElement();
    const toInput   = makeElement();
    const applyBtn  = makeElement();
    const prevBtn   = makeElement();
    const nextBtn   = makeElement();

    const replaceState = vi.fn();
    vi.stubGlobal('history', { replaceState });
    vi.stubGlobal('location', { search, href: `http://localhost/${search}` });
    vi.stubGlobal('URL', class {
        constructor() {
            this._p = new URLSearchParams(search);
            this.searchParams = {
                delete: (k) => this._p.delete(k),
                set:    (k, v) => this._p.set(k, v),
                get:    (k) => this._p.get(k),
            };
        }
        toString() { return `http://localhost/?${this._p}`; }
    });

    const created = [];
    vi.stubGlobal('document', {
        getElementById: vi.fn((id) => {
            const map = {
                'trp-trigger': trigger,
                'trp-popover': popover,
                'trp-quick':   quickDiv,
                'trpFrom':     fromInput,
                'trpTo':       toInput,
                'trpApply':    applyBtn,
                'trp-prev':    prevBtn,
                'trp-next':    nextBtn,
            };
            return map[id] ?? null;
        }),
        addEventListener: vi.fn(),
        createElement: vi.fn((tag) => {
            const el = makeElement();
            el.tagName = tag.toUpperCase();
            created.push(el);
            return el;
        }),
    });

    return { trigger, popover, fromInput, toInput, applyBtn, prevBtn, nextBtn, replaceState, created };
}

beforeEach(() => {});
afterEach(() => { vi.unstubAllGlobals(); });

describe('setupTimeRangePicker — initialisation', () => {
    it('sets trigger text to reflect the default range', () => {
        const { trigger } = makeDOM();
        setupTimeRangePicker({ onApply: vi.fn() });
        expect(typeof trigger.textContent).toBe('string');
        expect(trigger.textContent.length).toBeGreaterThan(0);
    });

    it('restores from/to expressions from URL on load', () => {
        const { fromInput, toInput } = makeDOM({ search: '?from=now-12h&to=now' });
        setupTimeRangePicker({ onApply: vi.fn() });
        expect(fromInput.value).toBe('now-12h');
        expect(toInput.value).toBe('now');
    });

    it('defaults to a sensible range when no URL params', () => {
        const { fromInput, toInput } = makeDOM();
        setupTimeRangePicker({ onApply: vi.fn() });
        expect(fromInput.value).toMatch(/^now/);
        expect(toInput.value).toBe('now');
    });
});

describe('setupTimeRangePicker — getCurrentRange', () => {
    it('returns fromMs and toMs as numbers', () => {
        makeDOM({ search: '?from=now-1h&to=now' });
        const { getCurrentRange } = setupTimeRangePicker({ onApply: vi.fn() });
        const { fromMs, toMs } = getCurrentRange();
        expect(typeof fromMs).toBe('number');
        expect(typeof toMs).toBe('number');
    });

    it('fromMs is earlier than toMs', () => {
        makeDOM({ search: '?from=now-1h&to=now' });
        const { getCurrentRange } = setupTimeRangePicker({ onApply: vi.fn() });
        const { fromMs, toMs } = getCurrentRange();
        expect(fromMs).toBeLessThan(toMs);
    });

    it('fromMs is approximately 1h before toMs', () => {
        makeDOM({ search: '?from=now-1h&to=now' });
        const { getCurrentRange } = setupTimeRangePicker({ onApply: vi.fn() });
        const { fromMs, toMs } = getCurrentRange();
        expect(toMs - fromMs).toBeCloseTo(3_600_000, -3); // within ~1s
    });

    it('reflects absolute timestamps from URL', () => {
        const from = new Date('2025-01-01T00:00:00Z').getTime();
        const to   = new Date('2025-01-02T00:00:00Z').getTime();
        makeDOM({ search: `?from=${from}&to=${to}` });
        const { getCurrentRange } = setupTimeRangePicker({ onApply: vi.fn() });
        expect(getCurrentRange().fromMs).toBe(from);
        expect(getCurrentRange().toMs).toBe(to);
    });
});

describe('setupTimeRangePicker — URL persistence', () => {
    it('writes from/to to URL when applyRange is called', () => {
        const { replaceState } = makeDOM();
        const { applyRange } = setupTimeRangePicker({ onApply: vi.fn() });
        applyRange('now-6h', 'now');
        expect(replaceState).toHaveBeenCalled();
        const url = replaceState.mock.calls.at(-1)[2].toString();
        expect(url).toContain('from=now-6h');
        expect(url).toContain('to=now');
    });

    it('calls onApply when applyRange is called', () => {
        makeDOM();
        const onApply = vi.fn();
        const { applyRange } = setupTimeRangePicker({ onApply });
        applyRange('now-6h', 'now');
        expect(onApply).toHaveBeenCalledOnce();
    });

    it('updates trigger label after applyRange', () => {
        const { trigger } = makeDOM();
        const { applyRange } = setupTimeRangePicker({ onApply: vi.fn() });
        const qr = QUICK_RANGES.find(r => r.label === 'Last 12 hours');
        applyRange(qr.from, qr.to);
        expect(trigger.textContent).toBe('Last 12 hours');
    });
});
