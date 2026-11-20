package hostlist

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

type Host struct {
	Host    string `json:"host"`
	Enabled bool   `json:"enabled"`
}

type HostList []Host

func (l HostList) Value() (driver.Value, error) {
	if len(l) == 0 {
		return "[]", nil
	}
	b, err := json.Marshal([]Host(l))
	return string(b), err
}

func (l *HostList) Scan(v any) error {
	if v == nil {
		*l = nil
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return fmt.Errorf("HostList: unsupported scan type %T", v)
	}
	if len(b) == 0 {
		*l = nil
		return nil
	}
	var hosts []Host
	if err := json.Unmarshal(b, &hosts); err != nil {
		return err
	}
	*l = hosts
	return nil
}

func (l HostList) Enabled() []string {
	out := make([]string, 0, len(l))
	for _, h := range l {
		if h.Enabled {
			out = append(out, h.Host)
		}
	}
	return out
}

func (l HostList) Blocks(host string) bool {
	listed := false
	for _, h := range l {
		if h.Host == host {
			listed = true
			if h.Enabled {
				return false
			}
		}
	}
	return listed
}

func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return ""
	}
	if i := strings.LastIndex(host, "]"); i >= 0 {
		host = host[:i+1]
	} else if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	return strings.TrimSuffix(host, ".")
}
