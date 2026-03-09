package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// RawJSON is an opaque JSON blob used for pass-through payloads.
// It preserves raw transformer/state-update objects without Go-side schema parsing.
type RawJSON json.RawMessage

func (r *RawJSON) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("RawJSON: UnmarshalJSON on nil pointer")
	}
	if len(data) == 0 {
		*r = nil
		return nil
	}
	if !json.Valid(data) {
		return fmt.Errorf("RawJSON: invalid json")
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	*r = RawJSON(cp)
	return nil
}

func (r RawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	if !json.Valid([]byte(r)) {
		return nil, fmt.Errorf("RawJSON: invalid json")
	}
	cp := make([]byte, len(r))
	copy(cp, r)
	return cp, nil
}

func (r RawJSON) MarshalYAML() (interface{}, error) {
	if len(r) == 0 || bytes.Equal([]byte(r), []byte("null")) {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal([]byte(r), &v); err != nil {
		return nil, err
	}
	return v, nil
}
