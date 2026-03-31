// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { dragZoomPlugin, registerChart, unregisterChart, setZoomCallback, getDragState, registerHeatmap, unregisterHeatmap } from '../drag-zoom.js';

const HEATMAP_LABEL_W = 38;
const HEATMAP_WIDTH = 400;

function makeHeatmapCanvas() {
    const canvas = document.createElement('canvas');
    canvas.getBoundingClientRect = () => ({ left: 0, top: 0, width: HEATMAP_WIDTH, height: 72 });
    return canvas;
}

function makeHeatmapCallbacks(xMin, xMax) {
    const plotW = HEATMAP_WIDTH - HEATMAP_LABEL_W;
    const toTime = (cssX) => {
        if (cssX < HEATMAP_LABEL_W || cssX > HEATMAP_WIDTH) return null;
        return xMin + ((cssX - HEATMAP_LABEL_W) / plotW) * (xMax - xMin);
    };
    const toPx = (t) => HEATMAP_LABEL_W + ((t - xMin) / (xMax - xMin)) * plotW;
    return { toTime, toPx };
}

function makeCtx() {
    return {
        save: vi.fn(),
        restore: vi.fn(),
        fillRect: vi.fn(),
        strokeRect: vi.fn(),
        fillStyle: '',
        strokeStyle: '',
        lineWidth: 1,
    };
}

// In jsdom, getBoundingClientRect returns {left:0, top:0, ...} by default,
// so clientX maps directly to chart-pixel coords — no offset needed.
function makeChart(xMin, xMax, width = 1200) {
    const ctx = makeCtx();
    const canvas = document.createElement('canvas');
    const xScale = {
        min: xMin,
        max: xMax,
        getPixelForValue: (t) => ((t - xMin) / (xMax - xMin)) * width,
        getValueForPixel: (px) => xMin + (px / width) * (xMax - xMin),
    };
    const chart = {
        id: Math.random(),
        ctx,
        canvas,
        chartArea: { top: 0, bottom: 400, left: 0, right: width },
        scales: { x: xScale },
        draw: vi.fn(),
    };
    return { chart, ctx, xScale };
}

function mousedown(chart, clientX) {
    chart.canvas.dispatchEvent(new MouseEvent('mousedown', { clientX, bubbles: true }));
}
function mousemove(clientX) {
    document.dispatchEvent(new MouseEvent('mousemove', { clientX }));
}
function mouseup(clientX) {
    document.dispatchEvent(new MouseEvent('mouseup', { clientX }));
}

describe('dragZoomPlugin', () => {
    beforeEach(() => {
        setZoomCallback(null);
    });

    describe('zoom callback', () => {
        it('calls onZoom with correct time range when drag exceeds threshold', () => {
            const { chart, xScale } = makeChart(0, 1000);
            registerChart(chart);
            const onZoom = vi.fn();
            setZoomCallback(onZoom);

            mousedown(chart, 100);
            mousemove(400);
            mouseup(400);

            expect(onZoom).toHaveBeenCalledOnce();
            expect(onZoom).toHaveBeenCalledWith({
                min: xScale.getValueForPixel(100),
                max: xScale.getValueForPixel(400),
            });

            unregisterChart(chart);
        });

        it('produces min < max regardless of drag direction (right-to-left)', () => {
            const { chart, xScale } = makeChart(0, 1000);
            registerChart(chart);
            const onZoom = vi.fn();
            setZoomCallback(onZoom);

            mousedown(chart, 800);
            mouseup(200);

            expect(onZoom).toHaveBeenCalledWith({
                min: xScale.getValueForPixel(200),
                max: xScale.getValueForPixel(800),
            });

            unregisterChart(chart);
        });

        it('does not call onZoom for drags shorter than the threshold', () => {
            const { chart } = makeChart(0, 1000);
            registerChart(chart);
            const onZoom = vi.fn();
            setZoomCallback(onZoom);

            mousedown(chart, 100);
            mouseup(103); // 3px — below 5px threshold

            expect(onZoom).not.toHaveBeenCalled();

            unregisterChart(chart);
        });

        it('does not call onZoom when no callback is set', () => {
            const { chart } = makeChart(0, 1000);
            registerChart(chart);
            // onZoomCallback is null (set in beforeEach)

            // Should not throw
            mousedown(chart, 100);
            mouseup(600);

            unregisterChart(chart);
        });

        it('does not respond to drags on unregistered charts', () => {
            const { chart } = makeChart(0, 1000);
            // Intentionally NOT registering
            const onZoom = vi.fn();
            setZoomCallback(onZoom);

            mousedown(chart, 100);
            mouseup(600);

            expect(onZoom).not.toHaveBeenCalled();
        });

        it('does not start a drag when mousedown is outside chart area', () => {
            const { chart } = makeChart(0, 1000);
            registerChart(chart);
            const onZoom = vi.fn();
            setZoomCallback(onZoom);

            // clientX 1300 is beyond chartArea.right (1200)
            mousedown(chart, 1300);
            mouseup(600);

            expect(onZoom).not.toHaveBeenCalled();

            unregisterChart(chart);
        });
    });

    describe('cross-chart sync during drag', () => {
        it('redraws sibling charts on mousemove while dragging', () => {
            const { chart: c1 } = makeChart(0, 1000);
            const { chart: c2 } = makeChart(0, 1000);
            registerChart(c1);
            registerChart(c2);

            mousedown(c1, 100);
            mousemove(400);

            expect(c2.draw).toHaveBeenCalled();

            // Clean up
            mouseup(400);
            unregisterChart(c1);
            unregisterChart(c2);
        });

        it('redraws sibling charts on mouseup to clear their overlays', () => {
            const { chart: c1 } = makeChart(0, 1000);
            const { chart: c2 } = makeChart(0, 1000);
            registerChart(c1);
            registerChart(c2);

            mousedown(c1, 100);
            mouseup(600);

            expect(c2.draw).toHaveBeenCalled();

            unregisterChart(c1);
            unregisterChart(c2);
        });
    });

    describe('afterDatasetsDraw — selection overlay', () => {
        it('draws no overlay when not dragging', () => {
            const { chart, ctx } = makeChart(0, 1000);
            registerChart(chart);

            dragZoomPlugin.afterDatasetsDraw(chart);

            expect(ctx.fillRect).not.toHaveBeenCalled();

            unregisterChart(chart);
        });

        it('draws an overlay rectangle during a drag', () => {
            const { chart, ctx } = makeChart(0, 1000);
            registerChart(chart);

            mousedown(chart, 200);
            mousemove(700);

            dragZoomPlugin.afterDatasetsDraw(chart);

            expect(ctx.fillRect).toHaveBeenCalled();
            expect(ctx.strokeRect).toHaveBeenCalled();

            // Clean up
            mouseup(700);
            unregisterChart(chart);
        });

        it('draws overlay on sibling charts at the same time range', () => {
            const { chart: c1 } = makeChart(0, 1000);
            const { chart: c2, ctx: ctx2 } = makeChart(0, 1000);
            registerChart(c1);
            registerChart(c2);

            mousedown(c1, 200);
            mousemove(700);

            ctx2.fillRect.mockClear();
            dragZoomPlugin.afterDatasetsDraw(c2);

            expect(ctx2.fillRect).toHaveBeenCalled();

            // Clean up
            mouseup(700);
            unregisterChart(c1);
            unregisterChart(c2);
        });

        it('draws no overlay on unregistered charts', () => {
            const { chart: c1 } = makeChart(0, 1000);
            const { chart: c2, ctx: ctx2 } = makeChart(0, 1000);
            registerChart(c1);
            // c2 not registered

            mousedown(c1, 200);
            mousemove(700);

            dragZoomPlugin.afterDatasetsDraw(c2);

            expect(ctx2.fillRect).not.toHaveBeenCalled();

            // Clean up
            mouseup(700);
            unregisterChart(c1);
        });

        it('clears overlay after drag completes', () => {
            const { chart, ctx } = makeChart(0, 1000);
            registerChart(chart);

            mousedown(chart, 200);
            mousemove(700);
            mouseup(700);

            ctx.fillRect.mockClear();
            dragZoomPlugin.afterDatasetsDraw(chart);

            expect(ctx.fillRect).not.toHaveBeenCalled();

            unregisterChart(chart);
        });
    });
});

describe('getDragState', () => {
    it('returns null when no drag is active', () => {
        expect(getDragState()).toBeNull();
    });
});

describe('registerHeatmap / unregisterHeatmap', () => {
    beforeEach(() => { setZoomCallback(null); });

    it('calls onRedraw when a registered chart is dragged', () => {
        const { chart } = makeChart(0, 1000);
        registerChart(chart);
        const canvas = makeHeatmapCanvas();
        const { toTime, toPx } = makeHeatmapCallbacks(0, 1000);
        const onRedraw = vi.fn();
        registerHeatmap(canvas, toTime, toPx, onRedraw);

        mousedown(chart, 100);
        mousemove(400);

        expect(onRedraw).toHaveBeenCalled();

        mouseup(400);
        unregisterChart(chart);
        unregisterHeatmap(canvas);
    });

    it('starts a drag from the heatmap and fires the zoom callback', () => {
        const canvas = makeHeatmapCanvas();
        const { toTime, toPx } = makeHeatmapCallbacks(0, 1000);
        const onZoom = vi.fn();
        setZoomCallback(onZoom);
        registerHeatmap(canvas, toTime, toPx, vi.fn());

        canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 50, bubbles: true }));
        document.dispatchEvent(new MouseEvent('mousemove', { clientX: 200 }));
        document.dispatchEvent(new MouseEvent('mouseup', { clientX: 200 }));

        expect(onZoom).toHaveBeenCalled();

        unregisterHeatmap(canvas);
    });

    it('does not start drag when mousedown is in the label area', () => {
        const canvas = makeHeatmapCanvas();
        const { toTime, toPx } = makeHeatmapCallbacks(0, 1000);
        const onZoom = vi.fn();
        setZoomCallback(onZoom);
        registerHeatmap(canvas, toTime, toPx, vi.fn());

        canvas.dispatchEvent(new MouseEvent('mousedown', { clientX: 20, bubbles: true })); // 20 < LABEL_W=38
        document.dispatchEvent(new MouseEvent('mouseup', { clientX: 200 }));

        expect(onZoom).not.toHaveBeenCalled();

        unregisterHeatmap(canvas);
    });

    it('stops calling onRedraw after unregisterHeatmap', () => {
        const { chart } = makeChart(0, 1000);
        registerChart(chart);
        const canvas = makeHeatmapCanvas();
        const { toTime, toPx } = makeHeatmapCallbacks(0, 1000);
        const onRedraw = vi.fn();
        registerHeatmap(canvas, toTime, toPx, onRedraw);
        unregisterHeatmap(canvas);

        mousedown(chart, 100);
        mousemove(400);

        expect(onRedraw).not.toHaveBeenCalled();

        mouseup(400);
        unregisterChart(chart);
    });
});
