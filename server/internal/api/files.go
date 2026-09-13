package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/internal/models"
	"github.com/octarq-org/octarq/plugin"
	"gorm.io/gorm"
)

// UploadFileInput handles file uploads via multipart/form-data or MD5 precheck via JSON.
type UploadFileInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *UploadFileInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

// UploadPrecheckRequest represents an MD5 precheck payload.
type UploadPrecheckRequest struct {
	MD5         string `json:"md5"`
	Name        string `json:"name,omitempty"`
	Size        int64  `json:"size,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

// UploadFileResponse is the response returned when a file is uploaded or deduplicated.
type UploadFileResponse struct {
	File         models.File `json:"file"`
	Deduplicated bool        `json:"deduplicated"`
	Message      string      `json:"message,omitempty"`
}

// UploadFileOutput wraps UploadFileResponse for Huma.
type UploadFileOutput struct {
	Body UploadFileResponse
}

func (h *Handler) uploadFile(ctx context.Context, input *UploadFileInput) (*UploadFileOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("missing context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgID := h.auth.OrgID(r)
	if orgID == 0 {
		return nil, huma.Error400BadRequest("invalid organization")
	}

	if h.storage == nil {
		return nil, huma.Error500InternalServerError("storage service unavailable")
	}

	contentType := r.Header.Get("Content-Type")

	// 1. Check for JSON MD5 precheck request
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var req UploadPrecheckRequest
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, huma.Error400BadRequest("failed to read json body")
		}
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			return nil, huma.Error400BadRequest("invalid json: " + err.Error())
		}
		cleanMD5 := strings.TrimSpace(strings.ToLower(req.MD5))
		if cleanMD5 == "" {
			return nil, huma.Error400BadRequest("md5 is required for precheck")
		}
		if len(cleanMD5) != 32 {
			return nil, huma.Error400BadRequest("invalid md5 hash length")
		}

		// Check database for existing file record with matching MD5
		var existing models.File
		if err := h.db.Where("md5 = ?", cleanMD5).First(&existing).Error; err == nil {
			// Found in database: instant dedup reuse!
			fileName := req.Name
			if fileName == "" {
				fileName = existing.Name
			}
			fileCT := req.ContentType
			if fileCT == "" {
				fileCT = existing.ContentType
			}
			newRecord := models.File{
				OrgID:       orgID,
				Name:        fileName,
				Size:        existing.Size,
				ContentType: fileCT,
				MD5:         cleanMD5,
				Path:        existing.Path,
			}
			if err := h.db.Create(&newRecord).Error; err != nil {
				return nil, huma.Error500InternalServerError("failed to save file record")
			}
			return &UploadFileOutput{
				Body: UploadFileResponse{
					File:         newRecord,
					Deduplicated: true,
					Message:      "File reused via MD5 precheck deduplication",
				},
			}, nil
		}

		// Also check if object exists on disk
		if existingKey, existingSize, exists := h.storage.CheckMD5Exists(cleanMD5, filepath.Ext(req.Name)); exists {
			fileName := req.Name
			if fileName == "" {
				fileName = "file"
			}
			newRecord := models.File{
				OrgID:       orgID,
				Name:        fileName,
				Size:        existingSize,
				ContentType: req.ContentType,
				MD5:         cleanMD5,
				Path:        existingKey,
			}
			if err := h.db.Create(&newRecord).Error; err != nil {
				return nil, huma.Error500InternalServerError("failed to save file record")
			}
			return &UploadFileOutput{
				Body: UploadFileResponse{
					File:         newRecord,
					Deduplicated: true,
					Message:      "File content found on disk and reused",
				},
			}, nil
		}

		return nil, huma.Error404NotFound("file not found for md5 precheck, full upload required")
	}

	// 2. Multipart form upload
	if strings.Contains(strings.ToLower(contentType), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, huma.Error400BadRequest("failed to parse multipart form: " + err.Error())
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}

		// Check if client passed an MD5 form hint for fast dedup precheck
		formMD5 := strings.TrimSpace(strings.ToLower(r.FormValue("md5")))
		if formMD5 != "" && len(formMD5) == 32 {
			var existing models.File
			if err := h.db.Where("md5 = ?", formMD5).First(&existing).Error; err == nil {
				fileName := strings.TrimSpace(r.FormValue("name"))
				if fileName == "" {
					fileName = existing.Name
				}
				newRecord := models.File{
					OrgID:       orgID,
					Name:        fileName,
					Size:        existing.Size,
					ContentType: existing.ContentType,
					MD5:         formMD5,
					Path:        existing.Path,
				}
				if err := h.db.Create(&newRecord).Error; err != nil {
					return nil, huma.Error500InternalServerError("failed to save file record")
				}
				return &UploadFileOutput{
					Body: UploadFileResponse{
						File:         newRecord,
						Deduplicated: true,
						Message:      "File reused via MD5 form hint deduplication",
					},
				}, nil
			}
		}

		// Retrieve uploaded file
		file, header, err := r.FormFile("file")
		if err != nil {
			// If no file but md5 was provided and missed
			if formMD5 != "" {
				return nil, huma.Error404NotFound("file not found for md5 precheck, full upload required")
			}
			return nil, huma.Error400BadRequest("missing file in form-data: " + err.Error())
		}
		defer file.Close()

		fileName := strings.TrimSpace(r.FormValue("name"))
		if fileName == "" {
			fileName = header.Filename
		}
		if fileName == "" {
			fileName = "uploaded_file"
		}
		ext := filepath.Ext(fileName)
		fileCT := header.Header.Get("Content-Type")
		if fileCT == "" {
			fileCT = "application/octet-stream"
		}

		// PutObjectByHash computes MD5 and atomically stores or dedups the file
		key, md5Hash, sizeWritten, deduplicated, err := h.storage.PutObjectByHash(ctx, file, ext)
		if err != nil {
			return nil, huma.Error500InternalServerError("failed to store file: " + err.Error())
		}

		newRecord := models.File{
			OrgID:       orgID,
			Name:        fileName,
			Size:        sizeWritten,
			ContentType: fileCT,
			MD5:         md5Hash,
			Path:        key,
		}
		if err := h.db.Create(&newRecord).Error; err != nil {
			return nil, huma.Error500InternalServerError("failed to save file record")
		}

		msg := "File uploaded successfully"
		if deduplicated {
			msg = "File uploaded and deduplicated via existing MD5 hash"
		}
		return &UploadFileOutput{
			Body: UploadFileResponse{
				File:         newRecord,
				Deduplicated: deduplicated,
				Message:      msg,
			},
		}, nil
	}

	// 3. Raw stream upload
	fileName := r.Header.Get("X-File-Name")
	if fileName == "" {
		fileName = "raw_upload"
	}
	ext := filepath.Ext(fileName)
	rawCT := contentType
	if rawCT == "" {
		rawCT = "application/octet-stream"
	}

	key, md5Hash, sizeWritten, deduplicated, err := h.storage.PutObjectByHash(ctx, r.Body, ext)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to store raw file: " + err.Error())
	}

	newRecord := models.File{
		OrgID:       orgID,
		Name:        fileName,
		Size:        sizeWritten,
		ContentType: rawCT,
		MD5:         md5Hash,
		Path:        key,
	}
	if err := h.db.Create(&newRecord).Error; err != nil {
		return nil, huma.Error500InternalServerError("failed to save file record")
	}

	return &UploadFileOutput{
		Body: UploadFileResponse{
			File:         newRecord,
			Deduplicated: deduplicated,
			Message:      "Raw file stored successfully",
		},
	}, nil
}

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
		return nil, huma.Error500InternalServerError("missing context")
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
			return nil, huma.Error401Unauthorized("invalid or expired download token")
		}
	}

	if !hasValidToken {
		// No valid token provided: require session / bearer authentication
		r2, ok := h.auth.AuthenticateRequest(r)
		if !ok {
			return nil, huma.Error401Unauthorized("unauthorized: valid session or download token required")
		}
		authedOrgID = h.auth.OrgID(r2)
	}

	var file models.File
	if err := h.db.First(&file, input.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("file not found")
		}
		return nil, huma.Error500InternalServerError("database error")
	}

	// Verify org isolation when authenticated via session (not via signed token)
	if !hasValidToken && authedOrgID != 0 && file.OrgID != authedOrgID {
		return nil, huma.Error403Forbidden("access denied to file from another organization")
	}

	if h.storage == nil {
		return nil, huma.Error500InternalServerError("storage service unavailable")
	}

	rc, err := h.storage.GetObject(ctx, file.Path)
	if err != nil {
		if errors.Is(err, plugin.ErrStorageObjectNotFound) {
			return nil, huma.Error404NotFound("file object not found in storage")
		}
		return nil, huma.Error500InternalServerError("failed to open file object")
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
			_, _ = io.Copy(w, rc)
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
		return nil, huma.Error500InternalServerError("missing context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgID := h.auth.OrgID(r)

	if input.Body.FileID == 0 {
		return nil, huma.Error400BadRequest("file_id is required")
	}

	var file models.File
	if err := h.db.Where("id = ? AND owner_id = ?", input.Body.FileID, orgID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("file not found")
		}
		return nil, huma.Error500InternalServerError("database error")
	}

	if h.storage == nil {
		return nil, huma.Error500InternalServerError("storage service unavailable")
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
		return nil, huma.Error500InternalServerError("missing context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	orgID := h.auth.OrgID(r)

	var file models.File
	if err := h.db.Where("id = ? AND owner_id = ?", input.ID, orgID).First(&file).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("file not found")
		}
		return nil, huma.Error500InternalServerError("database error")
	}

	return &GetFileOutput{Body: file}, nil
}
