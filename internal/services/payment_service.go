package services

import (
    "encoding/json"
    "log"
    "strconv"
    "strings"

    "backend/internal/domain"
    "backend/internal/repositories"
    "backend/internal/utils"
)

// PaymentService menangani validasi pembayaran dan sinkron trip role.
type PaymentService struct {
    PaymentRepo     repositories.PaymentRepository
    BookingRepo     repositories.BookingRepository
    BookingSeatRepo repositories.BookingSeatRepository
    DepartureSvc    DepartureService
    ReturnSvc       ReturnService
    RequestID       string
    PassengerSvc    PassengerService
}

// ValidatePayment menyimpan payment_validations, update status booking, dan sinkron ke departure/return sesuai trip_role.
func (s PaymentService) ValidatePayment(bookingID int64, raw json.RawMessage) error {
    if bookingID <= 0 {
        return domain.ValidationError{Field: "booking_id", Msg: "id tidak valid"}
    }

    // simpan payment_validations jika ada payload
    if len(raw) > 0 {
        if err := s.PaymentRepo.CreateOrUpdateValidation(bookingID, raw); err != nil {
            utils.LogEvent(s.RequestID, "payment", "validate", "upsert warning: "+err.Error())
            // jangan return, karena sistem kamu sebelumnya memang "warning"
        }
    }

    // update status booking ke Lunas
    if err := s.BookingRepo.UpdatePaymentStatus(bookingID, "Lunas", readMethod(raw)); err != nil {
        // sebelumnya hanya warning, tapi untuk flow "setelah lunas harus ada passengers"
        // lebih aman: return error supaya jelas kalau status gagal diupdate
        utils.LogEvent(s.RequestID, "payment", "validate", "update booking status failed: "+err.Error())
        return err
    }

    // buat/rapikan departure/return settings dari booking
    booking, err := s.BookingRepo.GetByID(bookingID)
    if err != nil {
        return err
    }
    seats, _ := s.BookingSeatRepo.GetSeats(bookingID)

    tripRole := readTripRole(raw)
    if tripRole == "" {
        if existing, _ := s.PaymentRepo.GetByBookingID(bookingID); existing.TripRole != "" {
            tripRole = existing.TripRole
        }
    }
    utils.LogEvent(s.RequestID, "payment", "validate", "booking_id="+strconv.FormatInt(bookingID, 10)+" role="+tripRole)

    s.DepartureSvc.RequestID = s.RequestID
    s.ReturnSvc.RequestID = s.RequestID
    s.PassengerSvc.RequestID = s.RequestID

    if isReturnTrip(tripRole) {
        ret, err := s.ReturnSvc.CreateOrUpdateFromBooking(booking, seats)
        if err != nil {
            log.Println("[PAYMENT] create/update return warning:", err)
            return err
        }

        // JANGAN DIBUANG ERROR-NYA: sinkron penumpang lebih awal agar e-ticket per seat siap
        if err := s.PassengerSvc.SyncFromReturn(ret); err != nil {
            log.Println("[PAYMENT] passenger sync return failed:", err)
            utils.LogEvent(s.RequestID, "payment", "validate", "passenger sync return failed: "+err.Error())
            return err
        }
        return nil
    }

    dep, err := s.DepartureSvc.CreateOrUpdateFromBooking(booking, seats)
    if err != nil {
        log.Println("[PAYMENT] create/update departure warning:", err)
        return err
    }

    // JANGAN DIBUANG ERROR-NYA:
    if err := s.PassengerSvc.SyncFromDeparture(dep); err != nil {
        log.Println("[PAYMENT] passenger sync departure failed:", err)
        utils.LogEvent(s.RequestID, "payment", "validate", "passenger sync departure failed: "+err.Error())
        return err
    }
    return nil
}

// ValidateLunas wrapper agar kompatibel dengan flow lama.
func (s PaymentService) ValidateLunas(bookingID int64, raw json.RawMessage) error {
    return s.ValidatePayment(bookingID, raw)
}

func readMethod(raw json.RawMessage) string {
    if len(raw) == 0 {
        return ""
    }
    var m map[string]any
    if err := json.Unmarshal(raw, &m); err != nil {
        return ""
    }
    for _, k := range []string{"payment_method", "paymentMethod"} {
        if v, ok := m[k]; ok {
            if s, ok := v.(string); ok {
                return strings.TrimSpace(s)
            }
        }
        if v, ok := m[strings.ToLower(k)]; ok {
            if s, ok := v.(string); ok {
                return strings.TrimSpace(s)
            }
        }
    }
    return ""
}

func readTripRole(raw json.RawMessage) string {
    if len(raw) == 0 {
        return ""
    }
    var m map[string]any
    if err := json.Unmarshal(raw, &m); err != nil {
        return ""
    }
    for _, k := range []string{"trip_role", "tripRole"} {
        if v, ok := m[k]; ok {
            if s, ok := v.(string); ok {
                return strings.TrimSpace(s)
            }
        }
        if v, ok := m[strings.ToLower(k)]; ok {
            if s, ok := v.(string); ok {
                return strings.TrimSpace(s)
            }
        }
    }
    return ""
}

func isReturnTrip(role string) bool {
    role = strings.ToLower(strings.TrimSpace(role))
    switch role {
    case "pulang", "return", "kepulangan", "return_trip":
        return true
    default:
        return false
    }
}
