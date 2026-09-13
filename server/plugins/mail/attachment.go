package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/emersion/go-message/mail"
	"github.com/octarq-org/octarq/plugin"
)

type RawEmailInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id"`
}

func (i *RawEmailInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) loadRawEmail(ctx context.Context, e *Email) []byte {
	if len(e.Raw) > 0 {
		return e.Raw
	}
	key := e.StorageKey
	if key == "" {
		var mb Mailbox
		_ = p.db.First(&mb, e.MailboxID).Error
		key = fmt.Sprintf("mail/%d/%d.eml", mb.OrgID, e.ID)
	}
	if storageProv, err := p.getStorageProvider(); err == nil {
		if data, getErr := storageProv.Get(ctx, key); getErr == nil {
			return data
		}
	}
	dbProv := NewDBStorageProvider(p.db)
	if data, getErr := dbProv.Get(ctx, key); getErr == nil {
		return data
	}
	return nil
}

// rawEmail streams the original RFC822 message as a downloadable .eml file.
func (p *Plugin) rawEmail(ctx context.Context, input *RawEmailInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	ctx = plugin.WithOrgID(ctx, p.orgID(r))
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}

	rawBytes := p.loadRawEmail(ctx, &e)

	w.Header().Set("Content-Type", "message/rfc822")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"email-%d.eml\"", e.ID))
	w.Write(rawBytes)
	return nil, nil
}

type GetAttachmentInput struct {
	Ctx   huma.Context `hidden:"true"`
	ID    uint         `path:"id"`
	Index int          `path:"index"`
}

func (i *GetAttachmentInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (p *Plugin) getAttachment(ctx context.Context, input *GetAttachmentInput) (*struct{}, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, w := humago.Unwrap(input.Ctx)
	if p.orgID(r) == 0 {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	ctx = plugin.WithOrgID(ctx, p.orgID(r))
	orgMailboxes := p.db.Model(&Mailbox{}).Select("id").Where("owner_id = ?", p.orgID(r))
	var e Email
	if p.db.Where("id = ? AND mailbox_id IN (?)", input.ID, orgMailboxes).First(&e).Error != nil {
		return nil, huma.Error404NotFound("not found")
	}
	var atts []struct {
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
		Size        int    `json:"size"`
	}
	if e.Attachments != "" {
		_ = json.Unmarshal([]byte(e.Attachments), &atts)
	}
	if input.Index < 0 || input.Index >= len(atts) {
		return nil, huma.Error404NotFound("attachment not found")
	}
	meta := atts[input.Index]
	rawBytes := p.loadRawEmail(ctx, &e)
	if len(rawBytes) > 0 {
		if data, fname, ctype, err := extractAttachment(rawBytes, input.Index); err == nil && data != nil {
			if fname != "" {
				meta.Filename = fname
			}
			if ctype != "" {
				meta.ContentType = ctype
			}
			if meta.ContentType == "" {
				meta.ContentType = "application/octet-stream"
			}
			w.Header().Set("Content-Type", meta.ContentType)
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", meta.Filename))
			w.Header().Set("Content-Length", fmt.Sprint(len(data)))
			_, _ = w.Write(data)
			return nil, nil
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":       "attachment content not available, download raw email instead",
		"attachment":  meta,
		"rawUrl":      fmt.Sprintf("/api/emails/%d/raw", e.ID),
		"contentType": meta.ContentType,
		"filename":    meta.Filename,
	})
	return nil, nil
}

func extractAttachment(raw []byte, wantIndex int) ([]byte, string, string, error) {
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	if err != nil {
		return nil, "", "", err
	}
	idx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		switch h := part.Header.(type) {
		case *mail.InlineHeader:
			ctype, _, _ := h.ContentType()
			if strings.HasPrefix(ctype, "text/plain") || strings.HasPrefix(ctype, "text/html") {
				continue
			}
			filename := ""
			if _, params, err := h.ContentType(); err == nil {
				filename = params["name"]
			}
			if filename == "" {
				if cd := h.Get("Content-Disposition"); cd != "" {
					for _, part2 := range strings.Split(cd, ";") {
						part2 = strings.TrimSpace(part2)
						if strings.HasPrefix(strings.ToLower(part2), "filename=") {
							filename = strings.Trim(part2[9:], "\"")
							break
						}
					}
				}
			}
			if filename == "" {
				continue
			}
			if idx == wantIndex {
				data, _ := io.ReadAll(part.Body)
				return data, filename, ctype, nil
			}
			idx++
		case *mail.AttachmentHeader:
			filename, _ := h.Filename()
			ctype, _, _ := h.ContentType()
			if idx == wantIndex {
				data, _ := io.ReadAll(part.Body)
				return data, filename, ctype, nil
			}
			idx++
		default:
			continue
		}
	}
	return nil, "", "", fmt.Errorf("attachment index out of range")
}
