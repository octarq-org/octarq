package llmgateway

import (
	"encoding/json"
	"net/http"
	"strings"
)

// opencode discovers a workspace the same way it discovers an identity
// provider: it fetches {origin}/.well-known/opencode before it holds any
// credential. The document names the command that yields a workspace token and
// the address that returns the configuration, so the person never types a
// model name — the list arrives from the provider.
const (
	// AgentWellKnownPath is the discovery document opencode fetches.
	AgentWellKnownPath = "/.well-known/opencode"
	// AgentOpencodeConfigPath returns the opencode configuration, and needs a
	// workspace token: model names say which provider the workspace spends.
	AgentOpencodeConfigPath = "/v1/opencode/config"
	// opencodeTokenEnv is the variable the credential lands in, both for
	// opencode's own substitution and for the provider entry below.
	opencodeTokenEnv = "OCTARQ_TOKEN"
	// opencodeProviderID names the provider entry opencode shows.
	opencodeProviderID = "octarq"
)

// opencodeTokenCommand writes a workspace token to stdout, which opencode reads
// as the credential. An exported token answers without a prompt, so a scripted
// login works; otherwise the command asks for one.
var opencodeTokenCommand = []string{"sh", "-c",
	`if [ -n "$OCTARQ_TOKEN" ]; then printf '%s' "$OCTARQ_TOKEN"; ` +
		`else printf 'Paste a workspace token: ' >&2; IFS= read -r t; printf '%s' "$t"; fi`}

// opencodeManifest is the discovery document. opencode reads auth before it has
// a credential and remote_config after, so the two halves stay apart.
type opencodeManifest struct {
	Auth         opencodeAuth         `json:"auth"`
	RemoteConfig opencodeRemoteConfig `json:"remote_config"`
}

type opencodeAuth struct {
	// Command is argv, not a shell line: opencode spawns it directly.
	Command []string `json:"command"`
	Env     string   `json:"env"`
}

type opencodeRemoteConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// opencodeConfig is the answered configuration. opencode validates it against
// its own config schema, so it is the plain config object with a wrapper key.
type opencodeConfig struct {
	Config opencodeConfigBody `json:"config"`
}

type opencodeConfigBody struct {
	Provider map[string]opencodeProvider `json:"provider"`
}

// opencodeProvider mirrors the custom-provider block of opencode's config.
// models must be present: opencode keeps its own list and never calls
// /v1/models itself, so an empty map leaves the provider with nothing to pick.
type opencodeProvider struct {
	NPM     string                   `json:"npm"`
	Name    string                   `json:"name"`
	Env     []string                 `json:"env"`
	Options opencodeProviderOptions  `json:"options"`
	Models  map[string]opencodeModel `json:"models"`
}

type opencodeProviderOptions struct {
	BaseURL string `json:"baseURL"`
}

type opencodeModel struct {
	Name string `json:"name"`
}

// handleOpencodeManifest serves the discovery document.
//
// It is deliberately unauthenticated: opencode fetches it before it can hold a
// token, and it names no workspace data — not the provider, not a model, not a
// key.
func (p *Plugin) handleOpencodeManifest(w http.ResponseWriter, r *http.Request) {
	doc := opencodeManifest{
		Auth: opencodeAuth{Command: opencodeTokenCommand, Env: opencodeTokenEnv},
		RemoteConfig: opencodeRemoteConfig{
			URL: workspaceBase(r) + AgentOpencodeConfigPath,
			Headers: map[string]string{
				"Authorization": "Bearer {env:" + opencodeTokenEnv + "}",
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(doc)
}

// handleOpencodeConfig serves the provider block opencode loads once the token
// is known. The model names come from the provider, so a model added in the
// console appears in the agent without a second edit here.
func (p *Plugin) handleOpencodeConfig(w http.ResponseWriter, r *http.Request) {
	orgID, ok := p.agentOrg(w, r)
	if !ok {
		return
	}
	id, ok := agentProviderID(w, r)
	if !ok {
		return
	}
	ap, err := p.agentProviderFor(r.Context(), orgID, id)
	if err != nil {
		writeAgentError(w, http.StatusServiceUnavailable, "invalid_request_error", agentErrorText(err))
		return
	}
	models, err := p.discoveredModels(r.Context(), ap)
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if len(models) == 0 {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", "The provider listed no models.")
		return
	}
	catalogue := make(map[string]opencodeModel, len(models))
	for _, m := range models {
		catalogue[m.ID] = opencodeModel{Name: m.ID}
	}
	body := opencodeConfig{Config: opencodeConfigBody{Provider: map[string]opencodeProvider{
		opencodeProviderID: {
			NPM:     "@ai-sdk/openai-compatible",
			Name:    "Octarq",
			Env:     []string{opencodeTokenEnv},
			Options: opencodeProviderOptions{BaseURL: workspaceBase(r) + "/v1"},
			Models:  catalogue,
		},
	}}}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(body)
}

// workspaceBase reconstructs the address the caller reached us on, because the
// discovery document has to hand back an absolute URL.
//
// X-Forwarded-Proto is honoured without a trust flag. The document carries no
// secret, and a caller that forges the scheme only breaks its own discovery;
// the alternative — guessing http behind a TLS-terminating proxy — breaks every
// real deployment.
func workspaceBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
