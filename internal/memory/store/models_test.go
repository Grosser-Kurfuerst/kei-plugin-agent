package store

import (
	"testing"
	"time"
)

func TestLocalDate(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 3, 17, 0, 0, 0, time.UTC)

	got := LocalDate(at, shanghai)
	want := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) || got.Location() != time.UTC {
		t.Fatalf("LocalDate = %v, want %v", got, want)
	}

	if got := LocalDate(at, time.UTC); !got.Equal(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("LocalDate(UTC) = %v", got)
	}
}
