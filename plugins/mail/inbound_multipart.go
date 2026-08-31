package mail

import (
	"io"
	"log"
	"net/http"
	"strings"
)

func (p *Plugin) recordInboundAuthFailure(r *http.Request, orgID uint, route string) {
	ip := reporterIP(r)
	log.Printf("inbound: rejected bad token for org %d via %s from %s", orgID, route, ip)
	if p.audit == nil || r == nil {
		return
	}
	p.audit(r, "email.inbound.auth_failed", "org", orgID, map[string]any{
		"route": route,
		"ip":    ip,
	})
}

func findMultipartField(r *http.Request, names []string) []byte {
	if r.MultipartForm == nil {
		return nil
	}
	for _, name := range names {
		if files, ok := r.MultipartForm.File[name]; ok && len(files) > 0 {
			f, err := files[0].Open()
			if err == nil {
				b, readErr := io.ReadAll(io.LimitReader(f, 25<<20))
				_ = f.Close()
				if readErr == nil && len(b) > 0 {
					return b
				}
			}
		}
	}
	for _, name := range names {
		if vals, ok := r.MultipartForm.Value[name]; ok && len(vals) > 0 && vals[0] != "" {
			return []byte(vals[0])
		}
	}
	return nil
}

func extractRawEmail(r *http.Request) ([]byte, error) {
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(strings.ToLower(ct), "multipart/form-data") {
		if err := r.ParseMultipartForm(25 << 20); err == nil {
			if b := findMultipartField(r, []string{"email", "raw", "body-mime", "message", "eml", "body"}); len(b) > 0 {
				return b, nil
			}
		}
	}
	return io.ReadAll(io.LimitReader(r.Body, 25<<20))
}
