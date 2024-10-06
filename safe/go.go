package safe

import (
	"fmt"
	"github.com/go-zeus/zeus/log"
	"runtime/debug"
)

func GO(fn func() error) {
	go func() error {
		defer func() {
			if err := recover(); err != nil {
				log.Panic(fmt.Sprintf("Go panic:%v \n%s", err, debug.Stack()))
			}
		}()
		return fn()
	}()
}
