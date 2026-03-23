import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { getParam, setParam } from '../url-state.js';

let replaceState;

beforeEach(() => {
    replaceState = vi.fn();
    vi.stubGlobal('history', { replaceState });
});

afterEach(() => {
    vi.unstubAllGlobals();
});

function stubLocation(search) {
    vi.stubGlobal('location', { search, href: `http://localhost/${search}` });
}

function stubURL(searchParams) {
    vi.stubGlobal('URL', class {
        constructor() { this.searchParams = searchParams; }
        toString() { return 'http://localhost/'; }
    });
}

describe('getParam', () => {
    it('returns null when param is absent', () => {
        stubLocation('');
        expect(getParam('peaks')).toBeNull();
    });

    it('returns the value when param is present', () => {
        stubLocation('?peaks=0');
        expect(getParam('peaks')).toBe('0');
    });

    it('returns empty string for param with no value', () => {
        stubLocation('?peaks=');
        expect(getParam('peaks')).toBe('');
    });
});

describe('setParam', () => {
    it('deletes the param when value is null', () => {
        const deleted = [];
        stubLocation('?peaks=0');
        stubURL({ delete: (k) => deleted.push(k), set: vi.fn() });
        setParam('peaks', null);
        expect(deleted).toContain('peaks');
    });

    it('does not call set when value is null', () => {
        const set = vi.fn();
        stubLocation('');
        stubURL({ delete: vi.fn(), set });
        setParam('peaks', null);
        expect(set).not.toHaveBeenCalled();
    });

    it('sets the param when value is provided', () => {
        const set = [];
        stubLocation('');
        stubURL({ delete: vi.fn(), set: (k, v) => set.push([k, v]) });
        setParam('humidity', '0');
        expect(set).toContainEqual(['humidity', '0']);
    });

    it('does not call delete when value is provided', () => {
        const del = vi.fn();
        stubLocation('');
        stubURL({ delete: del, set: vi.fn() });
        setParam('outdoorTemp', '1');
        expect(del).not.toHaveBeenCalled();
    });

    it('calls history.replaceState after updating params', () => {
        stubLocation('');
        stubURL({ delete: vi.fn(), set: vi.fn() });
        setParam('outdoorTemp', '1');
        expect(replaceState).toHaveBeenCalledWith(null, '', expect.anything());
    });
});
