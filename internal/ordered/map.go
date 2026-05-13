package ordered

import (
	"bytes"
	stdjson "encoding/json"
	"fmt"
	"io"
)

type Map struct {
	keys   []string
	values map[string]interface{}
}

type KVPair struct {
	Key   string
	Value interface{}
}

func New() *Map {
	return &Map{
		keys:   make([]string, 0),
		values: make(map[string]interface{}),
	}
}

func (m *Map) Get(key string) interface{} {
	return m.values[key]
}

func (m *Map) GetValue(key string) (interface{}, bool) {
	v, ok := m.values[key]
	return v, ok
}

func (m *Map) Set(key string, value interface{}) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

func (m *Map) Delete(key string) {
	if _, exists := m.values[key]; !exists {
		return
	}
	delete(m.values, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			break
		}
	}
}

func (m *Map) Has(key string) bool {
	_, ok := m.values[key]
	return ok
}

func (m *Map) Keys() []string {
	result := make([]string, len(m.keys))
	copy(result, m.keys)
	return result
}

func (m *Map) Len() int {
	return len(m.keys)
}

func (m *Map) Entries() []KVPair {
	result := make([]KVPair, len(m.keys))
	for i, key := range m.keys {
		result[i] = KVPair{Key: key, Value: m.values[key]}
	}
	return result
}

func (m *Map) UnmarshalJSON(data []byte) error {
	dec := stdjson.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(stdjson.Delim)
	if !ok || delim != '{' {
		return fmt.Errorf("expected '{', got %v", t)
	}

	m.keys = make([]string, 0)
	m.values = make(map[string]interface{})

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %T", keyTok)
		}

		var rawVal stdjson.RawMessage
		if err := dec.Decode(&rawVal); err != nil {
			return err
		}

		val, err := decodeValue(rawVal)
		if err != nil {
			return err
		}
		m.Set(key, val)
	}

	t, err = dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := t.(stdjson.Delim); !ok || delim != '}' {
		return fmt.Errorf("expected '}', got %v", t)
	}
	return nil
}

func (m *Map) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyBytes, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyBytes)
		buf.WriteByte(':')
		valBytes, err := json.Marshal(m.values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(valBytes)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// ToGoMap converts to a standard map[string]interface{}, losing key order.
// Nested *Map values are recursively converted.
func (m *Map) ToGoMap() map[string]interface{} {
	result := make(map[string]interface{}, len(m.keys))
	for _, key := range m.keys {
		result[key] = toGoValue(m.values[key])
	}
	return result
}

func toGoValue(v interface{}) interface{} {
	switch val := v.(type) {
	case *Map:
		return val.ToGoMap()
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = toGoValue(item)
		}
		return result
	default:
		return v
	}
}

// Each yields [key, value] pairs in insertion order.
// Implements the iteration interface expected by liquidgo's SliceCollection.
func (m *Map) Each(fn func(interface{})) {
	for _, key := range m.keys {
		fn([]interface{}{key, m.values[key]})
	}
}

// LiquidMethodMissing enables property access from Liquid templates.
// {{ site.data.team.alice }} calls LiquidMethodMissing("alice").
func (m *Map) LiquidMethodMissing(key string) interface{} {
	return m.values[key]
}

// Size returns the number of entries (accessed as .size in Liquid).
func (m *Map) Size() int {
	return len(m.keys)
}

// First returns the first [key, value] pair (accessed as .first in Liquid).
func (m *Map) First() interface{} {
	if len(m.keys) == 0 {
		return nil
	}
	return []interface{}{m.keys[0], m.values[m.keys[0]]}
}

// decodeValue recursively decodes a JSON raw message, producing *Map for
// objects and []interface{} for arrays, preserving key order throughout.
func decodeValue(raw stdjson.RawMessage) (interface{}, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	switch trimmed[0] {
	case '{':
		m := New()
		if err := m.UnmarshalJSON(trimmed); err != nil {
			return nil, err
		}
		return m, nil
	case '[':
		return decodeArray(trimmed)
	default:
		var v interface{}
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
}

func decodeArray(data []byte) ([]interface{}, error) {
	dec := stdjson.NewDecoder(bytes.NewReader(data))
	t, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := t.(stdjson.Delim); !ok || delim != '[' {
		return nil, fmt.Errorf("expected '[', got %v", t)
	}

	result := make([]interface{}, 0)
	for dec.More() {
		var raw stdjson.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		val, err := decodeValue(raw)
		if err != nil {
			return nil, err
		}
		result = append(result, val)
	}
	if t, err := dec.Token(); err != nil {
		return nil, err
	} else if delim, ok := t.(stdjson.Delim); !ok || delim != ']' {
		return nil, fmt.Errorf("expected ']', got %v", t)
	}
	return result, nil
}

// RewrapValue recursively converts map[string]interface{} values to *Map,
// restoring Each()/LiquidMethodMissing() after JSON round-trip through
// the plugin serialization boundary.
func RewrapValue(v interface{}) interface{} {
	switch val := v.(type) {
	case *Map:
		return val
	case map[string]interface{}:
		m := New()
		for k, v := range val {
			m.Set(k, RewrapValue(v))
		}
		return m
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = RewrapValue(item)
		}
		return result
	default:
		return v
	}
}

// UnmarshalJSONValue parses a top-level JSON value, using *Map for objects.
// For arrays and scalars, returns the standard Go types.
func UnmarshalJSONValue(data []byte) (interface{}, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	switch trimmed[0] {
	case '{':
		m := New()
		if err := m.UnmarshalJSON(trimmed); err != nil {
			return nil, err
		}
		return m, nil
	case '[':
		return decodeArray(trimmed)
	default:
		var v interface{}
		if err := json.Unmarshal(trimmed, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
}
