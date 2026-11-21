import { describe, it, expect } from 'vitest';
import { nearestPoint, readoutFor, createGesture } from '../readout.js';

const T0 = Date.parse('2026-11-20T12:00:00Z');
const at = (min) => new Date(T0 + min * 60_000).toISOString();

describe('nearestPoint', () => {
    const data = [{ x: at(0), y: 1 }, { x: at(10), y: 2 }, { x: at(20), y: null }, { x: at(30), y: 4 }];
    it('finds the closest non-null point', () => {
        expect(nearestPoint(data, T0 + 4 * 60_000).y).toBe(1);
        expect(nearestPoint(data, T0 + 6 * 60_000).y).toBe(2);
        expect(nearestPoint(data, T0 + 21 * 60_000).y).toBe(4); // skips the null gap marker
    });
    it('returns null far from any point', () => {
        expect(nearestPoint(data, T0 + 300 * 60_000)).toBeNull();
    });
});

describe('readoutFor', () => {
    const chart = {
        data: { datasets: [
            { label: 'Temperature', data: [{ x: at(0), y: 50.04 }, { x: at(10), y: 51.2 }], peakUnit: '°F' },
            { label: 'From', data: [{ x: at(0), y: 70 }, { x: at(10), y: 80 }], tooltipFormat: (v) => `${v}° E` },
            { label: 'Hidden', data: [{ x: at(0), y: 1 }], hidden: true },
            { label: 'AC', data: [{ x: at(0), y: 1 }, { x: at(10), y: 0 }], stepped: true },
        ] },
        isDatasetVisible: (i) => !chart.data.datasets[i].hidden,
    };
    it('lists each visible dataset at the scrubbed time', () => {
        const r = readoutFor(chart, T0 + 60_000);
        expect(r.lines.map(l => l.text)).toEqual(['Temperature 50.0°F', 'From 70° E', 'AC on']);
        expect(r.title).toBeTruthy();
    });
});

describe('createGesture', () => {
    it('a quick tap shows the readout', () => {
        const g = createGesture();
        g.start(100, 100, 0);
        expect(g.end(120)).toBe('tap');
    });
    it('a vertical swipe scrolls and never scrubs', () => {
        const g = createGesture();
        g.start(100, 100, 0);
        expect(g.move(102, 140, 50)).toBe('scroll');
        expect(g.longPress()).toBe(false);
        expect(g.end(200)).toBe('none');
    });
    it('press and hold, then drag, scrubs', () => {
        const g = createGesture();
        g.start(100, 100, 0);
        expect(g.longPress()).toBe(true);
        expect(g.move(160, 104, 400)).toBe('scrub');
        expect(g.end(600)).toBe('scrub-end');
    });
    it('tap then drag scrubs (second touch within the window)', () => {
        const g = createGesture();
        g.start(100, 100, 0); g.end(100);   // tap
        g.start(105, 100, 300);
        expect(g.move(180, 102, 350)).toBe('scrub');
    });
});
