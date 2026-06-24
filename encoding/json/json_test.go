package json_test

import (
	jsonStd "encoding/json"
	"testing"

	"github.com/go-zeus/zeus/encoding"
	"github.com/go-zeus/zeus/encoding/json"
)

// TestNew 验证 New() 返回满足 Codec 接口的实例
func TestNew(t *testing.T) {
	var c encoding.Codec = json.New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.Name() != "json" {
		t.Errorf("Name() = %q, want %q", c.Name(), "json")
	}
}

func TestMarshal(t *testing.T) {
	c := json.New()
	m := map[string]string{"name": "zeus"}
	_, err := c.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
}

func TestUnmarshal(t *testing.T) {
	c := json.New()
	data := `{"name":"zeus"}`
	user := struct {
		Name string `json:"name"`
	}{}
	if err := c.Unmarshal([]byte(data), &user); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if user.Name != "zeus" {
		t.Errorf("Name = %q, want %q", user.Name, "zeus")
	}
}

// TestMarshal_Unmarshal 验证序列化与反序列化往返一致性
func TestMarshal_Unmarshal(t *testing.T) {
	c := json.New()
	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	original := sample{Name: "zeus", Value: 42}

	data, err := c.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal 错误: %v", err)
	}

	var result sample
	if err := c.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal 错误: %v", err)
	}
	if result.Name != original.Name || result.Value != original.Value {
		t.Errorf("got %+v, want %+v", result, original)
	}
}

// TestMarshal_InvalidType 验证序列化不支持的类型会返回错误
func TestMarshal_InvalidType(t *testing.T) {
	c := json.New()
	ch := make(chan int)
	_, err := c.Marshal(ch)
	if err == nil {
		t.Error("Marshal 不支持的类型应返回错误")
	}
}

// TestMarshal_JSONMarshaler 实现 encoding/json.Marshaler 接口的类型走 MarshalJSON 分支
func TestMarshal_JSONMarshaler(t *testing.T) {
	c := json.New()
	// customMarshaler 实现MarshalJSON，返回固定字节序列
	v := customMarshaler{}
	data, err := c.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal err: %v", err)
	}
	if string(data) != `"custom"` {
		t.Errorf("data = %q, want \"custom\"", string(data))
	}
}

type customMarshaler struct{}

func (customMarshaler) MarshalJSON() ([]byte, error) {
	return []byte(`"custom"`), nil
}

// TestUnmarshal_JSONUnmarshaler 实现_encoding/json.Unmarshaler 接口的类型走 UnmarshalJSON 分支
func TestUnmarshal_JSONUnmarshaler(t *testing.T) {
	c := json.New()
	var v customUnmarshaler
	if err := c.Unmarshal([]byte(`"hello"`), &v); err != nil {
		t.Fatalf("Unmarshal err: %v", err)
	}
	if v.value != "hello" {
		t.Errorf("value = %q, want hello", v.value)
	}
}

type customUnmarshaler struct {
	value string
}

func (cu *customUnmarshaler) UnmarshalJSON(data []byte) error {
	// data 是原始 JSON 文本（含引号），用 json.Unmarshal 解出字符串
	return jsonStd.Unmarshal(data, &cu.value)
}

// TestName 返回常量 "json"
func TestName(t *testing.T) {
	if json.Name != "json" {
		t.Errorf("Name = %q, want json", json.Name)
	}
}

// TestMarshal_NilValue nil 值序列化为 "null"
func TestMarshal_NilValue(t *testing.T) {
	c := json.New()
	data, err := c.Marshal(nil)
	if err != nil {
		t.Fatalf("Marshal(nil) err: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("data = %q, want null", string(data))
	}
}
