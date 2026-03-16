// Package purpleair provides a client for local PurpleAir sensors.
package purpleair

// SensorReading represents the JSON response from a local PurpleAir sensor's /json endpoint.
type SensorReading struct {
	// Metadata
	SensorID           string  `json:"SensorId"`
	DateTime           string  `json:"DateTime"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	Place              string  `json:"place"`
	Version            string  `json:"version"`
	Uptime             int     `json:"uptime"`
	RSSI               int     `json:"rssi"`
	SSID               string  `json:"ssid"`
	HardwareVersion    string  `json:"hardwareversion"`
	HardwareDiscovered string  `json:"hardwarediscovered"`
	LoggingRate        int     `json:"loggingrate"`
	Period             int     `json:"period"`

	// Environmental (BME280/BME680)
	TempF         float64 `json:"current_temp_f"`
	Humidity      float64 `json:"current_humidity"`
	DewPointF     float64 `json:"current_dewpoint_f"`
	Pressure      float64 `json:"pressure"`
	TempF680      float64 `json:"current_temp_f_680"`
	Humidity680   float64 `json:"current_humidity_680"`
	DewPointF680  float64 `json:"current_dewpoint_f_680"`
	Pressure680   float64 `json:"pressure_680"`
	Gas680        float64 `json:"gas_680"`

	// PM - Channel A
	PM1CF1   float64 `json:"pm1_0_cf_1"`
	PM25CF1  float64 `json:"pm2_5_cf_1"`
	PM10CF1  float64 `json:"pm10_0_cf_1"`
	PM1ATM   float64 `json:"pm1_0_atm"`
	PM25ATM  float64 `json:"pm2_5_atm"`
	PM10ATM  float64 `json:"pm10_0_atm"`
	PM25AQI  float64 `json:"pm2.5_aqi"`
	AQIColor string  `json:"p25aqic"`

	// Particle counts - Channel A (per deciliter)
	P03  float64 `json:"p_0_3_um"`
	P05  float64 `json:"p_0_5_um"`
	P10  float64 `json:"p_1_0_um"`
	P25  float64 `json:"p_2_5_um"`
	P50  float64 `json:"p_5_0_um"`
	P100 float64 `json:"p_10_0_um"`

	// PM - Channel B
	PM1CF1B  float64 `json:"pm1_0_cf_1_b"`
	PM25CF1B float64 `json:"pm2_5_cf_1_b"`
	PM10CF1B float64 `json:"pm10_0_cf_1_b"`
	PM1ATMB  float64 `json:"pm1_0_atm_b"`
	PM25ATMB float64 `json:"pm2_5_atm_b"`
	PM10ATMB float64 `json:"pm10_0_atm_b"`
	PM25AQIB float64 `json:"pm2.5_aqi_b"`
	AQIColorB string `json:"p25aqic_b"`

	// Particle counts - Channel B (per deciliter)
	P03B  float64 `json:"p_0_3_um_b"`
	P05B  float64 `json:"p_0_5_um_b"`
	P10B  float64 `json:"p_1_0_um_b"`
	P25B  float64 `json:"p_2_5_um_b"`
	P50B  float64 `json:"p_5_0_um_b"`
	P100B float64 `json:"p_10_0_um_b"`

	// Misc
	Adc       float64 `json:"Adc"`
	PALatency int     `json:"pa_latency"`
}
