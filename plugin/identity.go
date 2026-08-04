package plugin

import "strings"

// ProviderOIDC is the Provider value for an OpenID Connect identity.
const ProviderOIDC = "oidc"

// ExternalIdentity is one verified assertion from an identity provider, as
// handed to Context.LoginByIdentity or Context.BindIdentity.
//
// It is a struct rather than a parameter list because it crosses a Go module
// boundary: octarq-pro consumes octarq as a module, so widening the contract
// (group claims, role mapping) must not be a breaking change for every plugin
// that already calls it.
//
// The decision table LoginByIdentity implements, given a normalized
// (Provider, Issuer, Subject):
//
//	bound already, member of OrgID          → sign in, OrgID becomes active
//	bound already, not a member, AllowJIT   → join OrgID as JITRole, sign in
//	bound already, not a member, !AllowJIT  → ErrLoginRegistrationDisabled
//	unbound, email has no account, MayCreateUser  → create user, bind, join per AllowJIT
//	unbound, email has no account, !MayCreateUser → ErrLoginRegistrationDisabled
//	unbound, email already has an account   → ErrAccountLinkRequired
//
// The last two rows are what stop a tenant-supplied issuer from being a way
// into somebody else's account — see MayCreateUser.
type ExternalIdentity struct {
	// Provider is the identity mechanism, e.g. ProviderOIDC.
	Provider string
	// Issuer identifies the IdP. Pass it through NormalizeIssuer first; the core
	// stores and queries the normalized form and does not re-normalize for you.
	Issuer string
	// Subject is the provider's stable, issuer-scoped identifier for the person.
	// Never an email — email claims change and are not unique across issuers.
	Subject string
	// Email is what the provider asserted. It is recorded for display and used
	// once, to detect the ErrAccountLinkRequired case; it never resolves a user.
	Email string
	// OrgID is the org this sign-in is for — for per-org SSO, the one the login
	// URL named. JIT membership only ever touches this org.
	OrgID uint
	// AllowJIT lets an authenticated identity that is not yet a member join
	// OrgID automatically. Owned by that org's admins: it decides who gets into
	// their workspace, which is theirs to decide.
	AllowJIT bool
	// JITRole is the role such a member joins with. Empty means "member".
	// Never grants instance admin — that privilege is bound to the configured
	// admin credential and no external assertion can reach it.
	JITRole string
	// MayCreateUser allows this IdP to mint a global account for an email that
	// does not exist on the instance yet. It is emphatically NOT the org's
	// decision, and must be gated on the instance administrator.
	//
	// An org admin configures their own issuer. If that were enough to create
	// accounts, they could assert victim@acme.com before that person ever signs
	// up, and the resulting User row — bound to the attacker's (issuer, subject)
	// — becomes the row every later "find the user with this email" lands on,
	// including the one that attaches an invitation to a workspace. Authenticating
	// an identity that already exists is an org-level decision; minting a new
	// global identity is an instance-level one.
	MayCreateUser bool
}

// NormalizeIssuer canonicalizes an OIDC issuer for storage and comparison.
//
// It exists so the SSO configuration and the identity binding cannot disagree.
// Providers advertise the issuer with and without a trailing slash
// interchangeably; store one spelling and query the other and every user of
// that IdP silently looks unbound, which lands them in the
// ErrAccountLinkRequired branch and locks them out. Both sides call this.
func NormalizeIssuer(issuer string) string {
	return strings.TrimRight(strings.TrimSpace(issuer), "/")
}
