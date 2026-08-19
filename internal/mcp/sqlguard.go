package mcp

import "github.com/octarq-org/octarq/plugin"

func validateReadOnlyQuery(query string) (string, error) {
	return plugin.ValidateReadOnlyQuery(query)
}

func redactRow(columns []string, row map[string]any) {
	plugin.RedactRow(columns, row)
}
