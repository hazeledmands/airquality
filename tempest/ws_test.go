package tempest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestParseMessage_Strike(t *testing.T) {
	now := time.Unix(1789564999, 0)
	evs, err := ParseMessage([]byte(`{"type":"evt_strike","device_id":2002,"evt":[1789564900,27,3848]}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("len = %d", len(evs))
	}
	e := evs[0]
	if e.Type != EventLightningStrike || e.DeviceID != 2002 || !e.Time.Equal(time.Unix(1789564900, 0)) {
		t.Errorf("event = %+v", e)
	}
	if !eqF(e.DistanceKM, f(27)) || !eqF(e.Energy, f(3848)) {
		t.Errorf("distance/energy = %v %v", deref(e.DistanceKM), deref(e.Energy))
	}
}

func TestParseMessage_RainStart(t *testing.T) {
	now := time.Unix(1789564999, 0)
	evs, err := ParseMessage([]byte(`{"type":"evt_precip","device_id":2002,"evt":[1789564950]}`), now)
	if err != nil || len(evs) != 1 {
		t.Fatalf("evs = %v, err = %v", evs, err)
	}
	if evs[0].Type != EventRainStart || !evs[0].Time.Equal(time.Unix(1789564950, 0)) || evs[0].DistanceKM != nil {
		t.Errorf("event = %+v", evs[0])
	}
	// The WebSocket docs show evt_precip without an evt array; fall back to
	// the receive time.
	evs, err = ParseMessage([]byte(`{"type":"evt_precip","device_id":2002}`), now)
	if err != nil || len(evs) != 1 || !evs[0].Time.Equal(now.Truncate(time.Second)) {
		t.Errorf("no-evt: evs = %+v, err = %v", evs, err)
	}
}

func TestParseMessage_IgnoresOtherTypes(t *testing.T) {
	for _, msg := range []string{
		`{"type":"connection_opened"}`,
		`{"type":"ack","id":"1"}`,
		`{"type":"obs_st","device_id":2002,"obs":[[1789564800,0,0.1]]}`,
		`{"type":"rapid_wind","device_id":2002,"ob":[1789564800,2.3,128]}`,
	} {
		evs, err := ParseMessage([]byte(msg), time.Now())
		if err != nil || len(evs) != 0 {
			t.Errorf("%s: evs = %v, err = %v", msg, evs, err)
		}
	}
	if _, err := ParseMessage([]byte(`not json`), time.Now()); err == nil {
		t.Error("garbage: want error")
	}
}

// fakeWS is a WebSocket server that records listen_start requests and sends
// whatever the test queues on each connection.
type fakeWS struct {
	mu       sync.Mutex
	listens  []int
	tokens   []string
	conns    atomic.Int32
	messages []string
}

func (f *fakeWS) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer c.CloseNow()
		f.conns.Add(1)
		f.mu.Lock()
		f.tokens = append(f.tokens, r.URL.Query().Get("token"))
		f.mu.Unlock()
		ctx := r.Context()
		c.Write(ctx, websocket.MessageText, []byte(`{"type":"connection_opened"}`))
		_, b, err := c.Read(ctx)
		if err != nil {
			return
		}
		var req struct {
			Type     string `json:"type"`
			DeviceID int    `json:"device_id"`
		}
		json.Unmarshal(b, &req)
		if req.Type == "listen_start" {
			f.mu.Lock()
			f.listens = append(f.listens, req.DeviceID)
			f.mu.Unlock()
		}
		for _, m := range f.messages {
			c.Write(ctx, websocket.MessageText, []byte(m))
		}
		// Close after sending, so the listener has to reconnect.
		c.Close(websocket.StatusNormalClosure, "bye")
	}
}

func TestListener_DeliversEventsAndReconnects(t *testing.T) {
	fs := &fakeWS{messages: []string{
		`{"type":"ack","id":"x"}`,
		`{"type":"evt_strike","device_id":2002,"evt":[` + strconv.FormatInt(time.Now().Unix(), 10) + `,12,500]}`,
	}}
	srv := httptest.NewServer(fs.handler(t))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	var mu sync.Mutex
	var got []Event
	l := &Listener{
		URL:          "ws" + strings.TrimPrefix(srv.URL, "http"),
		Token:        "tok",
		DeviceIDs:    []int{2002},
		MinBackoff:   time.Millisecond,
		MaxBackoff:   10 * time.Millisecond,
		StaleTimeout: time.Second,
		OnEvent: func(e Event) {
			mu.Lock()
			got = append(got, e)
			n := len(got)
			mu.Unlock()
			if n >= 2 {
				cancel()
			}
		},
		OnError: func(error) {},
	}
	l.Run(ctx)

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 {
		t.Fatalf("events = %d, want ≥2 (one per connection)", len(got))
	}
	if fs.conns.Load() < 2 {
		t.Errorf("connections = %d, want reconnect", fs.conns.Load())
	}
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.listens) == 0 || fs.listens[0] != 2002 || fs.tokens[0] != "tok" {
		t.Errorf("listens = %v tokens = %v", fs.listens, fs.tokens)
	}
}

func TestListener_ErrorsNeverContainToken(t *testing.T) {
	const token = "s3cr3t-ws-token"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var leaked atomic.Bool
	var errs atomic.Int32
	l := &Listener{
		URL:        "ws" + strings.TrimPrefix(srv.URL, "http"),
		Token:      token,
		DeviceIDs:  []int{1},
		MinBackoff: time.Millisecond,
		MaxBackoff: time.Millisecond,
		OnEvent:    func(Event) {},
		OnError: func(err error) {
			if strings.Contains(err.Error(), token) {
				leaked.Store(true)
			}
			if errs.Add(1) >= 2 {
				cancel()
			}
		},
	}
	l.Run(ctx)
	if errs.Load() == 0 {
		t.Fatal("no errors reported")
	}
	if leaked.Load() {
		t.Error("error leaks token")
	}
}

func TestParseMessage_LiveMessagesYieldNoEvents(t *testing.T) {
	data := fixture(t, "live_ws_messages.jsonl")
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		evs, err := ParseMessage([]byte(line), time.Now())
		if err != nil || len(evs) != 0 {
			t.Errorf("%s: evs = %v err = %v", line, evs, err)
		}
		n++
	}
	if n < 4 {
		t.Errorf("fixture has %d messages", n)
	}
}
