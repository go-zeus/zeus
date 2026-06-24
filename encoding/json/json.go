package json

import (
	stdjson "encoding/json"

	"github.com/go-zeus/zeus/encoding"
)

const Name = "json"

func New() encoding.Codec { return &codec{} }

type codec struct{}

func (codec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case stdjson.Marshaler:
		return m.MarshalJSON()
	default:
		return stdjson.Marshal(m)
	}
}

func (codec) Unmarshal(data []byte, v any) error {
	switch m := v.(type) {
	case stdjson.Unmarshaler:
		return m.UnmarshalJSON(data)
	default:
		return stdjson.Unmarshal(data, m)
	}
}

func (codec) Name() string { return Name }
