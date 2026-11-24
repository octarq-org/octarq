package llmgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/server/plugin"
)

const (
	AgentModelsPath    = "/v1/models"
	AgentChatPath      = "/v1/chat/completions"
	agentMaxBody       = 16 << 20
	agentListTimeout   = 15 * time.Second
	spendSourceProxy   = "gateway_api"
	spendWriteTimeout  = 10 * time.Second
	streamUsageTailCap = 1 << 20
	maxTurnIDLength    = 64
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

func extractAgentToken(r *http.Request) (string, bool) {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "bearer ") {
		if token := strings.TrimSpace(auth[7:]); token != "" {
			return token, true
		}
	}
	for _, name := range []string{"x-api-key", "x-goog-api-key"} {
		if token := strings.TrimSpace(r.Header.Get(name)); token != "" {
			return token, true
		}
	}
	if token := strings.TrimSpace(r.URL.Query().Get("key")); token != "" {
		return token, true
	}
	return "", false
}

func (p *Plugin) virtualKeyMiddleware(guard func(http.Handler) http.Handler, next http.Handler) http.Handler {
	guarded := guard(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := extractAgentToken(r)
		if !ok || p.pctx == nil || p.pctx.Host == nil || p.pctx.Host.SpendLedger() == nil {
			guarded.ServeHTTP(w, r)
			return
		}
		ledger := p.pctx.Host.SpendLedger()
		if !ledger.IsVirtualKey(token) {
			guarded.ServeHTTP(w, r)
			return
		}
		acct, err := ledger.ResolveVirtualKey(r.Context(), token)
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
	mux.Handle("POST "+AgentResponsesPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleAgentResponses)))
	mux.Handle("POST "+AgentMessagesPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleAgentMessages)))
	mux.Handle("POST "+AgentCountTokensPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleAgentCountTokens)))
	mux.Handle("GET "+AgentWellKnownPath, http.HandlerFunc(p.handleOpencodeManifest))
	mux.Handle("GET "+AgentOpencodeConfigPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleOpencodeConfig)))
	mux.Handle("GET "+AgentGeminiModelsPath, p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleGeminiModels)))
	mux.Handle("GET "+agentGeminiPrefix+"{model}", p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleGeminiModel)))
	mux.Handle("POST "+agentGeminiPrefix+"{model}", p.virtualKeyMiddleware(ctx.Guard, http.HandlerFunc(p.handleGeminiAction)))
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

type promptTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

type completionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

type tokenUsage struct {
	PromptTokens             int                      `json:"prompt_tokens"`
	CompletionTokens         int                      `json:"completion_tokens"`
	InputTokens              int                      `json:"input_tokens"`
	OutputTokens             int                      `json:"output_tokens"`
	CacheReadInputTokens     int                      `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int                      `json:"cache_creation_input_tokens"`
	PromptTokensDetails      *promptTokensDetails     `json:"prompt_tokens_details"`
	CompletionTokensDetails  *completionTokensDetails `json:"completion_tokens_details"`
}

type normalizedUsage struct {
	Input      int
	Output     int
	Reasoning  int
	CacheRead  int
	CacheWrite int
}

func normalizeUsage(u *tokenUsage) (normalizedUsage, bool) {
	if u == nil {
		return normalizedUsage{}, false
	}
	var n normalizedUsage
	cached := 0
	if u.PromptTokensDetails != nil {
		cached = u.PromptTokensDetails.CachedTokens
	}
	if u.PromptTokens > 0 {
		n.Input = u.PromptTokens - cached
		if n.Input < 0 {
			n.Input = 0
		}
	} else {
		n.Input = u.InputTokens
	}
	if cached > 0 {
		n.CacheRead = cached
	} else if u.CacheReadInputTokens > 0 {
		n.CacheRead = u.CacheReadInputTokens
	}
	if u.CompletionTokens > 0 || u.OutputTokens == 0 {
		n.Output = u.CompletionTokens
	} else {
		n.Output = u.OutputTokens
	}
	if u.CompletionTokensDetails != nil {
		n.Reasoning = u.CompletionTokensDetails.ReasoningTokens
	}
	if u.CacheCreationInputTokens > 0 {
		n.CacheWrite = u.CacheCreationInputTokens
	}
	return n, true
}

func responseTokenCounts(u *tokenUsage) (int, int) {
	n, ok := normalizeUsage(u)
	if !ok {
		return 0, 0
	}
	return n.Input, n.Output
}

func wireTokenTotals(u *tokenUsage) (int, int, int) {
	if u == nil {
		return 0, 0, 0
	}
	if u.PromptTokens > 0 || u.InputTokens == 0 {
		cached := 0
		if u.PromptTokensDetails != nil {
			cached = u.PromptTokensDetails.CachedTokens
		}
		output := u.CompletionTokens
		if u.CompletionTokens == 0 && u.OutputTokens > 0 {
			output = u.OutputTokens
		}
		return u.PromptTokens, output, cached
	}
	cached := u.CacheReadInputTokens
	if u.PromptTokensDetails != nil && u.PromptTokensDetails.CachedTokens > 0 {
		cached = u.PromptTokensDetails.CachedTokens
	}
	input := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	output := u.OutputTokens
	if u.CompletionTokens > 0 {
		output = u.CompletionTokens
	}
	return input, output, cached
}

type chatCompletionUsageEnvelope struct {
	ID    string      `json:"id"`
	Model string      `json:"model"`
	Usage *tokenUsage `json:"usage"`
}

func parseRelayedChatUsage(raw []byte, fallbackModel string) (plugin.TurnSpend, bool) {
	turnID := ""
	model := fallbackModel
	var total normalizedUsage
	sawUsage := false

	consume := func(env chatCompletionUsageEnvelope) {
		if env.ID != "" {
			turnID = env.ID
		}
		if env.Model != "" {
			model = env.Model
		}
		if env.Usage == nil {
			return
		}
		if n, ok := normalizeUsage(env.Usage); ok {
			total = n
			sawUsage = true
		}
	}

	var single chatCompletionUsageEnvelope
	if err := json.Unmarshal(raw, &single); err == nil && single.Usage != nil {
		consume(single)
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
				consume(chunk)
			}
		}
	}
	if !sawUsage {
		return plugin.TurnSpend{}, false
	}
	return plugin.TurnSpend{
		Model:            model,
		InputTokens:      total.Input,
		OutputTokens:     total.Output,
		ReasoningTokens:  total.Reasoning,
		CacheReadTokens:  total.CacheRead,
		CacheWriteTokens: total.CacheWrite,
		Source:           spendSourceProxy,
		TurnID:           gatewayTurnID(turnID),
	}, true
}

func gatewayTurnID(upstreamID string) string {
	if id := strings.TrimSpace(upstreamID); id != "" && len(id) <= maxTurnIDLength {
		return id
	}
	return fmt.Sprintf("gw-%d", time.Now().UnixNano())
}

func (p *Plugin) spendLedger() plugin.SpendLedger {
	if p == nil || p.pctx == nil || p.pctx.Host == nil {
		return nil
	}
	return p.pctx.Host.SpendLedger()
}

func (p *Plugin) recordSpend(parent context.Context, orgID uint, spend plugin.TurnSpend) {
	ledger := p.spendLedger()
	if ledger == nil {
		slog.WarnContext(parent, "llmgateway: no spend ledger to record a turn", "org", orgID, "turn", spend.TurnID)
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), spendWriteTimeout)
	defer cancel()
	if _, err := ledger.RecordSpend(ctx, orgID, spend); err != nil {
		slog.WarnContext(ctx, "llmgateway: record spend failed",
			"org", orgID, "account", spend.AccountID, "turn", spend.TurnID, "error", err)
	}
}

func modelRefused(acct *plugin.SpendAccountAuth, model string) bool {
	return acct != nil && len(acct.AllowedModels) > 0 && !isModelAllowed(strings.TrimSpace(model), acct.AllowedModels)
}

type billingRefusal struct {
	status  int
	message string
}

func billingRefusalFor(err error) billingRefusal {
	switch {
	case errors.Is(err, plugin.ErrSpendModelUnpriced),
		errors.Is(err, plugin.ErrModelUnpriced),
		errors.Is(err, plugin.ErrPricingUnavailable):
		return billingRefusal{http.StatusBadRequest, "The pricing dataset has no price for this model. Choose another model."}
	case errors.Is(err, plugin.ErrSpendAccountPaused), errors.Is(err, plugin.ErrSpendBudgetExhausted):
		return billingRefusal{http.StatusPaymentRequired, "The spend account is paused or has exhausted its budget."}
	case errors.Is(err, plugin.ErrSpendAccountNotFound):
		return billingRefusal{http.StatusNotFound, "The spend account does not exist in this workspace."}
	case errors.Is(err, plugin.ErrSpendUnavailable):
		return billingRefusal{http.StatusConflict, "This build has no spend ledger."}
	default:
		return billingRefusal{http.StatusInternalServerError, "The gateway cannot check the account budget. Retry the request."}
	}
}

func billingErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusPaymentRequired, http.StatusConflict:
		return "billing_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	default:
		return "server_error"
	}
}

func buildTurnSpend(acct *plugin.SpendAccountAuth, model, turnID string, n normalizedUsage) plugin.TurnSpend {
	return plugin.TurnSpend{
		AccountID:        acct.AccountID,
		Model:            model,
		InputTokens:      n.Input,
		OutputTokens:     n.Output,
		ReasoningTokens:  n.Reasoning,
		CacheReadTokens:  n.CacheRead,
		CacheWriteTokens: n.CacheWrite,
		Source:           spendSourceProxy,
		TurnID:           gatewayTurnID(turnID),
	}
}

func rawStringField(envelope map[string]json.RawMessage, key string) string {
	raw, ok := envelope[key]
	if !ok {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func rawBoolField(envelope map[string]json.RawMessage, key string) bool {
	raw, ok := envelope[key]
	if !ok {
		return false
	}
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	return value
}

func withStreamUsage(rawBody []byte, envelope map[string]json.RawMessage) ([]byte, error) {
	options := map[string]json.RawMessage{}
	if existing, ok := envelope["stream_options"]; ok {
		_ = json.Unmarshal(existing, &options)
	}
	options["include_usage"] = json.RawMessage("true")
	merged, err := json.Marshal(options)
	if err != nil {
		return nil, err
	}
	envelope["stream_options"] = merged
	return json.Marshal(envelope)
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
	captureLimit := 0

	if acct != nil {
		rawBody, readErr := io.ReadAll(body)
		if readErr != nil {
			writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request body cannot be read.")
			return
		}
		var envelope map[string]json.RawMessage
		_ = json.Unmarshal(rawBody, &envelope)
		if m := rawStringField(envelope, "model"); m != "" {
			reqModel = m
		}
		if modelRefused(acct, reqModel) {
			writeAgentError(w, http.StatusForbidden, "permission_error", "The requested model is not allowed for this spend account.")
			return
		}
		ledger := p.spendLedger()
		if ledger == nil {
			writeAgentError(w, http.StatusConflict, "billing_error", "This build has no spend ledger.")
			return
		}
		if err := ledger.AuthorizeTurn(r.Context(), orgID, acct.AccountID, reqModel); err != nil {
			refusal := billingRefusalFor(err)
			writeAgentError(w, refusal.status, billingErrorType(refusal.status), refusal.message)
			return
		}
		if rawBoolField(envelope, "stream") {
			injected, injErr := withStreamUsage(rawBody, envelope)
			if injErr != nil {
				writeAgentError(w, http.StatusBadRequest, "invalid_request_error", "The request body cannot be read.")
				return
			}
			rawBody = injected
			captureLimit = streamUsageTailCap
		} else {
			captureLimit = agentMaxBody
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
	captured := relayAndCapture(w, resp, captureLimit)
	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		if spend, hasUsage := parseRelayedChatUsage(captured, reqModel); hasUsage {
			spend.AccountID = acct.AccountID
			p.recordSpend(r.Context(), orgID, spend)
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
	var captured []byte
	buf := make([]byte, 32<<10)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if maxCapture > 0 {
				captured = append(captured, buf[:n]...)
				if len(captured) > maxCapture {
					copy(captured, captured[len(captured)-maxCapture:])
					captured = captured[:maxCapture]
				}
			}
			if _, werr := w.Write(buf[:n]); werr != nil {
				return captured
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return captured
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
