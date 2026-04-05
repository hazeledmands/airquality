// Parses Grafana-style relative time expressions into Date objects.

const UNIT_MS = {
    m: 60_000,
    h: 3_600_000,
    d: 86_400_000,
    w: 7 * 86_400_000,
};

// Pattern: "now" with optional offset (e.g. -12h) and optional floor (e.g. /d)
const REL_RE = /^now(?:([+-])(\d+)(m|h|d|w))?(?:\/(m|h|d))?$/;

// Minimum digit count to treat a numeric string as a Unix ms timestamp.
const MIN_TS_DIGITS = 11;

// Parses a time expression relative to `now` (defaults to current time).
// Supported forms:
//   now               → now
//   now-12h           → 12 hours ago
//   now-7d            → 7 days ago
//   now/d             → start of the current UTC day
//   now-1d/d          → start of yesterday (UTC)
//   <unix ms>         → absolute ms timestamp
//   <ISO date string> → parsed via Date constructor
//
// Returns a Date, or null if the expression is not recognised.
export function parseTimeExpr(expr, now = new Date()) {
    if (!expr) return null;

    const relMatch = expr.match(REL_RE);
    if (relMatch) {
        const [, sign, amount, unit, floor] = relMatch;
        let t = now.getTime();

        if (sign && amount && unit) {
            const delta = parseInt(amount, 10) * UNIT_MS[unit];
            t = sign === '-' ? t - delta : t + delta;
        }

        if (floor) {
            const d = new Date(t);
            if (floor === 'd') {
                t = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate());
            } else if (floor === 'h') {
                t = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCHours());
            } else if (floor === 'm') {
                t = Date.UTC(d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCHours(), d.getUTCMinutes());
            }
        }

        return new Date(t);
    }

    // Unix ms timestamp (long enough to be a real timestamp)
    if (/^\d+$/.test(expr) && expr.length >= MIN_TS_DIGITS) {
        return new Date(parseInt(expr, 10));
    }

    // Reject short pure-numeric strings — they're not valid timestamps
    if (/^\d+$/.test(expr)) return null;

    // ISO or other date string
    const d = new Date(expr);
    if (!isNaN(d.getTime())) return d;

    return null;
}

export function isRelativeExpr(expr) {
    return typeof expr === 'string' && expr.startsWith('now');
}

export function resolveRange(fromExpr, toExpr, now = new Date()) {
    return {
        from: parseTimeExpr(fromExpr, now),
        to:   parseTimeExpr(toExpr,   now),
    };
}
