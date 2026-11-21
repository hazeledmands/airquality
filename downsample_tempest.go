package main

import (
	"math"
	"sort"
	"time"

	"github.com/hazel/airquality/tempest"
)

// tempestBucket is an observation as served to the dashboard: raw, or a
// downsampled bucket. RainMaxMM is the wettest single interval in the
// bucket, so a summed bucket doesn't smooth away the peak rain rate.
type tempestBucket struct {
	tempest.Observation
	RainMaxMM *float64 `json:"rain_max_mm"`
}

func rawBuckets(obs []tempest.Observation) []tempestBucket {
	out := make([]tempestBucket, len(obs))
	for i, o := range obs {
		out[i] = tempestBucket{Observation: o, RainMaxMM: o.RainMM}
	}
	return out
}

// downsampleTempest buckets observations into fixed steps per device. Each
// field is aggregated by what it means, so a coarse view still shows the
// truth: gusts keep their maximum and lulls their minimum, rain and strike
// counts are summed (with the wettest interval kept in RainMaxMM), the daily
// rain total takes the bucket's last value (or, when the bucket spans the
// local-midnight reset, the day's final total), wind direction is a
// speed-weighted vector mean, strike distance is weighted by strike count,
// and the rest take the median. Null-only fields stay null.
func downsampleTempest(obs []tempest.Observation, from time.Time, stepMs int64) []tempestBucket {
	if stepMs <= 0 || len(obs) == 0 {
		return rawBuckets(obs)
	}
	type key struct {
		device   int
		bucketMs int64
	}
	buckets := make(map[key][]tempest.Observation)
	var order []key
	fromMs := from.UnixMilli()
	for _, o := range obs {
		offset := o.Time.UnixMilli() - fromMs
		b := fromMs + floorDiv(offset, stepMs)*stepMs
		k := key{o.DeviceID, b}
		if _, ok := buckets[k]; !ok {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], o)
	}
	out := make([]tempestBucket, 0, len(order))
	for _, k := range order {
		out = append(out, aggregateTempest(buckets[k], time.UnixMilli(k.bucketMs).UTC()))
	}
	return out
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

func aggregateTempest(recs []tempest.Observation, ts time.Time) tempestBucket {
	if len(recs) == 1 {
		o := recs[0]
		o.Time = ts
		return tempestBucket{Observation: o, RainMaxMM: o.RainMM}
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].Time.Before(recs[j].Time) })
	vals := func(f func(tempest.Observation) *float64) []float64 {
		var v []float64
		for _, r := range recs {
			if p := f(r); p != nil {
				v = append(v, *p)
			}
		}
		return v
	}
	ints := func(f func(tempest.Observation) *int) []float64 {
		return vals(func(o tempest.Observation) *float64 { return intToFloat(f(o)) })
	}
	median := func(f func(tempest.Observation) *float64) *float64 {
		v := vals(f)
		if len(v) == 0 {
			return nil
		}
		m := medianFloat64(v)
		return &m
	}
	reduce := func(v []float64, fn func(a, b float64) float64) *float64 {
		if len(v) == 0 {
			return nil
		}
		acc := v[0]
		for _, x := range v[1:] {
			acc = fn(acc, x)
		}
		return &acc
	}
	sum := func(a, b float64) float64 { return a + b }
	// dayTotal is the last value, unless the running total resets inside
	// the bucket; then it is the day's final (maximum) value, so the total
	// a day reached is never lost to the reset.
	dayTotal := func(f func(tempest.Observation) *float64) *float64 {
		v := vals(f)
		if len(v) == 0 {
			return nil
		}
		for i := 1; i < len(v); i++ {
			if v[i] < v[i-1] {
				return reduce(v, math.Max)
			}
		}
		return &v[len(v)-1]
	}
	toInt := func(p *float64) *int {
		if p == nil {
			return nil
		}
		n := int(math.Round(*p))
		return &n
	}

	o := tempest.Observation{
		DeviceID:               recs[0].DeviceID,
		Time:                   ts,
		WindLullMS:             reduce(vals(func(o tempest.Observation) *float64 { return o.WindLullMS }), math.Min),
		WindAvgMS:              median(func(o tempest.Observation) *float64 { return o.WindAvgMS }),
		WindGustMS:             reduce(vals(func(o tempest.Observation) *float64 { return o.WindGustMS }), math.Max),
		WindDirectionDeg:       vectorMeanDirection(recs),
		WindSampleIntervalS:    median(func(o tempest.Observation) *float64 { return o.WindSampleIntervalS }),
		StationPressureMB:      median(func(o tempest.Observation) *float64 { return o.StationPressureMB }),
		AirTempC:               median(func(o tempest.Observation) *float64 { return o.AirTempC }),
		RelativeHumidity:       median(func(o tempest.Observation) *float64 { return o.RelativeHumidity }),
		IlluminanceLux:         median(func(o tempest.Observation) *float64 { return o.IlluminanceLux }),
		UVIndex:                median(func(o tempest.Observation) *float64 { return o.UVIndex }),
		SolarRadiationWM2:      median(func(o tempest.Observation) *float64 { return o.SolarRadiationWM2 }),
		RainMM:                 reduce(vals(func(o tempest.Observation) *float64 { return o.RainMM }), sum),
		PrecipType:             toInt(reduce(ints(func(o tempest.Observation) *int { return o.PrecipType }), math.Max)),
		LightningStrikeCount:   toInt(reduce(ints(func(o tempest.Observation) *int { return o.LightningStrikeCount }), sum)),
		LightningAvgDistanceKM: strikeWeightedDistance(recs),
		BatteryV:               median(func(o tempest.Observation) *float64 { return o.BatteryV }),
		ReportIntervalMin:      toInt(median(func(o tempest.Observation) *float64 { return intToFloat(o.ReportIntervalMin) })),
		LocalDayRainMM:         dayTotal(func(o tempest.Observation) *float64 { return o.LocalDayRainMM }),
		RainFinalMM:            reduce(vals(func(o tempest.Observation) *float64 { return o.RainFinalMM }), sum),
		LocalDayRainFinalMM:    dayTotal(func(o tempest.Observation) *float64 { return o.LocalDayRainFinalMM }),
		PrecipAnalysisType:     toInt(reduce(ints(func(o tempest.Observation) *int { return o.PrecipAnalysisType }), math.Max)),
	}
	return tempestBucket{Observation: o, RainMaxMM: reduce(vals(func(o tempest.Observation) *float64 { return o.RainMM }), math.Max)}
}

func intToFloat(p *int) *float64 {
	if p == nil {
		return nil
	}
	v := float64(*p)
	return &v
}

// vectorMeanDirection averages wind direction as vectors weighted by average
// speed, so 350° and 10° give 0°, not 180°. With no speeds it weights equally.
func vectorMeanDirection(recs []tempest.Observation) *float64 {
	var x, y, weightSum float64
	n := 0
	for _, r := range recs {
		if r.WindDirectionDeg == nil {
			continue
		}
		w := 1.0
		if r.WindAvgMS != nil {
			w = *r.WindAvgMS
		}
		rad := *r.WindDirectionDeg * math.Pi / 180
		x += w * math.Sin(rad)
		y += w * math.Cos(rad)
		weightSum += w
		n++
	}
	if n == 0 {
		return nil
	}
	if weightSum == 0 { // calm throughout: fall back to an unweighted mean
		x, y = 0, 0
		for _, r := range recs {
			if r.WindDirectionDeg != nil {
				rad := *r.WindDirectionDeg * math.Pi / 180
				x += math.Sin(rad)
				y += math.Cos(rad)
			}
		}
	}
	deg := math.Atan2(x, y) * 180 / math.Pi
	if deg < 0 {
		deg += 360
	}
	return &deg
}

func strikeWeightedDistance(recs []tempest.Observation) *float64 {
	var total, count float64
	for _, r := range recs {
		if r.LightningAvgDistanceKM == nil || r.LightningStrikeCount == nil || *r.LightningStrikeCount == 0 {
			continue
		}
		c := float64(*r.LightningStrikeCount)
		total += *r.LightningAvgDistanceKM * c
		count += c
	}
	if count == 0 {
		return nil
	}
	d := total / count
	return &d
}
