package ontology

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	core "github.com/octarq-org/octarq/server/internal/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

// MCPTier is the tier the whole plugin needs.
//
// MCPProvider declares one tier per plugin while a declaration declares one per
// operation, so the highest declared tier wins: hiding the free operations of an
// entity that also has a paid one is a coarser answer than licensing would
// allow, but it is the only one that cannot leak a paid operation to a free
// instance.
func (p *Plugin) MCPTier() string {
	tier := ""
	for _, e := range p.entities {
		for _, op := range core.Operations {
			acl := e.acl(op)
			if !acl.ExposeMCP || acl.Tier == "" {
				continue
			}
			if tierRank(acl.Tier) > tierRank(tier) {
				tier = acl.Tier
			}
		}
	}
	return tier
}

// tierRank orders the self-hosted tiers. An unknown tier outranks every known
// one, so a typo hides the group instead of exposing it.
func tierRank(tier string) int {
	switch tier {
	case "":
		return 0
	case "pro":
		return 1
	case "elite":
		return 2
	default:
		return 3
	}
}

// RegisterMCP publishes one tool per MCP-exposed operation.
//
// It runs once per MCP connection, from several goroutines, so it only reads the
// snapshot built at Mount: no DDL, no shared state, no lazy lookups.
func (p *Plugin) RegisterMCP(srv *mcp.Server) {
	for _, e := range p.entities {
		for _, op := range core.Operations {
			if !e.acl(op).ExposeMCP {
				continue
			}
			p.registerTool(srv, e, op)
		}
	}
}

// toolArgs is the argument object for one tool. The schema published with the
// tool is the declaration's; this type only carries the values.
type toolArgs map[string]any

// listResult is the list tool's structured output.
type listResult struct {
	Items  []map[string]any `json:"items"`
	Total  int64            `json:"total"`
	Limit  int              `json:"limit"`
	Offset int              `json:"offset"`
}

// deleteResult is the delete tool's structured output.
type deleteResult struct {
	OK bool `json:"ok"`
}

func (p *Plugin) registerTool(srv *mcp.Server, e *entity, op core.Operation) {
	tool := &mcp.Tool{
		Name:        mcpToolName(e.spec.Slug, op),
		Description: toolDescription(e, op),
		InputSchema: inputSchema(e, op),
	}
	owner := "ontology:" + e.spec.Slug

	switch op {
	case core.OpList:
		plugin.AddMCPTool(srv, owner, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in toolArgs) (*mcp.CallToolResult, listResult, error) {
			res, err := p.mcpList(ctx, e, in)
			if err != nil {
				return nil, listResult{}, agentError(err)
			}
			result, _, err := jsonResult(res)
			return result, res, err
		})
	case core.OpGet:
		plugin.AddMCPTool(srv, owner, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in toolArgs) (*mcp.CallToolResult, map[string]any, error) {
			row, err := p.mcpGet(ctx, e, in)
			if err != nil {
				return nil, nil, agentError(err)
			}
			result, _, err := jsonResult(row)
			return result, row, err
		})
	case core.OpCreate:
		plugin.AddMCPTool(srv, owner, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in toolArgs) (*mcp.CallToolResult, map[string]any, error) {
			row, err := p.mcpWrite(ctx, e, in, false)
			if err != nil {
				return nil, nil, agentError(err)
			}
			result, _, err := jsonResult(row)
			return result, row, err
		})
	case core.OpUpdate:
		plugin.AddMCPTool(srv, owner, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in toolArgs) (*mcp.CallToolResult, map[string]any, error) {
			row, err := p.mcpWrite(ctx, e, in, true)
			if err != nil {
				return nil, nil, agentError(err)
			}
			result, _, err := jsonResult(row)
			return result, row, err
		})
	case core.OpDelete:
		plugin.AddMCPTool(srv, owner, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in toolArgs) (*mcp.CallToolResult, deleteResult, error) {
			if err := p.mcpDelete(ctx, e, in); err != nil {
				return nil, deleteResult{}, agentError(err)
			}
			res := deleteResult{OK: true}
			result, _, err := jsonResult(res)
			return result, res, err
		})
	}
}

func (p *Plugin) mcpList(ctx context.Context, e *entity, in toolArgs) (listResult, error) {
	if err := p.ready(ctx, e, core.OpList); err != nil {
		return listResult{}, err
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return listResult{}, err
	}
	limit, offset, err := pageParams(in)
	if err != nil {
		return listResult{}, err
	}
	sort, err := parseSort(e, argString(in, "sort"))
	if err != nil {
		return listResult{}, err
	}
	rows, total, err := store{}.list(ctx, tdb, e, listParams{limit: limit, offset: offset, sort: sort})
	if err != nil {
		return listResult{}, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, core.FromRow(e.spec, row))
	}
	return listResult{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (p *Plugin) mcpGet(ctx context.Context, e *entity, in toolArgs) (map[string]any, error) {
	if err := p.ready(ctx, e, core.OpGet); err != nil {
		return nil, err
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	row, err := store{}.get(ctx, tdb, e, argString(in, "id"))
	if err != nil {
		return nil, err
	}
	return core.FromRow(e.spec, row), nil
}

// mcpWrite creates or updates one record. The arguments are re-encoded and run
// through the same decoder the HTTP surface uses, so both surfaces accept and
// refuse exactly the same values.
func (p *Plugin) mcpWrite(ctx context.Context, e *entity, in toolArgs, patch bool) (map[string]any, error) {
	op := core.OpCreate
	if patch {
		op = core.OpUpdate
	}
	if err := p.ready(ctx, e, op); err != nil {
		return nil, err
	}
	args := make(map[string]any, len(in))
	for k, v := range in {
		args[k] = v
	}
	id := ""
	if patch {
		id = argString(args, "id")
		delete(args, core.ColumnID)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("ontology: encode tool arguments: %w", err)
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return nil, err
	}
	if patch {
		values, err := core.DecodePatch(e.spec, raw)
		if err != nil {
			return nil, err
		}
		row, err := store{}.update(ctx, tdb, e, id, values)
		if err != nil {
			return nil, err
		}
		return core.FromRow(e.spec, row), nil
	}
	values, err := core.DecodeValues(e.spec, raw)
	if err != nil {
		return nil, err
	}
	row, err := store{}.create(ctx, tdb, e, values)
	if err != nil {
		return nil, err
	}
	return core.FromRow(e.spec, row), nil
}

func (p *Plugin) mcpDelete(ctx context.Context, e *entity, in toolArgs) error {
	if err := p.ready(ctx, e, core.OpDelete); err != nil {
		return err
	}
	tdb, err := p.tenantDB(ctx)
	if err != nil {
		return err
	}
	return (store{}).delete(ctx, tdb, e, argString(in, "id"))
}

// ready refuses a tool call the instance cannot serve: no database, schema never
// applied, or a role outside the operation's ACL. Each answer is a refusal, not
// an empty result.
func (p *Plugin) ready(ctx context.Context, e *entity, op core.Operation) error {
	if p.schemaErr != nil {
		return p.schemaErr
	}
	if p.db == nil {
		return ErrNoDatabase
	}
	// The stdio composition stamps a workspace but no identity, so a tool called
	// over `octarq mcp` has no role to compare. Saying so is the difference
	// between "your token is too weak" and "this transport cannot carry a token".
	role := plugin.RoleFromContext(ctx)
	if role == "" {
		return ErrNoRole
	}
	if !aclAllows(e.acl(op).Roles, role) {
		return ErrForbidden
	}
	return nil
}

// toolDescription tells an agent what the tool does and what it must send. One
// sentence per line, active voice.
func toolDescription(e *entity, op core.Operation) string {
	plural := pluralLabel(e.spec)
	switch op {
	case core.OpList:
		return fmt.Sprintf("List %s in this workspace. Use limit, offset and sort.", plural)
	case core.OpGet:
		return fmt.Sprintf("Read one %s by id.", e.spec.Label)
	case core.OpCreate:
		return fmt.Sprintf("Create a %s. Send the declared fields only.", e.spec.Label)
	case core.OpUpdate:
		return fmt.Sprintf("Update fields on one %s by id. Send only the fields to change.", e.spec.Label)
	default:
		return fmt.Sprintf("Delete one %s by id.", e.spec.Label)
	}
}

func argString(in toolArgs, key string) string {
	if v, ok := in[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// pageParams reads the paging arguments. The list tool publishes the same bounds
// this enforces, and the HTTP surface declares them to huma, so a page outside
// them is refused everywhere rather than served as a different page on one
// surface.
func pageParams(in toolArgs) (limit, offset int, err error) {
	limit = defaultPageSize
	if _, ok := in["limit"]; ok {
		if limit, err = argInt(in, "limit"); err != nil {
			return 0, 0, err
		}
	}
	if limit < 1 || limit > maxPageSize {
		return 0, 0, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidArgument, maxPageSize)
	}
	if _, ok := in["offset"]; ok {
		if offset, err = argInt(in, "offset"); err != nil {
			return 0, 0, err
		}
	}
	if offset < 0 {
		return 0, 0, fmt.Errorf("%w: offset must not be negative", ErrInvalidArgument)
	}
	return limit, offset, nil
}

// argInt reads an integer argument. A value of another type is refused: reading
// it as the default would run a request the caller did not make.
func argInt(in toolArgs, key string) (int, error) {
	v, ok := in[key]
	if !ok {
		return 0, fmt.Errorf("%w: %s is missing", ErrInvalidArgument, key)
	}
	switch n := v.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, fmt.Errorf("%w: %s must be a whole number", ErrInvalidArgument, key)
		}
		// The MCP SDK decodes arguments as float64. Beyond 2^53 a float64 can no
		// longer tell two integers apart, so the value that arrives is already
		// not the value the caller sent.
		if math.Abs(n) > 1<<53 {
			return 0, fmt.Errorf("%w: %s is out of range", ErrInvalidArgument, key)
		}
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("%w: %s must be a whole number", ErrInvalidArgument, key)
		}
		return int(i), nil
	}
	return 0, fmt.Errorf("%w: %s must be an integer", ErrInvalidArgument, key)
}

// jsonResult renders a value as both text and structured output, so a client can
// read either.
func jsonResult(v any) (*mcp.CallToolResult, any, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}}}, v, nil
}
