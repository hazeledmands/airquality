package calc

import (
	"math"
	"testing"
)

func TestCtoF(t *testing.T) {
	tests := []struct {
		name  string
		c     float64
		wantF float64
	}{
		{"freezing", 0.0, 32.0},
		{"boiling", 100.0, 212.0},
		{"body temp", 37.0, 98.6},
		{"negative", -40.0, -40.0},
		{"room temp", 22.0, 71.6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CtoF(tc.c)
			if math.Abs(got-tc.wantF) > 0.1 {
				t.Errorf("CtoF(%.1f) = %.1f, want %.1f", tc.c, got, tc.wantF)
			}
		})
	}
}

func TestFtoC(t *testing.T) {
	tests := []struct {
		name  string
		f     float64
		wantC float64
	}{
		{"freezing", 32.0, 0.0},
		{"boiling", 212.0, 100.0},
		{"body temp", 98.6, 37.0},
		{"negative", -40.0, -40.0},
		{"room temp", 71.6, 22.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FtoC(tc.f)
			if math.Abs(got-tc.wantC) > 0.1 {
				t.Errorf("FtoC(%.1f) = %.1f, want %.1f", tc.f, got, tc.wantC)
			}
		})
	}
}

func TestDewPoint(t *testing.T) {
	tests := []struct {
		name     string
		tempC    float64
		humidity float64
		wantC    float64
	}{
		{"typical indoor", 22.0, 50.0, 11.1},
		{"hot humid", 35.0, 80.0, 31.0},
		{"cold dry", 5.0, 30.0, -11.2},
		{"saturated", 20.0, 100.0, 20.0},
		{"very dry", 25.0, 10.0, -8.8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DewPoint(tc.tempC, tc.humidity)
			if math.Abs(got-tc.wantC) > 0.5 {
				t.Errorf("DewPoint(%.1f, %.1f) = %.1f, want %.1f (±0.5)", tc.tempC, tc.humidity, got, tc.wantC)
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	// CtoF and FtoC should be inverses.
	for _, c := range []float64{-40, 0, 22, 37, 100} {
		got := FtoC(CtoF(c))
		if math.Abs(got-c) > 0.01 {
			t.Errorf("FtoC(CtoF(%.1f)) = %.4f, want %.1f", c, got, c)
		}
	}
}
