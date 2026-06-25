package http

import (
	"encoding/json"
	"net/http"
)

// HealthChecker 健康检查接口
type HealthChecker interface {
	IsReady() bool
	IsAlive() bool
}

type healthResponse struct {
	Status string `json:"status"`
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Encode 写入 ResponseWriter 失败时只能记录，无法再修改响应（已 WriteHeader）
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}

func readinessHandler(w http.ResponseWriter, _ *http.Request, checker HealthChecker) {
	w.Header().Set("Content-Type", "application/json")
	if checker != nil && !checker.IsReady() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(healthResponse{Status: "not ready"})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}

func livenessHandler(w http.ResponseWriter, _ *http.Request, checker HealthChecker) {
	w.Header().Set("Content-Type", "application/json")
	if checker != nil && !checker.IsAlive() {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(healthResponse{Status: "not alive"})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}
