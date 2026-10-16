package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
	"gorm.io/gorm"
)

// DownloadFileInput defines parameters for file download.
type DownloadFileInput struct {
	Ctx         huma.Context `hidden:"true"`
	ID          uint         `path:"id" example:"1" doc:"File ID"`
	Token       string       `query:"token" doc:"Temporary HMAC download token"`
	Disposition string       `query:"disposition" doc:"Content disposition (inline or attachment)"`
}

func (i *DownloadFileInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (h *Handler) downloadFile(ctx context.Context, input *DownloadFileInput) (*huma.StreamResponse, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)

	var authedOrgID uint
	hasValidToken := false
	idStr := strconv.FormatUint(uint64(input.ID), 10)

	// Check if HMAC token is provided in query (?token=...)
	if strings.TrimSpace(input.Token) != "" {
		if h.storage != nil && h.storage.VerifyDownloadToken(input.Token, idStr) {
			hasValidToken = true
		} else {
			return nil, huma.Error401Unauthorized("Download token is invalid or expired; request a new download token")
		}
	}

	if !hasValidToken {
		// No valid token provided: require session / bearer authentication
		r2, ok := h.auth.AuthenticateRequest(r)
		if !ok {
			return nil, huma.Error401Unauthorized("Authentication required; provide a valid session or download token")
		}
		authedOrgID = h.auth.OrgID(r2)
	}

	var file models.File
	if err := h.db.First(&file, input.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("File not found; verify the file identifier and try again")
		}
		return nil, huma.Error500InternalServerError("Could not load file information; retry the request")
	}

	// Verify org isolation when authenticated via session (not via signed token)
	if !hasValidToken && authedOrgID != 0 && file.OrgID != authedOrgID {
		return nil, huma.Error403Forbidden("Workspace permission denied; this file belongs to another workspace")
	}

	if h.storage == nil {
		return nil, huma.Error500InternalServerError("Storage service is currently unavailable; retry later")
	}

	rc, err := h.storage.GetObject(ctx, file.Path)
	if err != nil {
		if errors.Is(err, plugin.ErrStorageObjectNotFound) {
			return nil, huma.Error404NotFound("File content not found in storage; check if the file was deleted")
		}
		return nil, huma.Error500InternalServerError("Could not open file; retry the request")
	}

	ct := file.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}

	dispType := "inline"
	if strings.EqualFold(input.Disposition, "attachment") {
		dispType = "attachment"
	}

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			defer rc.Close()
			ctx.SetHeader("Content-Type", ct)
			ctx.SetHeader("Content-Length", strconv.FormatInt(file.Size, 10))
			ctx.SetHeader("Content-Disposition", fmt.Sprintf("%s; filename=%q", dispType, file.Name))
			w := ctx.BodyWriter()
			if _, err := io.Copy(w, rc); err != nil {
				slog.Warn("file download stream ended with error", "fileId", file.ID, "err", err)
			}
		},
	}, nil
}

// TempTokenRequest is the payload to generate a short-lived download token.
type TempTokenRequest struct {
	FileID        uint  `json:"file_id" example:"1" doc:"File ID"`
	ExpireSeconds int64 `json:"expire_seconds,omitempty" example:"900" doc:"Expiry duration in seconds (default: 900s / 15m)"`
}

// TempTokenInput wraps TempTokenRequest for Huma.
type TempTokenInput struct {
	Ctx  huma.Context `hidden:"true"`
	Body TempTokenRequest
}

func (i *TempTokenInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// TempTokenResponse contains the generated token and access URL.
type TempTokenResponse struct {
	Token       string `json:"token"`
	FileID      uint   `json:"file_id"`
	ExpiresAt   int64  `json:"expires_at"`
	DownloadURL string `json:"download_url"`
}

// TempTokenOutput wraps TempTokenResponse for Huma.
type TempTokenOutput struct {
	Body TempTokenResponse
}

func (h *Handler) createTempToken(ctx context.Context, input *TempTokenInput) (*TempTokenOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	orgID := h.auth.OrgID(r)

	if input.Body.FileID == 0 {
		return nil, huma.Error400BadRequest("File ID is required; provide a valid file identifier")
	}

	var file models.File
	if err := h.db.Where("id = ? AND owner_id = ?", input.Body.FileID, orgID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("File not found; verify the file identifier and try again")
		}
		return nil, huma.Error500InternalServerError("Could not load file information; retry the request")
	}

	if h.storage == nil {
		return nil, huma.Error500InternalServerError("Storage service is currently unavailable; retry later")
	}

	expire := 15 * time.Minute
	if input.Body.ExpireSeconds > 0 {
		expire = time.Duration(input.Body.ExpireSeconds) * time.Second
	}

	idStr := strconv.FormatUint(uint64(file.ID), 10)
	token := h.storage.GenerateDownloadToken(idStr, expire)
	expiresAt := time.Now().Add(expire).Unix()
	downloadURL := fmt.Sprintf("/api/files/%d/download?token=%s", file.ID, token)

	return &TempTokenOutput{
		Body: TempTokenResponse{
			Token:       token,
			FileID:      file.ID,
			ExpiresAt:   expiresAt,
			DownloadURL: downloadURL,
		},
	}, nil
}

// GetFileInput specifies the file ID for metadata lookup.
type GetFileInput struct {
	Ctx huma.Context `hidden:"true"`
	ID  uint         `path:"id" example:"1" doc:"File ID"`
}

func (i *GetFileInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// GetFileOutput wraps models.File for Huma.
type GetFileOutput struct {
	Body models.File
}

func (h *Handler) getFile(ctx context.Context, input *GetFileInput) (*GetFileOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	orgID := h.auth.OrgID(r)

	var file models.File
	if err := h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("File not found; verify the file identifier and try again")
		}
		return nil, huma.Error500InternalServerError("Could not load file information; retry the request")
	}

	return &GetFileOutput{Body: file}, nil
}
