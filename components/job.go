package components

import (
	"context"

	"github.com/go-zeus/zeus/job"
	"github.com/go-zeus/zeus/log"
)

// JobComponent 任务调度器组件适配器。
//
// 职责：
//   - 持有 job.Scheduler 实例
//   - OnStart 时把所有 JobRegistration 收集的 Spec 注册到 Scheduler，再 Start
//   - OnStop 时优雅停止 Scheduler
//
// 与 JobRegistration 配合使用，实现声明式任务调度：
//
//	components.NewApp(
//	    components.NewJobComponent(job.NewIntervalScheduler()),
//	    components.NewJobRegistration(job.Spec{
//	        Name: "heartbeat", Every: 30*time.Second,
//	        Handler: heartbeatFn,
//	    }),
//	    components.NewJobRegistration(job.Spec{
//	        Name: "cleanup", Every: 5*time.Minute,
//	        Handler: cleanupFn,
//	    }),
//	)
//
// App 启动时自动注册并启动所有 Job；停止时优雅关闭。
type JobComponent struct {
	sched job.Scheduler
}

// NewJobComponent 创建调度器组件。
// sched 为 nil 时返回的组件为 no-op（不会启动调度，但其他 JobRegistration 注册时仍会报错）。
func NewJobComponent(sched job.Scheduler) *JobComponent {
	return &JobComponent{sched: sched}
}

func (j *JobComponent) Name() string      { return "job" }
func (j *JobComponent) Depends() []string { return nil }

// Provide 把 Scheduler 实例发布到容器，供其他组件通过 ctx.Get("job") 取用
func (j *JobComponent) Provide(ctx Context) (any, error) {
	return j.sched, nil
}

// Lifecycle OnStart 收集所有 JobRegistration，按字典序注册后启动；
// OnStop 优雅关闭（10s 超时由 App 控制）。
func (j *JobComponent) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(ctx Context) error {
			if j.sched == nil {
				return nil // 无 Scheduler 时 no-op
			}
			// 收集所有 JobRegistration（按注册顺序）
			specs, err := AllByType[*JobRegistration](ctx)
			if err != nil {
				return err
			}
			for _, reg := range specs {
				if err := j.sched.Register(reg.spec); err != nil {
					return err
				}
			}
			if err := j.sched.Start(ctx); err != nil {
				return err
			}
			log.Info("job scheduler started with %d job(s)", len(specs))
			return nil
		},
		OnStop: func(ctx Context) error {
			if j.sched == nil {
				return nil
			}
			return j.sched.Stop(ctx)
		},
	}
}

// JobRegistration 单个 Job 的注册装饰器。
//
// 不直接调 Scheduler.Register，而是通过 Provide 把自己发布到容器，
// 让 JobComponent.OnStart 统一收集并注册（保证 Scheduler 已就绪）。
type JobRegistration struct {
	spec job.Spec
}

// NewJobRegistration 包装单个 Job Spec 为组件。
// 多个 JobRegistration 可同时注册到 NewApp。
func NewJobRegistration(spec job.Spec) *JobRegistration {
	return &JobRegistration{spec: spec}
}

func (j *JobRegistration) Name() string      { return "job_registration:" + j.spec.Name }
func (j *JobRegistration) Depends() []string { return []string{"job"} }

// Provide 返回自身指针，JobComponent.OnStart 通过 AllByType[*JobRegistration] 收集
func (j *JobRegistration) Provide(_ Context) (any, error) {
	return j, nil
}

// Lifecycle 不做事，全部由 JobComponent 统一编排
func (j *JobRegistration) Lifecycle() Lifecycle {
	return Lifecycle{
		OnStart: func(_ Context) error {
			// 兼容 ctx 作为 context.Context 的隐式断言（用于编译期检查）
			_ = context.Background()
			return nil
		},
	}
}
