package main

import (
	"sort"
	"time"

	"github.com/hazel/airquality/logger"
	"github.com/hazel/airquality/openmeteo"
)

func medianFloat64(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := make([]float64, n)
	copy(sorted, vals)
	sort.Float64s(sorted)
	return sorted[n/2]
}

// downsampleReadings buckets records into fixed-width time steps and returns
// one record per (sensor, bucket) with the median value of each numeric field.
func downsampleReadings(records []logger.Record, from time.Time, stepMs int64) []logger.Record {
	if stepMs <= 0 || len(records) == 0 {
		return records
	}

	type key struct {
		source, sensorID string
		bucketMs         int64
	}

	buckets := make(map[key][]logger.Record)
	var order []key

	fromMs := from.UnixMilli()
	for _, r := range records {
		offset := r.Timestamp.UnixMilli() - fromMs
		bucketMs := fromMs + (offset/stepMs)*stepMs
		k := key{r.Source, r.SensorID, bucketMs}
		if _, exists := buckets[k]; !exists {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], r)
	}

	result := make([]logger.Record, 0, len(order))
	for _, k := range order {
		result = append(result, medianRecord(buckets[k], time.UnixMilli(k.bucketMs).UTC()))
	}
	return result
}

func medianRecord(recs []logger.Record, ts time.Time) logger.Record {
	n := len(recs)
	if n == 1 {
		r := recs[0]
		r.Timestamp = ts
		return r
	}

	base := recs[0]
	get := func(f func(logger.Record) float64) float64 {
		vals := make([]float64, n)
		for i, r := range recs {
			vals[i] = f(r)
		}
		return medianFloat64(vals)
	}

	return logger.Record{
		Timestamp: ts,
		Source:    base.Source,
		SensorID:  base.SensorID,
		Location:  base.Location,
		TempC:     get(func(r logger.Record) float64 { return r.TempC }),
		Humidity:  get(func(r logger.Record) float64 { return r.Humidity }),
		DewPointC: get(func(r logger.Record) float64 { return r.DewPointC }),
		Pressure:  get(func(r logger.Record) float64 { return r.Pressure }),
		PM1:       get(func(r logger.Record) float64 { return r.PM1 }),
		PM25:      get(func(r logger.Record) float64 { return r.PM25 }),
		PM10:      get(func(r logger.Record) float64 { return r.PM10 }),
		AQI:       get(func(r logger.Record) float64 { return r.AQI }),
		VOC:       get(func(r logger.Record) float64 { return r.VOC }),
		P03:       get(func(r logger.Record) float64 { return r.P03 }),
		P05:       get(func(r logger.Record) float64 { return r.P05 }),
		P10:       get(func(r logger.Record) float64 { return r.P10 }),
		P25:       get(func(r logger.Record) float64 { return r.P25 }),
		P50:       get(func(r logger.Record) float64 { return r.P50 }),
		P100:      get(func(r logger.Record) float64 { return r.P100 }),
	}
}

// downsampleWeather buckets weather readings into fixed-width time steps and
// returns one reading per (location, bucket) with the median of each numeric field.
// WeatherCode uses the middle record's value; Sunrise/Sunset use the first non-zero
// value in the bucket (they are per-day, not per-reading).
func downsampleWeather(records []openmeteo.Reading, from time.Time, stepMs int64) []openmeteo.Reading {
	if stepMs <= 0 || len(records) == 0 {
		return records
	}

	type key struct {
		lat, lon float64
		bucketMs int64
	}

	buckets := make(map[key][]openmeteo.Reading)
	var order []key

	fromMs := from.UnixMilli()
	for _, r := range records {
		offset := r.Time.UnixMilli() - fromMs
		bucketMs := fromMs + (offset/stepMs)*stepMs
		k := key{r.Latitude, r.Longitude, bucketMs}
		if _, exists := buckets[k]; !exists {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], r)
	}

	result := make([]openmeteo.Reading, 0, len(order))
	for _, k := range order {
		result = append(result, medianWeatherReading(buckets[k], time.UnixMilli(k.bucketMs).UTC()))
	}
	return result
}

func medianWeatherReading(recs []openmeteo.Reading, ts time.Time) openmeteo.Reading {
	n := len(recs)
	if n == 1 {
		r := recs[0]
		r.Time = ts
		return r
	}

	base := recs[0]
	get := func(f func(openmeteo.Reading) float64) float64 {
		vals := make([]float64, n)
		for i, r := range recs {
			vals[i] = f(r)
		}
		return medianFloat64(vals)
	}

	// WeatherCode: use the middle record (median index after sorting by time is reasonable)
	sorted := make([]openmeteo.Reading, n)
	copy(sorted, recs)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })
	weatherCode := sorted[n/2].WeatherCode

	// Sunrise/Sunset: per-day values — take first non-zero in the bucket
	var sunrise, sunset time.Time
	for _, r := range recs {
		if sunrise.IsZero() && !r.Sunrise.IsZero() {
			sunrise = r.Sunrise
		}
		if sunset.IsZero() && !r.Sunset.IsZero() {
			sunset = r.Sunset
		}
		if !sunrise.IsZero() && !sunset.IsZero() {
			break
		}
	}

	return openmeteo.Reading{
		Time:            ts,
		Latitude:        base.Latitude,
		Longitude:       base.Longitude,
		TempC:           get(func(r openmeteo.Reading) float64 { return r.TempC }),
		Humidity:        get(func(r openmeteo.Reading) float64 { return r.Humidity }),
		DewPointC:       get(func(r openmeteo.Reading) float64 { return r.DewPointC }),
		PrecipitationMM: get(func(r openmeteo.Reading) float64 { return r.PrecipitationMM }),
		WeatherCode:     weatherCode,
		PressureHPa:     get(func(r openmeteo.Reading) float64 { return r.PressureHPa }),
		WindSpeedKMH:    get(func(r openmeteo.Reading) float64 { return r.WindSpeedKMH }),
		UVIndex:         get(func(r openmeteo.Reading) float64 { return r.UVIndex }),
		Sunrise:         sunrise,
		Sunset:          sunset,
	}
}
