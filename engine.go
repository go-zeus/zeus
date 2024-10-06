package zeus

import (
	"github.com/go-zeus/zeus/app"
)

func NewApp(opts ...app.Option) app.App {
	return app.New(opts...)
}
