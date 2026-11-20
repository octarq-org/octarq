package llmgateway

import (
	"encoding/json"
	"net/http"
	"strings"
)

const (
	AgentWellKnownPath      = "/.well-known/opencode"
	AgentOpencodeConfigPath = "/v1/opencode/config"
	opencodeTokenEnv        = "OCTARQ_TOKEN"
	opencodeProviderID      = "octarq"
)

var opencodeTokenCommand = []string{"sh", "-c",
	`if [ -n "$OCTARQ_TOKEN" ]; then printf '%s' "$OCTARQ_TOKEN"; ` +
		`else printf 'Paste a workspace token: ' >&2; IFS= read -r t; printf '%s' "$t"; fi`}

type opencodeManifest struct {
	Auth         opencodeAuth         `json:"auth"`
	RemoteConfig opencodeRemoteConfig `json:"remote_config"`
}

type opencodeAuth struct {
	Command []string `json:"command"`
	Env     string   `json:"env"`
}

type opencodeRemoteConfig struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type opencodeConfig struct {
	Config opencodeConfigBody `json:"config"`
}

type opencodeConfigBody struct {
	Provider map[string]opencodeProvider `json:"provider"`
}

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

func workspaceBase(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
