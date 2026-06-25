package protobuf

import (
	"bytes"
	"testing"

	"github.com/go-zeus/zeus/encoding"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNew_ReturnsCodec(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("expected non-nil Codec")
	}
	if c.Name() != "protobuf" {
		t.Fatalf("expected name 'protobuf', got %q", c.Name())
	}
	var _ encoding.Codec = c // 编译期接口校验
}

func TestCodec_MarshalUnmarshal_Roundtrip(t *testing.T) {
	c := New()
	orig := wrapperspb.String("hello zeus")

	data, err := c.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty payload")
	}

	var got wrapperspb.StringValue
	if err := c.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.GetValue() != orig.GetValue() {
		t.Fatalf("roundtrip mismatch: got %q, want %q", got.GetValue(), orig.GetValue())
	}
}

func TestCodec_Marshal_Int(t *testing.T) {
	c := New()
	data, err := c.Marshal(wrapperspb.Int32(42))
	if err != nil {
		t.Fatalf("marshal int32: %v", err)
	}
	var got wrapperspb.Int32Value
	if err := c.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal int32: %v", err)
	}
	if got.GetValue() != 42 {
		t.Fatalf("expected 42, got %d", got.GetValue())
	}
}

func TestCodec_Marshal_NonProtoMessage(t *testing.T) {
	c := New()
	if _, err := c.Marshal("not a proto"); err == nil {
		t.Fatal("expected error for non-proto.Message")
	}
	if _, err := c.Marshal(123); err == nil {
		t.Fatal("expected error for int")
	}
	if _, err := c.Marshal(struct{}{}); err == nil {
		t.Fatal("expected error for struct{}")
	}
}

func TestCodec_Unmarshal_NonProtoMessage(t *testing.T) {
	c := New()
	if err := c.Unmarshal([]byte{0x0a, 0x01, 0x41}, "not a proto"); err == nil {
		t.Fatal("expected error for non-proto.Message target")
	}
}

func TestCodec_Unmarshal_EmptyInput(t *testing.T) {
	c := New()
	var got wrapperspb.StringValue
	// 空输入应 no-op，不返回错误（对齐 json.Unmarshal 行为）
	if err := c.Unmarshal(nil, &got); err != nil {
		t.Fatalf("unmarshal(nil): %v", err)
	}
	if err := c.Unmarshal([]byte{}, &got); err != nil {
		t.Fatalf("unmarshal([]byte{}): %v", err)
	}
	// 默认零值（Empty 字符串）
	if got.GetValue() != "" {
		t.Fatalf("expected empty value, got %q", got.GetValue())
	}
}

func TestCodec_Name_Stable(t *testing.T) {
	c := New()
	// 名称用于 registry 索引，必须稳定
	if c.Name() != "protobuf" {
		t.Fatalf("expected 'protobuf', got %q", c.Name())
	}
}

func TestCodec_Idempotent_MarshalDeterministic(t *testing.T) {
	// protobuf 序列化对相同输入应输出相同字节串
	c := New()
	msg := wrapperspb.String("stable-output")

	a, _ := c.Marshal(msg)
	b, _ := c.Marshal(msg)
	if !bytes.Equal(a, b) {
		t.Fatalf("protobuf Marshal 应确定性输出")
	}
}
