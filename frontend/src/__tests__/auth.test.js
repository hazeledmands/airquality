import { describe, it, expect, vi } from 'vitest';
import { fetchAuth, loginUrl } from '../auth.js';

describe('fetchAuth', () => {
    it('returns the user when /api/me answers', async () => {
        const fetchFn = vi.fn(async () => ({ ok: true, type: 'basic', json: async () => ({ user: 'hazel', canEdit: true }) }));
        expect(await fetchAuth(fetchFn)).toEqual({ user: 'hazel', canEdit: true });
        expect(fetchFn).toHaveBeenCalledWith('/api/me', expect.objectContaining({ redirect: 'manual' }));
    });

    it('treats the proxy sign-in redirect as signed out', async () => {
        const fetchFn = vi.fn(async () => ({ ok: false, type: 'opaqueredirect', status: 0 }));
        expect(await fetchAuth(fetchFn)).toEqual({ user: '', canEdit: false });
    });

    it('treats a network error as signed out', async () => {
        const fetchFn = vi.fn(async () => { throw new Error('boom'); });
        expect(await fetchAuth(fetchFn)).toEqual({ user: '', canEdit: false });
    });

    it('treats a missing canEdit as signed out', async () => {
        const fetchFn = vi.fn(async () => ({ ok: true, type: 'basic', json: async () => ({}) }));
        expect(await fetchAuth(fetchFn)).toEqual({ user: '', canEdit: false });
    });
});

describe('loginUrl', () => {
    it('returns to the current path and query', () => {
        expect(loginUrl({ pathname: '/', search: '?range=7d&peaks=0' }))
            .toBe('/login?next=%2F%3Frange%3D7d%26peaks%3D0');
    });

    it('handles an empty query', () => {
        expect(loginUrl({ pathname: '/', search: '' })).toBe('/login?next=%2F');
    });
});
