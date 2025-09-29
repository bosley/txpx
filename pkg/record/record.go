package record

import (
	"encoding/json"
	"errors"
	"strconv"
	"time"

	insiModels "github.com/InsulaLabs/insi/db/models"
	"github.com/bosley/txpx/pkg/planar"
)

var (
	ErrFieldNotFound = errors.New("field not found")
	ErrFieldMismatch = errors.New("field mismatch")
)

type Record interface {
	Schema() string
	ToKV() ([]insiModels.KVPayload, error)
	FromKV(data []insiModels.KVPayload) error

	ToPlanarKV() ([]planar.KVItem, error)
	FromPlanarKV(data []planar.KVItem) error

	GetString(field string) string
	SetString(field, value string) error

	GetInt(field string) (int, error)
	SetInt(field string, value int) error

	GetBool(field string) (bool, error)
	SetBool(field string, value bool) error

	GetFloat(field string) (float64, error)
	SetFloat(field string, value float64) error

	GetBytes(field string) []byte
	SetBytes(field string, value []byte) error

	GetTime(field string) (time.Time, error)
	SetTime(field string, value time.Time) error

	ToMap() map[string]any
	FromMap(data map[string]any) error
}

type fieldType int

const (
	FieldString fieldType = iota
	FieldInt
	FieldBool
	FieldFloat
	FieldBytes
	FieldTime
)

type fieldDef struct {
	name     string
	typ      fieldType
	required bool
}

type schema struct {
	name   string
	fields map[string]fieldDef
	order  []string
}

type record struct {
	schema *schema
	data   map[string][]byte
}

var _ Record = &record{}

func (r *record) Schema() string {
	return r.schema.name
}

func (r *record) ToKV() ([]insiModels.KVPayload, error) {
	result := make([]insiModels.KVPayload, 0, len(r.data))
	for k, v := range r.data {
		result = append(result, insiModels.KVPayload{
			Key:   k,
			Value: string(v),
		})
	}
	return result, nil
}

func (r *record) FromKV(data []insiModels.KVPayload) error {
	r.data = make(map[string][]byte, len(data))
	for _, item := range data {
		if _, exists := r.schema.fields[item.Key]; !exists {
			continue
		}
		r.data[item.Key] = []byte(item.Value)
	}
	return nil
}

func (r *record) ToPlanarKV() ([]planar.KVItem, error) {
	result := make([]planar.KVItem, 0, len(r.data))
	for k, v := range r.data {
		result = append(result, planar.KVItem{
			Key:   []byte(k),
			Value: make([]byte, len(v)),
		})
		copy(result[len(result)-1].Value, v)
	}
	return result, nil
}

func (r *record) FromPlanarKV(data []planar.KVItem) error {
	r.data = make(map[string][]byte, len(data))
	for _, item := range data {
		key := string(item.Key)
		if _, exists := r.schema.fields[key]; !exists {
			continue
		}
		r.data[key] = make([]byte, len(item.Value))
		copy(r.data[key], item.Value)
	}
	return nil
}

func (r *record) GetString(field string) string {
	if data, exists := r.data[field]; exists {
		return string(data)
	}
	return ""
}

func (r *record) SetString(field, value string) error {
	if err := r.validateField(field, FieldString); err != nil {
		return err
	}
	r.data[field] = []byte(value)
	return nil
}

func (r *record) GetInt(field string) (int, error) {
	data, exists := r.data[field]
	if !exists {
		return 0, nil
	}
	return strconv.Atoi(string(data))
}

func (r *record) SetInt(field string, value int) error {
	if err := r.validateField(field, FieldInt); err != nil {
		return err
	}
	r.data[field] = []byte(strconv.Itoa(value))
	return nil
}

func (r *record) GetBool(field string) (bool, error) {
	data, exists := r.data[field]
	if !exists {
		return false, nil
	}
	return strconv.ParseBool(string(data))
}

func (r *record) SetBool(field string, value bool) error {
	if err := r.validateField(field, FieldBool); err != nil {
		return err
	}
	r.data[field] = []byte(strconv.FormatBool(value))
	return nil
}

func (r *record) GetFloat(field string) (float64, error) {
	data, exists := r.data[field]
	if !exists {
		return 0, nil
	}
	return strconv.ParseFloat(string(data), 64)
}

func (r *record) SetFloat(field string, value float64) error {
	if err := r.validateField(field, FieldFloat); err != nil {
		return err
	}
	r.data[field] = []byte(strconv.FormatFloat(value, 'f', -1, 64))
	return nil
}

func (r *record) GetBytes(field string) []byte {
	if data, exists := r.data[field]; exists {
		result := make([]byte, len(data))
		copy(result, data)
		return result
	}
	return nil
}

func (r *record) SetBytes(field string, value []byte) error {
	if err := r.validateField(field, FieldBytes); err != nil {
		return err
	}
	r.data[field] = make([]byte, len(value))
	copy(r.data[field], value)
	return nil
}

func (r *record) GetTime(field string) (time.Time, error) {
	data, exists := r.data[field]
	if !exists {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, string(data))
}

func (r *record) SetTime(field string, value time.Time) error {
	if err := r.validateField(field, FieldTime); err != nil {
		return err
	}
	r.data[field] = []byte(value.Format(time.RFC3339))
	return nil
}

func (r *record) ToMap() map[string]any {
	result := make(map[string]any)
	for field, def := range r.schema.fields {
		data, exists := r.data[field]
		if !exists {
			continue
		}

		switch def.typ {
		case FieldString:
			result[field] = string(data)
		case FieldInt:
			if val, err := strconv.Atoi(string(data)); err == nil {
				result[field] = val
			}
		case FieldBool:
			if val, err := strconv.ParseBool(string(data)); err == nil {
				result[field] = val
			}
		case FieldFloat:
			if val, err := strconv.ParseFloat(string(data), 64); err == nil {
				result[field] = val
			}
		case FieldBytes:
			result[field] = data
		case FieldTime:
			if val, err := time.Parse(time.RFC3339, string(data)); err == nil {
				result[field] = val
			}
		}
	}
	return result
}

func (r *record) FromMap(data map[string]any) error {
	for field, value := range data {
		def, exists := r.schema.fields[field]
		if !exists {
			continue
		}

		switch def.typ {
		case FieldString:
			if v, ok := value.(string); ok {
				r.SetString(field, v)
			}
		case FieldInt:
			if v, ok := value.(int); ok {
				r.SetInt(field, v)
			} else if v, ok := value.(float64); ok {
				r.SetInt(field, int(v))
			}
		case FieldBool:
			if v, ok := value.(bool); ok {
				r.SetBool(field, v)
			}
		case FieldFloat:
			if v, ok := value.(float64); ok {
				r.SetFloat(field, v)
			} else if v, ok := value.(int); ok {
				r.SetFloat(field, float64(v))
			}
		case FieldBytes:
			if v, ok := value.([]byte); ok {
				r.SetBytes(field, v)
			} else if v, ok := value.(string); ok {
				r.SetBytes(field, []byte(v))
			}
		case FieldTime:
			if v, ok := value.(time.Time); ok {
				r.SetTime(field, v)
			} else if v, ok := value.(string); ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					r.SetTime(field, t)
				}
			}
		}
	}
	return nil
}

func (r *record) validateField(field string, expectedType fieldType) error {
	def, exists := r.schema.fields[field]
	if !exists {
		return ErrFieldNotFound
	}
	if def.typ != expectedType {
		return ErrFieldMismatch
	}
	return nil
}

type SchemaBuilder struct {
	name   string
	fields map[string]fieldDef
	order  []string
}

func DefineSchema(name string) *SchemaBuilder {
	return &SchemaBuilder{
		name:   name,
		fields: make(map[string]fieldDef),
		order:  make([]string, 0),
	}
}

func (sb *SchemaBuilder) addField(name string, typ fieldType, required bool) *SchemaBuilder {
	if _, exists := sb.fields[name]; exists {
		return sb
	}
	sb.fields[name] = fieldDef{
		name:     name,
		typ:      typ,
		required: required,
	}
	sb.order = append(sb.order, name)
	return sb
}

func (sb *SchemaBuilder) String(name string) *SchemaBuilder {
	return sb.addField(name, FieldString, false)
}

func (sb *SchemaBuilder) RequiredString(name string) *SchemaBuilder {
	return sb.addField(name, FieldString, true)
}

func (sb *SchemaBuilder) Int(name string) *SchemaBuilder {
	return sb.addField(name, FieldInt, false)
}

func (sb *SchemaBuilder) RequiredInt(name string) *SchemaBuilder {
	return sb.addField(name, FieldInt, true)
}

func (sb *SchemaBuilder) Bool(name string) *SchemaBuilder {
	return sb.addField(name, FieldBool, false)
}

func (sb *SchemaBuilder) RequiredBool(name string) *SchemaBuilder {
	return sb.addField(name, FieldBool, true)
}

func (sb *SchemaBuilder) Float(name string) *SchemaBuilder {
	return sb.addField(name, FieldFloat, false)
}

func (sb *SchemaBuilder) RequiredFloat(name string) *SchemaBuilder {
	return sb.addField(name, FieldFloat, true)
}

func (sb *SchemaBuilder) Bytes(name string) *SchemaBuilder {
	return sb.addField(name, FieldBytes, false)
}

func (sb *SchemaBuilder) RequiredBytes(name string) *SchemaBuilder {
	return sb.addField(name, FieldBytes, true)
}

func (sb *SchemaBuilder) Time(name string) *SchemaBuilder {
	return sb.addField(name, FieldTime, false)
}

func (sb *SchemaBuilder) RequiredTime(name string) *SchemaBuilder {
	return sb.addField(name, FieldTime, true)
}

func (sb *SchemaBuilder) Build() *schema {
	return &schema{
		name:   sb.name,
		fields: sb.fields,
		order:  sb.order,
	}
}

func (s *schema) NewRecord() Record {
	return &record{
		schema: s,
		data:   make(map[string][]byte),
	}
}

func (s *schema) FromKV(data []insiModels.KVPayload) (Record, error) {
	r := s.NewRecord()
	if err := r.FromKV(data); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *schema) FromPlanarKV(data []planar.KVItem) (Record, error) {
	r := s.NewRecord()
	if err := r.FromPlanarKV(data); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *schema) FromJSON(data []byte) (Record, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	r := s.NewRecord()
	if err := r.FromMap(m); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *schema) FromMap(data map[string]any) (Record, error) {
	r := s.NewRecord()
	if err := r.FromMap(data); err != nil {
		return nil, err
	}
	return r, nil
}
