import { describe, it, expect } from 'vitest';
import { chartOpts } from '../charts.js';

describe('chartOpts', () => {
    it('returns an object with expected top-level keys', () => {
        const opts = chartOpts(2);
        expect(opts).toHaveProperty('responsive', true);
        expect(opts).toHaveProperty('maintainAspectRatio', true);
        expect(opts).toHaveProperty('aspectRatio');
        expect(opts).toHaveProperty('animation');
        expect(opts).toHaveProperty('interaction');
        expect(opts).toHaveProperty('scales');
        expect(opts).toHaveProperty('plugins');
        expect(opts).toHaveProperty('elements');
    });

    it('uses the provided aspect ratio', () => {
        expect(chartOpts(2).aspectRatio).toBe(2);
        expect(chartOpts(1.2).aspectRatio).toBe(1.2);
    });

    it('configures x-axis as time scale', () => {
        const opts = chartOpts(2);
        expect(opts.scales.x.type).toBe('time');
    });

    it('sets animation duration to 300ms', () => {
        const opts = chartOpts(2);
        expect(opts.animation.duration).toBe(300);
    });

    it('disables point radius by default', () => {
        const opts = chartOpts(2);
        expect(opts.elements.point.radius).toBe(0);
    });

    it('sets hover radius to 4', () => {
        const opts = chartOpts(2);
        expect(opts.elements.point.hoverRadius).toBe(4);
    });

    it('uses index interaction mode', () => {
        const opts = chartOpts(2);
        expect(opts.interaction.mode).toBe('index');
        expect(opts.interaction.intersect).toBe(false);
    });

    it('configures line tension to show actual data (no interpolation)', () => {
        const opts = chartOpts(2);
        expect(opts.elements.line.tension).toBe(0);
    });

    it('limits x-axis ticks', () => {
        const opts = chartOpts(2);
        expect(opts.scales.x.ticks.maxTicksLimit).toBe(8);
        expect(opts.scales.x.ticks.maxRotation).toBe(0);
    });
});

describe('xTickCallback on phones', () => {
    it('returns one-line labels (no second date line)', async () => {
        const { xTickCallback } = await import('../charts.js');
        const prevW = globalThis.innerWidth, prevH = globalThis.innerHeight;
        globalThis.innerWidth = 390; globalThis.innerHeight = 664;
        const ticks = [Date.parse('2026-11-19T00:00:00'), Date.parse('2026-11-19T12:00:00'), Date.parse('2026-11-20T00:00:00')].map(value => ({ value }));
        try {
            for (let i = 0; i < ticks.length; i++) expect(typeof xTickCallback(ticks[i].value, i, ticks)).toBe('string');
        } finally {
            globalThis.innerWidth = prevW; globalThis.innerHeight = prevH;
        }
    });
});
