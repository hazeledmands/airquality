import { describe, it, expect } from 'vitest';
import { DEFAULT_ORDER, parseOrder, serializeOrder } from '../reorder.js';

describe('DEFAULT_ORDER', () => {
    it('contains all five chart ids', () => {
        expect(DEFAULT_ORDER).toEqual(['temp', 'dewpoint', 'humidity', 'pm', 'acState']);
    });
});

describe('parseOrder', () => {
    it('returns default order when param is null', () => {
        expect(parseOrder(null)).toEqual(DEFAULT_ORDER);
    });

    it('returns default order when param is empty string', () => {
        expect(parseOrder('')).toEqual(DEFAULT_ORDER);
    });

    it('parses a comma-separated order string', () => {
        expect(parseOrder('humidity,temp,dewpoint,pm,acState'))
            .toEqual(['humidity', 'temp', 'dewpoint', 'pm', 'acState']);
    });

    it('ignores unknown chart ids', () => {
        expect(parseOrder('humidity,bogus,temp,dewpoint,pm,acState'))
            .toEqual(['humidity', 'temp', 'dewpoint', 'pm', 'acState']);
    });

    it('appends missing chart ids at the end', () => {
        expect(parseOrder('pm,temp'))
            .toEqual(['pm', 'temp', 'dewpoint', 'humidity', 'acState']);
    });

    it('handles duplicates by keeping first occurrence', () => {
        expect(parseOrder('temp,temp,dewpoint,humidity,pm,acState'))
            .toEqual(['temp', 'dewpoint', 'humidity', 'pm', 'acState']);
    });

    it('works with custom validIds', () => {
        const rooms = ['Office', 'Bedroom', 'Outside'];
        expect(parseOrder('Bedroom,Office', rooms))
            .toEqual(['Bedroom', 'Office', 'Outside']);
    });

    it('returns custom validIds when param is null', () => {
        const rooms = ['Office', 'Bedroom'];
        expect(parseOrder(null, rooms)).toEqual(['Office', 'Bedroom']);
    });
});

describe('serializeOrder', () => {
    it('joins order array with commas', () => {
        expect(serializeOrder(['humidity', 'temp', 'dewpoint', 'pm', 'acState']))
            .toBe('humidity,temp,dewpoint,pm,acState');
    });

    it('returns null when order matches default', () => {
        expect(serializeOrder(DEFAULT_ORDER)).toBeNull();
    });

    it('returns null when order matches custom default', () => {
        const custom = ['a', 'b', 'c'];
        expect(serializeOrder(['a', 'b', 'c'], custom)).toBeNull();
    });

    it('returns string when order differs from custom default', () => {
        const custom = ['a', 'b', 'c'];
        expect(serializeOrder(['c', 'a', 'b'], custom)).toBe('c,a,b');
    });
});
