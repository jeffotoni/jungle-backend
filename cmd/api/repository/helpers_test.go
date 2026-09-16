package repository

import (
	"testing"
	"time"

	"github.com/jeffotoni/jungle-backend-challenge/internal/application/ports"
)

func TestParseUUID(t *testing.T) {
	valid := "516be6a5-8338-4560-a723-0fc1e6e6e801"
	id, err := parseUUID(valid)
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != valid {
		t.Fatalf("id=%s want=%s", id, valid)
	}

	if _, err := parseUUID("not-a-uuid"); err == nil {
		t.Fatal("expected invalid UUID error")
	}
}

func TestLedgerCursorRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, time.September, 16, 7, 21, 23, 389163000, time.UTC)
	entry := ports.LedgerRecord{
		ID:        "ccc7135a-888e-4e95-ab4e-bb269bcf9af6",
		CreatedAt: createdAt,
	}

	cursor := encodeCursor(entry)
	decodedTime, decodedID, hasCursor, err := decodeCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCursor {
		t.Fatal("expected cursor to be present")
	}
	if !decodedTime.Equal(createdAt) {
		t.Fatalf("time=%s want=%s", decodedTime, createdAt)
	}
	if decodedID.String() != entry.ID {
		t.Fatalf("id=%s want=%s", decodedID, entry.ID)
	}
}

func TestLedgerCursorEmpty(t *testing.T) {
	_, _, hasCursor, err := decodeCursor(" ")
	if err != nil {
		t.Fatal(err)
	}
	if hasCursor {
		t.Fatal("expected empty cursor to be absent")
	}
}

func TestLedgerCursorRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{
		"not-base64",
		"dGVzdA",
		"MjAyNi0wOS0xNlQwNzoyMToyMy4zODkxNjNa|not-encoded",
	} {
		if _, _, _, err := decodeCursor(value); err == nil {
			t.Fatalf("expected invalid cursor error for %q", value)
		}
	}
}

func TestNullIf(t *testing.T) {
	if value := nullIf(""); value != nil {
		t.Fatalf("empty value=%v want nil", value)
	}
	if value := nullIf("round-001"); value != "round-001" {
		t.Fatalf("value=%v want round-001", value)
	}
}
