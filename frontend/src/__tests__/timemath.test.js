import { describe, it, expect } from 'vitest';
import { parseTimeExpr, isRelativeExpr, resolveRange } from '../timemath.js';

// Fixed reference time used across all tests
const REF = new Date('2025-04-05T15:30:00.000Z');

describe('parseTimeExpr — "now"', () => {
    it('returns the reference time for "now"', () => {
        expect(parseTimeExpr('now', REF)).toEqual(REF);
    });
});

describe('parseTimeExpr — subtraction offsets', () => {
    it('subtracts minutes: now-30m', () => {
        expect(parseTimeExpr('now-30m', REF).getTime()).toBe(REF.getTime() - 30 * 60_000);
    });

    it('subtracts hours: now-1h', () => {
        expect(parseTimeExpr('now-1h', REF).getTime()).toBe(REF.getTime() - 3_600_000);
    });

    it('subtracts hours: now-12h', () => {
        expect(parseTimeExpr('now-12h', REF).getTime()).toBe(REF.getTime() - 12 * 3_600_000);
    });

    it('subtracts days: now-1d', () => {
        expect(parseTimeExpr('now-1d', REF).getTime()).toBe(REF.getTime() - 86_400_000);
    });

    it('subtracts days: now-7d', () => {
        expect(parseTimeExpr('now-7d', REF).getTime()).toBe(REF.getTime() - 7 * 86_400_000);
    });

    it('subtracts weeks: now-1w', () => {
        expect(parseTimeExpr('now-1w', REF).getTime()).toBe(REF.getTime() - 7 * 86_400_000);
    });

    it('subtracts weeks: now-2w', () => {
        expect(parseTimeExpr('now-2w', REF).getTime()).toBe(REF.getTime() - 14 * 86_400_000);
    });
});

describe('parseTimeExpr — floor operations', () => {
    it('floors to start of UTC day: now/d', () => {
        expect(parseTimeExpr('now/d', REF)).toEqual(new Date('2025-04-05T00:00:00.000Z'));
    });

    it('floors to start of UTC hour: now/h', () => {
        expect(parseTimeExpr('now/h', REF)).toEqual(new Date('2025-04-05T15:00:00.000Z'));
    });

    it('combines offset and floor: now-1d/d (start of yesterday)', () => {
        expect(parseTimeExpr('now-1d/d', REF)).toEqual(new Date('2025-04-04T00:00:00.000Z'));
    });

    it('combines offset and floor: now-7d/d', () => {
        expect(parseTimeExpr('now-7d/d', REF)).toEqual(new Date('2025-03-29T00:00:00.000Z'));
    });
});

describe('parseTimeExpr — absolute values', () => {
    it('parses a Unix millisecond timestamp string', () => {
        const ms = REF.getTime();
        expect(parseTimeExpr(String(ms), REF).getTime()).toBe(ms);
    });

    it('parses an ISO 8601 date-time string', () => {
        expect(parseTimeExpr('2025-01-01T00:00:00.000Z', REF))
            .toEqual(new Date('2025-01-01T00:00:00.000Z'));
    });

    it('parses a date-only string', () => {
        const result = parseTimeExpr('2025-01-01', REF);
        expect(result).not.toBeNull();
        expect(result.getTime()).toBe(new Date('2025-01-01').getTime());
    });
});

describe('parseTimeExpr — invalid input', () => {
    it('returns null for empty string', () => {
        expect(parseTimeExpr('', REF)).toBeNull();
    });

    it('returns null for null', () => {
        expect(parseTimeExpr(null, REF)).toBeNull();
    });

    it('returns null for an unrecognised string', () => {
        expect(parseTimeExpr('yesterday', REF)).toBeNull();
    });

    it('returns null for a short numeric string that is not a timestamp', () => {
        // Small integers aren't valid ms timestamps
        expect(parseTimeExpr('42', REF)).toBeNull();
    });
});

describe('isRelativeExpr', () => {
    it('returns true for "now"', () => expect(isRelativeExpr('now')).toBe(true));
    it('returns true for "now-12h"', () => expect(isRelativeExpr('now-12h')).toBe(true));
    it('returns true for "now-7d/d"', () => expect(isRelativeExpr('now-7d/d')).toBe(true));
    it('returns false for an ISO timestamp', () => expect(isRelativeExpr('2025-01-01T00:00:00Z')).toBe(false));
    it('returns false for a ms timestamp string', () => expect(isRelativeExpr('1736000000000')).toBe(false));
    it('returns false for empty string', () => expect(isRelativeExpr('')).toBe(false));
});

describe('resolveRange', () => {
    it('resolves a relative from/to pair', () => {
        const { from, to } = resolveRange('now-1h', 'now', REF);
        expect(to).toEqual(REF);
        expect(from.getTime()).toBe(REF.getTime() - 3_600_000);
    });

    it('resolves absolute ISO strings', () => {
        const { from, to } = resolveRange('2025-01-01T00:00:00Z', '2025-01-02T00:00:00Z', REF);
        expect(from).toEqual(new Date('2025-01-01T00:00:00Z'));
        expect(to).toEqual(new Date('2025-01-02T00:00:00Z'));
    });

    it('returns null from/to for invalid expressions', () => {
        const { from, to } = resolveRange('bad', 'worse', REF);
        expect(from).toBeNull();
        expect(to).toBeNull();
    });
});
