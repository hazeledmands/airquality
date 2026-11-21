import { describe, it, expect, vi } from 'vitest';
import { isCompact, sizingFor, sizeOptions, CHART_HEIGHTS, zoomRange, resizeGate, compactTick, roundTick, niceRange, compactRangeLabel } from '../layout.js';

describe('isCompact', () => {
    it('is true for phones in portrait and landscape', () => {
        expect(isCompact(390, 664)).toBe(true);   // iPhone 13
        expect(isCompact(320, 568)).toBe(true);   // iPhone SE
        expect(isCompact(750, 340)).toBe(true);   // iPhone 13 landscape
        expect(isCompact(863, 360)).toBe(true);   // Pixel 7 landscape
    });
    it('is false for tablets and desktops', () => {
        expect(isCompact(700, 900)).toBe(false);
        expect(isCompact(1280, 800)).toBe(false);
    });
});

describe('sizingFor', () => {
    it('uses fixed pixel heights on phones, not aspect ratios', () => {
        for (const kind of ['main', 'secondary']) {
            const s = sizingFor(kind, true, 2.4);
            expect(s.maintainAspectRatio).toBe(false);
            expect(s.height).toBe(CHART_HEIGHTS[kind]);
        }
        expect(CHART_HEIGHTS.main).toBeGreaterThanOrEqual(200);
        expect(CHART_HEIGHTS.main).toBeLessThanOrEqual(240);
        expect(CHART_HEIGHTS.secondary).toBeGreaterThanOrEqual(200);
    });

    it('keeps the desktop aspect ratios unchanged', () => {
        expect(sizingFor('main', false, 4)).toEqual({ maintainAspectRatio: true, aspectRatio: 4, height: null });
        expect(sizingFor('secondary', false, 4)).toEqual({ maintainAspectRatio: true, aspectRatio: 6.4, height: null });
    });

    // Regression: at 601–820 px the strip was an aspect ratio of an aspect
    // ratio and collapsed to a 30–49 px plot.
    it('sizes the wind direction strip in pixels at every width', () => {
        for (const [w, compact, ar] of [[375, true, 2.4], [700, false, 4], [820, false, 4], [1280, false, 4]]) {
            const s = sizingFor('direction', compact, ar);
            expect(s.maintainAspectRatio, `width ${w}`).toBe(false);
            expect(s.height, `width ${w}`).toBe(CHART_HEIGHTS.direction);
        }
        expect(CHART_HEIGHTS.direction).toBeGreaterThanOrEqual(90);
    });
});

describe('zoomRange', () => {
    const H = 3600_000;
    it('halves and doubles the span around its centre', () => {
        expect(zoomRange(0, 4 * H, 0.5, 100 * H)).toEqual({ fromMs: H, toMs: 3 * H });
        expect(zoomRange(10 * H, 14 * H, 2, 100 * H)).toEqual({ fromMs: 8 * H, toMs: 16 * H });
    });
    it('does not zoom out past now; it shifts back instead', () => {
        const now = 100 * H;
        expect(zoomRange(now - 4 * H, now, 2, now)).toEqual({ fromMs: now - 8 * H, toMs: now });
    });
    it('stops zooming in at 10 minutes', () => {
        const r = zoomRange(0, 15 * 60_000, 0.5, 100 * H);
        expect(r.toMs - r.fromMs).toBe(10 * 60_000);
    });
});

describe('resizeGate', () => {
    it('debounces and ignores height-only changes (iOS address bar)', () => {
        vi.useFakeTimers();
        const fn = vi.fn();
        const onResize = resizeGate(fn, 150, () => 390);
        onResize(); onResize(); onResize();
        vi.advanceTimersByTime(200);
        expect(fn).not.toHaveBeenCalled(); // width unchanged since setup
        vi.useRealTimers();
    });
    it('runs once after the width settles', () => {
        vi.useFakeTimers();
        const fn = vi.fn();
        let w = 390;
        const onResize = resizeGate(fn, 150, () => w);
        w = 844; onResize(); onResize();
        vi.advanceTimersByTime(200);
        expect(fn).toHaveBeenCalledTimes(1);
        vi.useRealTimers();
    });
});

describe('compactTick', () => {
    const t = (iso) => Date.parse(iso);
    it('labels day boundaries with the date and other ticks with the hour, on one line', () => {
        const ticks = [t('2026-11-19T00:00:00'), t('2026-11-19T12:00:00'), t('2026-11-20T00:00:00')].map(value => ({ value }));
        const labels = ticks.map((_, i) => compactTick(ticks, i));
        for (const l of labels) expect(typeof l).toBe('string');
        expect(labels[0]).toMatch(/^Thu 11\/19$/);
        expect(labels[1]).toMatch(/^12 PM$/);
        expect(labels[2]).toMatch(/^Fri 11\/20$/);
    });
    it('shows only times within one day', () => {
        const ticks = [t('2026-11-19T09:00:00'), t('2026-11-19T10:30:00')].map(value => ({ value }));
        expect(compactTick(ticks, 0)).toBe('9 AM');
        expect(compactTick(ticks, 1)).toBe('10:30 AM');
    });
});

describe('roundTick', () => {
    it('rounds the padded end labels of a y axis', () => {
        expect(roundTick(80.07)).toBe('80');
        expect(roundTick(36.3)).toBe('36');
        expect(roundTick(-0.3)).toBe('-0.3');
        expect(roundTick(2.45)).toBe('2.5');
        expect(roundTick(0.05)).toBe('0.05');
        expect(roundTick(1013)).toBe('1013');
    });
});

describe('sizeOptions', () => {
    const base = () => ({ maintainAspectRatio: true, aspectRatio: 4, plugins: { legend: { display: true }, tooltip: { enabled: true } } });
    it('on phones: fixed height, no canvas tooltip, key row instead of the canvas legend', () => {
        const o = sizeOptions(base(), 'main', true, 2.4);
        expect(o.maintainAspectRatio).toBe(false);
        expect(o.plugins.tooltip.enabled).toBe(false);
        expect(o.plugins.legend.display).toBe(false);
    });
    it('on desktop: aspect ratio, tooltips and the canvas legend as before', () => {
        const o = sizeOptions(base(), 'secondary', false, 4);
        expect(o.maintainAspectRatio).toBe(true);
        expect(o.aspectRatio).toBe(6.4);
        expect(o.plugins.tooltip.enabled).toBe(true);
        expect(o.plugins.legend.display).toBe(true);
    });
    it('never turns on a legend a chart has switched off', () => {
        const o = base();
        o.plugins.legend = { display: false, keyRow: false };
        expect(sizeOptions(o, 'direction', false, 4).plugins.legend.display).toBe(false);
    });
});

describe('niceRange', () => {
    it('rounds a shared y range outward to whole steps, so no odd end labels', () => {
        expect(niceRange({ min: 38.8, max: 78.3 }, 5)).toEqual({ min: 35, max: 80 });
        expect(niceRange({ min: -0.3, max: 27.2 }, 5, 0)).toEqual({ min: 0, max: 30 });
        expect(niceRange(null, 5)).toBeNull();
    });
});

describe('compactRangeLabel', () => {
    it('drops "Last " so the range fits beside the buttons on a 320 px phone', () => {
        expect(compactRangeLabel('Last 3 days')).toBe('3 days');
        expect(compactRangeLabel('Last 24 hours')).toBe('24 hours');
        expect(compactRangeLabel('Nov 1 – Nov 3')).toBe('Nov 1 – Nov 3');
    });
});
