package repositories

import (
	"testing"

	legacy "backend/handlers"
)

func TestBuildReturnPatch_BookingIDPreservedWhenMissing(t *testing.T) {
	existing := legacy.DepartureSetting{ID: 10, BookingID: 44, PassengerCount: "2"}
	raw := []byte(`{"departure_status":"Pulang"}`)

	merged, presence, count, err := buildReturnPatch(existing, raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if presence.BookingID {
		t.Fatalf("booking_id should not be present")
	}
	if merged.BookingID != existing.BookingID {
		t.Fatalf("booking_id changed to %d", merged.BookingID)
	}
	if count != 2 {
		t.Fatalf("passenger count parsed incorrectly, got %d", count)
	}
}

func TestBuildReturnPatch_BookingIDUpdatedWhenPresent(t *testing.T) {
	existing := legacy.DepartureSetting{ID: 10, BookingID: 44, PassengerCount: "2"}
	raw := []byte(`{"booking_id":123,"departure_status":"Pulang"}`)

	merged, presence, _, err := buildReturnPatch(existing, raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !presence.BookingID {
		t.Fatalf("booking_id should be present")
	}
	if merged.BookingID != 123 {
		t.Fatalf("booking_id not updated, got %d", merged.BookingID)
	}
}

func TestBuildReturnPatch_BookingIDZeroIgnored(t *testing.T) {
	existing := legacy.DepartureSetting{ID: 10, BookingID: 44, PassengerCount: "2"}
	raw := []byte(`{"booking_id":0,"departure_status":"Pulang"}`)

	merged, presence, _, err := buildReturnPatch(existing, raw)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if presence.BookingID {
		t.Fatalf("booking_id presence should be false when value zero")
	}
	if merged.BookingID != existing.BookingID {
		t.Fatalf("booking_id should remain unchanged, got %d", merged.BookingID)
	}
}
