package session

import (
	"testing"

	"github.com/adam/tau/internal/types"
)

func TestValidateVersion_V1(t *testing.T) {
	if err := ValidateVersion(1); err != nil {
		t.Fatalf("version 1 should be valid: %v", err)
	}
}

func TestValidateVersion_Zero(t *testing.T) {
	if err := ValidateVersion(0); err == nil {
		t.Fatal("version 0 should be invalid")
	}
}

func TestValidateVersion_Future(t *testing.T) {
	if err := ValidateVersion(99); err == nil {
		t.Fatal("version 99 should be invalid")
	}
}

func TestValidateVersion_Negative(t *testing.T) {
	if err := ValidateVersion(-1); err == nil {
		t.Fatal("version -1 should be invalid")
	}
}

func TestMigrateV1ToV2_NoOp(t *testing.T) {
	header := &types.SessionHeader{Type: "session", Version: 1, ID: "abc12345"}
	entries := []types.SessionEntry{
		{Type: types.EntryMessage, ID: "msg-1"},
		{Type: types.EntryModelChange, ID: "mc-1"},
	}

	h, e, err := migrateV1ToV2(header, entries)
	if err != nil {
		t.Fatalf("migrateV1ToV2 should not error: %v", err)
	}
	if h != header {
		t.Error("header should be unchanged")
	}
	if len(e) != len(entries) {
		t.Errorf("entries length mismatch: got %d, want %d", len(e), len(entries))
	}
	for i := range entries {
		if e[i].ID != entries[i].ID {
			t.Errorf("entry %d ID mismatch", i)
		}
	}
}
