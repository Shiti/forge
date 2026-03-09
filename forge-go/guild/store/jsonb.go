package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

type RawJSON json.RawMessage

func (j *RawJSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	switch v := value.(type) {
	case []byte:
		if len(v) == 0 {
			*j = nil
			return nil
		}
		buf := make([]byte, len(v))
		copy(buf, v)
		*j = RawJSON(buf)
		return nil
	case string:
		if v == "" {
			*j = nil
			return nil
		}
		*j = RawJSON([]byte(v))
		return nil
	default:
		return fmt.Errorf("expected []byte or string for RawJSON, got %T", value)
	}
}

func (j RawJSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	raw := json.RawMessage(j)
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid RawJSON payload")
	}
	return []byte(raw), nil
}

func (j *JSONB) Scan(value interface{}) error {
	if value == nil {
		*j = JSONB{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for JSONB, got %T", value)
	}
	result := JSONB{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*j = result
	return nil
}

func (j JSONB) Value() (driver.Value, error) {
	if j == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(j)
}

func (j *JSONBList) Scan(value interface{}) error {
	if value == nil {
		*j = JSONBList{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for JSONBList, got %T", value)
	}
	result := JSONBList{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*j = result
	return nil
}

func (j JSONBList) Value() (driver.Value, error) {
	if j == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(j)
}

func (j *JSONBStringList) Scan(value interface{}) error {
	if value == nil {
		*j = JSONBStringList{}
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("expected []byte for JSONBStringList, got %T", value)
	}
	result := JSONBStringList{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return err
	}
	*j = result
	return nil
}

func (j JSONBStringList) Value() (driver.Value, error) {
	if j == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(j)
}
