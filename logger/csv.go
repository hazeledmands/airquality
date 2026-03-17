package logger

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
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
// It deduplicates by source+sensor_id+minute so that re-running
// within the same minute does not create duplicate entries.
type CSVLogger struct {
	file   *os.File
	writer *csv.Writer
	seen   map[string]bool
}

// NewCSVLogger opens (or creates) a CSV file for logging.
// If the file is new/empty, a header row is written.
// Existing records are loaded to prevent duplicate entries.
func NewCSVLogger(path string) (*CSVLogger, error) {
	seen, err := loadExistingRecordKeys(path)
	if err != nil {
		return nil, err
	}

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

	return &CSVLogger{file: f, writer: w, seen: seen}, nil
}

// Log writes a single record to the CSV file.
// If a record with the same source, sensor ID, and minute already exists,
// the write is silently skipped.
func (l *CSVLogger) Log(r Record) error {
	key := recordKey(r.Source, r.SensorID, r.Timestamp)
	if l.seen[key] {
		return nil
	}

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
	l.seen[key] = true
	return l.writer.Error()
}

// Close flushes and closes the underlying file.
func (l *CSVLogger) Close() error {
	l.writer.Flush()
	return l.file.Close()
}

// recordKey returns a dedup key for a record: "source:sensor_id:YYYY-MM-DDTHH:MM".
func recordKey(source, sensorID string, ts time.Time) string {
	return source + ":" + sensorID + ":" + ts.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

// loadExistingRecordKeys reads an existing CSV file and returns a set of
// dedup keys. Returns an empty map if the file does not exist.
func loadExistingRecordKeys(path string) (map[string]bool, error) {
	keys := make(map[string]bool)

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return keys, nil
		}
		return nil, fmt.Errorf("open log for dedup: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	// Skip header.
	if _, err := r.Read(); err != nil {
		if errors.Is(err, io.EOF) {
			return keys, nil
		}
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Columns: 0=timestamp, 1=source, 2=sensor_id
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read record: %w", err)
		}
		if len(record) < 3 {
			continue
		}
		ts, parseErr := time.Parse(time.RFC3339, record[0])
		if parseErr != nil {
			continue
		}
		keys[recordKey(record[1], record[2], ts)] = true
	}

	return keys, nil
}

func optionalFloat(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.1f", v)
}
