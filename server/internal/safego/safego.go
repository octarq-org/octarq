package safego

import (
	"net/http"

	pkgsafego "github.com/octarq-org/octarq/server/pkg/safego"
)

// Recover is re-exported from pkg/safego for backward compatibility.
// Note: recover() must be called directly within the deferred frame.
func Recover(name string) {
	if r := recover(); r != nil {
		pkgsafego.HandleRecover(name, r)
	}
}

// Go is re-exported from pkg/safego for backward compatibility.
func Go(name string, fn func()) {
	pkgsafego.Go(name, fn)
}

// RecoverHandler is re-exported from pkg/safego for backward compatibility.
func RecoverHandler(name string, next http.Handler) http.Handler {
	return pkgsafego.RecoverHandler(name, next)
}
