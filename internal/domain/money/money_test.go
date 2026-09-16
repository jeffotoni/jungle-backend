package money

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in string
		want int64
	}{
		{"25.00", 2500},
		{"100.50", 10050},
		{"0.01", 1},
		{"25.1", 2510},
		{"25", 2500},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, "BRL")
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got.Amount != tt.want {
			t.Fatalf("Parse(%q)=%d want=%d", tt.in, got.Amount, tt.want)
		}
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	for _, in := range []string{"", "1.001", "1e3", "NaN", "Infinity", "x.00"} {
		if _, err := Parse(in, "BRL"); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}

func TestString(t *testing.T) {
	m, _ := New(2501, "BRL")
	if got := m.String(); got != "25.01" {
		t.Fatalf("got %q", got)
	}
}
