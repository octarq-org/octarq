package mcp

import "github.com/octarq-org/led/plugin"

const maxRows = plugin.MaxRows

func validateReadOnlyQuery(query string) (string, error) {
	return plugin.ValidateReadOnlyQuery(query)
}

func redactRow(columns []string, row map[string]any) {
	plugin.RedactRow(columns, row)
}
