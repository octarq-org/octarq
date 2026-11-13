package ontology

import (
	"errors"

	"github.com/danielgtaylor/huma/v2"
	core "github.com/octarq-org/octarq/server/internal/ontology"
	"github.com/octarq-org/octarq/server/internal/tenancy"
	"github.com/octarq-org/octarq/server/plugin"
)

// ErrForbidden is returned when the caller's role is not in an operation's ACL.
var ErrForbidden = errors.New("ontology: insufficient role for this operation")

// ErrInvalidArgument is returned when a caller sends a value the platform does
// not serve, such as a page size outside the declared bounds. Both surfaces
// refuse it; neither substitutes a different value.
var ErrInvalidArgument = errors.New("ontology: invalid argument")

// failure is a domain error with the HTTP status it maps to. Both surfaces read
// it, so an MCP caller and a REST caller are told the same thing about the same
// mistake.
type failure struct {
	status     int
	code       string
	message    string
	suggestion string
}

func (f *failure) Error() string { return f.message }

// asFailure classifies an error. It is the single place a status is chosen,
// which is what keeps a 409 from being reported as a 500 on one surface and a
// 422 on the other.
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
	case errors.Is(err, ErrNoTenantScope):
		return &failure{status: 401, code: "UNAUTHENTICATED", message: "Sign in to continue."}
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

// humaError converts a domain error into the response huma writes. Validation
// issues are joined into the message because huma's own error shape carries one
// message, and an operator fixing three fields should see all three at once.
func humaError(err error) error {
	f := asFailure(err)
	if f == nil {
		return nil
	}
	// The underlying error is kept for the log (huma does not print it to the
	// client beyond the message).
	return huma.NewError(f.status, f.message)
}

// agentError converts a domain error for an MCP tool result.
func agentError(err error) error {
	f := asFailure(err)
	if f == nil {
		return nil
	}
	return plugin.NewAgentError(f.status, f.code, f.message, f.suggestion, f.status >= 500)
}
