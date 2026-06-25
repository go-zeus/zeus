package testutil

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/go-zeus/zeus/types"
)

// —— FreePort ——

func TestFreePort_ReturnsValidPort(t *testing.T) {
	p, err := FreePort()
	if err != nil {
		t.Fatalf("FreePort: %v", err)
	}
	if p <= 0 || p > 65535 {
		t.Errorf("port = %d out of range", p)
	}
}

func TestFreePort_IsAvailable(t *testing.T) {
	p := MustFreePort(t)
	// 立即监听该端口应该成功（OS 给的就是空闲的）
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
	if err != nil {
		t.Fatalf("listen on FreePort: %v", err)
	}
	defer ln.Close()
}

// —— WaitUntil ——

func TestWaitUntil_Immediate(t *testing.T) {
	err := WaitUntil(time.Second, 50*time.Millisecond, func() bool { return true })
	if err != nil {
		t.Errorf("immediate true should not error: %v", err)
	}
}

func TestWaitUntil_Timeout(t *testing.T) {
	start := time.Now()
	err := WaitUntil(200*time.Millisecond, 50*time.Millisecond, func() bool { return false })
	elapsed := time.Since(start)
	if err == nil {
		t.Error("should timeout with error")
	}
	if elapsed < 200*time.Millisecond {
		t.Errorf("should wait at least 200ms, got %s", elapsed)
	}
}

func TestWaitUntil_EventuallyTrue(t *testing.T) {
	n := 0
	err := WaitUntil(time.Second, 50*time.Millisecond, func() bool {
		n++
		return n >= 3
	})
	if err != nil {
		t.Errorf("should succeed: %v", err)
	}
	if n < 3 {
		t.Errorf("should call at least 3 times, got %d", n)
	}
}

// —— Poll ——

func TestPoll_ReturnsWhenReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	n := 0
	err := Poll(ctx, 20*time.Millisecond, func() error {
		n++
		if n >= 2 {
			return nil
		}
		return errors.New("not yet")
	})
	if err != nil {
		t.Errorf("Poll should succeed: %v", err)
	}
}

func TestPoll_CtxCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := Poll(ctx, 10*time.Millisecond, func() error {
		return errors.New("never")
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// —— HTTPServer ——

func TestHTTPServer_ServeAndReady(t *testing.T) {
	srv := NewHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	if err := srv.WaitReady(2 * time.Second); err != nil {
		t.Fatalf("not ready: %v", err)
	}

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d", resp.StatusCode)
	}
}

// —— MockRegistry ——

func TestMockRegistry_RegisterDeregister(t *testing.T) {
	reg := NewMockRegistry()
	ctx := context.Background()

	ins := &types.Instance{ID: "1", Name: "svc", Cluster: "default"}
	if err := reg.Register(ctx, ins); err != nil {
		t.Fatal(err)
	}
	if reg.Count() != 1 {
		t.Errorf("count = %d, want 1", reg.Count())
	}
	if reg.CountByName("svc") != 1 {
		t.Errorf("svc count = %d", reg.CountByName("svc"))
	}

	_ = reg.Deregister(ctx, ins)
	if reg.Count() != 0 {
		t.Errorf("after dereg, count = %d", reg.Count())
	}
}

func TestMockRegistry_GetService(t *testing.T) {
	reg := NewMockRegistry()
	ctx := context.Background()

	_ = reg.Register(ctx, &types.Instance{ID: "1", Name: "svc", Cluster: "default"})
	_ = reg.Register(ctx, &types.Instance{ID: "2", Name: "svc", Cluster: "default"})
	_ = reg.Register(ctx, &types.Instance{ID: "3", Name: "other", Cluster: "default"})

	entry, err := reg.GetService(ctx, "svc")
	if err != nil {
		t.Fatal(err)
	}
	if len(entry.Instances) != 2 {
		t.Errorf("svc instances = %d, want 2", len(entry.Instances))
	}
}

func TestMockRegistry_Watch_NotifyOnRegister(t *testing.T) {
	reg := NewMockRegistry()
	ctx := context.Background()

	ch, _ := reg.Watch(ctx, "svc")

	_ = reg.Register(ctx, &types.Instance{ID: "1", Name: "svc", Cluster: "default"})

	select {
	case <-ch:
		// 收到通知
	case <-time.After(time.Second):
		t.Error("should receive notify after register")
	}
}

// —— MockBalancer ——

func TestMockBalancer_NextReturnsPreset(t *testing.T) {
	ins := &types.Instance{ID: "1", Name: "svc"}
	b := NewMockBalancer(ins)
	got, err := b.Next()
	if err != nil {
		t.Fatal(err)
	}
	if got != ins {
		t.Error("should return preset instance")
	}
	if b.Calls() != 1 {
		t.Errorf("calls = %d, want 1", b.Calls())
	}
}

func TestMockBalancer_ReloadKeepsCandidates(t *testing.T) {
	b := NewMockBalancer(nil)
	candidates := []*types.Instance{
		{ID: "1", Name: "svc"},
		{ID: "2", Name: "svc"},
	}
	b.Reload(candidates)
	got, err := b.Next()
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "1" {
		t.Errorf("should return first candidate, got %s", got.ID)
	}
}

// —— AssertPanic ——

func TestAssertPanic_Panics(t *testing.T) {
	AssertPanic(t, "boom", func() {
		panic("boom!")
	})
}

// —— RequireNoError ——

func TestRequireNoError_NoErr(t *testing.T) {
	RequireNoError(t, nil)
}

// —— AssertEqual ——

func TestAssertEqual_Equal(t *testing.T) {
	AssertEqual(t, 1, 1)
	AssertNotEqual(t, 1, 2)
}

// —— MustFreePort ——

func TestMustFreePort_ReturnsInt(t *testing.T) {
	p := MustFreePort(t)
	if p <= 0 {
		t.Errorf("port = %d", p)
	}
}

// —— MustWaitUntil ——

func TestMustWaitUntil_Success(t *testing.T) {
	MustWaitUntil(t, time.Second, 50*time.Millisecond, func() bool { return true }, "should be true")
}
