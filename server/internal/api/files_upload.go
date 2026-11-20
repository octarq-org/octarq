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

type UploadFileInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *UploadFileInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type UploadPrecheckRequest struct {
	MD5         string `json:"md5"`
	Name        string `json:"name,omitempty"`
	Size        int64  `json:"size,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

type UploadFileResponse struct {
	File         models.File `json:"file"`
	Deduplicated bool        `json:"deduplicated"`
	Message      string      `json:"message,omitempty"`
}

type UploadFileOutput struct {
	Body UploadFileResponse
}

func (h *Handler) uploadFile(ctx context.Context, input *UploadFileInput) (*UploadFileOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	orgID := h.auth.OrgID(r)
	if orgID == 0 {
		return nil, huma.Error400BadRequest("Workspace context is invalid; select a workspace and try again")
	}

	if h.storage == nil {
		return nil, huma.Error500InternalServerError("Storage service is currently unavailable; retry later")
	}

	contentType := r.Header.Get("Content-Type")

	if strings.Contains(strings.ToLower(contentType), "application/json") {
		var req UploadPrecheckRequest
		bodyBytes, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, huma.Error400BadRequest("Could not read request body; retry the request")
		}
		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			return nil, huma.Error400BadRequest("Malformed JSON body; check request payload formatting")
		}
		cleanMD5 := strings.TrimSpace(strings.ToLower(req.MD5))
		if cleanMD5 == "" {
			return nil, huma.Error400BadRequest("MD5 checksum is required for upload precheck; provide a 32-character hex string")
		}
		if len(cleanMD5) != 32 {
			return nil, huma.Error400BadRequest("Invalid MD5 checksum length; expected a 32-character hex string")
		}

		var existing models.File
		if err := h.db.Where("md5 = ? AND owner_id = ?", cleanMD5, orgID).First(&existing).Error; err == nil {
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
				return nil, huma.Error500InternalServerError("Could not save file record; retry the request")
			}
			return &UploadFileOutput{
				Body: UploadFileResponse{
					File:         newRecord,
					Deduplicated: true,
					Message:      "File reused via MD5 precheck deduplication",
				},
			}, nil
		}

		return nil, huma.Error404NotFound("File hash not found in workspace; upload the complete file")
	}

	if strings.Contains(strings.ToLower(contentType), "multipart/form-data") {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			return nil, huma.Error400BadRequest("Could not parse multipart form; check upload format and try again")
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}

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
					return nil, huma.Error500InternalServerError("Could not save file record; retry the request")
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

		file, header, err := r.FormFile("file")
		if err != nil {
			if formMD5 != "" {
				return nil, huma.Error404NotFound("File hash not found in workspace; upload the complete file")
			}
			return nil, huma.Error400BadRequest("File is missing from upload; select a file and try again")
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

		key, md5Hash, sizeWritten, deduplicated, err := h.storage.PutObjectByHash(ctx, file, ext)
		if err != nil {
			return nil, huma.Error500InternalServerError("Could not save file content; retry the upload")
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
			return nil, huma.Error500InternalServerError("Could not save file record; retry the request")
		}

		msg := "File uploaded"
		if deduplicated {
			msg = "File uploaded and deduplicated by MD5 hash"
		}
		return &UploadFileOutput{
			Body: UploadFileResponse{
				File:         newRecord,
				Deduplicated: deduplicated,
				Message:      msg,
			},
		}, nil
	}

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
		return nil, huma.Error500InternalServerError("Could not save file content; retry the upload")
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
		return nil, huma.Error500InternalServerError("Could not save file record; retry the request")
	}

	return &UploadFileOutput{
		Body: UploadFileResponse{
			File:         newRecord,
			Deduplicated: deduplicated,
			Message:      "Raw file stored",
		},
	}, nil
}
