package tempest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/coder/websocket"
)

// DefaultWSURL is the WeatherFlow WebSocket endpoint.
const DefaultWSURL = "wss://ws.weatherflow.com/swd/data"

// ParseMessage extracts the events in one WebSocket message. Observation,
// rapid-wind and control messages yield no events: observations come from the
// REST API, which can fill gaps after an outage.
func ParseMessage(b []byte, now time.Time) ([]Event, error) {
	var msg struct {
		Type     string     `json:"type"`
		DeviceID int        `json:"device_id"`
		Evt      []*float64 `json:"evt"`
	}
	if err := json.Unmarshal(b, &msg); err != nil {
		return nil, fmt.Errorf("parse websocket message: %w", err)
	}
	at := func(i int) *float64 {
		if i < len(msg.Evt) {
			return msg.Evt[i]
		}
		return nil
	}
	when := now.Truncate(time.Second).UTC()
	if ts := at(0); ts != nil {
		when = time.Unix(int64(*ts), 0).UTC()
	}
	switch msg.Type {
	case "evt_strike":
		if at(0) == nil {
			return nil, nil
		}
		return []Event{{DeviceID: msg.DeviceID, Time: when, Type: EventLightningStrike, DistanceKM: at(1), Energy: at(2)}}, nil
	case "evt_precip":
		return []Event{{DeviceID: msg.DeviceID, Time: when, Type: EventRainStart}}, nil
	}
	return nil, nil
}

// Listener keeps one WebSocket connection open and reports events for its
// devices, reconnecting with exponential backoff.
type Listener struct {
	URL       string // DefaultWSURL when empty
	Token     string
	DeviceIDs []int
	OnEvent   func(Event)
	OnError   func(error)

	MinBackoff time.Duration // default 5s
	MaxBackoff time.Duration // default 5m
	// StaleTimeout reconnects when nothing arrives for this long. listen_start
	// delivers an observation every minute, so silence means a dead link.
	StaleTimeout time.Duration // default 3m
	// MaxEventsPerMinute caps delivered events per wall-clock minute, so a
	// misbehaving feed can't flood the database. Default 600: an intense
	// storm produces a few strikes a second at most.
	MaxEventsPerMinute int

	windowStart time.Time
	windowCount int
}

// eventSkew is how far an event's timestamp may be from now. Events are
// pushed as they happen; anything older or newer is a replay or a bad clock.
const eventSkew = 10 * time.Minute

// accept reports whether e should be delivered.
func (l *Listener) accept(e Event, now time.Time) bool {
	subscribed := false
	for _, id := range l.DeviceIDs {
		if id == e.DeviceID {
			subscribed = true
			break
		}
	}
	if !subscribed || e.Time.Before(now.Add(-eventSkew)) || e.Time.After(now.Add(eventSkew)) {
		return false
	}
	limit := l.MaxEventsPerMinute
	if limit == 0 {
		limit = 600
	}
	minute := now.Truncate(time.Minute)
	if !minute.Equal(l.windowStart) {
		l.windowStart, l.windowCount = minute, 0
	}
	if l.windowCount >= limit {
		return false
	}
	l.windowCount++
	return true
}

// Run blocks until ctx is done.
func (l *Listener) Run(ctx context.Context) {
	minB, maxB := l.MinBackoff, l.MaxBackoff
	if minB == 0 {
		minB = 5 * time.Second
	}
	if maxB == 0 {
		maxB = 5 * time.Minute
	}
	backoff := minB
	for ctx.Err() == nil {
		started := time.Now()
		err := l.session(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil && l.OnError != nil {
			l.OnError(err)
		}
		// A session that stayed up a while was healthy; start over gently.
		if time.Since(started) > 2*maxB {
			backoff = minB
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, maxB)
	}
}

func (l *Listener) session(ctx context.Context) error {
	base := l.URL
	if base == "" {
		base = DefaultWSURL
	}
	stale := l.StaleTimeout
	if stale == 0 {
		stale = 3 * time.Minute
	}
	u := base + "?" + url.Values{"token": {l.Token}}.Encode()

	dialCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	conn, _, err := websocket.Dial(dialCtx, u, nil)
	cancel()
	if err != nil {
		return fmt.Errorf("tempest websocket dial: %s", redact(unwrapURL(err), l.Token))
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	for _, id := range l.DeviceIDs {
		req, _ := json.Marshal(map[string]any{
			"type":      "listen_start",
			"device_id": id,
			"id":        "airquality-" + strconv.Itoa(id),
		})
		if err := conn.Write(ctx, websocket.MessageText, req); err != nil {
			return fmt.Errorf("tempest websocket listen_start: %s", redact(err, l.Token))
		}
	}

	for {
		readCtx, cancel := context.WithTimeout(ctx, stale)
		_, b, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("tempest websocket: no message for %s, reconnecting", stale)
			}
			return fmt.Errorf("tempest websocket read: %s", redact(err, l.Token))
		}
		now := time.Now()
		evs, err := ParseMessage(b, now)
		if err != nil {
			if l.OnError != nil {
				l.OnError(err)
			}
			continue
		}
		for _, e := range evs {
			if l.accept(e, now) {
				l.OnEvent(e)
			}
		}
	}
}

func unwrapURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}
