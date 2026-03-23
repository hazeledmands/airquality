package main

import (
	"net/http/httptest"
	"testing"
)

func TestDataHandler_ReturnsJSON(t *testing.T) {
	handler := dataHandler(nil)
	req := httptest.NewRequest("GET", "/api/data", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
}

func TestRoomsHandler_ReturnsHTML(t *testing.T) {
	handler := roomsHandler()
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if len(body) < 100 {
		t.Error("response too short to be the rooms HTML")
	}
}
