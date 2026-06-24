package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockHealthChecker 模拟健康检查器
type mockHealthChecker struct {
	ready bool
	alive bool
}

func (m *mockHealthChecker) IsReady() bool { return m.ready }
func (m *mockHealthChecker) IsAlive() bool { return m.alive }

func TestHealthHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("状态码 = %d, want %d", w.Code, http.StatusOK)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解析响应体失败: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want %q", resp.Status, "ok")
	}
}

func TestReadinessHandler_NoChecker(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	readinessHandler(w, req, nil)

	if w.Code != http.StatusOK {
		t.Errorf("无 checker 时状态码 = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestReadinessHandler_Ready(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	checker := &mockHealthChecker{ready: true}
	readinessHandler(w, req, checker)

	if w.Code != http.StatusOK {
		t.Errorf("IsReady=true 时状态码 = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestReadinessHandler_NotReady(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()

	checker := &mockHealthChecker{ready: false}
	readinessHandler(w, req, checker)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("IsReady=false 时状态码 = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解析响应体失败: %v", err)
	}
	if resp.Status != "not ready" {
		t.Errorf("status = %q, want %q", resp.Status, "not ready")
	}
}

func TestLivenessHandler_Alive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	checker := &mockHealthChecker{alive: true}
	livenessHandler(w, req, checker)

	if w.Code != http.StatusOK {
		t.Errorf("IsAlive=true 时状态码 = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestLivenessHandler_NotAlive(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()

	checker := &mockHealthChecker{alive: false}
	livenessHandler(w, req, checker)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("IsAlive=false 时状态码 = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}

	var resp healthResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("解析响应体失败: %v", err)
	}
	if resp.Status != "not alive" {
		t.Errorf("status = %q, want %q", resp.Status, "not alive")
	}
}
