package vault

import (
	"fmt"
	"sort"
	"strings"

	"github.com/octarq-org/octarq/server/plugin"
)

type EnvEntry struct {
	Key   string
	Value string
}

func parseDotEnv(raw string) ([]EnvEntry, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var out []EnvEntry
	seen := map[string]int{}
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "export ") {
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "export "))
		}
		eq := strings.IndexByte(trimmed, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE", idx+1)
		}
		k := strings.TrimSpace(trimmed[:eq])
		v := strings.TrimSpace(trimmed[eq+1:])
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			quote := v[0]
			v = v[1 : len(v)-1]
			if quote == '"' {
				v = strings.ReplaceAll(v, `\n`, "\n")
				v = strings.ReplaceAll(v, `\"`, `"`)
			}
		}
		if k == "" {
			return nil, fmt.Errorf("line %d: empty key", idx+1)
		}
		if prevIdx, exists := seen[k]; exists {
			out[prevIdx].Value = v
		} else {
			seen[k] = len(out)
			out = append(out, EnvEntry{Key: k, Value: v})
		}
	}
	return out, nil
}

func formatDotEnv(secrets []plugin.VaultSecret) string {
	type pair struct {
		key string
		val string
	}
	var pairs []pair
	for _, s := range secrets {
		baseKey := strings.TrimSpace(s.Meta.KeyName)
		if baseKey == "" {
			baseKey = normalizeSlug(s.Meta.Name, fmt.Sprintf("SECRET_%d", s.Meta.ID))
		}
		baseKey = strings.ToUpper(strings.ReplaceAll(baseKey, "-", "_"))
		if val, ok := s.Payload["value"]; ok && len(s.Payload) == 1 {
			pairs = append(pairs, pair{key: baseKey, val: fmt.Sprint(val)})
			continue
		}
		keys := make([]string, 0, len(s.Payload))
		for k := range s.Payload {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, fk := range keys {
			fullKey := baseKey + "_" + strings.ToUpper(strings.ReplaceAll(fk, "-", "_"))
			pairs = append(pairs, pair{key: fullKey, val: fmt.Sprint(s.Payload[fk])})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	var b strings.Builder
	for _, p := range pairs {
		v := p.val
		if strings.ContainsAny(v, " \t\n\"'#") {
			v = `"` + strings.ReplaceAll(strings.ReplaceAll(v, `"`, `\"`), "\n", `\n`) + `"`
		}
		b.WriteString(p.key)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('\n')
	}
	return b.String()
}
