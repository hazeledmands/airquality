package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
	"github.com/hazel/airquality/store"
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
}

func newDataCache(ctx context.Context, db *store.Store) (*dataCache, error) {
	c := &dataCache{
		seenReadings: make(map[string]bool),
		seenACStates: make(map[string]bool),
		seenWeather:  make(map[string]bool),
	}
	if err := c.backfill(ctx, db); err != nil {
		return nil, err
	}
	return c, nil
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

func readingKey(r logger.Record) string {
	return r.Source + "/" + r.SensorID + "/" +
		strconv.FormatInt(r.Timestamp.Truncate(time.Minute).Unix(), 10)
}

func weatherKey(r openmeteo.Reading) string {
	return strconv.FormatFloat(r.Latitude, 'f', 6, 64) + "/" +
		strconv.FormatFloat(r.Longitude, 'f', 6, 64) + "/" +
		strconv.FormatInt(r.Time.Truncate(time.Minute).Unix(), 10)
}
