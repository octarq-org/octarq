package llmgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

const (
	AgentModelsPath  = "/v1/models"
	AgentChatPath    = "/v1/chat/completions"
	agentMaxBody     = 16 << 20
	agentListTimeout = 15 * time.Second
)

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

func (p *Plugin) registerAgentRoutes(mux plugin.Mux, ctx *plugin.Context) {
	if mux == nil || ctx == nil || ctx.Guard == nil {
		return
	}
	mux.Handle("GET "+AgentModelsPath, ctx.Guard(http.HandlerFunc(p.handleAgentModels)))
	mux.Handle("POST "+AgentChatPath, ctx.Guard(http.HandlerFunc(p.handleAgentChat)))
	mux.Handle("POST "+AgentResponsesPath, ctx.Guard(http.HandlerFunc(p.handleAgentResponses)))
	mux.Handle("POST "+AgentMessagesPath, ctx.Guard(http.HandlerFunc(p.handleAgentMessages)))
	mux.Handle("POST "+AgentCountTokensPath, ctx.Guard(http.HandlerFunc(p.handleAgentCountTokens)))
	mux.Handle("GET "+AgentWellKnownPath, http.HandlerFunc(p.handleOpencodeManifest))
	mux.Handle("GET "+AgentOpencodeConfigPath, ctx.Guard(http.HandlerFunc(p.handleOpencodeConfig)))
	mux.Handle("GET "+AgentGeminiModelsPath, ctx.Guard(http.HandlerFunc(p.handleGeminiModels)))
	mux.Handle("GET "+agentGeminiPrefix+"{model}", ctx.Guard(http.HandlerFunc(p.handleGeminiModel)))
	mux.Handle("POST "+agentGeminiPrefix+"{model}", ctx.Guard(http.HandlerFunc(p.handleGeminiAction)))
}

type agentProvider struct {
	BaseURL   string
	APIKey    string
	Backend   string
	Model     string
	Anthropic bool
}

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
	return p.agentProviderFromRow(ctx, row, "")
}

func (p *Plugin) agentProviderFromRow(ctx context.Context, row *Provider, keyOverride string) (agentProvider, error) {
	base, err := openAIBase(row)
	if err != nil {
		return agentProvider{}, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	}
	key, err := p.unsealKey(ctx, row, keyOverride)
	if err != nil {
		return agentProvider{}, err
	}
	return agentProvider{
		BaseURL: base,
		APIKey:  key,
		Backend: strings.ToLower(strings.TrimSpace(row.Backend)),
		Model:   strings.TrimSpace(row.Model),
	}, nil
}

func (p *Plugin) discoveryProviderFromRow(ctx context.Context, row *Provider, keyOverride string) (agentProvider, error) {
	base, err := openAIBase(row)
	anthropic := false
	switch {
	case err != nil && strings.EqualFold(strings.TrimSpace(row.Backend), "claude"):
		base, anthropic = anthropicBaseURL, true
	case err != nil:
		return agentProvider{}, fmt.Errorf("%w: %v", ErrInvalidProvider, err)
	case isAnthropicHost(base):
		base, anthropic = strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1"), true
	}
	key, err := p.unsealKey(ctx, row, keyOverride)
	if err != nil {
		return agentProvider{}, err
	}
	return agentProvider{
		BaseURL:   base,
		APIKey:    key,
		Backend:   strings.ToLower(strings.TrimSpace(row.Backend)),
		Model:     strings.TrimSpace(row.Model),
		Anthropic: anthropic,
	}, nil
}

func isAnthropicHost(base string) bool {
	u, err := neturl.Parse(base)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Hostname(), "api.anthropic.com")
}

func (p *Plugin) unsealKey(ctx context.Context, row *Provider, keyOverride string) (string, error) {
	if key := strings.TrimSpace(keyOverride); key != "" {
		return key, nil
	}
	return p.resolveAPIKey(ctx, row)
}

func (a agentProvider) sign(req *http.Request) {
	if a.APIKey == "" {
		return
	}
	req.Header.Set("Authorization", "Bearer "+a.APIKey)
}

type modelList struct {
	Object string       `json:"object"`
	Data   []modelEntry `json:"data"`
}

type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object,omitempty"`
	OwnedBy string `json:"owned_by,omitempty"`
}

const anthropicBaseURL = "https://api.anthropic.com"

const anthropicVersion = "2023-06-01"

const (
	anthropicListPage  = 1000
	anthropicListPages = 10
)

func anthropicModels(ctx context.Context, ap agentProvider) ([]modelEntry, error) {
	out := make([]modelEntry, 0, 8)
	seen := make(map[string]bool, 8)
	after := ""
	for page := 0; page < anthropicListPages; page++ {
		url := strings.TrimRight(ap.BaseURL, "/") + "/v1/models?limit=" + strconv.Itoa(anthropicListPage)
		if after != "" {
			url += "&after_id=" + neturl.QueryEscape(after)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if ap.APIKey != "" {
			req.Header.Set("x-api-key", ap.APIKey)
		}
		req.Header.Set("anthropic-version", anthropicVersion)
		req.Header.Set("Accept", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("the provider did not answer: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("the provider answer cannot be read: %w", readErr)
		}
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("the provider refused the model list: %s", upstreamMessage(resp.StatusCode, raw))
		}
		var body struct {
			Data    []modelEntry `json:"data"`
			HasMore bool         `json:"has_more"`
			LastID  string       `json:"last_id"`
		}
		if err := json.Unmarshal(raw, &body); err != nil {
			return nil, fmt.Errorf("the provider answer is not a model list: %w", err)
		}
		for _, m := range body.Data {
			if m.ID == "" || seen[m.ID] {
				continue
			}
			seen[m.ID] = true
			if m.Object == "" {
				m.Object = "model"
			}
			out = append(out, m)
		}
		if !body.HasMore || body.LastID == "" || body.LastID == after {
			return out, nil
		}
		after = body.LastID
	}
	return nil, fmt.Errorf("the provider model list did not end after %d pages", anthropicListPages)
}

func (p *Plugin) discoveredModels(ctx context.Context, ap agentProvider) ([]modelEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, agentListTimeout)
	defer cancel()
	if ap.Anthropic {
		return anthropicModels(ctx, ap)
	}
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

func writeAgentError(w http.ResponseWriter, status int, kind, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"message": message, "type": kind},
	})
}

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

func acceptOf(r *http.Request) string {
	if v := r.Header.Get("Accept"); v != "" {
		return v
	}
	return "application/json"
}

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

func agentProviderID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("provider"))
	if raw == "" {
		return 0, true
	}
	n, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || n == 0 {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The provider id is not a number.")
		return 0, false
	}
	return uint(n), true
}

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
