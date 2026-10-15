package db

import (
	"testing"
)

func TestCheckTimezoneOffset(t *testing.T) {
	tests := []struct {
		name          string
		goOffsetMenit int
		dbOffsetMenit int
		wantBeda      bool
	}{
		{
			name:          "sama-sama WIB (+7 jam = 420 menit)",
			goOffsetMenit: 420,
			dbOffsetMenit: 420,
			wantBeda:      false,
		},
		{
			name:          "sama-sama UTC (0 menit)",
			goOffsetMenit: 0,
			dbOffsetMenit: 0,
			wantBeda:      false,
		},
		{
			name:          "Go WIB (420 menit) tapi DB UTC (0 menit)",
			goOffsetMenit: 420,
			dbOffsetMenit: 0,
			wantBeda:      true,
		},
		{
			name:          "Go WITA (480 menit) tapi DB WIB (420 menit)",
			goOffsetMenit: 480,
			dbOffsetMenit: 420,
			wantBeda:      true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			beda, pesan := CheckTimezoneOffset(tc.goOffsetMenit, tc.dbOffsetMenit)
			if beda != tc.wantBeda {
				t.Fatalf("diharapkan beda = %v, dapat %v", tc.wantBeda, beda)
			}
			if tc.wantBeda && pesan == "" {
				t.Fatal("diharapkan ada pesan peringatan jika beda = true, dapat string kosong")
			}
			if !tc.wantBeda && pesan != "" {
				t.Fatalf("tidak diharapkan pesan peringatan jika beda = false, dapat %q", pesan)
			}
		})
	}
}
