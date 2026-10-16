package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/uuid"
	"github.com/octarq-org/octarq/server/internal/db"
)

type DownloadBackupInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *DownloadBackupInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

func (h *Handler) downloadBackup(ctx context.Context, input *DownloadBackupInput) (*huma.StreamResponse, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Request context is unavailable; retry the request")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("Authentication required; sign in to continue")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("Instance administrator role required; contact an instance administrator")
	}

	backupCfg := *h.cfg
	if backupCfg.DBDriver == "" {
		backupCfg.DBDriver = "sqlite"
	}
	if backupCfg.DBDSN == "" {
		backupCfg.DBDSN = "octarq.db"
	}

	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("tmp-%s-%s", uuid.NewString(), db.DefaultBackupFilename(backupCfg.DBDriver, time.Now())))
	if err := db.Backup(&backupCfg, tmpFile); err != nil {
		slog.Error("backup failed", "err", err)
		return nil, huma.Error500InternalServerError("Could not generate database backup; retry the request")
	}

	f, err := os.Open(tmpFile)
	if err != nil {
		if remErr := os.Remove(tmpFile); remErr != nil {
			slog.Warn("failed to remove tmp backup file", "file", tmpFile, "err", remErr)
		}
		return nil, huma.Error500InternalServerError("Could not read database backup; retry the request")
	}

	filename := db.DefaultBackupFilename(h.cfg.DBDriver, time.Now())

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			w := ctx.BodyWriter()
			ctx.SetHeader("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			ctx.SetHeader("Content-Type", "application/octet-stream")
			if _, copyErr := io.Copy(w, f); copyErr != nil {
				slog.Error("failed streaming backup file", "file", tmpFile, "err", copyErr)
			}
			f.Close()
			if remErr := os.Remove(tmpFile); remErr != nil {
				slog.Warn("failed to remove tmp backup file after streaming", "file", tmpFile, "err", remErr)
			}
		},
	}, nil
}
