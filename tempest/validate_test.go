package tempest

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func row(vals ...float64) []*float64 {
	out := make([]*float64, len(vals))
	for i := range vals {
		v := vals[i]
		out[i] = &v
	}
	return out
}

// A single implausible value must not cost the whole row, let alone the
// batch: it becomes NULL and is reported.
func TestParseObsST_NullsOutOfRangeFields(t *testing.T) {
	r := row(1789564800, 0.1, 0.4, 0.9, 75, 3, 1011, 15.8, 78, 1520, 0.12, 13, 0, 40000, 5, -2, 2.61, 1, 0, 0, 0, 9)
	nan := math.NaN()
	r[7] = &nan
	o, rejected, ok := ParseObsST(2002, r)
	if !ok {
		t.Fatal("row rejected")
	}
	if o.PrecipType != nil || o.LightningStrikeCount != nil || o.PrecipAnalysisType != nil || o.AirTempC != nil {
		t.Errorf("bad values kept: %+v", o)
	}
	if o.WindAvgMS == nil || *o.WindAvgMS != 0.4 || o.LightningAvgDistanceKM == nil {
		t.Errorf("good values lost: %+v", o)
	}
	joined := strings.Join(rejected, ",")
	for _, f := range []string{"precip_type", "lightning_strike_count", "precip_analysis_type", "air_temp_c"} {
		if !strings.Contains(joined, f) {
			t.Errorf("rejected = %v, missing %s", rejected, f)
		}
	}
}

func TestParseObsST_RejectsBadTimestamp(t *testing.T) {
	for _, ts := range []float64{-5, math.Inf(1), 1e15} {
		if _, _, ok := ParseObsST(1, row(ts, 1)); ok {
			t.Errorf("timestamp %v accepted", ts)
		}
	}
}

// Rows outside the requested window, or in the future, are dropped: a future
// timestamp would otherwise become the poller's resume point.
func TestDeviceObservations_DropsOutOfWindowAndFutureRows(t *testing.T) {
	now := time.Unix(1789568400, 0)
	start, end := now.Add(-time.Hour), now.Add(time.Hour) // end past now
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":{"status_code":0},"obs":[
			[1789560000, 1],
			[1789564800, 1],
			[1789568400, 1],
			[1789568640, 1],
			[1789575600, 1]
		]}`))
	}))
	defer srv.Close()
	c := newTestClient(srv, "tok")
	c.Now = func() time.Time { return now }
	obs, err := c.DeviceObservations(t.Context(), 1, start, end)
	if err != nil {
		t.Fatal(err)
	}
	var got []int64
	for _, o := range obs {
		got = append(got, o.Time.Unix())
	}
	// Keeps start..now+5m; drops before start-1m and the hours-ahead row.
	want := []int64{1789564800, 1789568400, 1789568640}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

// The body is redacted before truncation, so a token straddling the cut
// can't leak its first half.
func TestStatusErrorRedactsBeforeTruncating(t *testing.T) {
	const token = "abcdef0123456789-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(strings.Repeat("x", 190) + token + strings.Repeat("y", 50)))
	}))
	defer srv.Close()
	c := NewClient(token)
	c.BaseURL = srv.URL
	_, err := c.Stations(t.Context())
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), token[:8]) {
		t.Errorf("error leaks a token prefix: %v", err)
	}
}

func TestRetryAfterHonoured(t *testing.T) {
	var calls atomic.Int32
	var first, second time.Time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			first = time.Now()
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		second = time.Now()
		w.Write([]byte(`{"status":{"status_code":0},"stations":[]}`))
	}))
	defer srv.Close()
	c := newTestClient(srv, "tok") // RetryBase 1ms
	if _, err := c.Stations(t.Context()); err != nil {
		t.Fatal(err)
	}
	if second.Sub(first) < 900*time.Millisecond {
		t.Errorf("retried after %v, want ≥ Retry-After 1s", second.Sub(first))
	}
}

func TestNoRetryOnDNSFailure(t *testing.T) {
	c := NewClient("tok")
	c.BaseURL = "https://nonexistent.invalid"
	c.RetryBase = time.Second
	start := time.Now()
	if _, err := c.Stations(t.Context()); err == nil {
		t.Fatal("want error")
	}
	if time.Since(start) > 900*time.Millisecond {
		t.Errorf("DNS failure was retried (%v)", time.Since(start))
	}
}

func TestListener_FiltersDevicesStaleEventsAndFloods(t *testing.T) {
	now := time.Now()
	ts := func(d time.Duration) string { return itoa(now.Add(d).Unix()) }
	msgs := []string{
		`{"type":"evt_strike","device_id":2002,"evt":[` + ts(0) + `,5,10]}`,
		`{"type":"evt_strike","device_id":9999,"evt":[` + ts(0) + `,5,10]}`,          // not subscribed
		`{"type":"evt_strike","device_id":2002,"evt":[` + ts(-time.Hour) + `,5,10]}`, // stale
		`{"type":"evt_strike","device_id":2002,"evt":[` + ts(time.Hour) + `,5,10]}`,  // future
	}
	for i := range 20 { // flood: same minute
		msgs = append(msgs, `{"type":"evt_strike","device_id":2002,"evt":[`+ts(0)+`,`+itoa(int64(6+i))+`,10]}`)
	}
	fs := &fakeWS{messages: msgs}
	srv := httptest.NewServer(fs.handler(t))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var mu sync.Mutex
	var got []Event
	l := &Listener{
		URL: "ws" + strings.TrimPrefix(srv.URL, "http"), Token: "tok", DeviceIDs: []int{2002},
		MinBackoff: time.Hour, MaxEventsPerMinute: 10,
		OnEvent: func(e Event) { mu.Lock(); got = append(got, e); mu.Unlock() },
		OnError: func(error) { cancel() },
	}
	l.Run(ctx)
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 10 {
		t.Fatalf("events = %d, want 10 (1 valid + 9 before the cap)", len(got))
	}
	for _, e := range got {
		if e.DeviceID != 2002 || e.Time.Before(now.Add(-time.Minute)) || e.Time.After(now.Add(time.Minute)) {
			t.Errorf("unexpected event %+v", e)
		}
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
