package safe

import (
	"errors"
	"testing"
	"time"
)

func TestGO(t *testing.T) {
	fn := func() error {
		return errors.New("this go")
	}
	GO(fn)
	time.Sleep(time.Second)
}
