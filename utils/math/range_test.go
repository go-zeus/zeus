package util

import (
	"github.com/go-zeus/zeus/log"
	"testing"
)

func TestRangeRand(t *testing.T) {
	// 生成 1000 个 [-10, 10) 范围的安全随机数。
	min := int64(-10)
	max := int64(10)
	for i := 0; i < 1000; i++ {
		ret := RangeRand(min, max)
		log.Info("%d", ret)
		if ret < min || ret > max {
			t.Errorf("生成数据有误：%d", ret)
		}
	}
}
