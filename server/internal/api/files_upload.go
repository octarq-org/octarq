package api

import (
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/server/internal/models"
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

		// Check database for existing file record with matching MD5 within the caller's organization.
		// Cross-tenant deduplication based solely on client-provided MD5 is strictly prohibited to
		// prevent data exfiltration and existence oracle attacks (SEC-02).
		var existing models.File
		if err := h.db.Where("md5 = ? AND owner_id = ?", cleanMD5, orgID).First(&existing).Error; err == nil {
			// Found in caller's organization: instant dedup reuse within same tenant!
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

		// Check if client passed an MD5 form hint for fast dedup precheck within caller's organization
		formMD5 := strings.TrimSpace(strings.ToLower(r.FormValue("md5")))
		if formMD5 != "" && len(formMD5) == 32 {
			var existing models.File
			if err := h.db.Where("md5 = ? AND owner_id = ?", formMD5, orgID).First(&existing).Error; err == nil {
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
