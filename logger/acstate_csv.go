package logger

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
)

// ACStateRecord represents a single AC state change event.
type ACStateRecord struct {
	Timestamp         string
	DeviceID          string
	RoomName          string
	On                bool
	Mode              string
	TargetTemp        int
	TempUnit          string
	FanLevel          string
	Swing             string
	HorizontalSwing   string
	ChangedProperties string // semicolon-separated list
	Reason            string
	SensiboID         string // unique ID from the API, used for dedup
}

var acStateCSVHeader = []string{
	"timestamp", "device_id", "room", "on", "mode", "target_temp", "temp_unit",
	"fan_level", "swing", "horizontal_swing", "changed_properties", "reason", "sensibo_id",
}

// ACStateCSVLogger appends AC state change records to a CSV file.
type ACStateCSVLogger struct {
	file   *os.File
	writer *csv.Writer
}

// NewACStateCSVLogger opens (or creates) a CSV file for AC state logging.
// If the file is new/empty, a header row is written.
func NewACStateCSVLogger(path string) (*ACStateCSVLogger, error) {
	writeHeader := false
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		writeHeader = true
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open ac state log file: %w", err)
	}

	w := csv.NewWriter(f)

	if writeHeader {
		if err := w.Write(acStateCSVHeader); err != nil {
			f.Close()
			return nil, fmt.Errorf("write header: %w", err)
		}
		w.Flush()
	}

	return &ACStateCSVLogger{file: f, writer: w}, nil
}

// Log writes a single AC state change record to the CSV file.
func (l *ACStateCSVLogger) Log(r ACStateRecord) error {
	row := []string{
		r.Timestamp,
		r.DeviceID,
		r.RoomName,
		strconv.FormatBool(r.On),
		r.Mode,
		strconv.Itoa(r.TargetTemp),
		r.TempUnit,
		r.FanLevel,
		r.Swing,
		r.HorizontalSwing,
		r.ChangedProperties,
		r.Reason,
		r.SensiboID,
	}
	if err := l.writer.Write(row); err != nil {
		return fmt.Errorf("write ac state record: %w", err)
	}
	l.writer.Flush()
	return l.writer.Error()
}

// Close flushes and closes the underlying file.
func (l *ACStateCSVLogger) Close() error {
	l.writer.Flush()
	return l.file.Close()
}

// LoadExistingACStateIDs reads the AC state CSV and returns a set of
// already-recorded Sensibo IDs. Returns an empty map if the file does not exist.
func LoadExistingACStateIDs(path string) (map[string]bool, error) {
	ids := make(map[string]bool)

	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ids, nil
		}
		return nil, fmt.Errorf("open ac state log: %w", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	// Read and discard header.
	if _, err := r.Read(); err != nil {
		if errors.Is(err, io.EOF) {
			return ids, nil
		}
		return nil, fmt.Errorf("read header: %w", err)
	}

	// sensibo_id is the last column (index 12).
	for {
		record, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read ac state row: %w", err)
		}
		if len(record) > 12 {
			ids[record[12]] = true
		}
	}

	return ids, nil
}
