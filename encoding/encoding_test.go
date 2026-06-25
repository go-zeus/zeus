package encoding_test

import (
	"testing"

	"github.com/go-zeus/zeus/encoding"
	"github.com/go-zeus/zeus/encoding/json"
)

// TestCodecInterface 验证 Codec 接口可直接使用
func TestCodecInterface(t *testing.T) {
	var c encoding.Codec = json.New()
	if c.Name() != "json" {
		t.Errorf("Name() = %q, want %q", c.Name(), "json")
	}
}

// TestMarshalUnmarshal 验证序列化与反序列化往返一致
func TestMarshalUnmarshal(t *testing.T) {
	c := json.New()

	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := sample{Name: "zeus", Value: 42}
	data, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var result sample
	if err := c.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if result.Name != original.Name || result.Value != original.Value {
		t.Errorf("got %+v, want %+v", result, original)
	}
}
