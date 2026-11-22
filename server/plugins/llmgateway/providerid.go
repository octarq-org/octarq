package llmgateway

import (
	"net/url"
	"strings"

	"github.com/octarq-org/octarq/server/plugin"
)

type hostMapping struct {
	suffix     string
	providerID string
}

var hostMappings = []hostMapping{
	{"generativelanguage.googleapis.com", "google"},
	{"dashscope.aliyuncs.com", "alibaba"},
	{"api.minimax.chat", "minimax"},
	{"api.minimaxi.com", "minimax"},
	{"api.siliconflow.cn", "siliconflow"},
	{"api.siliconflow.com", "siliconflow"},
	{"api.anthropic.com", "anthropic"},
	{"api.perplexity.ai", "perplexity"},
	{"api.deepinfra.com", "deepinfra"},
	{"api.fireworks.ai", "fireworks-ai"},
	{"api.together.xyz", "togetherai"},
	{"api.moonshot.cn", "moonshotai"},
	{"api.moonshot.ai", "moonshotai"},
	{"open.bigmodel.cn", "zhipuai"},
	{"api.cerebras.ai", "cerebras"},
	{"api.deepseek.com", "deepseek"},
	{"api.novita.ai", "novita-ai"},
	{"api.mistral.ai", "mistral"},
	{"openrouter.ai", "openrouter"},
	{"api.openai.com", "openai"},
	{"api.groq.com", "groq"},
	{"api.x.ai", "xai"},
	{"api.z.ai", "zai"},
}

func ProviderIDFor(row Provider) (string, error) {
	backend := strings.ToLower(strings.TrimSpace(row.Backend))
	if backend == "anthropic" {
		return "anthropic", nil
	}
	if backend == "gemini" {
		return "google", nil
	}

	baseURL := strings.TrimSpace(row.BaseURL)
	if backend == "openai" && baseURL == "" {
		return "openai", nil
	}

	host := extractHost(baseURL)
	if host != "" {
		for _, m := range hostMappings {
			if host == m.suffix || strings.HasSuffix(host, "."+m.suffix) {
				return m.providerID, nil
			}
		}
	}

	return "", plugin.ErrModelUnpriced
}

func extractHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}
