package repositories

import (
	"testing"

	legacy "backend/handlers"
)

func TestBuildDeparturePatch_BookingIDNotTouchedWhenMissing(t *testing.T) {
	existing := legacy.DepartureSetting{ID: 1, BookingID: 55, PassengerCount: "2"}
	raw := []byte(`{"departure_status":"Berangkat"}`)

	merged, presence, count, err := buildDeparturePatch(existing, raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if presence.BookingID {
		t.Fatalf("booking_id should not be marked present when key missing")
	}
	if merged.BookingID != existing.BookingID {
		t.Fatalf("booking_id changed unexpectedly: got %d want %d", merged.BookingID, existing.BookingID)
	}
	if count != 2 {
		t.Fatalf("passenger count parsed incorrectly, got %d", count)
	}
}

func TestBuildDeparturePatch_BookingIDUpdatedWhenPresent(t *testing.T) {
	existing := legacy.DepartureSetting{ID: 1, BookingID: 55, PassengerCount: "3"}
	raw := []byte(`{"booking_id":123,"departure_status":"Berangkat"}`)

	merged, presence, _, err := buildDeparturePatch(existing, raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !presence.BookingID {
		t.Fatalf("booking_id should be marked present")
	}
	if merged.BookingID != 123 {
		t.Fatalf("booking_id not updated, got %d", merged.BookingID)
	}
}

func TestBuildDeparturePatch_BookingIDZeroIgnored(t *testing.T) {
	existing := legacy.DepartureSetting{ID: 1, BookingID: 77, PassengerCount: "1"}
	raw := []byte(`{"booking_id":0,"departure_status":"Berangkat"}`)

	merged, presence, _, err := buildDeparturePatch(existing, raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if presence.BookingID {
		t.Fatalf("booking_id presence should be false when value is zero")
	}
	if merged.BookingID != existing.BookingID {
		t.Fatalf("booking_id should stay the same, got %d", merged.BookingID)
	}
}
