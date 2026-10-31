import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
    filterAnnotations,
    findAnnotationAtPixel,
    annotationsPlugin,
    setAnnotations,
    setAnnotationsEnabled,
    setupAnnotationUI,
    setAnnotationsEditable,
    fillTooltip,
} from '../annotations.js';

// ---- helpers ----

function makeCtx() {
    return {
        save: vi.fn(),
        restore: vi.fn(),
        beginPath: vi.fn(),
        closePath: vi.fn(),
        moveTo: vi.fn(),
        lineTo: vi.fn(),
        stroke: vi.fn(),
        fill: vi.fn(),
        fillText: vi.fn(),
        setLineDash: vi.fn(),
        strokeStyle: '',
        lineWidth: 0,
        fillStyle: '',
        font: '',
    };
}

function makeChart(minMs, maxMs) {
    const ctx = makeCtx();
    const chart = {
        ctx,
        chartArea: { top: 0, bottom: 400, left: 0, right: 1000 },
        scales: {
            x: {
                getPixelForValue: (ms) => ((ms - minMs) / (maxMs - minMs)) * 1000,
                getValueForPixel: (px) => minMs + (px / 1000) * (maxMs - minMs),
                min: minMs,
                max: maxMs,
            },
        },
    };
    return { chart, ctx };
}

// ---- filterAnnotations ----

describe('filterAnnotations', () => {
    const annotations = [
        { id: 1, timestamp: '2026-03-01T10:00:00Z', note: 'early' },
        { id: 2, timestamp: '2026-03-01T12:00:00Z', note: 'mid' },
        { id: 3, timestamp: '2026-03-01T14:00:00Z', note: 'late' },
    ];
    const from = new Date('2026-03-01T11:00:00Z').getTime();
    const to   = new Date('2026-03-01T13:00:00Z').getTime();

    it('returns only annotations within [from, to)', () => {
        const result = filterAnnotations(annotations, from, to);
        expect(result).toHaveLength(1);
        expect(result[0].id).toBe(2);
    });

    it('includes the from boundary', () => {
        const fromExact = new Date('2026-03-01T10:00:00Z').getTime();
        const result = filterAnnotations(annotations, fromExact, to);
        expect(result.map(a => a.id)).toContain(1);
    });

    it('excludes the to boundary', () => {
        const toExact = new Date('2026-03-01T12:00:00Z').getTime();
        const result = filterAnnotations(annotations, from, toExact);
        expect(result.map(a => a.id)).not.toContain(2);
    });

    it('returns empty array when none match', () => {
        expect(filterAnnotations(annotations, 0, 1)).toHaveLength(0);
    });

    it('returns empty array for empty input', () => {
        expect(filterAnnotations([], from, to)).toHaveLength(0);
    });
});

// ---- findAnnotationAtPixel ----

describe('findAnnotationAtPixel', () => {
    const T  = new Date('2026-03-01T12:00:00Z').getTime();
    const T2 = new Date('2026-03-01T13:00:00Z').getTime();
    const annotations = [
        { id: 1, timestamp: new Date(T).toISOString(),  note: 'first'  },
        { id: 2, timestamp: new Date(T2).toISOString(), note: 'second' },
    ];

    it('returns null for empty annotations', () => {
        const { chart } = makeChart(T, T2);
        expect(findAnnotationAtPixel(chart, 500, [])).toBeNull();
    });

    it('returns null when no annotation is within threshold', () => {
        // T at x=0, T2 at x=1000 — clicking at x=500 is 500px from both
        const { chart } = makeChart(T, T2);
        expect(findAnnotationAtPixel(chart, 500, annotations, 8)).toBeNull();
    });

    it('returns annotation within threshold', () => {
        // T at x=0; click at x=3 (3px away, within default threshold 8)
        const { chart } = makeChart(T, T2);
        const result = findAnnotationAtPixel(chart, 3, annotations, 8);
        expect(result?.id).toBe(1);
    });

    it('returns null when click is outside threshold', () => {
        // T at x=0; click at x=20 (20px away, beyond threshold 8)
        const { chart } = makeChart(T, T2);
        expect(findAnnotationAtPixel(chart, 20, annotations, 8)).toBeNull();
    });

    it('returns the closest annotation when multiple are near', () => {
        // Both T at x=0 and T2 at x=1000, click at x=998 (2px from T2)
        const { chart } = makeChart(T, T2);
        const result = findAnnotationAtPixel(chart, 998, annotations, 8);
        expect(result?.id).toBe(2);
    });
});

// ---- annotationsPlugin ----

function makeChartWithRoom(minMs, maxMs, room) {
    const { chart, ctx } = makeChart(minMs, maxMs);
    chart.options = { plugins: { annotations: { room } } };
    return { chart, ctx };
}

describe('annotationsPlugin', () => {
    beforeEach(() => {
        setAnnotations([]);
        setAnnotationsEnabled(true);
    });

    it('has id "annotations"', () => {
        expect(annotationsPlugin.id).toBe('annotations');
    });

    it('does not draw when disabled', () => {
        setAnnotationsEnabled(false);
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test', room: '' }]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('does not draw when annotations array is empty', () => {
        setAnnotations([]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('draws a room annotation on its own chart', () => {
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test', room: 'Bedroom' }]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).toHaveBeenCalledTimes(2); // line + triangle
        expect(ctx.lineTo).toHaveBeenCalled();
    });

    it('does not draw a room annotation on a different chart', () => {
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test', room: 'Bedroom' }]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Office');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('draws a global annotation (empty room) on any chart', () => {
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test', room: '' }]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Office');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).toHaveBeenCalled();
    });

    it('draws at the correct x pixel', () => {
        setAnnotations([{ id: 1, timestamp: new Date(500000).toISOString(), note: 'test', room: 'Bedroom' }]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).toHaveBeenCalledWith(500, 0);
        expect(ctx.lineTo).toHaveBeenCalledWith(500, 400);
    });

    it('skips annotations outside chart bounds', () => {
        setAnnotations([{ id: 1, timestamp: new Date(-100000).toISOString(), note: 'outside', room: 'Bedroom' }]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).not.toHaveBeenCalled();
    });

    it('draws multiple annotations for the same room', () => {
        setAnnotations([
            { id: 1, timestamp: new Date(300000).toISOString(), note: 'first',  room: 'Bedroom' },
            { id: 2, timestamp: new Date(700000).toISOString(), note: 'second', room: 'Bedroom' },
        ]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).toHaveBeenCalledTimes(4); // 2 per annotation
    });

    it('only draws annotations matching the chart room', () => {
        setAnnotations([
            { id: 1, timestamp: new Date(300000).toISOString(), note: 'bedroom', room: 'Bedroom' },
            { id: 2, timestamp: new Date(700000).toISOString(), note: 'office',  room: 'Office' },
        ]);
        const { chart, ctx } = makeChartWithRoom(0, 1000000, 'Bedroom');
        annotationsPlugin.afterDraw(chart);
        expect(ctx.moveTo).toHaveBeenCalledTimes(2); // only the Bedroom annotation
    });
});

// ---- setupAnnotationUI ----

function makeCanvas() {
    const listeners = {};
    return {
        addEventListener: vi.fn((type, fn) => { listeners[type] = fn; }),
        removeEventListener: vi.fn(),
        style: {},
        getBoundingClientRect: () => ({ left: 0, top: 0 }),
        _listeners: listeners,
    };
}

describe('setupAnnotationUI', () => {
    let addedToDoc, removedFromDoc;

    beforeEach(() => {
        addedToDoc = [];
        removedFromDoc = [];
        vi.stubGlobal('document', {
            createElement: vi.fn(() => ({
                style: { cssText: '' },
                innerHTML: '',
                appendChild: vi.fn(),
                querySelector: vi.fn(() => ({ style: {}, value: '', addEventListener: vi.fn() })),
            })),
            body: { appendChild: vi.fn() },
            addEventListener: vi.fn((type, fn, opts) => addedToDoc.push({ type, fn, opts })),
            removeEventListener: vi.fn((type, fn, opts) => removedFromDoc.push({ type, fn, opts })),
        });
    });

    afterEach(() => {
        vi.unstubAllGlobals();
    });

    it('returns a cleanup function', () => {
        const canvas = makeCanvas();
        const chart = { canvas, scales: {}, options: {} };
        const cleanup = setupAnnotationUI([{ chart }], { onSave: vi.fn(), onDelete: vi.fn() });
        expect(typeof cleanup).toBe('function');
    });

    it('registers click, mousemove, mouseleave on each canvas', () => {
        const canvas = makeCanvas();
        const chart = { canvas, scales: {}, options: {} };
        setupAnnotationUI([{ chart }], { onSave: vi.fn(), onDelete: vi.fn() });
        expect(canvas.addEventListener).toHaveBeenCalledWith('click', expect.any(Function));
        expect(canvas.addEventListener).toHaveBeenCalledWith('mousemove', expect.any(Function));
        expect(canvas.addEventListener).toHaveBeenCalledWith('mouseleave', expect.any(Function));
    });

    it('registers a document click listener for outside-click dismissal', () => {
        const canvas = makeCanvas();
        const chart = { canvas, scales: {}, options: {} };
        setupAnnotationUI([{ chart }], { onSave: vi.fn(), onDelete: vi.fn() });
        expect(addedToDoc.some(e => e.type === 'click')).toBe(true);
    });

    it('cleanup removes the document click listener', () => {
        const canvas = makeCanvas();
        const chart = { canvas, scales: {}, options: {} };
        const cleanup = setupAnnotationUI([{ chart }], { onSave: vi.fn(), onDelete: vi.fn() });
        cleanup();
        expect(removedFromDoc.some(e => e.type === 'click')).toBe(true);
    });

    it('cleanup removes canvas listeners', () => {
        const canvas = makeCanvas();
        const chart = { canvas, scales: {}, options: {} };
        const cleanup = setupAnnotationUI([{ chart }], { onSave: vi.fn(), onDelete: vi.fn() });
        cleanup();
        expect(canvas.removeEventListener).toHaveBeenCalledWith('click', expect.any(Function));
        expect(canvas.removeEventListener).toHaveBeenCalledWith('mousemove', expect.any(Function));
        expect(canvas.removeEventListener).toHaveBeenCalledWith('mouseleave', expect.any(Function));
    });

    it('cleanup removes stored handler references from canvas', () => {
        const canvas = makeCanvas();
        const chart = { canvas, scales: {}, options: {} };
        const cleanup = setupAnnotationUI([{ chart }], { onSave: vi.fn(), onDelete: vi.fn() });
        expect(canvas._annotationHandlers).toBeDefined();
        cleanup();
        expect(canvas._annotationHandlers).toBeUndefined();
    });
});

describe('setupAnnotationUI read-only', () => {
    let created;

    beforeEach(() => {
        created = [];
        vi.stubGlobal('document', {
            createElement: vi.fn(() => {
                const el = {
                    style: { cssText: '', display: '' },
                    innerHTML: '',
                    textContent: '',
                    children: [],
                    appendChild: vi.fn((c) => el.children.push(c)),
                    replaceChildren: vi.fn((...cs) => { el.children = cs; }),
                    querySelector: vi.fn(() => ({ style: {}, value: '', addEventListener: vi.fn() })),
                };
                created.push(el);
                return el;
            }),
            body: { appendChild: vi.fn() },
            addEventListener: vi.fn(),
            removeEventListener: vi.fn(),
        });
    });

    afterEach(() => {
        setAnnotationsEditable(true);
        setAnnotations([]);
        vi.unstubAllGlobals();
    });

    function setup() {
        const { chart } = makeChart(0, 1000);
        chart.canvas = makeCanvas();
        chart.options = {};
        const onSave = vi.fn();
        setupAnnotationUI([{ chart }], { onSave, onDelete: vi.fn() });
        return { canvas: chart.canvas };
    }

    it('does not open the add popover on empty space', () => {
        setAnnotationsEditable(false);
        const { canvas } = setup();
        const before = created.length;
        canvas._listeners.click({ clientX: 500, clientY: 10 });
        expect(created.length).toBe(before);
    });

});

describe('fillTooltip', () => {
    it('renders the note as text, not HTML', () => {
        const doc = { createElement: () => ({ style: {}, textContent: '' }) };
        const el = { innerHTML: '', replaceChildren: vi.fn() };
        fillTooltip(el, 'Oct 31', '<img src=x onerror=alert(1)>', doc);
        const [time, note] = el.replaceChildren.mock.calls[0];
        expect(time.textContent).toBe('Oct 31');
        expect(note.textContent).toBe('<img src=x onerror=alert(1)>');
        expect(el.innerHTML).toBe('');
    });
});
