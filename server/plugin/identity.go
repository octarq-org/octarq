package plugin

import (
	"errors"
	"strings"
)

const ProviderOIDC = "oidc"

type ExternalIdentity struct {
	Provider      string
	Issuer        string
	Subject       string
	Email         string
	OrgID         uint
	AllowJIT      bool
	JITRole       string
	MayCreateUser bool
}

func NormalizeIssuer(issuer string) string {
	return strings.TrimRight(strings.TrimSpace(issuer), "/")
}

var ErrLoginRegistrationDisabled = errors.New("registration disabled")

var ErrAccountLinkRequired = errors.New("account link required")

type AuthMethod struct {
	ID        string      `json:"id"`
	Label     string      `json:"label"`
	LoginURL  string      `json:"loginUrl"`
	IconKey   string      `json:"iconKey"`
	Available func() bool `json:"-"`
}
