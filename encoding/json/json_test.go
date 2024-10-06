package json

import (
	"fmt"
	"testing"
)

func TestMarshal(t *testing.T) {
	m := map[string]string{"name": "zeus"}
	s, err := codec{}.Marshal(m)
	if err != nil {
		t.Fatal("marshal error")
	}
	fmt.Println(string(s))
}

func TestUnMarshal(t *testing.T) {
	m := `{"name":"zeus"}`
	user := struct {
		Name string `json:"name"`
	}{}
	err := codec{}.Unmarshal([]byte(m), &user)
	if err != nil {
		t.Fatal("Unmarshal error")
	}
	fmt.Println(fmt.Sprintf("%+v", user))
}
