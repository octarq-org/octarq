package safego

import (
	"net/http"

	pkgsafego "github.com/octarq-org/octarq/server/pkg/safego"
)

func Recover(name string) {
	if r := recover(); r != nil {
		pkgsafego.HandleRecover(name, r)
	}
}

func Go(name string, fn func()) {
	pkgsafego.Go(name, fn)
}

func RecoverHandler(name string, next http.Handler) http.Handler {
	return pkgsafego.RecoverHandler(name, next)
}
