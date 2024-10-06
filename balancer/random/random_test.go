package random

import (
	"github.com/go-zeus/zeus/types"
	"testing"
)

func TestRandomBalance_Next(t *testing.T) {
	lb := NewRandom()
	nodes := []*types.Instance{
		{Id: "1", Name: "111"},
		{Id: "2", Name: "222"},
		{Id: "3", Name: "333"},
	}
	lb.Reset(nodes)
	node, _ := lb.Next()
	t.Log(node.Name)

	node, _ = lb.Next()
	t.Log(node.Name)

	node, _ = lb.Next()
	t.Log(node.Name)
}
