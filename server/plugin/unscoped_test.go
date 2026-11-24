package plugin_test

import (
	"errors"
	"testing"

	"github.com/octarq-org/octarq/server/plugin"
)

func TestValidateUnscopedDeclaration(t *testing.T) {
	for _, ok := range []string{"/api/license/verify", "/api/license/", "/api/storefront", "/oauth/callback"} {
		if err := plugin.ValidateUnscopedDeclaration(ok); err != nil {
			t.Errorf("ValidateUnscopedDeclaration(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{
		"",
		" ",
		"/",
		"/api",
		"/api/",
		"/api/license/verify ",
		" /api/license",
		"/api/space here",
		"/api/{id}",
		"/api/*",
		"/api/x?",
		"api/x",
		"/api//x",
	} {
		if err := plugin.ValidateUnscopedDeclaration(bad); !errors.Is(err, plugin.ErrUnscopedInvalid) {
			t.Errorf("ValidateUnscopedDeclaration(%q) = %v, want ErrUnscopedInvalid", bad, err)
		}
	}
}
