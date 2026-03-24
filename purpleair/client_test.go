package purpleair

import (
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

// sampleJSON is taken from a real PurpleAir local sensor response.
const sampleJSON = `{
	"SensorId":"a4:e5:7c:28:49:7",
	"DateTime":"2026/03/16T21:33:22z",
	"Geo":"PurpleAir-497",
	"Mem":5688,"memfrag":57,"memfb":1864,"memcs":96,"Id":21568,
	"lat":37.841400,"lon":-122.273697,
	"Adc":0.02,"loggingrate":15,"place":"outside","version":"7.04",
	"uptime":2574388,"rssi":-68,"period":120,
	"httpsuccess":22162,"httpsends":22165,
	"hardwareversion":"3.0",
	"hardwarediscovered":"3.0+OPENLOG+31954 MB+RV3028+BME68X+PMSX003-A+PMSX003-B",
	"current_temp_f":94,"current_humidity":19,"current_dewpoint_f":46,
	"pressure":1015.08,
	"current_temp_f_680":94,"current_humidity_680":19,"current_dewpoint_f_680":46,
	"pressure_680":1015.08,"gas_680":69.16,
	"p25aqic_b":"rgb(4,228,0)","pm2.5_aqi_b":12,
	"pm1_0_cf_1_b":2.03,"p_0_3_um_b":524.59,
	"pm2_5_cf_1_b":2.95,"p_0_5_um_b":153.38,
	"pm10_0_cf_1_b":3.40,"p_1_0_um_b":18.97,
	"pm1_0_atm_b":2.03,"p_2_5_um_b":1.81,
	"pm2_5_atm_b":2.95,"p_5_0_um_b":0.66,
	"pm10_0_atm_b":3.40,"p_10_0_um_b":0.41,
	"p25aqic":"rgb(1,228,0)","pm2.5_aqi":7,
	"pm1_0_cf_1":1.13,"p_0_3_um":481.50,
	"pm2_5_cf_1":1.72,"p_0_5_um":140.87,
	"pm10_0_cf_1":2.62,"p_1_0_um":17.27,
	"pm1_0_atm":1.13,"p_2_5_um":1.23,
	"pm2_5_atm":1.72,"p_5_0_um":1.10,
	"pm10_0_atm":2.62,"p_10_0_um":0.87,
	"pa_latency":264,
	"wlstate":"Connected",
	"status_0":2,"status_1":2,"status_2":2,"status_3":2,
	"status_4":2,"status_5":2,"status_7":0,"status_8":2,"status_9":2,
	"ssid":"luka-the-wonderdog"
}`

func newTestServer(response string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		w.Write([]byte(response))
	}))
}

func TestFetchReading(t *testing.T) {
	srv := newTestServer(sampleJSON, http.StatusOK)
	defer srv.Close()

	client := NewClient([]string{srv.Listener.Addr().String()})
	readings, err := client.FetchAll(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readings) != 1 {
		t.Fatalf("expected 1 reading, got %d", len(readings))
	}

	r := readings[0]

	// Metadata
	if r.SensorID != "a4:e5:7c:28:49:7" {
		t.Errorf("SensorID = %q, want %q", r.SensorID, "a4:e5:7c:28:49:7")
	}
	if r.Place != "outside" {
		t.Errorf("Place = %q, want %q", r.Place, "outside")
	}
	if r.HardwareVersion != "3.0" {
		t.Errorf("HardwareVersion = %q, want %q", r.HardwareVersion, "3.0")
	}
	if r.SSID != "luka-the-wonderdog" {
		t.Errorf("SSID = %q, want %q", r.SSID, "luka-the-wonderdog")
	}

	// Environmental
	if r.TempF != 94 {
		t.Errorf("TempF = %v, want 94", r.TempF)
	}
	if r.Humidity != 19 {
		t.Errorf("Humidity = %v, want 19", r.Humidity)
	}
	if r.Pressure != 1015.08 {
		t.Errorf("Pressure = %v, want 1015.08", r.Pressure)
	}
	if r.Gas680 != 69.16 {
		t.Errorf("Gas680 = %v, want 69.16", r.Gas680)
	}

	// PM - Channel A
	if r.PM25CF1 != 1.72 {
		t.Errorf("PM25CF1 = %v, want 1.72", r.PM25CF1)
	}
	if r.PM25AQI != 7 {
		t.Errorf("PM25AQI = %v, want 7", r.PM25AQI)
	}
	if r.PM10CF1 != 2.62 {
		t.Errorf("PM10CF1 = %v, want 2.62", r.PM10CF1)
	}

	// PM - Channel B
	if r.PM25CF1B != 2.95 {
		t.Errorf("PM25CF1B = %v, want 2.95", r.PM25CF1B)
	}
	if r.PM25AQIB != 12 {
		t.Errorf("PM25AQIB = %v, want 12", r.PM25AQIB)
	}

	// Particle counts
	if r.P03 != 481.50 {
		t.Errorf("P03 = %v, want 481.50", r.P03)
	}
}

func TestFetchReading_HTTPError(t *testing.T) {
	srv := newTestServer("sensor offline", http.StatusServiceUnavailable)
	defer srv.Close()

	client := NewClient([]string{srv.Listener.Addr().String()})
	_, err := client.FetchAll(t.Context())
	if err == nil {
		t.Fatal("expected error for 503 response")
	}
}

func TestFetchReading_ConnectionRefused(t *testing.T) {
	client := NewClient([]string{"127.0.0.1:1"}) // nothing listening
	_, err := client.FetchAll(t.Context())
	if err == nil {
		t.Fatal("expected error for connection refused")
	}
}

func TestFetchAll_MultipleHosts(t *testing.T) {
	srv1 := newTestServer(sampleJSON, http.StatusOK)
	defer srv1.Close()
	srv2 := newTestServer(sampleJSON, http.StatusOK)
	defer srv2.Close()

	client := NewClient([]string{
		srv1.Listener.Addr().String(),
		srv2.Listener.Addr().String(),
	})

	readings, err := client.FetchAll(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(readings) != 2 {
		t.Fatalf("expected 2 readings, got %d", len(readings))
	}
}

func TestSummary(t *testing.T) {
	r := SensorReading{
		SensorID: "a4:e5:7c:28:49:7",
		Place:    "outside",
		TempF:    94,
		Humidity: 19,
		Pressure: 1015.08,
		PM25CF1:  1.72,
		PM25CF1B: 2.95,
		PM25AQI:  7,
		PM25AQIB: 12,
		PM10CF1:  2.62,
		PM10CF1B: 3.40,
		PM1CF1:   1.13,
		PM1CF1B:  2.03,
	}

	s := r.Summary()

	// Temperature conversion: 94°F = 34.44°C
	if math.Abs(s.TempC-34.44) > 0.1 {
		t.Errorf("TempC = %.2f, want ~34.44", s.TempC)
	}
	if s.TempF != 94 {
		t.Errorf("TempF = %v, want 94", s.TempF)
	}

	// Dew point calculated from temp (C) and humidity
	// DewPoint(34.44, 19) ≈ 7.7°C
	if s.DewPointC == 0 {
		t.Error("expected DewPointC to be calculated")
	}
	if math.Abs(s.DewPointC-7.7) > 1.0 {
		t.Errorf("DewPointC = %.1f, want ~7.7", s.DewPointC)
	}

	// PM2.5 average of both channels
	wantPM25 := (1.72 + 2.95) / 2
	if math.Abs(s.PM25-wantPM25) > 0.01 {
		t.Errorf("PM25 = %.2f, want %.2f", s.PM25, wantPM25)
	}

	// AQI average of both channels
	wantAQI := (7.0 + 12.0) / 2
	if math.Abs(s.AQI-wantAQI) > 0.01 {
		t.Errorf("AQI = %.1f, want %.1f", s.AQI, wantAQI)
	}
}
