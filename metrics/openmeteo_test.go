package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hazel/airquality/openmeteo"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

var outdoor = []string{"weather", "open-meteo", "outdoor", "outdoor"}

func TestUpdateWeather_CloudCover(t *testing.T) {
	m := New()
	m.UpdateWeather(openmeteo.Reading{Latitude: 12.3456, Longitude: -65.4321, CloudCover: fp(81)})
	if v := testutil.ToFloat64(m.cloudCover.WithLabelValues(outdoor...)); v != 81 {
		t.Errorf("cloud cover = %v", v)
	}
	m.UpdateWeather(openmeteo.Reading{Latitude: 12.3456, Longitude: -65.4321})
	if n := testutil.CollectAndCount(m.cloudCover); n != 0 {
		t.Errorf("null cloud cover left %d series", n)
	}
}

func TestUpdateAirQuality(t *testing.T) {
	m := New()
	m.UpdateAirQuality(openmeteo.AirQuality{
		Latitude: 12.3456, Longitude: -65.4321,
		PM25: fp(24), PM10: fp(26.7), Ozone: fp(13), NO2: fp(67.1), SO2: fp(5), CO: fp(279),
		USAQI: fp(86), EuropeanAQI: fp(64), Dust: fp(0), GrassPollen: fp(12),
	})
	// Outdoor PM2.5, PM10 and AQI share the gauges indoor sensors use.
	if v := testutil.ToFloat64(m.pm25.WithLabelValues(outdoor...)); v != 24 {
		t.Errorf("pm25 = %v", v)
	}
	if v := testutil.ToFloat64(m.aqi.WithLabelValues(outdoor...)); v != 86 {
		t.Errorf("aqi = %v", v)
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body := w.Body.String()
	for _, want := range []string{
		`airquality_ozone_ugm3{location="outdoor",room="outdoor",sensor_id="open-meteo",source="weather"} 13`,
		`airquality_no2_ugm3{`, `airquality_so2_ugm3{`, `airquality_co_ugm3{`,
		`airquality_european_aqi{`, `airquality_dust_ugm3{`,
		`airquality_pollen_grains_m3{location="outdoor",pollen="grass",room="outdoor",sensor_id="open-meteo",source="weather"} 12`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("/metrics lacks %s", want)
		}
	}
	// Pollen the model doesn't cover (nil) has no series at all.
	if strings.Contains(body, `pollen="ragweed"`) {
		t.Error("nil ragweed pollen exported")
	}
}

func TestUpdateForecast(t *testing.T) {
	m := New()
	now := time.Date(2026, 11, 21, 20, 30, 0, 0, time.UTC)
	var hours []openmeteo.ForecastHour
	for h := 0; h < 72; h++ {
		ts := time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC).Add(time.Duration(h) * time.Hour)
		hours = append(hours, openmeteo.ForecastHour{Time: ts, TempC: fp(float64(h)), PrecipProbability: fp(float64(h % 100)), CloudCover: fp(50)})
	}
	m.UpdateForecast(12.3456, -65.4321, hours, now)
	// 1 h ahead of 20:30 is the 22:00 hour (index 22), the first starting at or after 21:30.
	l1 := append(append([]string{}, outdoor...), "1")
	if v := testutil.ToFloat64(m.forecastTemp.WithLabelValues(l1...)); v != 22 {
		t.Errorf("1 h ahead temp = %v, want 22", v)
	}
	l24 := append(append([]string{}, outdoor...), "24")
	if v := testutil.ToFloat64(m.forecastPrecipProb.WithLabelValues(l24...)); v != 45 {
		t.Errorf("24 h ahead precip probability = %v, want 45", v)
	}
	if n := testutil.CollectAndCount(m.forecastTemp); n != 6 {
		t.Errorf("forecast temp series = %d, want one per horizon (1,3,6,12,24,48)", n)
	}
}

// The location must not appear in any label: only one is ever configured.
func TestOpenMeteoLabelsCarryNoCoordinates(t *testing.T) {
	m := New()
	m.UpdateWeather(openmeteo.Reading{Latitude: 12.3456, Longitude: -65.4321, TempC: 1, CloudCover: fp(5)})
	m.UpdateAirQuality(openmeteo.AirQuality{Latitude: 12.3456, Longitude: -65.4321, PM25: fp(1)})
	m.UpdateForecast(12.3456, -65.4321, []openmeteo.ForecastHour{{Time: time.Now().Add(100 * time.Hour), TempC: fp(1)}}, time.Now())
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(w.Body.String(), "12.3456") || strings.Contains(w.Body.String(), "65.4321") {
		t.Error("/metrics carries the coordinates")
	}
}
