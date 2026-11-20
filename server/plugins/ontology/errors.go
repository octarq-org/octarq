package ontology

import (
	"errors"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	core "github.com/octarq-org/octarq/server/ontology"
	"github.com/octarq-org/octarq/server/plugin"
)

var ErrForbidden = errors.New("ontology: insufficient role for this operation")

var ErrFeatureOff = errors.New("ontology: the plugin that declares this entity is off in this workspace")

var ErrInvalidArgument = errors.New("ontology: invalid argument")

type failure struct {
	status     int
	code       string
	message    string
	suggestion string
}

func (f *failure) Error() string { return f.message }

func asFailure(err error) *failure {
	if err == nil {
		return nil
	}
	var f *failure
	if errors.As(err, &f) {
		return f
	}

	var issues core.Errors
	if errors.As(err, &issues) {
		return &failure{
			status:     422,
			code:       "INVALID_VALUES",
			message:    issues.Error(),
			suggestion: "Correct the listed fields and send the request again.",
		}
	}

	switch {
	case errors.Is(err, ErrNotFound):
		return &failure{status: 404, code: "NOT_FOUND", message: "Record not found."}
	case errors.Is(err, ErrFeatureOff):
		return &failure{
			status:  404,
			code:    "FEATURE_OFF",
			message: "This feature is not enabled in this workspace.",
		}
	case errors.Is(err, ErrNoTenantScope):
		return &failure{status: 401, code: "UNAUTHENTICATED", message: "Sign in to continue."}
	case errors.Is(err, ErrNoDatabase):
		return &failure{
			status:     503,
			code:       "DATABASE_UNAVAILABLE",
			message:    ErrNoDatabase.Error(),
			suggestion: "Start the server with a database so it can apply the entity declarations.",
		}
	case errors.Is(err, ErrNoRole):
		return &failure{
			status:     403,
			code:       "FORBIDDEN",
			message:    ErrNoRole.Error(),
			suggestion: "Connect to the HTTP MCP endpoint with an API token whose role allows this operation.",
		}
	case errors.Is(err, ErrNoUserScope):
		return &failure{
			status:     403,
			code:       "FORBIDDEN",
			message:    ErrNoUserScope.Error(),
			suggestion: "This entity keeps one record set per member. Call it over HTTP with a session or an API token.",
		}
	case errors.Is(err, ErrForbidden):
		return &failure{
			status:     403,
			code:       "FORBIDDEN",
			message:    "Your role does not allow this operation.",
			suggestion: "Ask a workspace admin to grant access.",
		}
	case errors.Is(err, ErrUnknownSort):
		return &failure{
			status:  422,
			code:    "UNKNOWN_SORT",
			message: err.Error(),
		}
	case errors.Is(err, ErrInvalidArgument):
		return &failure{
			status:  422,
			code:    "INVALID_ARGUMENT",
			message: err.Error(),
		}
	case errors.Is(err, core.ErrBookkeepingMissing):
		return &failure{
			status:     503,
			code:       "SCHEMA_NOT_APPLIED",
			message:    core.ErrBookkeepingMissing.Error(),
			suggestion: "Start the server once so it applies the entity declarations.",
		}
	case tenancy.IsDuplicateKey(err):
		return &failure{
			status:  409,
			code:    "CONFLICT",
			message: "A record with the same unique field value already exists.",
		}
	}
	return &failure{status: 500, code: "INTERNAL", message: "The request failed. Try again."}
}

func humaError(err error) error {
	f := asFailure(err)
	if f == nil {
		return nil
	}
	if f.status >= 500 {
		slog.Error("ontology: request failed", "status", f.status, "code", f.code, "err", err)
	}
	return huma.NewError(f.status, f.message)
}

func agentError(err error) error {
	f := asFailure(err)
	if f == nil {
		return nil
	}
	return plugin.NewAgentError(f.status, f.code, f.message, f.suggestion, f.status >= 500)
}
