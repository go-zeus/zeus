package interval

import (
	"testing"

	"github.com/go-zeus/zeus/job"
)

// TestResolver_Registered "interval" scheme 注册到 job.RegisteredResolvers
func TestResolver_Registered(t *testing.T) {
	all := job.RegisteredResolvers()
	if _, ok := all["interval"]; !ok {
		t.Errorf("interval scheme not registered, got: %v", all)
	}
}

// TestResolver_NewSchedulerFromURL "interval://" 返回非 nil Scheduler
func TestResolver_NewSchedulerFromURL(t *testing.T) {
	s, err := job.NewSchedulerFromURL("interval://")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s == nil {
		t.Fatal("scheduler is nil")
	}
	// 返回类型不需要强制是 *intervalScheduler，但底层应是本包 New() 产物
	// 通过 Stop(未 Start) 应是 no-op 来验证实现 job.Scheduler 接口
}

// TestResolver_UnknownParamsIgnored 未知 query 参数静默忽略
func TestResolver_UnknownParamsIgnored(t *testing.T) {
	s, err := job.NewSchedulerFromURL("interval://?whatever=foo&unused=1")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s == nil {
		t.Fatal("scheduler is nil")
	}
}
