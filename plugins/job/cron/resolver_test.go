package cron

import (
	"context"
	"testing"

	"github.com/go-zeus/zeus/job"
)

// TestResolver_Registered "cron" scheme 注册到 job.RegisteredResolvers
func TestResolver_Registered(t *testing.T) {
	all := job.RegisteredResolvers()
	if _, ok := all["cron"]; !ok {
		t.Errorf("cron scheme not registered, got: %v", all)
	}
}

// TestResolver_BasicURL "cron://" 返回非 nil Scheduler
func TestResolver_BasicURL(t *testing.T) {
	s, err := job.NewSchedulerFromURL("cron://")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s == nil {
		t.Fatal("scheduler is nil")
	}
}

// TestResolver_WithSeconds "cron://?seconds=true" 透传到 WithSeconds
//
// 验证手段：注册一个 6 字段表达式（含秒），如果 WithSeconds 没生效，会校验失败。
func TestResolver_WithSeconds(t *testing.T) {
	s, err := job.NewSchedulerFromURL("cron://?seconds=true")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// 6 字段 cron 表达式（含秒），仅 WithSeconds 启用后才会校验通过
	err = s.Register(job.Spec{
		Name:     "every-second",
		Schedule: "*/1 * * * * *",
		Handler:  func(context.Context) error { return nil },
	})
	if err != nil {
		t.Errorf("with seconds=true, register should succeed for 6-field expr, got err = %v", err)
	}
}

// TestResolver_WithLocation "cron://?loc=UTC" 不报错
func TestResolver_WithLocation(t *testing.T) {
	s, err := job.NewSchedulerFromURL("cron://?loc=UTC")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s == nil {
		t.Fatal("scheduler is nil")
	}
}

// TestResolver_UnknownParamsIgnored 未知 query 参数静默忽略
func TestResolver_UnknownParamsIgnored(t *testing.T) {
	s, err := job.NewSchedulerFromURL("cron://?foo=bar&unknown=1&seconds=false")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if s == nil {
		t.Fatal("scheduler is nil")
	}
}
