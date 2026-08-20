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
	"github.com/octarq-org/octarq/internal/db"
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
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin role required")
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
		return nil, huma.Error500InternalServerError("backup failed")
	}

	f, err := os.Open(tmpFile)
	if err != nil {
		_ = os.Remove(tmpFile)
		return nil, huma.Error500InternalServerError("open backup file failed")
	}

	filename := db.DefaultBackupFilename(h.cfg.DBDriver, time.Now())

	return &huma.StreamResponse{
		Body: func(ctx huma.Context) {
			w := ctx.BodyWriter()
			ctx.SetHeader("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
			ctx.SetHeader("Content-Type", "application/octet-stream")
			_, _ = io.Copy(w, f)
			f.Close()
			_ = os.Remove(tmpFile)
		},
	}, nil
}
