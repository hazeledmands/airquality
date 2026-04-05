import { parseTimeExpr, resolveRange } from './timemath.js';

export const QUICK_RANGES = [
    { label: 'Last 5 minutes',  from: 'now-5m',  to: 'now' },
    { label: 'Last 15 minutes', from: 'now-15m', to: 'now' },
    { label: 'Last 30 minutes', from: 'now-30m', to: 'now' },
    { label: 'Last 1 hour',     from: 'now-1h',  to: 'now' },
    { label: 'Last 3 hours',    from: 'now-3h',  to: 'now' },
    { label: 'Last 6 hours',    from: 'now-6h',  to: 'now' },
    { label: 'Last 12 hours',   from: 'now-12h', to: 'now' },
    { label: 'Last 24 hours',   from: 'now-24h', to: 'now' },
    { label: 'Last 2 days',     from: 'now-2d',  to: 'now' },
    { label: 'Last 7 days',     from: 'now-7d',  to: 'now' },
    { label: 'Last 30 days',    from: 'now-30d', to: 'now' },
    { label: 'Last 90 days',    from: 'now-90d', to: 'now' },
];

const DEFAULT_FROM = 'now-3d';
const DEFAULT_TO   = 'now';

const DATE_PARAMS = new Set(['from', 'to']);

function readURL() {
    const p = new URLSearchParams(location.search);
    return { from: p.get('from'), to: p.get('to') };
}

function writeURL(fromExpr, toExpr) {
    const url = new URL(location.href);
    for (const k of DATE_PARAMS) url.searchParams.delete(k);
    if (fromExpr) url.searchParams.set('from', fromExpr);
    if (toExpr)   url.searchParams.set('to',   toExpr);
    history.replaceState(null, '', url);
}

// Returns a human-readable label for the given expression pair.
// Matches against QUICK_RANGES first; falls back to formatted absolute dates.
export function formatRangeLabel(fromExpr, toExpr, now = new Date()) {
    const match = QUICK_RANGES.find(r => r.from === fromExpr && r.to === toExpr);
    if (match) return match.label;

    const from = parseTimeExpr(fromExpr, now);
    const to   = parseTimeExpr(toExpr,   now);
    if (!from || !to) return `${fromExpr} \u2013 ${toExpr}`;

    const fmt = (d) => d.toLocaleString('en', {
        month: 'short', day: 'numeric',
        hour: 'numeric', minute: '2-digit',
    });
    return `${fmt(from)} \u2013 ${fmt(to)}`;
}

// Sets up the time range picker.
//
// Expects the following elements in the DOM:
//   #trp-trigger   — button showing the current range label
//   #trp-popover   — the dropdown panel
//   #trp-quick     — container where quick-range buttons are injected
//   #trpFrom       — text input for the "from" expression
//   #trpTo         — text input for the "to" expression
//   #trpApply      — apply button inside the popover
//   #trp-prev      — shift range backwards
//   #trp-next      — shift range forwards
//
// Returns { getCurrentRange, applyRange }.
export function setupTimeRangePicker({ onApply }) {
    const trigger  = document.getElementById('trp-trigger');
    const popover  = document.getElementById('trp-popover');
    const quickDiv = document.getElementById('trp-quick');
    const fromInput = document.getElementById('trpFrom');
    const toInput   = document.getElementById('trpTo');
    const applyBtn  = document.getElementById('trpApply');
    const prevBtn   = document.getElementById('trp-prev');
    const nextBtn   = document.getElementById('trp-next');

    // Current state — stored as expressions, resolved on demand
    let fromExpr = DEFAULT_FROM;
    let toExpr   = DEFAULT_TO;

    function updateTrigger() {
        trigger.textContent = formatRangeLabel(fromExpr, toExpr);
    }

    function applyRange(newFrom, newTo) {
        fromExpr = newFrom;
        toExpr   = newTo;
        fromInput.value = fromExpr;
        toInput.value   = toExpr;
        updateTrigger();
        writeURL(fromExpr, toExpr);
        onApply();
    }

    function getCurrentRange() {
        const now = new Date();
        const { from, to } = resolveRange(fromExpr, toExpr, now);
        return {
            fromMs: from ? from.getTime() : now.getTime() - 3 * 86_400_000,
            toMs:   to   ? to.getTime()   : now.getTime(),
        };
    }

    // Restore state from URL, or fall back to defaults
    const saved = readURL();
    if (saved.from && parseTimeExpr(saved.from)) fromExpr = saved.from;
    if (saved.to   && parseTimeExpr(saved.to))   toExpr   = saved.to;

    fromInput.value = fromExpr;
    toInput.value   = toExpr;
    updateTrigger();

    // Build quick-range buttons
    for (const qr of QUICK_RANGES) {
        const btn = document.createElement('button');
        btn.textContent = qr.label;
        btn.addEventListener('click', () => {
            closePopover();
            applyRange(qr.from, qr.to);
        });
        quickDiv.appendChild(btn);
    }

    // Toggle popover
    function openPopover()  { popover.style.display = 'block'; }
    function closePopover() { popover.style.display = 'none';  }

    trigger.addEventListener('click', (e) => {
        e.stopPropagation();
        popover.style.display === 'none' ? openPopover() : closePopover();
    });

    // Apply button inside popover
    applyBtn.addEventListener('click', () => {
        closePopover();
        applyRange(fromInput.value.trim(), toInput.value.trim());
    });

    // Enter key in inputs
    [fromInput, toInput].forEach(el => {
        el.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                closePopover();
                applyRange(fromInput.value.trim(), toInput.value.trim());
            }
        });
    });

    // Prev / Next: shift the range by its current span
    prevBtn.addEventListener('click', () => {
        const { fromMs, toMs } = getCurrentRange();
        const span = toMs - fromMs;
        applyRange(String(fromMs - span), String(toMs - span));
    });

    nextBtn.addEventListener('click', () => {
        const { fromMs, toMs } = getCurrentRange();
        const span = toMs - fromMs;
        applyRange(String(fromMs + span), String(toMs + span));
    });

    // Close popover on outside click
    document.addEventListener('click', (e) => {
        if (popover.style.display === 'none') return;
        if (!popover.contains(e.target) && e.target !== trigger) {
            closePopover();
        }
    });

    return { getCurrentRange, applyRange };
}
