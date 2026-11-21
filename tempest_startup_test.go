package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/metrics"
	"github.com/hazel/airquality/tempest"
)

func TestResolveTempestURL(t *testing.T) {
	cases := []struct {
		raw, secure, insecure string
		wantHost              string
		wantErr               bool
	}{
		{"https://swd.example.com", "https", "http", "swd.example.com", false},
		{"wss://ws.example.com/swd/data", "wss", "ws", "ws.example.com", false},
		{"http://127.0.0.1:58080", "https", "http", "127.0.0.1:58080", false},
		{"http://localhost:8080", "https", "http", "localhost:8080", false},
		{"ws://[::1]:9000", "wss", "ws", "[::1]:9000", false},
		{"http://swd.example.com", "https", "http", "", true}, // plaintext to a remote host
		{"ws://10.0.0.5:9000", "wss", "ws", "", true},         // private but not loopback
		{"ftp://localhost", "https", "http", "", true},        // wrong scheme
		{"https://", "https", "http", "", true},               // no host
		{"wss://ws.example.com", "https", "http", "", true},   // WS scheme for the REST API
	}
	for _, c := range cases {
		host, err := resolveTempestURL(c.raw, c.secure, c.insecure)
		if (err != nil) != c.wantErr || host != c.wantHost {
			t.Errorf("%s: host=%q err=%v, want host=%q err=%v", c.raw, host, err, c.wantHost, c.wantErr)
		}
	}
}

// The URL may carry credentials or a path worth keeping private; errors and
// logs name only the host.
func TestResolveTempestURL_ErrorNamesHostOnly(t *testing.T) {
	_, err := resolveTempestURL("http://user:secret@remote.example.com/path?token=abc", "https", "http")
	if err == nil {
		t.Fatal("want error")
	}
	for _, leak := range []string{"secret", "token=abc", "/path"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("error leaks %q: %v", leak, err)
		}
	}
}

type fakeTables struct {
	ok  bool
	err error
}

func (f fakeTables) TempestTablesExist(context.Context) (bool, error) { return f.ok, f.err }

func TestTempestStartup_MissingTablesDisablesTempest(t *testing.T) {
	m := metrics.New()
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }

	enabled := tempestStartup(context.Background(), fakeTables{ok: false}, "tok", m, logf)
	if enabled {
		t.Fatal("enabled = true with the tables missing")
	}
	if v := gaugeValue(t, m, "airquality_tempest_enabled"); v != 0 {
		t.Errorf("airquality_tempest_enabled = %v, want 0", v)
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "TEMPEST DISABLED") || strings.Contains(logged[0], "<nil>") ||
		!strings.Contains(logged[0], "tempest_observations") {
		t.Errorf("log = %q, want a reason naming the missing tables", logged)
	}

	// The rest of the app still serves: the dashboard answers, without Tempest.
	c := newTestCache()
	c.tempestEnabled = enabled
	w := httptest.NewRecorder()
	dataHandler(c, config.Config{}).ServeHTTP(w, httptest.NewRequest("GET", "/api/data", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("status %d body %s", w.Code, w.Body.String())
	}
}

func TestTempestStartup_CheckErrorIsReported(t *testing.T) {
	m := metrics.New()
	var logged []string
	logf := func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }
	if tempestStartup(context.Background(), fakeTables{err: errors.New("permission denied")}, "tok", m, logf) {
		t.Fatal("enabled = true after a failed check")
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "permission denied") {
		t.Errorf("log = %q", logged)
	}
}

func TestTempestStartup_Enabled(t *testing.T) {
	m := metrics.New()
	if !tempestStartup(context.Background(), fakeTables{ok: true}, "tok", m, func(string, ...any) {}) {
		t.Fatal("enabled = false with tables present")
	}
	if v := gaugeValue(t, m, "airquality_tempest_enabled"); v != 1 {
		t.Errorf("gauge = %v", v)
	}
	if tempestStartup(context.Background(), fakeTables{ok: true}, "", m, func(string, ...any) {}) {
		t.Error("enabled without a token")
	}
}

// A non-finite derived value must not break encoding of the whole response.
func TestTempestJSON_NonFiniteIsNull(t *testing.T) {
	nan := math.NaN()
	st := &tempest.Station{ElevationM: 50}
	j := tempestJSON(tempestBucket{Observation: tempest.Observation{StationPressureMB: &nan, AirTempC: fp(10), RelativeHumidity: &nan}}, st)
	if j.SeaLevelPressureMB != nil || j.DewPointC != nil {
		t.Errorf("non-finite derived values kept: %+v", j)
	}
	if _, err := json.Marshal(j); err != nil {
		t.Errorf("marshal: %v", err)
	}
}

func gaugeValue(t *testing.T, m *metrics.Metrics, name string) float64 {
	t.Helper()
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, line := range strings.Split(w.Body.String(), "\n") {
		if strings.HasPrefix(line, name+" ") {
			var v float64
			if _, err := fmt.Sscan(strings.TrimPrefix(line, name+" "), &v); err == nil {
				return v
			}
		}
	}
	t.Fatalf("%s not exported", name)
	return 0
}
