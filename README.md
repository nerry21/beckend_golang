# Travel App Backend

## Menjalankan
- Atur environment: `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASS`, `DB_NAME`, `PORT` (default `8080`).
- Pastikan MySQL aktif dengan kredensial yang sesuai.
- Jalankan server: `go run .
- Router utama ada di `internal/http/router.go`.

## Alur Utama
1) Booking dibuat (reguler/custom) lalu seat dipilih per penumpang.
2) Input nama & no HP per seat: `POST /api/bookings/:id/passengers`.
3) Validasi pembayaran (approve/reject/cash) menjaga status booking.
4) Buat/ubah departure_settings atau return_settings sesuai trip role.
5) Mark berangkat/pulang men-trigger sinkronisasi ke `passengers` & `trip_information`.
6) Dokumen per penumpang: e-ticket & invoice per seat.
7) Laporan keuangan: berangkat dari `departure_settings`, pulang dari `return_settings`.

## Debug Request ID
- Sertakan header `X-Request-ID`; jika kosong akan dibuat otomatis.
- Semua log HTTP dan error response menyertakan `request_id`.

## Catatan Pengembangan
- Handler tipis, logika di services/repositories.
- PATCH aman: field hanya diperbarui jika key hadir (hindari payload parsial mengosongkan data).
- Logging aman: hindari pencetakan header sensitif atau payload biner.
