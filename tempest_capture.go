package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/hazel/airquality/config"
	"github.com/hazel/airquality/metrics"
	"github.com/hazel/airquality/store"
	"github.com/hazel/airquality/tempest"
)

// Tempest data comes from the WeatherFlow cloud, not the hub's LAN UDP
// broadcast: the app runs in a cluster on a different subnet from the hub, so
// port 50222 broadcasts never reach it. The REST API serves the same obs_st
// rows (plus Rain Check fields UDP lacks) and keeps history, so a poll after
// an outage fills the gap. The WebSocket API adds the events REST does not
// keep individually.

const (
	// tempestCatchUp bounds how far back a live poll reaches for missing
	// minutes; older gaps are for `airquality tempest-backfill`.
	tempestCatchUp = 24 * time.Hour
	// tempestRecheckSpan is refetched once a day so Rain Check corrections,
	// which WeatherFlow fills in during the following local day, reach the
	// database. 72h covers a whole local day plus its correction day from
	// any time the recheck happens to run.
	tempestRecheckSpan  = 72 * time.Hour
	tempestRecheckEvery = 24 * time.Hour
)

type tempestAPI interface {
	Stations(ctx context.Context) ([]tempest.Station, error)
	DeviceObservations(ctx context.Context, deviceID int, start, end time.Time) ([]tempest.Observation, error)
}

type tempestStore interface {
	UpsertTempestObservations(ctx context.Context, obs []tempest.Observation) (int64, error)
	LatestTempestObservation(ctx context.Context, deviceID int) (time.Time, bool, error)
	InsertTempestEvents(ctx context.Context, evs []tempest.Event) ([]tempest.Event, error)
}

// tempestRecorder polls the REST API for new observations and stores them,
// deduplicated on (device, timestamp), at its own cadence.
type tempestRecorder struct {
	api       tempestAPI
	db        tempestStore
	stationID int
	now       func() time.Time
	// onObs receives every fetched observation (the cache replaces repeats);
	// onNewObs only those newer than anything stored before, so running
	// totals count each minute once.
	onObs    func(tempest.Station, []tempest.Observation)
	onNewObs func(tempest.Station, []tempest.Observation)
	onEvent  func(tempest.Station, tempest.Event)
	// onResolved runs once, when the station and devices are first known.
	onResolved func(tempest.Station, []tempest.Device)

	station     tempest.Station
	devices     []tempest.Device
	last        map[int]time.Time // newest stored timestamp per device
	lastRecheck time.Time
}

func (r *tempestRecorder) ensureDevices(ctx context.Context) error {
	if len(r.devices) > 0 {
		return nil
	}
	stations, err := r.api.Stations(ctx)
	if err != nil {
		return err
	}
	st, devs, err := tempest.SelectDevices(stations, r.stationID)
	if err != nil {
		return err
	}
	r.station, r.devices = st, devs
	if r.onResolved != nil {
		r.onResolved(st, devs)
	}
	return nil
}

// Poll stores every observation newer than the last stored one (bounded by
// tempestCatchUp), and once a day refetches the last tempestRecheckSpan.
func (r *tempestRecorder) Poll(ctx context.Context) error {
	if err := r.ensureDevices(ctx); err != nil {
		return fmt.Errorf("tempest discovery: %w", err)
	}
	if r.last == nil {
		r.last = make(map[int]time.Time)
	}
	now := r.now().UTC()
	nowMin := now.Truncate(time.Minute)
	recheck := r.lastRecheck.IsZero() || now.Sub(r.lastRecheck) >= tempestRecheckEvery

	var errs []error
	for _, d := range r.devices {
		last, known := r.last[d.ID]
		if !known {
			ts, ok, err := r.db.LatestTempestObservation(ctx, d.ID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if ok {
				last = ts
			}
		}
		// A future timestamp (bad clock, bad row) must not become the
		// resume point, or polling would stop until the clock caught up.
		if last.After(now) {
			last = now
		}
		r.last[d.ID] = last
		floor := nowMin.Add(-tempestCatchUp)
		if recheck {
			floor = nowMin.Add(-tempestRecheckSpan)
		}
		from := floor
		if !recheck && !last.IsZero() && last.Add(time.Second).After(floor) {
			from = last.Add(time.Second)
		}
		if !last.IsZero() && last.Before(floor) {
			log.Printf("Tempest device %d: gap from %s to %s is older than the live catch-up; run `airquality tempest-backfill --from %s` to fill it",
				d.ID, last.Format(time.RFC3339), floor.Format(time.RFC3339), last.Format("2006-01-02"))
		}
		if err := r.fetchInto(ctx, d.ID, from, now); err != nil {
			errs = append(errs, err)
		}
	}
	if recheck {
		// Even on failure: a failing recheck retried every minute would
		// hammer the API. The live catch-up still runs each poll.
		r.lastRecheck = now
		if len(errs) > 0 {
			log.Printf("Tempest: Rain Check recheck failed, next try in %s", tempestRecheckEvery)
		}
	}
	return errors.Join(errs...)
}

func (r *tempestRecorder) fetchInto(ctx context.Context, deviceID int, from, to time.Time) error {
	for _, w := range tempest.Windows(from, to, tempest.MaxWindow) {
		obs, err := r.api.DeviceObservations(ctx, deviceID, w.Start, w.End)
		if err != nil {
			return err
		}
		if len(obs) == 0 {
			continue
		}
		if _, err := r.db.UpsertTempestObservations(ctx, obs); err != nil {
			return err
		}
		prev := r.last[deviceID]
		var fresh []tempest.Observation
		for _, o := range obs {
			if o.Time.After(prev) {
				fresh = append(fresh, o)
			}
			if o.Time.After(r.last[deviceID]) {
				r.last[deviceID] = o.Time
			}
		}
		if now := r.now(); r.last[deviceID].After(now) {
			r.last[deviceID] = now
		}
		if r.onObs != nil {
			r.onObs(r.station, obs)
		}
		if r.onNewObs != nil && len(fresh) > 0 {
			r.onNewObs(r.station, fresh)
		}
	}
	return nil
}

// HandleEvent stores a WebSocket event and, if it was new, passes it on, so
// the counters agree with the database.
func (r *tempestRecorder) HandleEvent(ctx context.Context, e tempest.Event) {
	inserted, err := r.db.InsertTempestEvents(ctx, []tempest.Event{e})
	if err != nil {
		log.Printf("DB tempest event write error: %v", err)
		return
	}
	if r.onEvent != nil {
		for _, ev := range inserted {
			r.onEvent(r.station, ev)
		}
	}
}

// runTempest polls at cfg.PollEvery() until ctx is done and, once the devices
// are known, listens for WebSocket events if enabled. once polls a single
// time and returns.
func runTempest(ctx context.Context, rec *tempestRecorder, token, wsURL string, cfg config.Tempest, once, verbose bool) {
	listening := false
	poll := func() {
		if err := rec.Poll(ctx); err != nil {
			log.Printf("Tempest error: %v", err)
		}
		if !listening && !once && cfg.EventsEnabled() && len(rec.devices) > 0 {
			listening = true
			ids := make([]int, len(rec.devices))
			for i, d := range rec.devices {
				ids[i] = d.ID
			}
			l := &tempest.Listener{
				URL:       wsURL,
				Token:     token,
				DeviceIDs: ids,
				OnEvent: func(e tempest.Event) {
					rec.HandleEvent(ctx, e)
					if verbose {
						log.Printf("  tempest event: %s device %d at %s", e.Type, e.DeviceID, e.Time.Format(time.RFC3339))
					}
				},
				OnError: func(err error) { log.Printf("Tempest websocket: %v", err) },
			}
			go l.Run(ctx)
			log.Printf("Tempest: listening for lightning and rain events on %v", ids)
		}
	}
	poll()
	if once {
		return
	}
	ticker := time.NewTicker(cfg.PollEvery())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

// newTempestRecorder wires a recorder to the database, cache and metrics.
func newTempestRecorder(api tempestAPI, db *store.Store, cache *dataCache, m *metrics.Metrics, fileCfg config.Config, stationID int, verbose bool) *tempestRecorder {
	room := func(st tempest.Station, deviceID int) string {
		return fileCfg.Room("tempest", strconv.Itoa(deviceID), st.Name)
	}
	return &tempestRecorder{
		api:       api,
		db:        db,
		stationID: stationID,
		now:       time.Now,
		onResolved: func(st tempest.Station, devs []tempest.Device) {
			cache.setTempestStation(st, devs)
			for _, d := range devs {
				log.Printf("Tempest: recording station %d (%s), device %d (%s, firmware %s)", st.ID, st.Name, d.ID, d.Serial, d.Firmware)
			}
		},
		onObs: func(st tempest.Station, obs []tempest.Observation) {
			cache.addTempest(obs)
			latest := obs[len(obs)-1]
			m.UpdateTempest(latest, st.Name, room(st, latest.DeviceID), st.ElevationM)
			if verbose {
				log.Printf("  tempest/%d: %d observation(s), latest %s", latest.DeviceID, len(obs), describeTempest(latest))
			} else if len(obs) > 0 {
				log.Printf("  tempest/%d (%s): %d fetched, latest %s", latest.DeviceID, st.Name, len(obs), latest.Time.Format(time.RFC3339))
			}
		},
		onNewObs: func(st tempest.Station, obs []tempest.Observation) {
			for _, o := range obs {
				m.AddTempestTotals(o, st.Name, room(st, o.DeviceID))
			}
		},
		onEvent: func(st tempest.Station, e tempest.Event) {
			cache.addTempestEvent(e)
			m.RecordTempestEvent(e, st.Name, room(st, e.DeviceID))
		},
	}
}

func describeTempest(o tempest.Observation) string {
	v := func(p *float64) string {
		if p == nil {
			return "-"
		}
		return strconv.FormatFloat(*p, 'f', -1, 64)
	}
	return fmt.Sprintf("%s temp %s°C rh %s%% wind %s/%s/%s m/s @%s° rain %s mm uv %s solar %s W/m² strikes %v batt %s V",
		o.Time.Format(time.RFC3339), v(o.AirTempC), v(o.RelativeHumidity),
		v(o.WindLullMS), v(o.WindAvgMS), v(o.WindGustMS), v(o.WindDirectionDeg),
		v(o.RainMM), v(o.UVIndex), v(o.SolarRadiationWM2), ptrInt(o.LightningStrikeCount), v(o.BatteryV))
}

func ptrInt(p *int) any {
	if p == nil {
		return "-"
	}
	return *p
}

// resolveTempestStationID prefers TEMPEST_STATION_ID, then the config file.
func resolveTempestStationID(env string, file config.Config) (int, error) {
	if env == "" {
		return file.Tempest.StationID, nil
	}
	id, err := strconv.Atoi(env)
	if err != nil {
		return 0, fmt.Errorf("TEMPEST_STATION_ID %q is not a number", env)
	}
	return id, nil
}

// --- backfill ---

type backfillOptions struct {
	from, to   time.Time
	stationID  int
	delay      time.Duration
	configPath string
}

type backfillResult struct {
	fetched, changed int64
}

func parseBackfillTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("%q is neither YYYY-MM-DD nor RFC 3339", s)
}

func parseBackfillArgs(args []string, now time.Time) (backfillOptions, error) {
	fs := flag.NewFlagSet("tempest-backfill", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var from, to string
	var o backfillOptions
	fs.StringVar(&from, "from", "", "")
	fs.StringVar(&to, "to", "", "")
	fs.IntVar(&o.stationID, "station", 0, "")
	fs.DurationVar(&o.delay, "delay", time.Second, "")
	fs.StringVar(&o.configPath, "config", os.Getenv("AIRQUALITY_CONFIG"), "")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	var err error
	if from != "" {
		if o.from, err = parseBackfillTime(from); err != nil {
			return o, fmt.Errorf("--from: %w", err)
		}
	}
	o.to = now.UTC()
	if to != "" {
		if o.to, err = parseBackfillTime(to); err != nil {
			return o, fmt.Errorf("--to: %w", err)
		}
	}
	if !o.from.IsZero() && !o.from.Before(o.to) {
		return o, errors.New("--from must be before --to")
	}
	return o, nil
}

// runTempestBackfill fetches [from, to] in one-day windows (one-minute
// resolution needs ranges of five days or less) and upserts every row, so
// re-running it never duplicates and does pick up Rain Check corrections.
func runTempestBackfill(ctx context.Context, api tempestAPI, db tempestStore, o backfillOptions, logf func(string, ...any)) (backfillResult, error) {
	var res backfillResult
	stations, err := api.Stations(ctx)
	if err != nil {
		return res, err
	}
	st, devs, err := tempest.SelectDevices(stations, o.stationID)
	if err != nil {
		return res, err
	}
	if o.from.IsZero() {
		if st.Created.IsZero() {
			return res, errors.New("station has no creation date; pass --from")
		}
		o.from = st.Created
	}
	windows := tempest.Windows(o.from, o.to, tempest.MaxWindow)
	for _, d := range devs {
		logf("Backfilling station %d (%s) device %d: %s to %s, %d request(s)",
			st.ID, st.Name, d.ID, o.from.Format(time.RFC3339), o.to.Format(time.RFC3339), len(windows))
		for i, w := range windows {
			if i > 0 && o.delay > 0 {
				select {
				case <-ctx.Done():
					return res, ctx.Err()
				case <-time.After(o.delay):
				}
			}
			obs, err := api.DeviceObservations(ctx, d.ID, w.Start, w.End)
			if err != nil {
				return res, fmt.Errorf("window %s: %w", w.Start.Format(time.RFC3339), err)
			}
			n, err := db.UpsertTempestObservations(ctx, obs)
			if err != nil {
				return res, err
			}
			res.fetched += int64(len(obs))
			res.changed += n
			logf("  %s: %d observation(s), %d new or changed", w.Start.Format("2006-01-02 15:04"), len(obs), n)
		}
	}
	return res, nil
}

func tempestBackfillMain(args []string) {
	o, err := parseBackfillArgs(args, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, `tempest-backfill: %v

Usage: airquality tempest-backfill [--from DATE] [--to DATE] [--station ID] [--delay 1s] [--config path]

Fetches historical Tempest observations in one-day requests and stores them.
Re-running is safe: rows are upserted on (device, timestamp).
DATE is YYYY-MM-DD (UTC midnight) or RFC 3339. --from defaults to when the
station was created, --to to now.
Needs TEMPEST_TOKEN and DATABASE_URL; the station comes from --station,
TEMPEST_STATION_ID or the config file (default: the first with a Tempest).
`, err)
		os.Exit(2)
	}
	token := os.Getenv("TEMPEST_TOKEN")
	if token == "" {
		log.Fatal("TEMPEST_TOKEN is required.")
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required.")
	}
	if o.stationID == 0 {
		fileCfg, err := config.Load(o.configPath)
		if err != nil {
			log.Fatalf("Config: %v", err)
		}
		if o.stationID, err = resolveTempestStationID(os.Getenv("TEMPEST_STATION_ID"), fileCfg); err != nil {
			log.Fatal(err)
		}
	}
	ctx := context.Background()
	db, err := store.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()
	if ok, err := db.TempestTablesExist(ctx); err != nil || !ok {
		log.Fatalf("Tempest tables missing (%v): apply supabase/migrations/20261120000000_create_tempest.sql first", err)
	}
	client := tempest.NewClient(token)
	if u := os.Getenv("TEMPEST_API_URL"); u != "" {
		client.BaseURL = u
	}
	res, err := runTempestBackfill(ctx, client, db, o, log.Printf)
	if err != nil {
		log.Fatalf("Backfill failed after %d observation(s): %v", res.fetched, err)
	}
	log.Printf("Backfill done: %d observation(s) fetched, %d new or changed", res.fetched, res.changed)
}
