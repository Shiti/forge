package store

import "testing"

func TestJSONBDefaultsForNilScanAndValue(t *testing.T) {
	var m JSONB
	if err := (&m).Scan(nil); err != nil {
		t.Fatalf("JSONB scan nil should not fail: %v", err)
	}
	if m == nil {
		t.Fatalf("JSONB scan nil should produce empty map, got nil")
	}
	v, err := JSONB(nil).Value()
	if err != nil {
		t.Fatalf("JSONB nil value should not fail: %v", err)
	}
	if string(v.([]byte)) != "{}" {
		t.Fatalf("expected JSONB nil value to serialize as {}, got %q", string(v.([]byte)))
	}
}

func TestJSONBListDefaultsForNilScanAndValue(t *testing.T) {
	var l JSONBList
	if err := (&l).Scan(nil); err != nil {
		t.Fatalf("JSONBList scan nil should not fail: %v", err)
	}
	if l == nil {
		t.Fatalf("JSONBList scan nil should produce empty slice, got nil")
	}
	v, err := JSONBList(nil).Value()
	if err != nil {
		t.Fatalf("JSONBList nil value should not fail: %v", err)
	}
	if string(v.([]byte)) != "[]" {
		t.Fatalf("expected JSONBList nil value to serialize as [], got %q", string(v.([]byte)))
	}
}

func TestJSONBStringListDefaultsForNilScanAndValue(t *testing.T) {
	var l JSONBStringList
	if err := (&l).Scan(nil); err != nil {
		t.Fatalf("JSONBStringList scan nil should not fail: %v", err)
	}
	if l == nil {
		t.Fatalf("JSONBStringList scan nil should produce empty slice, got nil")
	}
	v, err := JSONBStringList(nil).Value()
	if err != nil {
		t.Fatalf("JSONBStringList nil value should not fail: %v", err)
	}
	if string(v.([]byte)) != "[]" {
		t.Fatalf("expected JSONBStringList nil value to serialize as [], got %q", string(v.([]byte)))
	}
}

func TestRawJSONDefaultsForNilScanAndValue(t *testing.T) {
	var raw RawJSON
	if err := (&raw).Scan(nil); err != nil {
		t.Fatalf("RawJSON scan nil should not fail: %v", err)
	}
	if raw != nil {
		t.Fatalf("RawJSON scan nil should remain nil")
	}

	v, err := RawJSON(nil).Value()
	if err != nil {
		t.Fatalf("RawJSON nil value should not fail: %v", err)
	}
	if string(v.([]byte)) != "null" {
		t.Fatalf("expected RawJSON nil value to serialize as null, got %q", string(v.([]byte)))
	}
}
