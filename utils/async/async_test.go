package async_test

import (
	"github.com/go-zeus/zeus/utils/async"
	"testing"
	"time"
)

type Te string

func (t Te) Echo() string {
	return string(t)
}

func testAsync() Te {
	return "hello async"
}

func testAsync2() Te {
	time.Sleep(3 * time.Second)
	return "hello async2"
}

func TestAwait(t *testing.T) {
	val := async.Exec(testAsync)
	val2 := async.Exec(testAsync2)

	a, _ := val.Await()
	t.Log(a)
	time.Sleep(2 * time.Second)
	b, _ := val2.Await()
	t.Log(b)
}
