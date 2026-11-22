package llmgateway

import (
	"bytes"
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
	virtualKeyPrefix = "sk-octarq-"
	spendSourceProxy = "gateway_api"
)

type spendAuthContextKey struct{}

func withSpendAccount(ctx context.Context, acct *plugin.SpendAccountAuth) context.Context {
	ctx = plugin.WithOrgID(ctx, acct.OrgID)
	return context.WithValue(ctx, spendAuthContextKey{}, acct)
}

func spendAccountFromContext(ctx context.Context) *plugin.SpendAccountAuth {
	acct, _ := ctx.Value(spendAuthContextKey{}).(*plugin.SpendAccountAuth)
	return acct
}

func extractVirtualKey(r *http.Request) (string, bool) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) < 7 || !strings.EqualFold(auth[:7], "bearer ") {
		return "", false
	}
	token := strings.TrimSpace(auth[7:])
	if !strings.HasPrefix(token, virtualKeyPrefix) {
		return "", false
	}
	return token, true
}

func (p *Plugin) virtualKeyMiddleware(guard func(http.Handler) http.Handler, next http.Handler) http.Handler {
	guarded := guard(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := extractVirtualKey(r)
		if !ok || p.pctx == nil || p.pctx.Host == nil || p.pctx.Host.SpendLedger() == nil {
			guarded.ServeHTTP(w, r)
			return
		}
		acct, err := p.pctx.Host.SpendLedger().ResolveVirtualKey(r.Context(), token)
		if errors.Is(err, plugin.ErrSpendAccountPaused) || errors.Is(err, plugin.ErrSpendBudgetExhausted) {
			writeAgentError(w, http.StatusPaymentRequired, "billing_error", "The spend account is paused or has exhausted its budget.")
			return
		}
		if err != nil || acct == nil || acct.OrgID == 0 {
			writeAgentError(w, http.StatusUnauthorized, "authentication_error", "The virtual key is invalid.")
			return
		}
		if strings.EqualFold(acct.Status, "paused") || (acct.BudgetLimit > 0 && acct.BudgetUsed >= acct.BudgetLimit) {
			writeAgentError(w, http.StatusPaymentRequired, "billing_error", "The spend account is paused or has exhausted its budget.")
			return
		}
		next.ServeHTTP(w, r.WithContext(withSpendAccount(r.Context(), acct)))
	})
}

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
	mux.Handle("GET "+AgentModelsPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleAgentModels)))
	mux.Handle("POST "+AgentChatPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleAgentChat)))
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

func isModelAllowed(model string, allowed []string) bool {
	if model == "" {
		return false
	}
	for _, a := range allowed {
		trimmed := strings.TrimSpace(a)
		if trimmed == "*" || strings.EqualFold(trimmed, model) {
			return true
		}
	}
	return false
}

type chatCompletionUsageEnvelope struct {
	ID    string `json:"id"`
	Model string `json:"model"`
	Usage *struct {
		PromptTokens            int `json:"prompt_tokens"`
		CompletionTokens        int `json:"completion_tokens"`
		InputTokens             int `json:"input_tokens"`
		OutputTokens            int `json:"output_tokens"`
		CacheReadInputTokens    int `json:"cache_read_input_tokens"`
		CacheCreationInputToken int `json:"cache_creation_input_tokens"`
		PromptTokensDetails     *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionTokensDetails *struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
}

func extractUsageFromEnvelope(env chatCompletionUsageEnvelope, turnID, model *string, in, out, reasoning, cacheRead, cacheWrite *int, sawUsage *bool) {
	if env.ID != "" {
		*turnID = env.ID
	}
	if env.Model != "" {
		*model = env.Model
	}
	if env.Usage == nil {
		return
	}
	*sawUsage = true
	if env.Usage.PromptTokens > 0 || env.Usage.InputTokens == 0 {
		*in = env.Usage.PromptTokens
	} else {
		*in = env.Usage.InputTokens
	}
	if env.Usage.CompletionTokens > 0 || env.Usage.OutputTokens == 0 {
		*out = env.Usage.CompletionTokens
	} else {
		*out = env.Usage.OutputTokens
	}
	if env.Usage.CompletionTokensDetails != nil {
		*reasoning = env.Usage.CompletionTokensDetails.ReasoningTokens
	}
	if env.Usage.PromptTokensDetails != nil && env.Usage.PromptTokensDetails.CachedTokens > 0 {
		*cacheRead = env.Usage.PromptTokensDetails.CachedTokens
	} else if env.Usage.CacheReadInputTokens > 0 {
		*cacheRead = env.Usage.CacheReadInputTokens
	}
	if env.Usage.CacheCreationInputToken > 0 {
		*cacheWrite = env.Usage.CacheCreationInputToken
	}
}

func parseRelayedChatUsage(raw []byte, fallbackModel string) (plugin.TurnSpend, bool) {
	turnID := ""
	model := fallbackModel
	var inTok, outTok, reasonTok, cacheReadTok, cacheWriteTok int
	sawUsage := false

	var single chatCompletionUsageEnvelope
	if err := json.Unmarshal(raw, &single); err == nil && single.Usage != nil {
		extractUsageFromEnvelope(single, &turnID, &model, &inTok, &outTok, &reasonTok, &cacheReadTok, &cacheWriteTok, &sawUsage)
	} else {
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}
			var chunk chatCompletionUsageEnvelope
			if err := json.Unmarshal([]byte(payload), &chunk); err == nil {
				extractUsageFromEnvelope(chunk, &turnID, &model, &inTok, &outTok, &reasonTok, &cacheReadTok, &cacheWriteTok, &sawUsage)
			}
		}
	}
	if !sawUsage {
		return plugin.TurnSpend{}, false
	}
	if turnID == "" {
		turnID = fmt.Sprintf("gw-%d", time.Now().UnixNano())
	}
	return plugin.TurnSpend{
		Model:            model,
		InputTokens:      inTok,
		OutputTokens:     outTok,
		ReasoningTokens:  reasonTok,
		CacheReadTokens:  cacheReadTok,
		CacheWriteTokens: cacheWriteTok,
		Source:           spendSourceProxy,
		TurnID:           turnID,
	}, true
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

	acct := spendAccountFromContext(r.Context())
	var forwardBody io.Reader = body
	contentLength := r.ContentLength
	reqModel := ap.Model

	if acct != nil {
		rawBody, readErr := io.ReadAll(body)
		if readErr != nil {
			writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request body cannot be read.")
			return
		}
		var chatReq struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(rawBody, &chatReq)
		if m := strings.TrimSpace(chatReq.Model); m != "" {
			reqModel = m
		}
		if len(acct.AllowedModels) > 0 && !isModelAllowed(strings.TrimSpace(chatReq.Model), acct.AllowedModels) {
			writeAgentError(w, http.StatusForbidden, "permission_error", "The requested model is not allowed for this spend account.")
			return
		}
		forwardBody = bytes.NewReader(rawBody)
		contentLength = int64(len(rawBody))
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		ap.BaseURL+"/chat/completions", forwardBody)
	if err != nil {
		writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request cannot be built.")
		return
	}
	if contentLength > 0 && contentLength <= agentMaxBody {
		req.ContentLength = contentLength
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

	if acct == nil {
		relay(w, resp)
		return
	}
	captured := relayAndCapture(w, resp, 2<<20)
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 && p.pctx != nil && p.pctx.Host != nil && p.pctx.Host.SpendLedger() != nil {
		if spend, hasUsage := parseRelayedChatUsage(captured, reqModel); hasUsage {
			spend.AccountID = acct.AccountID
			_, _ = p.pctx.Host.SpendLedger().RecordSpend(r.Context(), orgID, spend)
		}
	}
}

func acceptOf(r *http.Request) string {
	if v := r.Header.Get("Accept"); v != "" {
		return v
	}
	return "application/json"
}

func relay(w http.ResponseWriter, resp *http.Response) {
	_ = relayAndCapture(w, resp, 0)
}

func relayAndCapture(w http.ResponseWriter, resp *http.Response, maxCapture int) []byte {
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
	var captured bytes.Buffer
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if maxCapture > 0 && captured.Len() < maxCapture {
				rem := maxCapture - captured.Len()
				if n < rem {
					rem = n
				}
				captured.Write(buf[:rem])
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				return captured.Bytes()
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return captured.Bytes()
		}
	}
}

func (p *Plugin) agentOrg(w http.ResponseWriter, r *http.Request) (uint, bool) {
	if acct := spendAccountFromContext(r.Context()); acct != nil && acct.OrgID != 0 {
		return acct.OrgID, true
	}
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
