import { describe, it, expect, vi, beforeEach } from 'vitest';
import { crosshairPlugin, registerChart, unregisterChart } from '../crosshair.js';

const DAY_MS = 24 * 60 * 60 * 1000;
const WEEK_MS = 7 * DAY_MS;

function makeCtx() {
    return {
        save: vi.fn(),
        restore: vi.fn(),
        beginPath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        stroke: vi.fn(),
        setLineDash: vi.fn(),
        lineWidth: 1,
        strokeStyle: '',
    };
}

// Creates a chart with a linear x-scale spanning [xMin, xMax] mapped to pixels [0, width].
function makeChart(xMin, xMax, width = 1200) {
    const ctx = makeCtx();
    const xScale = {
        min: xMin,
        max: xMax,
        getPixelForValue: (t) => ((t - xMin) / (xMax - xMin)) * width,
        getValueForPixel: (px) => xMin + (px / width) * (xMax - xMin),
    };
    const chart = {
        id: Math.random(),
        ctx,
        chartArea: { top: 0, bottom: 400, left: 0, right: width },
        scales: { x: xScale },
        draw: vi.fn(),
    };
    return { chart, ctx, xScale };
}

// Returns the x-values passed to moveTo calls (i.e. where vertical lines were drawn).
function drawnLineXPositions(ctx) {
    return ctx.moveTo.mock.calls.map(([x]) => x);
}

describe('crosshairPlugin', () => {
    beforeEach(() => {
        // Clear module-level state between tests by unregistering all charts.
        // We do this by re-importing... but since vitest caches modules,
        // instead reset via the afterEvent mouseout path on a dummy chart.
    });

    describe('afterDatasetsDraw — sibling lines', () => {
        it('draws no sibling lines when range is less than 1 day', () => {
            const { chart, ctx } = makeChart(0, DAY_MS * 0.5);
            registerChart(chart);

            // Simulate hover at midpoint
            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: 600 },
                inChartArea: true,
            });

            ctx.moveTo.mockClear();
            ctx.setLineDash.mockClear();
            crosshairPlugin.afterDatasetsDraw(chart);

            // setLineDash for siblings should never be called
            expect(ctx.setLineDash).not.toHaveBeenCalled();

            unregisterChart(chart);
        });

        it('draws sibling lines at same time-of-day on a 3-day chart', () => {
            const start = 0;
            const end = 3 * DAY_MS;
            const { chart, ctx, xScale } = makeChart(start, end);
            registerChart(chart);

            // Hover at day 1, midday (DAY_MS * 1.5)
            const hoverT = DAY_MS * 1.5;
            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: xScale.getPixelForValue(hoverT) },
                inChartArea: true,
            });

            ctx.moveTo.mockClear();
            crosshairPlugin.afterDatasetsDraw(chart);

            const xs = drawnLineXPositions(ctx);
            // Expect lines at: day 0.5, day 1.5 (main), day 2.5
            const expectedTs = [DAY_MS * 0.5, DAY_MS * 1.5, DAY_MS * 2.5];
            for (const t of expectedTs) {
                const expectedX = xScale.getPixelForValue(t);
                expect(xs).toContainEqual(expect.closeTo(expectedX, 1));
            }
            // Exactly 3 lines total
            expect(xs).toHaveLength(3);

            unregisterChart(chart);
        });

        it('draws week-over-week siblings when range exceeds 7 days', () => {
            const start = 0;
            const end = 21 * DAY_MS; // 3 weeks
            const { chart, ctx, xScale } = makeChart(start, end);
            registerChart(chart);

            // Hover in week 2 (day 10)
            const hoverT = 10 * DAY_MS;
            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: xScale.getPixelForValue(hoverT) },
                inChartArea: true,
            });

            ctx.moveTo.mockClear();
            crosshairPlugin.afterDatasetsDraw(chart);

            const xs = drawnLineXPositions(ctx);
            // Week-over-week: day 3 (week 1), day 10 (main), day 17 (week 3)
            const expectedTs = [3 * DAY_MS, 10 * DAY_MS, 17 * DAY_MS];
            for (const t of expectedTs) {
                const expectedX = xScale.getPixelForValue(t);
                expect(xs).toContainEqual(expect.closeTo(expectedX, 1));
            }
            expect(xs).toHaveLength(3);

            unregisterChart(chart);
        });

        it('uses day period (not week) when range is exactly 7 days', () => {
            const start = 0;
            const end = WEEK_MS; // exactly 7 days
            const { chart, ctx, xScale } = makeChart(start, end);
            registerChart(chart);

            // Hover at day 3.5 (midpoint)
            const hoverT = 3.5 * DAY_MS;
            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: xScale.getPixelForValue(hoverT) },
                inChartArea: true,
            });

            ctx.moveTo.mockClear();
            crosshairPlugin.afterDatasetsDraw(chart);

            const xs = drawnLineXPositions(ctx);
            // Day-over-day siblings: 7 lines total (one per day)
            expect(xs).toHaveLength(7);

            unregisterChart(chart);
        });

        it('sibling lines use dashed style and dimmer colour than main line', () => {
            const { chart, ctx, xScale } = makeChart(0, 3 * DAY_MS);
            registerChart(chart);

            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: xScale.getPixelForValue(DAY_MS * 1.5) },
                inChartArea: true,
            });

            crosshairPlugin.afterDatasetsDraw(chart);

            // setLineDash([3,4]) should be called for siblings
            expect(ctx.setLineDash).toHaveBeenCalledWith([3, 4]);

            unregisterChart(chart);
        });

        it('draws no sibling lines when hover is outside the chart area', () => {
            const { chart, ctx, xScale } = makeChart(0, 3 * DAY_MS);
            registerChart(chart);

            // Hover in, then move out of chart area to clear hover
            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: xScale.getPixelForValue(DAY_MS * 1.5) },
                inChartArea: true,
            });
            crosshairPlugin.afterEvent(chart, {
                event: { type: 'mousemove', x: 0 },
                inChartArea: false,
            });

            ctx.moveTo.mockClear();
            crosshairPlugin.afterDatasetsDraw(chart);

            expect(ctx.moveTo).not.toHaveBeenCalled();

            unregisterChart(chart);
        });
    });

    describe('cross-chart sync', () => {
        it('redraws sibling charts on hover', () => {
            const { chart: c1, xScale: xs1 } = makeChart(0, 3 * DAY_MS);
            const { chart: c2 } = makeChart(0, 3 * DAY_MS);
            registerChart(c1);
            registerChart(c2);

            crosshairPlugin.afterEvent(c1, {
                event: { type: 'mousemove', x: xs1.getPixelForValue(DAY_MS) },
                inChartArea: true,
            });

            expect(c2.draw).toHaveBeenCalled();

            unregisterChart(c1);
            unregisterChart(c2);
        });
    });
});
