package idgen

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestNewShortUUIDFormat(t *testing.T) {
	id := NewShortUUID()
	if len(id) != shortUUIDLength {
		t.Fatalf("expected length %d, got %d (%q)", shortUUIDLength, len(id), id)
	}
	for _, ch := range id {
		if !strings.ContainsRune(shortUUIDAlphabet, ch) {
			t.Fatalf("unexpected character %q in %q", ch, id)
		}
	}
}

func TestEncodeUUIDZero(t *testing.T) {
	id := EncodeUUID(uuid.Nil)
	expected := strings.Repeat(string(shortUUIDAlphabet[0]), shortUUIDLength)
	if id != expected {
		t.Fatalf("expected %q, got %q", expected, id)
	}
}
