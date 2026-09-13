package api

// Instance-scope plugin menus — the backend half of plugin.InstanceMenuProvider.
// Own file by design: tenant menus live in tenant_menu.go, instance menus here;
// the file name is the scope boundary, and the two endpoints must never merge.

import (
	"context"
	"sort"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/octarq-org/octarq/plugin"
)

type ListInstanceMenusInput struct {
	Ctx huma.Context `hidden:"true"`
}

func (i *ListInstanceMenusInput) Resolve(ctx huma.Context) []error {
	i.Ctx = ctx
	return nil
}

type ListInstanceMenusOutput struct {
	Body []MenuItem
}

// listInstanceMenus aggregates the InstanceMenus() of every plugin that
// implements plugin.InstanceMenuProvider.
// GET /api/instance/menus — instance-admin only, mirroring listInstancePlugins:
// 401 before the admin check, 403 for a logged-in non-admin. There is NO
// per-workspace toggle here (tenant_menu.go's filter has no meaning: instance
// config belongs to no workspace, and a disabled workspace must not hide a
// deployment-wide page). Order asc, then ID asc, so the console rail is stable.
func (h *Handler) listInstanceMenus(ctx context.Context, input *ListInstanceMenusInput) (*ListInstanceMenusOutput, error) {
	if input.Ctx == nil {
		return nil, huma.Error500InternalServerError("Missing huma context")
	}
	r, _ := humago.Unwrap(input.Ctx)
	r, ok := h.auth.AuthenticateRequest(r)
	if !ok {
		return nil, huma.Error401Unauthorized("unauthorized")
	}
	if !h.isInstanceAdmin(r) {
		return nil, huma.Error403Forbidden("instance admin required")
	}

	var menus []MenuItem
	for _, p := range h.plugins {
		mp, ok := p.(plugin.InstanceMenuProvider)
		if !ok {
			continue
		}
		for _, m := range mp.InstanceMenus() {
			menus = append(menus, MenuItem{
				ID:       m.ID,
				Label:    m.Label,
				Path:     m.Path,
				Icon:     m.Icon,
				Category: m.Category,
				Order:    m.Order,
			})
		}
	}
	sort.SliceStable(menus, func(i, j int) bool {
		if menus[i].Order != menus[j].Order {
			return menus[i].Order < menus[j].Order
		}
		return menus[i].ID < menus[j].ID
	})

	return &ListInstanceMenusOutput{Body: menus}, nil
}
