package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

// The agent API is the workspace's model, served to coding agents.
//
// A coding agent (Claude Code, Codex, Cline, aider) speaks the OpenAI wire
// format. It asks GET /v1/models what the workspace offers and then posts to
// POST /v1/chat/completions. The gateway answers both by calling the provider
// the workspace configured, so an agent never holds the provider's key and
// never needs a model name typed into it: the list comes from the provider's
// own /models endpoint.
//
// The two routes sit behind the ordinary token guard. A coding agent sends the
// same bearer token the console mints, and the workspace it names is the
// workspace whose provider is spent.
const (
	// AgentModelsPath is the OpenAI-compatible model list.
	AgentModelsPath = "/v1/models"
	// AgentChatPath is the OpenAI-compatible chat completion.
	AgentChatPath = "/v1/chat/completions"
	// agentMaxBody bounds one chat request. A coding agent sends whole files;
	// 16 MiB is far above a real turn and small enough to refuse a mistake.
	agentMaxBody = 16 << 20
	// agentListTimeout bounds the model-list call to the provider.
	agentListTimeout = 15 * time.Second
)

// hopByHop headers are connection-scoped and must not be relayed.
var hopByHop = map[string]bool{
	"connection":          true,
	"keep-alive":          true,
	"proxy-authenticate":  true,
	"proxy-authorization": true,
	"te":                  true,
	"trailer":             true,
	"transfer-encoding":   true,
	"upgrade":             true,
	"content-length":      true,
}

// registerAgentRoutes mounts the OpenAI-compatible surface on the plain mux.
//
// These routes are not huma operations: a chat completion streams the vendor's
// own bytes, and a huma handler would have to decode and re-encode them. The
// guard is the token guard, so a coding agent authorises with the same bearer
// token the console mints.
//
// A mount without a guard is a test composition, not a deployment: the app
// always supplies one, and the routes are simply absent when it does not.
func (p *Plugin) registerAgentRoutes(mux plugin.Mux, ctx *plugin.Context) {
	if mux == nil || ctx == nil || ctx.Guard == nil {
		return
	}
	mux.Handle("GET "+AgentModelsPath, ctx.Guard(http.HandlerFunc(p.handleAgentModels)))
	mux.Handle("POST "+AgentChatPath, ctx.Guard(http.HandlerFunc(p.handleAgentChat)))
}

// agentProvider is one workspace provider resolved for an outbound call: the
// base URL that speaks the OpenAI API and the key that opens it.
type agentProvider struct {
	BaseURL string
	APIKey  string
	Backend string
}

// openAIBase returns where the OpenAI-compatible API of a backend lives.
//
// A provider's own BaseURL always wins. The defaults exist so a workspace that
// named only a backend still works; a backend with no OpenAI surface is refused
// by name rather than guessed at.
func openAIBase(row *Provider) (string, error) {
	if base := strings.TrimRight(strings.TrimSpace(row.BaseURL), "/"); base != "" {
		return base, nil
	}
	switch strings.ToLower(strings.TrimSpace(row.Backend)) {
	case "openai":
		return "https://api.openai.com/v1", nil
	case "ollama":
		return "http://localhost:11434/v1", nil
	case "claude", "gemini", "mistral", "cohere":
		return "", fmt.Errorf("the %s backend has no OpenAI-compatible endpoint; set the provider's base URL to a gateway that has one", row.Backend)
	default:
		return "", fmt.Errorf("the %s backend has no OpenAI-compatible endpoint", row.Backend)
	}
}

// agentProviderFor resolves the provider a request spends. An empty id means
// the workspace's default provider.
func (p *Plugin) agentProviderFor(ctx context.Context, orgID uint, id uint) (agentProvider, error) {
	if p.store == nil {
		return agentProvider{}, ErrNotConfigured
	}
	var row *Provider
	var err error
	if id == 0 {
		row, err = p.store.ActiveProvider(ctx, orgID)
	} else {
		row, err = p.store.GetProvider(ctx, orgID, id)
	}
	if err != nil {
		return agentProvider{}, err
	}
	return p.agentProviderFromRow(row, "")
}

// agentProviderFromRow builds the outbound call from one row. A non-empty
// keyOverride is a key the caller typed and has not saved yet; the stored
// sealed key serves every other case.
func (p *Plugin) agentProviderFromRow(row *Provider, keyOverride string) (agentProvider, error) {
	base, err := openAIBase(row)
	if err != nil {
		return agentProvider{}, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	key := strings.TrimSpace(keyOverride)
	if key == "" && row.APIKeyEncr != "" {
		vault := p.vault()
		if vault == nil {
			return agentProvider{}, fmt.Errorf("%w: secret storage is unavailable", ErrInvalidProvider)
		}
		plain, err := vault.Decrypt(row.APIKeyEncr)
		if err != nil {
			return agentProvider{}, fmt.Errorf("%w: unseal API key: %v", ErrInvalidProvider, err)
		}
		key = string(plain)
	}
	return agentProvider{BaseURL: base, APIKey: key, Backend: strings.ToLower(strings.TrimSpace(row.Backend))}, nil
}

// sign adds the credential as a bearer token. Every backend that reaches this
// point speaks the OpenAI wire format, so none of them wants another scheme.
func (a agentProvider) sign(req *http.Request) {
	if a.APIKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.APIKey)
}

// modelList is the OpenAI shape and the only list shape this API answers.
type modelList struct {
	Object string       `json:"object"`
	Data   []modelEntry `json:"data"`
}

// modelEntry is one model a provider offers.
type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

// discoveredModels calls the provider's own /models and normalises the answer.
//
// OpenAI-compatible backends answer {"data":[{"id":…}]}; Ollama answers
// {"models":[{"name":…}]}. Both are read, so a workspace pointed at either
// lists its models without anyone typing them in.
func (p *Plugin) discoveredModels(ctx context.Context, ap agentProvider) ([]modelEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, agentListTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ap.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	ap.sign(req)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("the provider did not answer: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("the provider answer cannot be read: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("the provider refused the model list: %s", upstreamMessage(resp.StatusCode, raw))
	}
	var body struct {
		Data   []modelEntry `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("the provider answer is not a model list: %w", err)
	}
	out := make([]modelEntry, 0, len(body.Data)+len(body.Models))
	for _, m := range body.Data {
		if m.ID == "" {
			continue
		}
		if m.Object == "" {
			m.Object = "model"
		}
		out = append(out, m)
	}
	for _, m := range body.Models {
		id := m.ID
		if id == "" {
			id = m.Name
		}
		if id == "" {
			continue
		}
		out = append(out, modelEntry{ID: id, Object: "model"})
	}
	if len(out) == 0 {
		return nil, errors.New("the provider listed no models")
	}
	return out, nil
}

// upstreamMessage keeps an upstream error legible without leaking its headers.
func upstreamMessage(status int, raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 300 {
		text = text[:300]
	}
	if text == "" {
		return fmt.Sprintf("HTTP %d", status)
	}
	return fmt.Sprintf("HTTP %d: %s", status, text)
}

// writeAgentError answers in the OpenAI error shape, because the client on the
// other end parses that shape and nothing else.
func writeAgentError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"message": message, "type": kind},
	})
}

// handleAgentModels serves GET /v1/models.
func (p *Plugin) handleAgentModels(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(modelList{Object: "list", Data: models})
}

// handleAgentChat serves POST /v1/chat/completions by relaying the call.
//
// The body is forwarded byte for byte, so tools, streaming and response_format
// keep working without this gateway knowing each vendor's dialect. The response
// is streamed back the moment the provider writes it, which is what makes a
// coding agent feel local.
func (p *Plugin) handleAgentChat(w http.ResponseWriter, r *http.Request) {
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
	body := http.MaxBytesReader(w, r.Body, agentMaxBody)
	defer body.Close()

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		ap.BaseURL+"/chat/completions", body)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request cannot be built.")
		return
	}
	// Keep the declared length so the provider receives a plain body rather
	// than a chunked one. Some backends read Content-Length only, and a coding
	// agent always knows the size of the turn it sends.
	if r.ContentLength > 0 && r.ContentLength <= agentMaxBody {
		req.ContentLength = r.ContentLength
	}
	ap.sign(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", acceptOf(r))
	if v := r.Header.Get("OpenAI-Beta"); v != "" {
		req.Header.Set("OpenAI-Beta", v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeAgentError(w, http.StatusBadGateway, "upstream_error", "The provider did not answer.")
		return
	}
	defer resp.Body.Close()
	relay(w, resp)
}

// acceptOf keeps the caller's streaming preference.
func acceptOf(r *http.Request) string {
	if v := r.Header.Get("Accept"); v != "" {
		return v
	}
	return "application/json"
}

// relay copies an upstream answer to the client, flushing as it goes.
func relay(w http.ResponseWriter, resp *http.Response) {
	for k, values := range resp.Header {
		if hopByHop[strings.ToLower(k)] {
			continue
		}
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// agentOrg is the workspace the caller's token names.
func (p *Plugin) agentOrg(w http.ResponseWriter, r *http.Request) (uint, bool) {
	if p.pctx == nil || p.pctx.Host == nil || p.pctx.Host.Session() == nil {
		writeAgentError(w, http.StatusInternalServerError, "server_error", "The workspace cannot be read.")
		return 0, false
	}
	orgID := p.pctx.Host.Session().OrgID(r)
	if orgID == 0 {
		writeAgentError(w, http.StatusForbidden, "invalid_request_error", "The token names no workspace.")
		return 0, false
	}
	return orgID, true
}

// agentProviderID reads the optional ?provider= selector.
func agentProviderID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("provider"))
	if raw == "" {
		return 0, true
	}
	// Parsed strictly: "12abc" is a mistake, not provider 12.
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || n == 0 {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The provider id is not a number.")
		return 0, false
	}
	return uint(n), true
}

// agentErrorText turns a resolution failure into one sentence a client can read.
func agentErrorText(err error) string {
	switch {
	case errors.Is(err, ErrNotConfigured):
		return "This workspace has no model provider. Add one in the console."
	case errors.Is(err, ErrNotFound):
		return "The provider does not exist in this workspace."
	default:
		return err.Error()
	}
}
