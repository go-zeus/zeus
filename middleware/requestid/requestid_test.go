package requestid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-zeus/zeus/middleware"
)

// mockRequest 模拟 middleware.Request
type mockRequest struct {
	method string
	path   string
	header map[string]string
	body   any
}

func (m *mockRequest) Method() string         { return m.method }
func (m *mockRequest) Path() string           { return m.path }
func (m *mockRequest) Header(k string) string { return m.header[k] }
func (m *mockRequest) Body() any              { return m.body }

type mockResponse struct{ code int }

func (m *mockResponse) StatusCode() int { return m.code }
func (m *mockResponse) Body() any       { return nil }

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()
	if len(id1) != 32 {
		t.Errorf("id length = %d, want 32 (16 bytes hex)", len(id1))
	}
	if id1 == id2 {
		t.Error("two generated IDs should differ")
	}
}

func TestFromContext_RoundTrip(t *testing.T) {
	ctx := context.Background()
	if got := FromContext(ctx); got != "" {
		t.Errorf("empty ctx should return empty id, got %q", got)
	}
	ctx = WithID(ctx, "abc-123")
	if got := FromContext(ctx); got != "abc-123" {
		t.Errorf("got %q, want abc-123", got)
	}
}

func TestHTTPMiddleware_GeneratesID(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(200)
	})
	h := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if captured == "" {
		t.Error("middleware should generate id when not provided")
	}
	if rec.Header().Get(HeaderRequestID) != captured {
		t.Errorf("response header should match ctx id: header=%q ctx=%q",
			rec.Header().Get(HeaderRequestID), captured)
	}
}

func TestHTTPMiddleware_PreservesExistingID(t *testing.T) {
	var captured string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
	})
	h := HTTPMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "incoming-id")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if captured != "incoming-id" {
		t.Errorf("should preserve incoming id, got %q", captured)
	}
	if got := rec.Header().Get(HeaderRequestID); got != "incoming-id" {
		t.Errorf("response header = %q, want incoming-id", got)
	}
}

// TestNew_Name 验证 New() 返回的 Interceptor Name 正确
func TestNew_Name(t *testing.T) {
	mw := New()
	if mw.Name() != "requestid" {
		t.Errorf("Name() = %q, want requestid", mw.Name())
	}
}

// TestIntercept_GeneratesID 请求未带 X-Request-ID 时自动生成
func TestIntercept_GeneratesID(t *testing.T) {
	mw := New()
	req := &mockRequest{method: "GET", path: "/", header: map[string]string{}}
	var captured string
	handler := func(ctx context.Context, r middleware.Request) (middleware.Response, error) {
		captured = FromContext(ctx)
		return &mockResponse{code: 200}, nil
	}
	resp, err := mw.Intercept(context.Background(), req, handler)
	if err != nil {
		t.Fatalf("Intercept err: %v", err)
	}
	if resp.StatusCode() != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode())
	}
	if captured == "" {
		t.Error("Intercept should generate id when header missing")
	}
	if len(captured) != 32 {
		t.Errorf("generated id length = %d, want 32", len(captured))
	}
}

// TestIntercept_PreservesExistingID 请求带 X-Request-ID 时沿用
func TestIntercept_PreservesExistingID(t *testing.T) {
	mw := New()
	req := &mockRequest{
		method: "GET",
		path:   "/",
		header: map[string]string{HeaderRequestID: "incoming-xyz"},
	}
	var captured string
	handler := func(ctx context.Context, r middleware.Request) (middleware.Response, error) {
		captured = FromContext(ctx)
		return &mockResponse{code: 200}, nil
	}
	if _, err := mw.Intercept(context.Background(), req, handler); err != nil {
		t.Fatalf("Intercept err: %v", err)
	}
	if captured != "incoming-xyz" {
		t.Errorf("captured = %q, want incoming-xyz", captured)
	}
}
