package logger

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
