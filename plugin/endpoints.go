package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// HandlerFunc is the unified business handler signature for dual endpoints.
// The framework automatically handles input deserialization, validation,
// and output/error wrapping across both HTTP and MCP transports.
type HandlerFunc[In any, Out any] func(ctx context.Context, input In) (*Out, error)

// EndpointSpec declaratively defines an endpoint.
// The host automatically mounts it as a Huma REST API and/or an MCP AI Agent tool.
type EndpointSpec[In any, Out any] struct {
	// Base metadata
	Name        string // Unique identifier, e.g. "create_link"
	Summary     string // Short summary
	Description string // Detailed description for OpenAPI and MCP Tool

	// HTTP routing
	Method string // "GET", "POST", "PUT", "DELETE", "PATCH"
	Path   string // e.g. "/api/links", "/api/links/{id}"

	// Auth & Access Control
	RequireAuth bool     // Whether authentication is required (default true)
	RequireRole []string // Required workspace roles (e.g. ["owner", "admin"])

	// Risk & Governance
	RiskLevel       string // "read" | "write" | "destructive"; empty = inferred from Method
	RequireApproval bool   // HITL: manual approval required before execution (destructive must be true)

	// Agent-Native options
	ExposeMCP bool // Whether to expose as MCP Tool (default true)

	// Core business handler
	Handler HandlerFunc[In, Out]
}

// Risk level constants.
const (
	RiskLevelRead        = "read"
	RiskLevelWrite       = "write"
	RiskLevelDestructive = "destructive"
)

// DefaultRiskLevel returns the inferred risk level based on the HTTP method.
// GET -> "read", POST/PUT/PATCH/DELETE -> "write".
func DefaultRiskLevel(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "GET", "HEAD":
		return RiskLevelRead
	case "POST", "PUT", "PATCH", "DELETE", "":
		return RiskLevelWrite
	default:
		return RiskLevelWrite
	}
}

// Endpoint is the untyped interface implemented by EndpointSpec[In, Out].
type Endpoint interface {
	EndpointName() string
	EndpointSummary() string
	EndpointDescription() string
	EndpointMethod() string
	EndpointPath() string
	EndpointRequireAuth() bool
	EndpointRequireRole() []string
	EndpointExposeMCP() bool
	EndpointRequireApproval() bool
	Execute(ctx context.Context, input any) (any, error)
	ExecuteAgentJSON(ctx context.Context, argsJSON string) (any, error)
	Spec() any
}

// HTTPOptions provides authentication and role checking hooks for HTTP endpoint execution.
type HTTPOptions struct {
	RequireAuth func(ctx context.Context) (uint, error)
	RequireRole func(ctx context.Context, roles []string) error
}

// HTTPRegistrar enables automatic registration to a Huma API instance.
type HTTPRegistrar interface {
	RegisterHTTP(api huma.API, opts HTTPOptions) error
}

// MCPRegistrar enables automatic registration to an MCP server instance.
type MCPRegistrar interface {
	RegisterMCP(srv *mcp.Server) error
}

func (s EndpointSpec[In, Out]) EndpointName() string        { return s.Name }
func (s EndpointSpec[In, Out]) EndpointSummary() string     { return s.Summary }
func (s EndpointSpec[In, Out]) EndpointDescription() string { return s.Description }
func (s EndpointSpec[In, Out]) EndpointMethod() string {
	if s.Method == "" {
		return "POST"
	}
	return s.Method
}
func (s EndpointSpec[In, Out]) EndpointPath() string          { return s.Path }
func (s EndpointSpec[In, Out]) EndpointRequireAuth() bool     { return s.RequireAuth }
func (s EndpointSpec[In, Out]) EndpointRequireRole() []string { return s.RequireRole }
func (s EndpointSpec[In, Out]) EndpointExposeMCP() bool       { return s.ExposeMCP }
func (s EndpointSpec[In, Out]) EndpointRequireApproval() bool { return s.RequireApproval }
func (s EndpointSpec[In, Out]) Spec() any                     { return s }

// EffectiveRisk returns the effective risk level of the endpoint.
// Explicit valid RiskLevel takes precedence; otherwise it is inferred from Method.
// If an invalid explicit RiskLevel is set, it returns an empty string.
func (s EndpointSpec[In, Out]) EffectiveRisk() string {
	if s.RiskLevel != "" {
		switch s.RiskLevel {
		case RiskLevelRead, RiskLevelWrite, RiskLevelDestructive:
			return s.RiskLevel
		default:
			return ""
		}
	}
	return DefaultRiskLevel(s.EndpointMethod())
}

// Validate verifies the declarative contract for the endpoint.
// It checks that Name is non-empty and Path starts with "/api".
func (s EndpointSpec[In, Out]) Validate() error {
	if s.Name == "" {
		return errors.New("endpoint name cannot be empty")
	}
	if !strings.HasPrefix(s.Path, "/api") {
		return errors.New("endpoint path must start with /api")
	}
	return nil
}

func (s EndpointSpec[In, Out]) Execute(ctx context.Context, input any) (any, error) {
	if s.Handler == nil {
		return nil, fmt.Errorf("no handler configured for endpoint %q", s.Name)
	}
	if input == nil {
		var zero In
		return s.Handler(ctx, zero)
	}
	typed, ok := input.(In)
	if !ok {
		if ptr, isPtr := input.(*In); isPtr && ptr != nil {
			return s.Handler(ctx, *ptr)
		}
		return nil, fmt.Errorf("endpoint %q expected input type %T, got %T", s.Name, *new(In), input)
	}
	return s.Handler(ctx, typed)
}

// ExecuteAgentJSON bridges raw JSON strings to the typed Handler, allowing
// callers (e.g. ToolExecutor) that only have a JSON blob to invoke the
// endpoint without knowing the concrete In type.
//
// P3 前 API 易变.
func (s EndpointSpec[In, Out]) ExecuteAgentJSON(ctx context.Context, argsJSON string) (any, error) {
	if s.Handler == nil {
		return nil, fmt.Errorf("no handler configured for endpoint %q", s.Name)
	}
	var in In
	if argsJSON != "" && argsJSON != "null" {
		if err := json.Unmarshal([]byte(argsJSON), &in); err != nil {
			return nil, NewAgentError(
				400,
				"INVALID_TOOL_ARGS",
				err.Error(),
				"修正参数后重试",
				false,
			)
		}
	}
	return s.Handler(ctx, in)
}

type httpBodyInput[T any] struct {
	Body T
}

type httpBodyOutput[T any] struct {
	Body T
}

func (s EndpointSpec[In, Out]) RegisterHTTP(api huma.API, opts HTTPOptions) error {
	if api == nil {
		return nil
	}
	if err := s.Validate(); err != nil {
		return err
	}
	op := huma.Operation{
		Method:      s.EndpointMethod(),
		Path:        s.EndpointPath(),
		Summary:     s.EndpointSummary(),
		Description: s.EndpointDescription(),
	}
	if s.Method == "POST" || s.Method == "PUT" || s.Method == "PATCH" {
		huma.Register(api, op, func(ctx context.Context, input *httpBodyInput[In]) (*httpBodyOutput[Out], error) {
			if s.RequireAuth && opts.RequireAuth != nil {
				if _, err := opts.RequireAuth(ctx); err != nil {
					return nil, err
				}
			}
			if len(s.RequireRole) > 0 && opts.RequireRole != nil {
				if err := opts.RequireRole(ctx, s.RequireRole); err != nil {
					return nil, err
				}
			}
			var in In
			if input != nil {
				in = input.Body
			}
			out, err := s.Handler(ctx, in)
			if err != nil {
				if ae, ok := AsAgentError(err); ok {
					return nil, huma.NewError(ae.HTTPCode, ae.Message)
				}
				return nil, err
			}
			if out == nil {
				var zero Out
				return &httpBodyOutput[Out]{Body: zero}, nil
			}
			return &httpBodyOutput[Out]{Body: *out}, nil
		})
	} else {
		huma.Register(api, op, func(ctx context.Context, input *In) (*httpBodyOutput[Out], error) {
			if s.RequireAuth && opts.RequireAuth != nil {
				if _, err := opts.RequireAuth(ctx); err != nil {
					return nil, err
				}
			}
			if len(s.RequireRole) > 0 && opts.RequireRole != nil {
				if err := opts.RequireRole(ctx, s.RequireRole); err != nil {
					return nil, err
				}
			}
			var in In
			if input != nil {
				in = *input
			}
			out, err := s.Handler(ctx, in)
			if err != nil {
				if ae, ok := AsAgentError(err); ok {
					return nil, huma.NewError(ae.HTTPCode, ae.Message)
				}
				return nil, err
			}
			if out == nil {
				var zero Out
				return &httpBodyOutput[Out]{Body: zero}, nil
			}
			return &httpBodyOutput[Out]{Body: *out}, nil
		})
	}
	return nil
}

func (s EndpointSpec[In, Out]) RegisterMCP(srv *mcp.Server) error {
	if srv == nil || !s.EndpointExposeMCP() {
		return nil
	}
	desc := s.EndpointDescription()
	if desc == "" {
		desc = s.EndpointSummary()
	}
	riskSuffix := fmt.Sprintf("[risk: %s]", s.EffectiveRisk())
	if s.RequireApproval {
		riskSuffix = fmt.Sprintf("[risk: %s, approval required]", s.EffectiveRisk())
	}
	if desc != "" {
		desc = desc + " " + riskSuffix
	} else {
		desc = riskSuffix
	}
	mcp.AddTool(srv, &mcp.Tool{
		Name:        s.EndpointName(),
		Description: desc,
	}, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		if s.RequireAuth {
			if OrgIDFromContext(ctx) == 0 {
				return nil, nil, NewAgentError(
					401,
					"UNAUTHORIZED_NO_ORG",
					"no workspace in this request",
					"Every tool execution requires a valid workspace/tenant context. Ensure an API token or session is provided.",
					false,
				)
			}
		}
		out, err := s.Handler(ctx, in)
		if err != nil {
			return nil, nil, err
		}
		buf, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(buf)}},
		}, out, nil
	})
	return nil
}

// RegisterEndpoint is a type-safe generic helper to register an EndpointSpec onto a Context.
func RegisterEndpoint[In any, Out any](ctx *Context, spec EndpointSpec[In, Out]) error {
	if spec.RiskLevel != "" {
		if spec.RiskLevel != RiskLevelRead && spec.RiskLevel != RiskLevelWrite && spec.RiskLevel != RiskLevelDestructive {
			return fmt.Errorf("invalid RiskLevel %q: must be %q, %q, or %q", spec.RiskLevel, RiskLevelRead, RiskLevelWrite, RiskLevelDestructive)
		}
	}
	if spec.EffectiveRisk() == RiskLevelDestructive && !spec.RequireApproval {
		return fmt.Errorf("destructive endpoint %q must have RequireApproval set to true", spec.Name)
	}
	if ctx == nil || ctx.RegisterEndpoint == nil {
		return nil // Backward compatibility on hosts that predate it
	}
	return ctx.RegisterEndpoint(spec)
}
