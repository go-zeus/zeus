package chain

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// 将自己的“标签”写入RW，不做任何其他事情。
func tagMiddleware(tag string) Middleware {
	return func(h http.Handler) (http.Handler, error) {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(tag))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			h.ServeHTTP(w, r)
		}), nil
	}
}

// 相等比较（不推荐）
func funcEqual(f1, f2 interface{}) bool {
	val1 := reflect.ValueOf(f1)
	val2 := reflect.ValueOf(f2)
	return val1.Pointer() == val2.Pointer()
}

var testApp = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, err := w.Write([]byte("app\n"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
})

func TestNew(t *testing.T) {
	c1 := func(h http.Handler) (http.Handler, error) {
		return nil, nil
	}

	c2 := func(h http.Handler) (http.Handler, error) {
		return http.StripPrefix("potato", nil), nil
	}

	middles := []Middleware{c1, c2}

	chain := New(middles...)
	for k := range middles {
		if !funcEqual(chain.middlewares[k], middles[k]) {
			t.Error("New does not add middlewares correctly")
		}
	}
}

func TestThenWorksWithNoMiddleware(t *testing.T) {
	handler, err := New().Then(testApp)
	if err != nil {
		t.Error(err)
		return
	}

	if !funcEqual(handler, testApp) {
		t.Error("Then does not work with no middleware")
	}
}

func TestThenTreatsNilAsDefaultServeMux(t *testing.T) {
	handler, err := New().Then(nil)
	if err != nil {
		t.Error(err)
		return
	}

	if handler != http.DefaultServeMux {
		t.Error("Then does not treat nil as DefaultServeMux")
	}
}

func TestThenFuncTreatsNilAsDefaultServeMux(t *testing.T) {
	handler, err := New().ThenFunc(nil)
	if err != nil {
		t.Error(err)
	}

	if handler != http.DefaultServeMux {
		t.Error("ThenFunc does not treat nil as DefaultServeMux")
	}
}

func TestThenFuncConstructsHandlerFunc(t *testing.T) {
	fn := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	chained, err := New().ThenFunc(fn)
	if err != nil {
		t.Error(err)
		return
	}
	rec := httptest.NewRecorder()

	chained.ServeHTTP(rec, (*http.Request)(nil))

	if reflect.TypeOf(chained) != reflect.TypeOf((http.HandlerFunc)(nil)) {
		t.Error("ThenFunc does not construct HandlerFunc")
	}
}

func TestThenOrdersHandlersCorrectly(t *testing.T) {
	t1 := tagMiddleware("t1\n")
	t2 := tagMiddleware("t2\n")
	t3 := tagMiddleware("t3\n")

	chained, err := New(t1, t2, t3).Then(testApp)
	if err != nil {
		t.Error(err)
		return
	}

	w := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Error(err)
		return
	}

	chained.ServeHTTP(w, r)

	if w.Body.String() != "t1\nt2\nt3\napp\n" {
		t.Error("Then does not order handlers correctly")
	}
}

func TestAppendAddsHandlersCorrectly(t *testing.T) {
	chain := New(tagMiddleware("t1\n"), tagMiddleware("t2\n"))
	newChain := chain.Append(tagMiddleware("t3\n"), tagMiddleware("t4\n"))

	if len(chain.middlewares) != 2 {
		t.Error("长度错误")
	}
	if len(newChain.middlewares) != 4 {
		t.Error("长度错误")
	}

	chained, err := newChain.Then(testApp)
	if err != nil {
		t.Error(err)
		return
	}

	w := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Error(err)
		return
	}

	chained.ServeHTTP(w, r)

	if w.Body.String() != "t1\nt2\nt3\nt4\napp\n" {
		t.Error("Append does not add handlers correctly")
	}
}

func TestAppendRespectsImmutability(t *testing.T) {
	chain := New(tagMiddleware(""))
	newChain := chain.Append(tagMiddleware(""))

	if &chain.middlewares[0] == &newChain.middlewares[0] {
		t.Error("Apppend does not respect immutability")
	}
}

func TestExtendAddsHandlersCorrectly(t *testing.T) {
	chain1 := New(tagMiddleware("t1\n"), tagMiddleware("t2\n"))
	chain2 := New(tagMiddleware("t3\n"), tagMiddleware("t4\n"))
	newChain := chain1.Extend(chain2)
	if len(chain1.middlewares) != 2 {
		t.Error("长度错误")
	}
	if len(chain2.middlewares) != 2 {
		t.Error("长度错误")
	}
	if len(newChain.middlewares) != 4 {
		t.Error("长度错误")
	}

	chained, err := newChain.Then(testApp)
	if err != nil {
		t.Error(err)
		return
	}

	w := httptest.NewRecorder()
	r, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Error(err)
		return
	}

	chained.ServeHTTP(w, r)

	if w.Body.String() != "t1\nt2\nt3\nt4\napp\n" {
		t.Error("Extend does not add handlers in correctly")
	}
}

func TestExtendRespectsImmutability(t *testing.T) {
	chain := New(tagMiddleware(""))
	newChain := chain.Extend(New(tagMiddleware("")))

	if &chain.middlewares[0] == &newChain.middlewares[0] {
		t.Error("Extend does not respect immutability")
	}
}
