package api

import (
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/internal/apierror"
)

// init installs octarq's single error envelope into huma, exactly once, for the
// whole process.
//
// huma.NewError and huma.NewErrorWithContext are package-level variables — the
// library's own extension point, and unavoidably process-global. What used to
// be here instead was an assignment inside Routes() that captured the previous
// value and wrapped it:
//
//	oldNewError := huma.NewError
//	huma.NewError = func(...) { if status == 422 { status = 400 }; return oldNewError(...) }
//
// That had two defects. app.Run and app.RunMCP both call Routes(), so the
// wrapper wrapped itself — one nesting level per call, each re-running the
// rewrite. And the rewrite was unconditional, so a Pro module returning a
// declared huma.Error422UnprocessableEntity silently shipped a 400 to its
// clients: one repository invisibly overriding another repository's documented
// status code.
//
// Doing it in init() fixes both. It runs once per process, before any
// huma.Register call (registration is what samples NewError to derive the
// error schema), captures nothing, and is idempotent by construction.
func init() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		status, code := normalizeErrorStatus(status, msg)
		return apierror.New(status, code, msg, errs...)
	}
	// NewErrorWithContext receives the huma.Context, which is what lets the
	// envelope carry the request ID the edge middleware minted — without every
	// handler having to thread it through.
	huma.NewErrorWithContext = func(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
		status, code := normalizeErrorStatus(status, msg)
		e := apierror.New(status, code, msg, errs...)
		if ctx != nil {
			e.WithRequestID(ctx.Context())
		}
		return e
	}
}

// validationFailedMsg is the message huma uses for its own request-validation
// refusals (see huma.Register's contextError construction). Matching on it is
// what separates "huma could not parse/validate the request" from "a handler
// deliberately returned 422".
const validationFailedMsg = "validation failed"

// normalizeErrorStatus maps huma's request-validation 422 onto 400, and leaves
// every other status alone.
//
// octarq has answered 400 for malformed requests since before huma, and the
// dashboard and published clients branch on it — moving it now would be a
// breaking change for every consumer. A status a handler *declared*, however,
// is that handler's contract and is passed through untouched, which is exactly
// what the old blanket rewrite broke.
func normalizeErrorStatus(status int, msg string) (int, string) {
	if status == http.StatusUnprocessableEntity && msg == validationFailedMsg {
		return http.StatusBadRequest, apierror.CodeValidationFailed
	}
	return status, apierror.CodeForStatus(status)
}

// normalizeValidationResponse keeps the document honest about the rewrite
// above: huma documents a 422 for any operation that declares Errors and takes
// a body or path parameter, but that 422 is served as a 400. Runs as an
// OnAddOperation hook so it covers plugin operations too, not just core ones.
func normalizeValidationResponse(_ *huma.OpenAPI, op *huma.Operation) {
	if op == nil || op.Responses == nil {
		return
	}
	const unprocessable = "422"
	resp, ok := op.Responses[unprocessable]
	if !ok {
		return
	}
	if _, taken := op.Responses["400"]; taken {
		// A handler declared its own 400; the validation 422 collapses into it.
		delete(op.Responses, unprocessable)
		return
	}
	resp.Description = "Bad Request"
	op.Responses["400"] = resp
	delete(op.Responses, unprocessable)
}

// deprecationMetadataKey marks an operation as deprecated. The value is either
// a bool (deprecated, no sunset date announced yet) or an RFC 3339 date string
// ("2027-06-30") naming the day the operation is removed.
//
// One key drives three things that must never disagree: the `deprecated` flag
// in the published spec, the `x-sunset` extension next to it, and the
// Deprecation / Sunset response headers a running instance emits. A runway a
// client can only learn about by reading a changelog is not a runway.
const deprecationMetadataKey = "deprecated"

// applyDeprecation projects Metadata["deprecated"] into the OpenAPI document.
func applyDeprecation(_ *huma.OpenAPI, op *huma.Operation) {
	if op == nil {
		return
	}
	sunset, deprecated := deprecationOf(op)
	if !deprecated {
		return
	}
	op.Deprecated = true
	if sunset.IsZero() {
		return
	}
	if op.Extensions == nil {
		op.Extensions = map[string]any{}
	}
	op.Extensions["x-sunset"] = sunset.Format(time.RFC3339)
}

// deprecationOf reads the deprecation metadata off an operation. The zero time
// means "deprecated, no sunset date announced".
func deprecationOf(op *huma.Operation) (sunset time.Time, deprecated bool) {
	if op == nil || op.Metadata == nil {
		return time.Time{}, false
	}
	switch v := op.Metadata[deprecationMetadataKey].(type) {
	case bool:
		return time.Time{}, v
	case string:
		if v == "" {
			return time.Time{}, false
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			// Date-only form ("2027-06-30") is the one humans write.
			t, err = time.Parse("2006-01-02", v)
		}
		if err != nil {
			return time.Time{}, true
		}
		return t, true
	default:
		return time.Time{}, false
	}
}

// deprecationHeaders emits RFC 8594 signals for a deprecated operation, so a
// client learns its integration has a deadline from the responses it is already
// reading — not from a changelog it never subscribed to.
func deprecationHeaders(ctx huma.Context) {
	sunset, deprecated := deprecationOf(ctx.Operation())
	if !deprecated {
		return
	}
	ctx.SetHeader("Deprecation", "true")
	if !sunset.IsZero() {
		ctx.SetHeader("Sunset", sunset.UTC().Format(http.TimeFormat))
	}
}

// ErrorCodes returns the closed registry of machine-readable error codes, for
// the spec's enum and for the guard test that refuses an undocumented one.
func ErrorCodes() []string { return apierror.Codes() }

// documentErrorCodes writes the code registry into the published Error schema's
// enum, so an integrator can see the complete, closed set they may branch on
// instead of discovering codes one production incident at a time.
func documentErrorCodes(doc *huma.OpenAPI) {
	if doc == nil || doc.Components == nil || doc.Components.Schemas == nil {
		return
	}
	schema := doc.Components.Schemas.SchemaFromRef("#/components/schemas/Error")
	if schema == nil || schema.Properties == nil {
		return
	}
	code, ok := schema.Properties["code"]
	if !ok || code == nil {
		return
	}
	codes := apierror.Codes()
	enum := make([]any, 0, len(codes))
	for _, c := range codes {
		enum = append(enum, c)
	}
	code.Enum = enum
	code.Description = "Stable machine-readable error code. This set is closed: codes are added " +
		"additively and never renamed or removed without going through the deprecation process. " +
		"Branch on this field; never string-match `message`."
}
