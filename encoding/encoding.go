package encoding

import "strings"

type Codec interface {
	// Marshal returns the wire format of v.
	Marshal(v any) ([]byte, error)
	// Unmarshal parses the wire format into v.
	Unmarshal(data []byte, v any) error
	Name() string
}

var registeredCodecs = make(map[string]Codec)

// Register 注册编解码器
func Register(codec Codec) {
	if codec == nil {
		panic("cannot register a nil Codec")
	}
	if codec.Name() == "" {
		panic("cannot register Codec with empty string result for Name()")
	}
	contentSubtype := strings.ToLower(codec.Name())
	registeredCodecs[contentSubtype] = codec
}

// Get 获取编解码器
func Get(contentSubtype string) Codec {
	return registeredCodecs[contentSubtype]
}
