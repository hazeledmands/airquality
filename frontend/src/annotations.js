// annotations.js — persistent log-style chart annotations

let _annotations = [];
let _enabled = true;
let _editable = true;

// ---- State setters ----

export function setAnnotations(annotations) {
    _annotations = annotations || [];
}

export function setAnnotationsEnabled(enabled) {
    _enabled = enabled;
}

// When false, annotations are view-only: clicking empty chart space does
// nothing and the info popover has no Delete button.
export function setAnnotationsEditable(editable) {
    _editable = editable;
}

// ---- Pure helpers ----

// Returns annotations whose timestamp falls within [fromMs, toMs).
export function filterAnnotations(annotations, fromMs, toMs) {
    return annotations.filter(a => {
        const t = new Date(a.timestamp).getTime();
        return t >= fromMs && t < toMs;
    });
}

// Returns the annotation closest to pixel x on the chart's x-axis, or null if
// none falls within `threshold` pixels.
export function findAnnotationAtPixel(chart, x, annotations, threshold = 8) {
    let closest = null;
    let minDist = Infinity;
    for (const a of annotations) {
        const px = chart.scales.x.getPixelForValue(new Date(a.timestamp).getTime());
        const dist = Math.abs(px - x);
        if (dist < threshold && dist < minDist) {
            minDist = dist;
            closest = a;
        }
    }
    return closest;
}

// ---- Chart.js plugin ----

export const annotationsPlugin = {
    id: 'annotations',
    afterDraw(chart) {
        if (!_enabled || _annotations.length === 0) return;
        const { ctx, chartArea, scales } = chart;
        if (!scales.x) return;

        const chartRoom = chart.options?.plugins?.annotations?.room ?? '';
        const visible = _annotations.filter(a => !a.room || a.room === chartRoom);
        if (visible.length === 0) return;

        ctx.save();
        ctx.strokeStyle = 'rgba(251, 191, 36, 0.65)';
        ctx.lineWidth = 1.5;
        ctx.setLineDash([4, 4]);

        for (const a of visible) {
            const x = scales.x.getPixelForValue(new Date(a.timestamp).getTime());
            if (x < chartArea.left || x > chartArea.right) continue;
            ctx.beginPath();
            ctx.moveTo(x, chartArea.top);
            ctx.lineTo(x, chartArea.bottom);
            ctx.stroke();
        }

        // Small triangle markers at the top of each line
        ctx.setLineDash([]);
        ctx.fillStyle = 'rgba(251, 191, 36, 0.85)';
        for (const a of visible) {
            const x = scales.x.getPixelForValue(new Date(a.timestamp).getTime());
            if (x < chartArea.left || x > chartArea.right) continue;
            ctx.beginPath();
            ctx.moveTo(x - 5, chartArea.top);
            ctx.lineTo(x + 5, chartArea.top);
            ctx.lineTo(x, chartArea.top + 8);
            ctx.closePath();
            ctx.fill();
        }

        ctx.restore();
    },
};

// ---- UI (DOM) ----

let _popover = null;
let _tooltip = null;

function getPopover() {
    if (_popover) return _popover;
    _popover = document.createElement('div');
    _popover.style.cssText = [
        'display:none', 'position:fixed', 'z-index:1000',
        'background:#1e1e30', 'border:1px solid #333', 'border-radius:6px',
        'padding:12px', 'min-width:240px', 'box-shadow:0 4px 16px rgba(0,0,0,.4)',
    ].join(';');
    _popover.innerHTML = `
        <div class="ann-time" style="color:#888;font-size:11px;margin-bottom:8px"></div>
        <input class="ann-input" type="text" placeholder="Add note\u2026" style="
            width:100%;box-sizing:border-box;background:#0e0e1a;border:1px solid #333;
            border-radius:4px;color:#ccc;padding:6px 8px;font-size:13px;outline:none">
        <div style="display:flex;gap:8px;margin-top:8px;justify-content:flex-end">
            <button class="ann-cancel" style="
                background:transparent;border:1px solid #333;border-radius:4px;
                color:#888;padding:4px 12px;cursor:pointer;font-size:12px">Cancel</button>
            <button class="ann-save" style="
                background:rgba(251,191,36,.15);border:1px solid rgba(251,191,36,.35);
                border-radius:4px;color:rgba(251,191,36,.9);padding:4px 12px;
                cursor:pointer;font-size:12px">Save</button>
        </div>`;
    document.body.appendChild(_popover);
    return _popover;
}

// Fills the hover tooltip. Notes are user-entered and shown to every viewer,
// so they go in as text — never innerHTML.
export function fillTooltip(el, timeText, note, doc = document) {
    const time = doc.createElement('div');
    time.style.cssText = 'color:rgba(251,191,36,.9);font-size:11px;margin-bottom:4px';
    time.textContent = timeText;
    const body = doc.createElement('div');
    body.style.cssText = 'color:#ccc;font-size:13px';
    body.textContent = note;
    el.replaceChildren(time, body);
}

function getTooltip() {
    if (_tooltip) return _tooltip;
    _tooltip = document.createElement('div');
    _tooltip.style.cssText = [
        'display:none', 'position:fixed', 'z-index:1001',
        'background:#1e1e30', 'border:1px solid rgba(251,191,36,.3)', 'border-radius:6px',
        'padding:8px 12px', 'max-width:280px', 'box-shadow:0 4px 16px rgba(0,0,0,.4)',
        'pointer-events:none',
    ].join(';');
    document.body.appendChild(_tooltip);
    return _tooltip;
}

function hidePopover() {
    if (!_popover) return;
    _popover.style.display = 'none';
    const input = _popover.querySelector('.ann-input');
    if (input && input._onKey) {
        input.removeEventListener('keydown', input._onKey);
        input._onKey = null;
    }
}

function showAddPopover(screenX, screenY, isoTimestamp, room, onSave) {
    const popover = getPopover();
    popover.querySelector('.ann-time').textContent = new Date(isoTimestamp).toLocaleString();

    const input = popover.querySelector('.ann-input');
    input.value = '';

    // Position near click, staying on screen
    const pw = 260, ph = 110;
    let left = screenX + 10;
    let top  = screenY - ph / 2;
    if (left + pw > window.innerWidth)  left = screenX - pw - 10;
    if (top < 4)                         top  = 4;
    if (top + ph > window.innerHeight)   top  = window.innerHeight - ph - 4;
    popover.style.left = left + 'px';
    popover.style.top  = top  + 'px';
    popover.style.display = 'block';
    setTimeout(() => input.focus(), 0);

    // Replace buttons to drop stale listeners
    const oldSave   = popover.querySelector('.ann-save');
    const oldCancel = popover.querySelector('.ann-cancel');
    const newSave   = oldSave.cloneNode(true);
    const newCancel = oldCancel.cloneNode(true);
    // The info popover restyles this button as Delete (or hides it when
    // read-only); undo that, or the add popover inherits it.
    newSave.textContent = 'Save';
    newSave.style.display = '';
    newSave.style.background = 'rgba(251,191,36,.15)';
    newSave.style.border     = '1px solid rgba(251,191,36,.35)';
    newSave.style.color      = 'rgba(251,191,36,.9)';
    oldSave.replaceWith(newSave);
    oldCancel.replaceWith(newCancel);

    const doSave = async () => {
        const note = input.value.trim();
        if (!note) return;
        hidePopover();
        await onSave(isoTimestamp, note, room);
    };

    newSave.addEventListener('click', doSave);
    newCancel.addEventListener('click', hidePopover);

    if (input._onKey) input.removeEventListener('keydown', input._onKey);
    const onKey = (e) => {
        if (e.key === 'Enter')  doSave();
        if (e.key === 'Escape') hidePopover();
    };
    input._onKey = onKey;
    input.addEventListener('keydown', onKey);
}

function showInfoPopover(screenX, screenY, annotation, onDelete) {
    const popover = getPopover();
    popover.querySelector('.ann-time').textContent = new Date(annotation.timestamp).toLocaleString();

    const input = popover.querySelector('.ann-input');
    input.style.display = 'none';

    const oldSave   = popover.querySelector('.ann-save');
    const oldCancel = popover.querySelector('.ann-cancel');
    const newSave   = oldSave.cloneNode(true);
    const newCancel = oldCancel.cloneNode(true);
    newSave.style.display = onDelete ? '' : 'none';
    newSave.textContent = 'Delete';
    newSave.style.background = 'rgba(239,68,68,.15)';
    newSave.style.border      = '1px solid rgba(239,68,68,.35)';
    newSave.style.color       = 'rgba(239,68,68,.9)';
    oldSave.replaceWith(newSave);
    oldCancel.replaceWith(newCancel);

    // Note text above buttons
    let noteEl = popover.querySelector('.ann-note');
    if (!noteEl) {
        noteEl = document.createElement('div');
        noteEl.className = 'ann-note';
        noteEl.style.cssText = 'color:#ccc;font-size:13px;margin-bottom:8px';
        input.insertAdjacentElement('afterend', noteEl);
    }
    noteEl.textContent = annotation.note;
    noteEl.style.display = 'block';

    const pw = 260, ph = 120;
    let left = screenX + 10;
    let top  = screenY - ph / 2;
    if (left + pw > window.innerWidth) left = screenX - pw - 10;
    if (top < 4)                        top  = 4;
    if (top + ph > window.innerHeight)  top  = window.innerHeight - ph - 4;
    popover.style.left = left + 'px';
    popover.style.top  = top  + 'px';
    popover.style.display = 'block';

    if (onDelete) {
        newSave.addEventListener('click', async () => {
            hidePopover();
            await onDelete(annotation.id);
        });
    }
    newCancel.addEventListener('click', () => {
        hidePopover();
        // Restore input for next add
        input.style.display = '';
        if (noteEl) noteEl.style.display = 'none';
    });
}

// Sets up click (add/delete) and mousemove (tooltip) handlers on every chart canvas.
// Call this after building the chart DOM. `callbacks` is `{ onSave, onDelete }`.
// Returns a cleanup function — call it before rebuilding the DOM to remove all listeners.
export function setupAnnotationUI(charts, { onSave, onDelete }) {
    const tooltip = getTooltip();

    // Close popover on outside click
    const onDocClick = (e) => {
        if (_popover && _popover.style.display !== 'none' &&
            !_popover.contains(e.target) && e.target.tagName !== 'CANVAS') {
            hidePopover();
            const input = _popover.querySelector('.ann-input');
            if (input) input.style.display = '';
            const noteEl = _popover.querySelector('.ann-note');
            if (noteEl) noteEl.style.display = 'none';
        }
    };
    document.addEventListener('click', onDocClick, { capture: true });

    for (const { chart } of charts) {
        const canvas = chart.canvas;
        if (!canvas) continue;

        const onClick = (e) => {
            if (!chart.scales.x) return;
            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const { chartArea } = chart;
            if (x < chartArea.left || x > chartArea.right) return;

            const chartRoom = chart.options?.plugins?.annotations?.room ?? '';
            const visible = _annotations.filter(a => !a.room || a.room === chartRoom);
            const ann = findAnnotationAtPixel(chart, x, visible);
            if (ann) {
                showInfoPopover(e.clientX, e.clientY, ann, _editable ? onDelete : null);
            } else if (_editable) {
                const tsMs = chart.scales.x.getValueForPixel(x);
                showAddPopover(e.clientX, e.clientY, new Date(tsMs).toISOString(), chartRoom, onSave);
            }
        };

        const onMouseMove = (e) => {
            const rect = canvas.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const chartRoom = chart.options?.plugins?.annotations?.room ?? '';
            const visible = _annotations.filter(a => !a.room || a.room === chartRoom);
            const ann = findAnnotationAtPixel(chart, x, visible);

            if (ann) {
                const ts = new Date(ann.timestamp).toLocaleString();
                fillTooltip(tooltip, ts, ann.note);
                let left = e.clientX + 12;
                let top  = e.clientY - 40;
                if (left + 300 > window.innerWidth) left = e.clientX - 312;
                if (top < 0) top = 4;
                tooltip.style.left = left + 'px';
                tooltip.style.top  = top  + 'px';
                tooltip.style.display = 'block';
                canvas.style.cursor = 'pointer';
            } else {
                tooltip.style.display = 'none';
                canvas.style.cursor = '';
            }
        };

        const onMouseLeave = () => {
            tooltip.style.display = 'none';
            canvas.style.cursor = '';
        };

        canvas.addEventListener('click', onClick);
        canvas.addEventListener('mousemove', onMouseMove);
        canvas.addEventListener('mouseleave', onMouseLeave);

        canvas._annotationHandlers = { onClick, onMouseMove, onMouseLeave };
    }

    return function cleanup() {
        document.removeEventListener('click', onDocClick, { capture: true });
        for (const { chart } of charts) {
            const canvas = chart.canvas;
            if (!canvas || !canvas._annotationHandlers) continue;
            const { onClick, onMouseMove, onMouseLeave } = canvas._annotationHandlers;
            canvas.removeEventListener('click', onClick);
            canvas.removeEventListener('mousemove', onMouseMove);
            canvas.removeEventListener('mouseleave', onMouseLeave);
            delete canvas._annotationHandlers;
        }
    };
}
