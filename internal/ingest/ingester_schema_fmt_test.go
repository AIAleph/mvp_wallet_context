package ingest

import "testing"

func TestSchemaMode_DefaultsAndUnknown(t *testing.T) {
	if got := New("0x", Options{}).SchemaMode(); got != DefaultSchemaMode {
		t.Fatalf("SchemaMode default got %q, want %s", got, DefaultSchemaMode)
	}
	if got := New("0x", Options{Schema: "canonical"}).SchemaMode(); got != "canonical" {
		t.Fatalf("SchemaMode canonical got %q, want canonical", got)
	}
	t.Run("invalid schema panics", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic for invalid schema")
			}
		}()
		_ = New("0x", Options{Schema: "something"})
	})
}

func TestNormalizeSchema(t *testing.T) {
	tcs := []struct {
		name    string
		input   string
		expect  string
		wantErr bool
	}{
		{"empty defaults", "", DefaultSchemaMode, false},
		{"canonical", "canonical", "canonical", false},
		{"dev", "dev", "dev", false},
		{"mixed case", "CanonICaL", "canonical", false},
		{"invalid", "staging", "", true},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeSchema(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NormalizeSchema(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeSchema(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.expect {
				t.Fatalf("NormalizeSchema(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
	}
}

func TestFmtDT64_Edges(t *testing.T) {
	if got := fmtDT64(0); got != "1970-01-01 00:00:00.000" {
		t.Fatalf("fmtDT64(0) = %q", got)
	}
	if got := fmtDT64(1_234); got == "1970-01-01 00:00:00.000" {
		t.Fatalf("fmtDT64(>0) unexpected epoch: %q", got)
	}
}
