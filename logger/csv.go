package logger

import (
	"encoding/csv"
	"fmt"
	"os"
	"time"
)

// Record represents a single measurement from any sensor.
type Record struct {
	Timestamp time.Time
	Source    string
	SensorID string
	Location string
	TempC     float64
	Humidity  float64
	DewPointC float64
	Pressure  float64 // purpleair only
	PM1       float64 // purpleair only
	PM25      float64 // purpleair only
	PM10      float64 // purpleair only
	AQI       float64 // purpleair only
	VOC       float64 // purpleair only (gas_680 kOhm)
}

var csvHeader = []string{
	"timestamp", "source", "sensor_id", "location",
	"temp_c", "humidity", "dewpoint_c",
	"pressure", "pm1", "pm25", "pm10", "aqi", "voc",
}

// CSVLogger appends measurement records to a CSV file.
type CSVLogger struct {
	file   *os.File
	writer *csv.Writer
}

// NewCSVLogger opens (or creates) a CSV file for logging.
// If the file is new/empty, a header row is written.
func NewCSVLogger(path string) (*CSVLogger, error) {
	writeHeader := false
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		writeHeader = true
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	w := csv.NewWriter(f)

	if writeHeader {
		if err := w.Write(csvHeader); err != nil {
			f.Close()
			return nil, fmt.Errorf("write header: %w", err)
		}
		w.Flush()
	}

	return &CSVLogger{file: f, writer: w}, nil
}

// Log writes a single record to the CSV file.
func (l *CSVLogger) Log(r Record) error {
	row := []string{
		r.Timestamp.UTC().Format(time.RFC3339),
		r.Source,
		r.SensorID,
		r.Location,
		fmt.Sprintf("%.1f", r.TempC),
		fmt.Sprintf("%.1f", r.Humidity),
		fmt.Sprintf("%.1f", r.DewPointC),
		optionalFloat(r.Pressure),
		optionalFloat(r.PM1),
		optionalFloat(r.PM25),
		optionalFloat(r.PM10),
		optionalFloat(r.AQI),
		optionalFloat(r.VOC),
	}
	if err := l.writer.Write(row); err != nil {
		return fmt.Errorf("write record: %w", err)
	}
	l.writer.Flush()
	return l.writer.Error()
}

// Close flushes and closes the underlying file.
func (l *CSVLogger) Close() error {
	l.writer.Flush()
	return l.file.Close()
}

func optionalFloat(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.1f", v)
}
