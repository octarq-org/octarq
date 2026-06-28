package mcp

import "github.com/Jungley8/led/plugin"

const maxRows = plugin.MaxRows

func validateReadOnlyQuery(query string) (string, error) {
	return plugin.ValidateReadOnlyQuery(query)
}

func redactRow(columns []string, row map[string]any) {
	plugin.RedactRow(columns, row)
}
