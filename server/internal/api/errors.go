package api

import (
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/octarq-org/octarq/server/internal/apierror"
)

func init() {
	huma.NewError = func(status int, msg string, errs ...error) huma.StatusError {
		status, code := normalizeErrorStatus(status, msg)
		return apierror.New(status, code, msg, errs...)
	}
	huma.NewErrorWithContext = func(ctx huma.Context, status int, msg string, errs ...error) huma.StatusError {
		status, code := normalizeErrorStatus(status, msg)
		e := apierror.New(status, code, msg, errs...)
		if ctx != nil {
			e.WithRequestID(ctx.Context())
		}
		return e
	}
}

const validationFailedMsg = "validation failed"

func normalizeErrorStatus(status int, msg string) (int, string) {
	if status == http.StatusUnprocessableEntity && msg == validationFailedMsg {
		return http.StatusBadRequest, apierror.CodeValidationFailed
	}
	return status, apierror.CodeForStatus(status)
}

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
		delete(op.Responses, unprocessable)
		return
	}
	resp.Description = "Bad Request"
	op.Responses["400"] = resp
	delete(op.Responses, unprocessable)
}

const deprecationMetadataKey = "deprecated"

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

func ErrorCodes() []string { return apierror.Codes() }

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
