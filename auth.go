package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// authConfig describes how the app learns who is signed in. The app does no
// authentication itself: a reverse proxy (Authentik forward-auth behind
// Traefik in the homelab) authenticates the request and names the user in
// `header`. Reads are public; writes require that header.
//
// The header is only trustworthy if every request reaches the app through
// that proxy, and the proxy overwrites the header on every route that leads
// to a write. If clients can reach the app directly, or a write route is
// served without forward-auth, anyone can forge it.
//
// An empty header disables auth entirely — everyone can edit. That is the
// local-development default.
type authConfig struct {
	header string
}

// user returns the signed-in user's name and whether they may edit.
func (a authConfig) user(r *http.Request) (string, bool) {
	if a.header == "" {
		return "", true
	}
	u := r.Header.Get(a.header)
	return u, u != ""
}

// requireUser rejects requests that carry no user header with 401.
func (a authConfig) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := a.user(r); !ok {
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

type meResponse struct {
	User    string `json:"user"`
	CanEdit bool   `json:"canEdit"`
}

// meHandler reports who the request is from. In the homelab this path is
// behind forward-auth, so a signed-out browser never reaches it — the
// frontend sees the proxy's redirect instead and treats that as signed out.
func (a authConfig) meHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := a.user(r)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(meResponse{User: u, CanEdit: ok})
	}
}

// loginHandler sends the browser back to `next`. Its only job is to be a
// URL the proxy gates: visiting it runs the sign-in flow, and by the time
// the request reaches here the session cookie is set.
//
// `next` must be a same-origin path; anything else would make this an open
// redirect.
func loginHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := r.URL.Query().Get("next")
		if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, `\`) {
			next = "/"
		}
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, next, http.StatusFound)
	}
}
