package main

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/store"
	"github.com/hazel/airquality/tempest"
)

const cacheDuration = 30 * 24 * time.Hour

// dataCache is an in-memory mirror of the last 30 days of data.
// It is backfilled from Postgres at startup, then kept current by
// the capture loop without any further DB reads.
type dataCache struct {
	mu          sync.RWMutex
	readings    []logger.Record
	acStates    []logger.ACStateRecord
	weather     []openmeteo.Reading
	annotations []store.Annotation

	// dedup sets prevent double-counting data that arrives via both
	// the backfill query and the live capture loop.
	seenReadings map[string]bool
	seenACStates map[string]bool
	seenWeather  map[string]bool

	// Tempest observations are replaced, not skipped, on a repeated
	// (device, timestamp): the daily recheck brings Rain Check corrections.
	tempestObs        []tempest.Observation
	tempestIdx        map[string]int
	tempestEvents     []tempest.Event
	seenTempestEvents map[string]bool
	tempestStation    *tempest.Station
	tempestDevices    []tempest.Device
	// Open-Meteo outdoor air quality (hourly, replaced on a repeated hour)
	// and the latest forecast.
	outdoorAir    []openmeteo.AirQuality
	outdoorAirIdx map[int64]int
	forecast      []openmeteo.ForecastHour

	// tempestEnabled is true when Tempest recording runs; the dashboard
	// then explains an empty range instead of hiding the section.
	tempestEnabled bool
}

// newDataCache backfills from Postgres. withTempest also loads the Tempest
// tables, which exist only once their migration is applied.
func newDataCache(ctx context.Context, db *store.Store, withTempest bool) (*dataCache, error) {
	c := &dataCache{
		seenReadings: make(map[string]bool),
		seenACStates: make(map[string]bool),
		seenWeather:  make(map[string]bool),
	}
	if err := c.backfill(ctx, db); err != nil {
		return nil, err
	}
	if db.OpenMeteoExtras() {
		to := time.Now().UTC()
		aq, err := db.QueryAirQuality(ctx, to.Add(-cacheDuration), to)
		if err != nil {
			return nil, fmt.Errorf("backfill air quality: %w", err)
		}
		c.addAirQuality(aq)
		fc, err := db.QueryForecast(ctx, to.Add(-24*time.Hour), to.Add(5*24*time.Hour))
		if err != nil {
			return nil, fmt.Errorf("backfill forecast: %w", err)
		}
		hours := make([]openmeteo.ForecastHour, len(fc))
		for i, f := range fc {
			hours[i] = f.ForecastHour
		}
		c.setForecast(hours)
		log.Printf("Cache backfilled: %d outdoor air quality rows, %d forecast hours", len(aq), len(hours))
	} else {
		log.Printf("OPEN-METEO EXTRAS DISABLED: apply supabase/migrations/20261121000000_open_meteo_more.sql and restart to record cloud cover, outdoor air quality and forecasts")
	}
	c.tempestEnabled = withTempest
	if withTempest {
		to := time.Now().UTC()
		obs, err := db.QueryTempestObservations(ctx, to.Add(-cacheDuration), to)
		if err != nil {
			return nil, fmt.Errorf("backfill tempest observations: %w", err)
		}
		evs, err := db.QueryTempestEvents(ctx, to.Add(-cacheDuration), to)
		if err != nil {
			return nil, fmt.Errorf("backfill tempest events: %w", err)
		}
		c.addTempest(obs)
		for _, e := range evs {
			c.addTempestEvent(e)
		}
		log.Printf("Cache backfilled: %d Tempest observations, %d Tempest events", len(obs), len(evs))
	}
	return c, nil
}

func tempestObsKey(o tempest.Observation) string {
	return strconv.Itoa(o.DeviceID) + "/" + strconv.FormatInt(o.Time.Unix(), 10)
}

// tempestEventKey matches the tempest_events dedup index: two strikes in the
// same second with different distance or energy are two events.
func tempestEventKey(e tempest.Event) string {
	f := func(p *float64) string {
		if p == nil {
			return "-"
		}
		return strconv.FormatFloat(*p, 'g', -1, 64)
	}
	return strconv.Itoa(e.DeviceID) + "/" + e.Type + "/" + strconv.FormatInt(e.Time.Unix(), 10) + "/" + f(e.DistanceKM) + "/" + f(e.Energy)
}

// addTempest inserts observations, replacing any with the same device and
// timestamp, and keeps them in time order.
func (c *dataCache) addTempest(obs []tempest.Observation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tempestIdx == nil {
		c.tempestIdx = make(map[string]int)
	}
	resort := false
	for _, o := range obs {
		k := tempestObsKey(o)
		if i, ok := c.tempestIdx[k]; ok {
			c.tempestObs[i] = o
			continue
		}
		if n := len(c.tempestObs); n > 0 && o.Time.Before(c.tempestObs[n-1].Time) {
			resort = true
		}
		c.tempestIdx[k] = len(c.tempestObs)
		c.tempestObs = append(c.tempestObs, o)
	}
	if resort {
		sort.SliceStable(c.tempestObs, func(i, j int) bool { return c.tempestObs[i].Time.Before(c.tempestObs[j].Time) })
		for i, o := range c.tempestObs {
			c.tempestIdx[tempestObsKey(o)] = i
		}
	}
}

func (c *dataCache) addTempestEvent(e tempest.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seenTempestEvents == nil {
		c.seenTempestEvents = make(map[string]bool)
	}
	k := tempestEventKey(e)
	if c.seenTempestEvents[k] {
		return
	}
	c.seenTempestEvents[k] = true
	c.tempestEvents = append(c.tempestEvents, e)
	// Events arrive for as long as the server runs; keep only the cache
	// window so memory stays bounded.
	cutoff := time.Now().Add(-cacheDuration)
	if len(c.tempestEvents) > 0 && c.tempestEvents[0].Time.Before(cutoff) {
		kept := c.tempestEvents[:0]
		for _, ev := range c.tempestEvents {
			if ev.Time.Before(cutoff) {
				delete(c.seenTempestEvents, tempestEventKey(ev))
				continue
			}
			kept = append(kept, ev)
		}
		c.tempestEvents = kept
	}
}

// latestTempest returns the newest raw observation in [from, to], so the
// latest-values strip never shows a downsampled bucket as "now".
func (c *dataCache) latestTempest(from, to time.Time) *tempest.Observation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for i := len(c.tempestObs) - 1; i >= 0; i-- {
		o := c.tempestObs[i]
		if o.Time.After(to) {
			continue
		}
		if o.Time.Before(from) {
			return nil
		}
		return &o
	}
	return nil
}

func (c *dataCache) setTempestStation(st tempest.Station, devs []tempest.Device) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tempestStation = &st
	c.tempestDevices = devs
}

// tempestStationInfo returns the recorded station and devices, or nil.
func (c *dataCache) tempestStationInfo() (*tempest.Station, []tempest.Device) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tempestStation, c.tempestDevices
}

// snapshotTempest returns copies of Tempest observations and events in [from, to].
func (c *dataCache) snapshotTempest(from, to time.Time) ([]tempest.Observation, []tempest.Event) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	obs := make([]tempest.Observation, 0)
	for _, o := range c.tempestObs {
		if !o.Time.Before(from) && !o.Time.After(to) {
			obs = append(obs, o)
		}
	}
	evs := make([]tempest.Event, 0)
	for _, e := range c.tempestEvents {
		if !e.Time.Before(from) && !e.Time.After(to) {
			evs = append(evs, e)
		}
	}
	return obs, evs
}

func (c *dataCache) backfill(ctx context.Context, db *store.Store) error {
	to := time.Now().UTC()
	from := to.Add(-cacheDuration)

	readings, err := db.QueryReadings(ctx, from, to)
	if err != nil {
		return fmt.Errorf("backfill readings: %w", err)
	}
	acStates, err := db.QueryACStates(ctx, from, to)
	if err != nil {
		return fmt.Errorf("backfill ac states: %w", err)
	}
	weather, err := db.QueryWeather(ctx, from, to)
	if err != nil {
		return fmt.Errorf("backfill weather: %w", err)
	}
	annotations, err := db.QueryAnnotations(ctx, from, to)
	if err != nil {
		return fmt.Errorf("backfill annotations: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, r := range readings {
		c.seenReadings[readingKey(r)] = true
		c.readings = append(c.readings, r)
	}
	for _, r := range acStates {
		c.seenACStates[r.SensiboID] = true
		c.acStates = append(c.acStates, r)
	}
	for _, r := range weather {
		c.seenWeather[weatherKey(r)] = true
		c.weather = append(c.weather, r)
	}
	c.annotations = annotations

	log.Printf("Cache backfilled: %d readings, %d AC states, %d weather rows, %d annotations",
		len(c.readings), len(c.acStates), len(c.weather), len(c.annotations))
	return nil
}

func (c *dataCache) addReading(r logger.Record) {
	key := readingKey(r)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seenReadings[key] {
		return
	}
	c.seenReadings[key] = true
	c.readings = append(c.readings, r)
}

func (c *dataCache) addACState(r logger.ACStateRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seenACStates[r.SensiboID] {
		return
	}
	c.seenACStates[r.SensiboID] = true
	c.acStates = append(c.acStates, r)
}

func (c *dataCache) addWeather(r openmeteo.Reading) {
	key := weatherKey(r)
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seenWeather[key] {
		return
	}
	c.seenWeather[key] = true
	c.weather = append(c.weather, r)
}

func (c *dataCache) addAnnotation(a store.Annotation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.annotations = append(c.annotations, a)
}

func (c *dataCache) deleteAnnotation(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, a := range c.annotations {
		if a.ID != id {
			c.annotations[n] = a
			n++
		}
	}
	c.annotations = c.annotations[:n]
}

// snapshot returns copies of all cached data within the last 30 days.
func (c *dataCache) snapshot() ([]logger.Record, []logger.ACStateRecord, []openmeteo.Reading, []store.Annotation) {
	cutoff := time.Now().UTC().Add(-cacheDuration)
	return c.snapshotRange(cutoff, time.Now().UTC())
}

// snapshotRange returns copies of cached data within [from, to].
func (c *dataCache) snapshotRange(from, to time.Time) ([]logger.Record, []logger.ACStateRecord, []openmeteo.Reading, []store.Annotation) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	readings := make([]logger.Record, 0, len(c.readings))
	for _, r := range c.readings {
		if !r.Timestamp.Before(from) && !r.Timestamp.After(to) {
			readings = append(readings, r)
		}
	}

	acStates := make([]logger.ACStateRecord, 0, len(c.acStates))
	for _, r := range c.acStates {
		ts, err := time.Parse(time.RFC3339, r.Timestamp)
		if err == nil && !ts.Before(from) && !ts.After(to) {
			acStates = append(acStates, r)
		}
	}

	weather := make([]openmeteo.Reading, 0, len(c.weather))
	for _, r := range c.weather {
		if !r.Time.Before(from) && !r.Time.After(to) {
			weather = append(weather, r)
		}
	}

	annotations := make([]store.Annotation, 0, len(c.annotations))
	for _, a := range c.annotations {
		if !a.Timestamp.Before(from) && !a.Timestamp.After(to) {
			annotations = append(annotations, a)
		}
	}

	return readings, acStates, weather, annotations
}

// latestLocations returns the most recent location name for each source/sensor_id
// key seen in the cache (scanning all 30 days, so last-written wins).
func (c *dataCache) latestLocations() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	locs := make(map[string]string, 16)
	for _, r := range c.readings { // ascending order — last entry per key wins
		locs[r.Source+"/"+r.SensorID] = r.Location
	}
	return locs
}

// latestACRooms returns the most recent room name for each AC device_id
// seen in the cache (ascending order, so last entry per key wins).
func (c *dataCache) latestACRooms() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rooms := make(map[string]string, 8)
	for _, r := range c.acStates {
		rooms[r.DeviceID] = r.RoomName
	}
	return rooms
}

func readingKey(r logger.Record) string {
	return r.Source + "/" + r.SensorID + "/" +
		strconv.FormatInt(r.Timestamp.Truncate(time.Minute).Unix(), 10)
}

func weatherKey(r openmeteo.Reading) string {
	return strconv.FormatFloat(r.Latitude, 'f', 6, 64) + "/" +
		strconv.FormatFloat(r.Longitude, 'f', 6, 64) + "/" +
		strconv.FormatInt(r.Time.Truncate(time.Minute).Unix(), 10)
}

// addAirQuality inserts samples, replacing any for the same hour, in time order.
func (c *dataCache) addAirQuality(rows []openmeteo.AirQuality) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.outdoorAirIdx == nil {
		c.outdoorAirIdx = make(map[int64]int)
	}
	for _, a := range rows {
		k := a.Time.Unix()
		if i, ok := c.outdoorAirIdx[k]; ok {
			c.outdoorAir[i] = a
			continue
		}
		c.outdoorAirIdx[k] = len(c.outdoorAir)
		c.outdoorAir = append(c.outdoorAir, a)
	}
	sort.SliceStable(c.outdoorAir, func(i, j int) bool { return c.outdoorAir[i].Time.Before(c.outdoorAir[j].Time) })
	for i, a := range c.outdoorAir {
		c.outdoorAirIdx[a.Time.Unix()] = i
	}
}

// snapshotAirQuality returns samples in [from, to], plus the last sample
// before from (if within two hours), so the hourly line starts at the left
// edge of the chart.
func (c *dataCache) snapshotAirQuality(from, to time.Time) []openmeteo.AirQuality {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]openmeteo.AirQuality, 0)
	var before *openmeteo.AirQuality
	for i, a := range c.outdoorAir {
		if a.Time.Before(from) {
			if from.Sub(a.Time) <= 2*time.Hour {
				before = &c.outdoorAir[i]
			}
			continue
		}
		if !a.Time.After(to) {
			out = append(out, a)
		}
	}
	if before != nil {
		// Served at from, as the value in effect when the range starts, so
		// it doesn't stretch the shared x axis to the left.
		first := *before
		first.Time = from
		out = append([]openmeteo.AirQuality{first}, out...)
	}
	return out
}

// setForecast replaces the cached forecast with the latest one.
func (c *dataCache) setForecast(hours []openmeteo.ForecastHour) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forecast = append([]openmeteo.ForecastHour(nil), hours...)
}

// forecastFrom returns the forecast hours starting at or after t.
func (c *dataCache) forecastFrom(t time.Time) []openmeteo.ForecastHour {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]openmeteo.ForecastHour, 0)
	for _, h := range c.forecast {
		if !h.Time.Before(t) {
			out = append(out, h)
		}
	}
	return out
}
