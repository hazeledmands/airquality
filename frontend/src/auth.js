// auth.js — who is signed in, as far as the dashboard can tell.
//
// Viewing is public; editing annotations needs a signed-in user. In the
// homelab, /api/me sits behind Authentik forward-auth: a signed-in browser
// gets JSON, a signed-out one gets a redirect to the sign-in page. Fetching
// with redirect: 'manual' turns that redirect into an opaque response
// instead of following it cross-origin, so it reads as "signed out".

const SIGNED_OUT = { user: '', canEdit: false };

export async function fetchAuth(fetchFn = fetch) {
    try {
        const resp = await fetchFn('/api/me', { redirect: 'manual', cache: 'no-store' });
        if (!resp.ok) return SIGNED_OUT;
        const me = await resp.json();
        return { user: me.user || '', canEdit: me.canEdit === true };
    } catch {
        return SIGNED_OUT;
    }
}

// URL that runs the sign-in flow and then returns to the current view.
export function loginUrl(loc) {
    return '/login?next=' + encodeURIComponent(loc.pathname + loc.search);
}
