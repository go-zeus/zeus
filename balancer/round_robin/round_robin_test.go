package round_robin

import (
	"github.com/go-zeus/zeus/types"
	"testing"
)

func TestNewRoundRobin_Next(t *testing.T) {
	lb := NewRoundRobin()
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

// 并发基准测试
func BenchmarkNewRoundRobin_Next(b *testing.B) {
	lb := NewRoundRobin()
	nodes := []*types.Instance{
		{Id: "1", Name: "111"},
		{Id: "2", Name: "222"},
		{Id: "3", Name: "333"},
	}
	lb.Reset(nodes)
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			lb.Next()
		}
	})
}
