package json

import (
	"encoding/json"
	"github.com/go-zeus/zeus/encoding"
)

const Name = "json"

func init() {
	encoding.Register(codec{})
}

type codec struct{}

func (codec) Marshal(v any) ([]byte, error) {
	switch m := v.(type) {
	case json.Marshaler:
		return m.MarshalJSON()
	default:
		return json.Marshal(m)
	}
}

func (codec) Unmarshal(data []byte, v any) error {
	switch m := v.(type) {
	case json.Unmarshaler:
		return m.UnmarshalJSON(data)
	default:
		return json.Unmarshal(data, m)
	}
}

func (codec) Name() string {
	return Name
}
