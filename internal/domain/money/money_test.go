package money

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
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
		if got.MinorUnits() != tt.want {
			t.Fatalf("Parse(%q)=%d want=%d", tt.in, got.MinorUnits(), tt.want)
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

func TestParseRejectsNonISO4217Currency(t *testing.T) {
	if _, err := Parse("10.00", "ZZZ"); err == nil {
		t.Fatal("expected error for non-ISO 4217 currency")
	}
}

func TestParseRejectsExternalNegativeAndInvalidScale(t *testing.T) {
	for _, in := range []string{"-1.00", "+1.00", "1.001", "1.000"} {
		if _, err := Parse(in, "BRL"); err == nil {
			t.Fatalf("expected error for %q", in)
		}
	}
}

func TestParseRejectsScientificNotation(t *testing.T) {
	for _, in := range []string{"1e3", "1E3", "2.5e1"} {
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

func TestOperations(t *testing.T) {
	amount, err := New(2500, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	fee, err := New(125, "BRL")
	if err != nil {
		t.Fatal(err)
	}

	added, err := amount.Add(fee)
	if err != nil || added.MinorUnits() != 2625 {
		t.Fatalf("unexpected addition: %v %v", added, err)
	}
	subtracted, err := amount.Sub(fee)
	if err != nil || subtracted.MinorUnits() != 2375 {
		t.Fatalf("unexpected subtraction: %v %v", subtracted, err)
	}
	negative, err := fee.Negate()
	if err != nil || negative.MinorUnits() != -125 {
		t.Fatalf("unexpected negation: %v %v", negative, err)
	}
	comparison, err := amount.Compare(fee)
	if err != nil || comparison != 1 {
		t.Fatalf("unexpected comparison: %d %v", comparison, err)
	}
}

func TestRehydrateAllowsInternalNegativeValue(t *testing.T) {
	value, err := Rehydrate(-125, "BRL")
	if err != nil {
		t.Fatal(err)
	}
	if value.MinorUnits() != -125 {
		t.Fatalf("value=%d want=-125", value.MinorUnits())
	}
}
