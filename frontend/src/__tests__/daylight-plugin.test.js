import { describe, it, expect, vi, beforeEach } from 'vitest';
import { daylightPlugin, setDaylightEnabled, setDaylightPairs } from '../daylight-plugin.js';

function makeCtx() {
    return {
        save: vi.fn(),
        restore: vi.fn(),
        fillRect: vi.fn(),
        fillStyle: '',
    };
}

function makeChart(xPixels = { left: 0, right: 1200 }) {
    const ctx = makeCtx();
    // xScale.getPixelForValue maps timestamps to x pixels
    const chart = {
        ctx,
        chartArea: { top: 0, bottom: 400, left: xPixels.left, right: xPixels.right },
        scales: {
            x: {
                // Simple linear mapping: timestamp 0 -> left, timestamp 1200000 -> right
                getPixelForValue: (ms) => xPixels.left + (ms / 1200000) * (xPixels.right - xPixels.left),
                min: 0,
                max: 1200000,
            },
        },
    };
    return { chart, ctx };
}

// For a chart spanning 0..1200000 ms (left=0, right=1200):
// ms 0       -> x 0
// ms 600000  -> x 600
// ms 1200000 -> x 1200
const DAY_START = 0;
const DAY_END = 1200000;
const SUNRISE = 300000;  // x = 300
const SUNSET  = 900000;  // x = 900

describe('daylightPlugin', () => {
    beforeEach(() => {
        setDaylightEnabled(true);
        setDaylightPairs([]);
    });

    it('has id "daylightShading"', () => {
        expect(daylightPlugin.id).toBe('daylightShading');
    });

    it('does not draw when globally disabled', () => {
        setDaylightEnabled(false);
        setDaylightPairs([{ sunrise: new Date(SUNRISE), sunset: new Date(SUNSET) }]);
        const { chart, ctx } = makeChart();
        daylightPlugin.beforeDatasetsDraw(chart);
        expect(ctx.fillRect).not.toHaveBeenCalled();
    });

    it('does not draw when pairs is empty', () => {
        setDaylightPairs([]);
        const { chart, ctx } = makeChart();
        daylightPlugin.beforeDatasetsDraw(chart);
        expect(ctx.fillRect).not.toHaveBeenCalled();
    });

    it('shades pre-dawn and post-dusk but not daytime', () => {
        setDaylightPairs([{ sunrise: new Date(SUNRISE), sunset: new Date(SUNSET) }]);
        const { chart, ctx } = makeChart();
        daylightPlugin.beforeDatasetsDraw(chart);

        // Should have drawn exactly 2 rects (pre-dawn and post-dusk)
        expect(ctx.fillRect).toHaveBeenCalledTimes(2);

        const calls = ctx.fillRect.mock.calls;
        // Pre-dawn: from left=0 to sunrise x=300
        const preDawn = calls.find(([x]) => x <= 0);
        expect(preDawn).toBeDefined();
        expect(preDawn[2]).toBeCloseTo(300, 0); // width ≈ 300

        // Post-dusk: from sunset x=900 to right=1200
        const postDusk = calls.find(([x]) => x >= 890);
        expect(postDusk).toBeDefined();
        expect(postDusk[2]).toBeCloseTo(300, 0); // width ≈ 300
    });

    it('does not shade the daylight region', () => {
        setDaylightPairs([{ sunrise: new Date(SUNRISE), sunset: new Date(SUNSET) }]);
        const { chart, ctx } = makeChart();
        daylightPlugin.beforeDatasetsDraw(chart);

        // No rect should span the daytime x=300..900
        for (const [x, , w] of ctx.fillRect.mock.calls) {
            const x1 = x + w;
            const overlapsDay = x < 900 && x1 > 300;
            const isDaylight = x >= 300 && x1 <= 900;
            if (overlapsDay) {
                expect(isDaylight).toBe(false);
            }
        }
    });

    it('handles multiple day pairs', () => {
        setDaylightPairs([
            { sunrise: new Date(SUNRISE), sunset: new Date(SUNSET) },
            { sunrise: new Date(SUNRISE + 1200000), sunset: new Date(SUNSET + 1200000) },
        ]);
        const { chart, ctx } = makeChart({ left: 0, right: 2400 });
        daylightPlugin.beforeDatasetsDraw(chart);
        // Each pair contributes up to 2 rects
        expect(ctx.fillRect.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
});
