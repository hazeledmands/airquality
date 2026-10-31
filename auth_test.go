package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testHeader = "X-authentik-username"

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }

func TestRequireUser_RejectsMissingHeader(t *testing.T) {
	h := authConfig{header: testHeader}.requireUser(okHandler)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("POST", "/api/annotations", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestRequireUser_AllowsHeader(t *testing.T) {
	h := authConfig{header: testHeader}.requireUser(okHandler)
	req := httptest.NewRequest("POST", "/api/annotations", nil)
	req.Header.Set(testHeader, "hazel")
	w := httptest.NewRecorder()
	h(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestRequireUser_DisabledAllowsAll(t *testing.T) {
	h := authConfig{}.requireUser(okHandler)
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("POST", "/api/annotations", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func decodeMe(t *testing.T, w *httptest.ResponseRecorder) meResponse {
	t.Helper()
	var me meResponse
	if err := json.NewDecoder(w.Body).Decode(&me); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return me
}

func TestMeHandler(t *testing.T) {
	cases := []struct {
		name     string
		cfg      authConfig
		user     string
		wantUser string
		wantEdit bool
	}{
		{"signed in", authConfig{header: testHeader}, "hazel", "hazel", true},
		{"no header", authConfig{header: testHeader}, "", "", false},
		{"auth disabled", authConfig{}, "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/me", nil)
			if tc.user != "" {
				req.Header.Set(testHeader, tc.user)
			}
			w := httptest.NewRecorder()
			tc.cfg.meHandler()(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}
			if got := w.Header().Get("Cache-Control"); got != "no-store" {
				t.Errorf("Cache-Control = %q, want no-store", got)
			}
			me := decodeMe(t, w)
			if me.User != tc.wantUser || me.CanEdit != tc.wantEdit {
				t.Errorf("got %+v, want user=%q canEdit=%v", me, tc.wantUser, tc.wantEdit)
			}
		})
	}
}

func TestLoginHandler_RedirectsToSafeNext(t *testing.T) {
	cases := map[string]string{
		"/login":                           "/",
		"/login?next=/%3Frange%3D7d":       "/?range=7d",
		"/login?next=//evil.example/":      "/",
		"/login?next=https://evil.example": "/",
		"/login?next=/%5Cevil.example":     "/",
		"/login?next=relative":             "/",
	}
	for target, want := range cases {
		w := httptest.NewRecorder()
		loginHandler()(w, httptest.NewRequest("GET", target, nil))
		if w.Code != http.StatusFound {
			t.Errorf("%s: status = %d, want 302", target, w.Code)
		}
		if got := w.Header().Get("Location"); got != want {
			t.Errorf("%s: Location = %q, want %q", target, got, want)
		}
	}
}
