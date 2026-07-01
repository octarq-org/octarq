package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	outDir := "/Volumes/PHD/code/led-pro-openapi-worktree/web/public"
	if err := os.MkdirAll(outDir, 0755); err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(outDir, "openapi.json")

	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Led API Reference",
			"version":     "1.0.0",
			"description": "API documentation for Led, the self-hosted link shortener and temporary email server.",
		},
		"servers": []any{
			map[string]any{
				"url":         "/",
				"description": "Current server instance",
			},
		},
		"security": []any{
			map[string]any{"cookieAuth": []any{}},
			map[string]any{"bearerAuth": []any{}},
		},
		"paths": map[string]any{
			// --- Auth Endpoints ---
			"/api/auth/login": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Auth"},
					"summary":     "Log in",
					"description": "Log in with credentials and establish a session. Returns a session cookie `led_session`.",
					"security":    []any{}, // Login doesn't require authentication
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"username": map[string]any{
											"type":        "string",
											"description": "The user's email address",
											"example":     "admin@example.com",
										},
										"password": map[string]any{
											"type":    "string",
											"example": "securepassword",
										},
									},
									"required": []string{"username", "password"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Successfully logged in",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok":       map[string]any{"type": "boolean", "example": true},
											"username": map[string]any{"type": "string", "example": "admin@example.com"},
										},
									},
								},
							},
						},
						"400": map[string]any{
							"description": "Invalid request body",
							"content":     errorResponseContent(),
						},
						"401": map[string]any{
							"description": "Invalid credentials",
							"content":     errorResponseContent(),
						},
						"429": map[string]any{
							"description": "Too many failed login attempts",
							"content":     errorResponseContent(),
						},
					},
				},
			},
			"/api/auth/logout": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Auth"},
					"summary":     "Log out",
					"description": "Log out the current user and clear the session cookie.",
					"security":    []any{}, // Logout does not require valid auth to succeed
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Successfully logged out",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
					},
				},
			},
			"/api/auth/me": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Auth"},
					"summary":     "Get current session info",
					"description": "Retrieve user details and the current organization ID of the authenticated user.",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Current session details",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"username": map[string]any{"type": "string", "example": "admin@example.com"},
											"orgId":    map[string]any{"type": "integer", "example": 1},
										},
									},
								},
							},
						},
						"401": map[string]any{
							"description": "Unauthorized / Session invalid",
							"content":     errorResponseContent(),
						},
					},
				},
			},
			"/api/auth/invite/accept": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Auth"},
					"summary":     "Accept invite",
					"description": "Accept an organization member invite and set the user password.",
					"security":    []any{}, // Accept invite doesn't require session auth
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"token": map[string]any{
											"type":        "string",
											"description": "Invitation token",
											"example":     "inv_abc123xyz",
										},
										"password": map[string]any{
											"type":        "string",
											"description": "New password, must be at least 8 characters",
											"example":     "newsecurepass123",
										},
									},
									"required": []string{"token", "password"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Invite successfully accepted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{
							"description": "Invalid token or password too short",
							"content":     errorResponseContent(),
						},
						"500": map[string]any{
							"description": "Database or hashing error",
							"content":     errorResponseContent(),
						},
					},
				},
			},

			// --- Overview ---
			"/api/overview": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Overview"},
					"summary":     "Get dashboard overview metrics",
					"description": "Retrieve aggregate statistics, analytical click series (30 days), top links, and recent emails.",
					"parameters": []map[string]any{
						{
							"name":        "includeBot",
							"in":          "query",
							"description": "If true, bot traffic is included in analytics counters and charts.",
							"required":    false,
							"schema":      map[string]any{"type": "boolean", "default": false},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Dashboard statistics",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"links":        map[string]any{"type": "integer", "description": "Total links"},
											"activeLinks":  map[string]any{"type": "integer", "description": "Total active links"},
											"domains":      map[string]any{"type": "integer", "description": "Total managed zones"},
											"linkDomains":  map[string]any{"type": "integer", "description": "Domains enabled for short links"},
											"mailDomains":  map[string]any{"type": "integer", "description": "Domains enabled for emails"},
											"mailboxes":    map[string]any{"type": "integer", "description": "Total mailboxes"},
											"emails":       map[string]any{"type": "integer", "description": "Total emails"},
											"unread":       map[string]any{"type": "integer", "description": "Total unread emails"},
											"tokens":       map[string]any{"type": "integer", "description": "Total API tokens"},
											"totalClicks":  map[string]any{"type": "integer", "description": "Total link clicks"},
											"clicks7d":     map[string]any{"type": "integer", "description": "Human clicks in the last 7 days"},
											"clicks30d":    map[string]any{"type": "integer", "description": "Human clicks in the last 30 days"},
											"botClicks7d":  map[string]any{"type": "integer", "description": "Bot clicks in the last 7 days"},
											"botClicks30d": map[string]any{"type": "integer", "description": "Bot clicks in the last 30 days"},
											"series": map[string]any{
												"type":        "array",
												"description": "Daily click counts for the last 30 days",
												"items":       map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"topLinks": map[string]any{
												"type": "array",
												"items": map[string]any{
													"type": "object",
													"properties": map[string]any{
														"id":     map[string]any{"type": "integer"},
														"slug":   map[string]any{"type": "string"},
														"host":   map[string]any{"type": "string"},
														"clicks": map[string]any{"type": "integer"},
													},
												},
											},
											"devices": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"countries": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"recentEmails": map[string]any{
												"type": "array",
												"items": map[string]any{
													"type": "object",
													"properties": map[string]any{
														"id":         map[string]any{"type": "integer"},
														"from":       map[string]any{"type": "string"},
														"subject":    map[string]any{"type": "string"},
														"read":       map[string]any{"type": "boolean"},
														"receivedAt": map[string]any{"type": "string", "format": "date-time"},
													},
												},
											},
											"includeBot": map[string]any{"type": "boolean"},
										},
									},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
			},

			// --- Settings ---
			"/api/settings": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Settings"},
					"summary":     "Get settings",
					"description": "Retrieve system-wide runtime settings.",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Current settings",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Settings"},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
				"put": map[string]any{
					"tags":        []string{"Settings"},
					"summary":     "Update settings",
					"description": "Modify system-wide runtime settings.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/SettingsPatch"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Updated settings",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Settings"},
								},
							},
						},
						"400": map[string]any{
							"description": "Invalid request body",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
						"500": map[string]any{
							"description": "Encryption failure",
							"content":     errorResponseContent(),
						},
					},
				},
			},

			// --- Links ---
			"/api/links": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Links"},
					"summary":     "List short links",
					"description": "Retrieve short links for the current organization.",
					"parameters": []map[string]any{
						{
							"name":        "archived",
							"in":          "query",
							"description": "If '1', only archived links are returned. Default '0' (active links only).",
							"required":    false,
							"schema":      map[string]any{"type": "string", "enum": []string{"0", "1"}, "default": "0"},
						},
						{
							"name":        "q",
							"in":          "query",
							"description": "Search query to filter links by slug, target, title, tags, or note.",
							"required":    false,
							"schema":      map[string]any{"type": "string"},
						},
						{
							"name":        "tag",
							"in":          "query",
							"description": "Filter links by tag.",
							"required":    false,
							"schema":      map[string]any{"type": "string"},
						},
						{
							"name":        "host",
							"in":          "query",
							"description": "Filter links by short-domain host.",
							"required":    false,
							"schema":      map[string]any{"type": "string"},
						},
						{
							"name":        "limit",
							"in":          "query",
							"description": "Maximum links to return (between 1 and 500, default 50).",
							"required":    false,
							"schema":      map[string]any{"type": "integer", "default": 50},
						},
						{
							"name":        "offset",
							"in":          "query",
							"description": "Offset for pagination.",
							"required":    false,
							"schema":      map[string]any{"type": "integer", "default": 0},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of links",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "array",
										"items": map[string]any{"$ref": "#/components/schemas/LinkView"},
									},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
				"post": map[string]any{
					"tags":        []string{"Links"},
					"summary":     "Create short link",
					"description": "Create a new short link. If title is empty, it is fetched from the target URL in the background.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/LinkDTO"},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Successfully created",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/LinkView"},
								},
							},
						},
						"400": map[string]any{
							"description": "Invalid request body or target URL is missing",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
						"409": map[string]any{
							"description": "Slug is reserved or already exists on this host",
							"content":     errorResponseContent(),
						},
					},
				},
			},
			"/api/links/{id}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Links"},
					"summary":     "Get short link",
					"description": "Retrieve details of a single short link.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Short link details",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/LinkView"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Link not found"},
					},
				},
				"put": map[string]any{
					"tags":        []string{"Links"},
					"summary":     "Update short link",
					"description": "Update settings and metadata for a short link.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/LinkDTO"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Short link successfully updated",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/LinkView"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format or invalid payload"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Link not found"},
						"409": map[string]any{"description": "Slug is reserved or already in use on host"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"Links"},
					"summary":     "Delete short link",
					"description": "Delete a short link and all its historical analytics click events.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Short link successfully deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Link not found"},
					},
				},
			},
			"/api/links/{id}/stats": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Links"},
					"summary":     "Get link stats",
					"description": "Retrieve comprehensive click analytics for a specific short link.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":        "days",
							"in":          "query",
							"description": "Number of days of data to look back (1 to 365, default 30).",
							"required":    false,
							"schema":      map[string]any{"type": "integer", "default": 30},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Link analytics stats",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"total":    map[string]any{"type": "integer", "description": "Lifetime total clicks"},
											"windowed": map[string]any{"type": "integer", "description": "Clicks in the selected days window"},
											"days":     map[string]any{"type": "integer"},
											"series": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"referers": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"countries": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"regions": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"devices": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
											"browsers": map[string]any{
												"type":  "array",
												"items": map[string]any{"$ref": "#/components/schemas/StatKV"},
											},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Link not found"},
					},
				},
			},

			// --- Domains ---
			"/api/domains": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "List domains",
					"description": "List all domains managed by the organization.",
					"parameters": []map[string]any{
						{
							"name":        "q",
							"in":          "query",
							"description": "Search query filtering by domain name or note.",
							"required":    false,
							"schema":      map[string]any{"type": "string"},
						},
						{
							"name":        "limit",
							"in":          "query",
							"required":    false,
							"schema":      map[string]any{"type": "integer", "default": 50},
						},
						{
							"name":        "offset",
							"in":          "query",
							"required":    false,
							"schema":      map[string]any{"type": "integer", "default": 0},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of domains",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "array",
										"items": map[string]any{"$ref": "#/components/schemas/Domain"},
									},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
				"post": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Create domain",
					"description": "Register a new domain zone and configure its settings. Verifies credentials against provider if Zone ID is present.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/DomainDTO"},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Successfully created domain",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Domain"},
								},
							},
						},
						"400": map[string]any{
							"description": "Missing inputs or provider verification failed",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
						"409": map[string]any{
							"description": "Domain already exists in database",
							"content":     errorResponseContent(),
						},
					},
				},
			},
			"/api/domains/{id}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Update domain settings",
					"description": "Update domain notes, hosts mapping, and services configuration.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/DomainDTO"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Successfully updated",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Domain"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format or invalid body"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Domain not found"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Delete domain",
					"description": "Deletes the domain configuration from database. Upstream DNS records are not deleted.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Successfully deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Domain not found"},
					},
				},
			},
			"/api/domains/{id}/verify-dns": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Verify DNS records status",
					"description": "Perform live DNS queries to verify SPF, DMARC, and DKIM setups for email sending on this domain.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "DNS records verification details",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"spf":   map[string]any{"$ref": "#/components/schemas/RecordStatus"},
											"dmarc": map[string]any{"$ref": "#/components/schemas/RecordStatus"},
											"dkim":  map[string]any{"$ref": "#/components/schemas/DKIMStatus"},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Domain not found"},
					},
				},
			},
			"/api/domains/{id}/records": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "List DNS records",
					"description": "List DNS records query live from the provider zone.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of DNS records",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "array",
										"items": map[string]any{"$ref": "#/components/schemas/DNSRecord"},
									},
								},
							},
						},
						"400": map[string]any{
							"description": "Live query to provider failed",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
					},
				},
				"post": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Create DNS record",
					"description": "Create a new DNS record on the provider zone.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/DNSRecordInput"},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Successfully created record",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/DNSRecord"},
								},
							},
						},
						"400": map[string]any{
							"description": "Missing inputs or provider request failed",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
					},
				},
			},
			"/api/domains/{id}/records/{rid}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Update DNS record",
					"description": "Modify an existing DNS record on the provider zone.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":        "rid",
							"in":          "path",
							"description": "The provider's unique record ID.",
							"required":    true,
							"schema":      map[string]any{"type": "string"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/DNSRecordInput"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Record successfully updated",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/DNSRecord"},
								},
							},
						},
						"400": map[string]any{
							"description": "Validation failed or provider request failed",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
					},
				},
				"delete": map[string]any{
					"tags":        []string{"Domains"},
					"summary":     "Delete DNS record",
					"description": "Remove a DNS record from the provider zone.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
						{
							"name":     "rid",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "string"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Successfully deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{
							"description": "Provider deletion failed",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
					},
				},
			},

			// --- Mailboxes ---
			"/api/mailboxes": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Mailboxes"},
					"summary":     "List mailboxes",
					"description": "Retrieve mailboxes for the current organization, annotated with computed unread email counts.",
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of mailboxes",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "array",
										"items": map[string]any{"$ref": "#/components/schemas/Mailbox"},
									},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
				"post": map[string]any{
					"tags":        []string{"Mailboxes"},
					"summary":     "Create mailbox",
					"description": "Provision a new temporary mailbox address.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/MailboxDTO"},
							},
						},
					},
					"responses": map[string]any{
						"201": map[string]any{
							"description": "Successfully created mailbox",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Mailbox"},
								},
							},
						},
						"400": map[string]any{
							"description": "Invalid input address (must contain @)",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
						"409": map[string]any{
							"description": "Mailbox address already exists",
							"content":     errorResponseContent(),
						},
					},
				},
			},
			"/api/mailboxes/{id}": map[string]any{
				"put": map[string]any{
					"tags":        []string{"Mailboxes"},
					"summary":     "Update mailbox",
					"description": "Update notes or toggle status of a mailbox.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{"$ref": "#/components/schemas/MailboxDTO"},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Successfully updated",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Mailbox"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format or invalid body"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Mailbox not found"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"Mailboxes"},
					"summary":     "Delete mailbox",
					"description": "Delete a mailbox and all its stored emails.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Mailbox successfully deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Mailbox not found"},
					},
				},
			},

			// --- Emails ---
			"/api/emails": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "List received emails",
					"description": "Retrieve received emails. Bandwidth friendly: the raw and HTML email body fields are omitted from list results.",
					"parameters": []map[string]any{
						{
							"name":        "mailbox",
							"in":          "query",
							"description": "Filter emails by mailbox ID.",
							"required":    false,
							"schema":      map[string]any{"type": "integer"},
						},
						{
							"name":        "q",
							"in":          "query",
							"description": "Search query filtering by subject, sender, text body, or note.",
							"required":    false,
							"schema":      map[string]any{"type": "string"},
						},
						{
							"name":     "limit",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "integer", "default": 50},
						},
						{
							"name":     "offset",
							"in":       "query",
							"required": false,
							"schema":   map[string]any{"type": "integer", "default": 0},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "List of emails",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "array",
										"items": map[string]any{"$ref": "#/components/schemas/EmailListEntry"},
									},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
			},
			"/api/emails/read-all": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "Mark all read",
					"description": "Mark all unread emails in the organization as read, optionally filtering by a specific mailbox.",
					"parameters": []map[string]any{
						{
							"name":        "mailbox",
							"in":          "query",
							"description": "Filter to a specific mailbox ID.",
							"required":    false,
							"schema":      map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Operation successful",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok":      map[string]any{"type": "boolean", "example": true},
											"updated": map[string]any{"type": "integer", "description": "Number of emails updated", "example": 5},
										},
									},
								},
							},
						},
						"401": unauthorizedResponse(),
					},
				},
			},
			"/api/emails/{id}": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "Get email details",
					"description": "Retrieve full email details including the HTML body. Implicitly marks the email as read.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Email details",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Email"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Email not found"},
					},
				},
				"put": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "Update email metadata",
					"description": "Manually update an email's read status or custom note.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"read": map[string]any{"type": "boolean"},
										"note": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Email successfully updated",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{"$ref": "#/components/schemas/Email"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format or invalid body"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Email not found"},
					},
				},
				"delete": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "Delete email",
					"description": "Permanently delete a received email.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Email successfully deleted",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Email not found"},
					},
				},
			},
			"/api/emails/{id}/raw": map[string]any{
				"get": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "Download raw email file (.eml)",
					"description": "Streams the original RFC822 raw message data as a downloadable attachment file.",
					"parameters": []map[string]any{
						{
							"name":     "id",
							"in":       "path",
							"required": true,
							"schema":   map[string]any{"type": "integer"},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Raw message download file stream",
							"headers": map[string]any{
								"Content-Type": map[string]any{
									"schema": map[string]any{"type": "string", "example": "message/rfc822"},
								},
								"Content-Disposition": map[string]any{
									"schema": map[string]any{"type": "string", "example": "attachment; filename=\"email-12.eml\""},
								},
							},
							"content": map[string]any{
								"message/rfc822": map[string]any{
									"schema": map[string]any{"type": "string", "format": "binary"},
								},
							},
						},
						"400": map[string]any{"description": "Invalid ID format"},
						"401": unauthorizedResponse(),
						"404": map[string]any{"description": "Email not found"},
					},
				},
			},
			"/api/emails/send": map[string]any{
				"post": map[string]any{
					"tags":        []string{"Emails"},
					"summary":     "Send email",
					"description": "Send an outbound email using one of the configured SMTP Sender credentials. Outbound email is rate limited to 100 emails per organization per hour.",
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{
							"application/json": map[string]any{
								"schema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"smtpSenderId": map[string]any{
											"type":        "integer",
											"description": "The SMTP configuration ID to use for relay.",
											"example":     1,
										},
										"trackLinks": map[string]any{
											"type":        "boolean",
											"description": "If true, URLs in the email body are wrapped into short URLs to track clicks.",
											"default":     false,
										},
										"to": map[string]any{
											"type": "array",
											"items": map[string]any{
												"type": "string",
											},
											"example": []string{"recipient@example.com"},
										},
										"cc": map[string]any{
											"type": "array",
											"items": map[string]any{
												"type": "string",
											},
										},
										"bcc": map[string]any{
											"type": "array",
											"items": map[string]any{
												"type": "string",
											},
										},
										"subject": map[string]any{
											"type":    "string",
											"example": "Hello from Led",
										},
										"text": map[string]any{
											"type":    "string",
											"example": "Body of the email in plain text.",
										},
										"html": map[string]any{
											"type":    "string",
											"example": "<p>Body of the email in HTML format.</p>",
										},
										"attachments": map[string]any{
											"type": "array",
											"items": map[string]any{
												"type": "object",
												"properties": map[string]any{
													"filename":    map[string]any{"type": "string", "example": "invoice.pdf"},
													"contentType": map[string]any{"type": "string", "example": "application/pdf"},
													"content":     map[string]any{"type": "string", "format": "base64", "description": "Base64 encoded file data"},
												},
												"required": []string{"filename", "contentType", "content"},
											},
										},
									},
									"required": []string{"smtpSenderId", "to"},
								},
							},
						},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Email sent successfully",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"ok": map[string]any{"type": "boolean", "example": true},
										},
									},
								},
							},
						},
						"400": map[string]any{
							"description": "Validation error or sending relay failed",
							"content":     errorResponseContent(),
						},
						"401": unauthorizedResponse(),
						"429": map[string]any{
							"description": "Outbound sending rate limit exceeded (100 emails/org/hour)",
							"content":     errorResponseContent(),
						},
					},
				},
			},

			// --- Health ---
			"/api/health": map[string]any{
				"get": map[string]any{
					"tags":        []string{"System"},
					"summary":     "System health check",
					"description": "Checks system availability and database connectivity.",
					"security":    []any{}, // Public endpoint
					"responses": map[string]any{
						"200": map[string]any{
							"description": "System is healthy",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"status":   map[string]any{"type": "string", "example": "healthy"},
											"database": map[string]any{"type": "string", "example": "up"},
											"time":     map[string]any{"type": "string", "format": "date-time"},
										},
									},
								},
							},
						},
						"503": map[string]any{
							"description": "System is unhealthy (e.g. database down)",
							"content": map[string]any{
								"application/json": map[string]any{
									"schema": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"status":   map[string]any{"type": "string", "example": "unhealthy"},
											"database": map[string]any{"type": "string", "example": "down"},
											"error":    map[string]any{"type": "string", "example": "driver: connection refused"},
											"time":     map[string]any{"type": "string", "format": "date-time"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"cookieAuth": map[string]any{
					"type": "apiKey",
					"in":   "cookie",
					"name": "led_session",
				},
				"bearerAuth": map[string]any{
					"type":   "http",
					"scheme": "bearer",
				},
			},
			"schemas": map[string]any{
				"StatKV": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"key":   map[string]any{"type": "string", "example": "US"},
						"count": map[string]any{"type": "integer", "example": 150},
					},
				},
				"Settings": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"reservedSlugs":         map[string]any{"type": "string", "example": "admin\napi\nassets"},
						"reservedMailboxes":     map[string]any{"type": "string", "example": "abuse\npostmaster"},
						"builtinReserved":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "example": []string{"admin", "api", "assets"}},
						"cloudflareTokenSet":    map[string]any{"type": "boolean", "example": true},
						"inboundToken":          map[string]any{"type": "string", "example": "secret_inbound_token"},
						"catchAll":              map[string]any{"type": "boolean", "example": false},
						"googleClientId":        map[string]any{"type": "string", "example": "google-oauth-client-id"},
						"googleClientSecretSet": map[string]any{"type": "boolean", "example": true},
						"githubClientId":        map[string]any{"type": "string", "example": "github-oauth-client-id"},
						"githubClientSecretSet": map[string]any{"type": "boolean", "example": false},
						"dataRetentionDays":     map[string]any{"type": "integer", "example": 90},
						"autoWrapLinks":         map[string]any{"type": "boolean", "example": true},
					},
				},
				"SettingsPatch": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"reservedSlugs":      map[string]any{"type": "string", "example": "admin\napi\nassets"},
						"reservedMailboxes":  map[string]any{"type": "string", "example": "abuse\npostmaster"},
						"cloudflareToken":    map[string]any{"type": "string", "description": "Global Cloudflare token credentials. Send \"\" to clear, omit to preserve.", "example": "cf_token_secret"},
						"inboundToken":       map[string]any{"type": "string", "example": "new_secret_inbound_token"},
						"catchAll":           map[string]any{"type": "boolean"},
						"googleClientId":     map[string]any{"type": "string"},
						"googleClientSecret": map[string]any{"type": "string", "description": "Google Client Secret credentials. Send \"\" to clear, omit to preserve."},
						"githubClientId":     map[string]any{"type": "string"},
						"githubClientSecret": map[string]any{"type": "string", "description": "GitHub Client Secret credentials. Send \"\" to clear, omit to preserve."},
						"dataRetentionDays":  map[string]any{"type": "integer"},
						"autoWrapLinks":      map[string]any{"type": "boolean"},
					},
				},
				"LinkView": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":           map[string]any{"type": "integer", "example": 10},
						"host":         map[string]any{"type": "string", "description": "Short link domain host name. Empty maps to current request host.", "example": "go.example.com"},
						"slug":         map[string]any{"type": "string", "example": "promo"},
						"target":       map[string]any{"type": "string", "example": "https://external.destination.url/page"},
						"note":         map[string]any{"type": "string", "example": "Marketing tracking link"},
						"title":        map[string]any{"type": "string", "example": "Promotion Landing Page"},
						"tags":         map[string]any{"type": "string", "description": "Comma separated tags.", "example": "marketing,summer-2026"},
						"expiresAt":    map[string]any{"type": "string", "format": "date-time", "nullable": true},
						"expiredUrl":   map[string]any{"type": "string", "description": "Redirect target once expired or limit reached.", "example": "https://fallback.com"},
						"clickLimit":   map[string]any{"type": "integer", "description": "Total clicks allowed before redirection to expiredUrl (0 = unlimited).", "example": 1000},
						"archived":     map[string]any{"type": "boolean", "example": false},
						"enabled":      map[string]any{"type": "boolean", "example": true},
						"clicks":       map[string]any{"type": "integer", "description": "Current total click counter.", "example": 342},
						"routingRules": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/RoutingRule"}},
						"hasPassword":  map[string]any{"type": "boolean", "description": "Specifies if the short link has a password protection enabled.", "example": false},
						"createdAt":    map[string]any{"type": "string", "format": "date-time"},
						"updatedAt":    map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"RoutingRule": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type":   map[string]any{"type": "string", "enum": []string{"geo", "device", "os", "language"}, "example": "geo"},
						"match":  map[string]any{"type": "string", "description": "Match target identifier (e.g. US, iOS, Mobile, zh-CN)", "example": "US"},
						"target": map[string]any{"type": "string", "description": "Redirect destination if matched.", "example": "https://us.destination.com"},
					},
				},
				"LinkDTO": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host":       map[string]any{"type": "string", "example": "go.example.com"},
						"slug":       map[string]any{"type": "string", "description": "Leave empty for random short slug generation.", "example": "promo"},
						"target":     map[string]any{"type": "string", "example": "https://external.destination.url/page"},
						"password":   map[string]any{"type": "string", "description": "Specify a password to protect link access."},
						"note":       map[string]any{"type": "string", "example": "Marketing tracking link"},
						"title":      map[string]any{"type": "string", "example": "Promotion Landing Page"},
						"tags":       map[string]any{"type": "string", "example": "marketing,summer-2026"},
						"expiresAt":  map[string]any{"type": "string", "format": "date-time", "nullable": true},
						"expiredUrl": map[string]any{"type": "string", "example": "https://fallback.com"},
						"clickLimit": map[string]any{"type": "integer", "example": 1000},
						"archived":   map[string]any{"type": "boolean"},
						"enabled":    map[string]any{"type": "boolean"},
					},
					"required": []string{"target"},
				},
				"Domain": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":                map[string]any{"type": "integer", "example": 2},
						"name":              map[string]any{"type": "string", "example": "example.com"},
						"providerAccountId": map[string]any{"type": "integer", "description": "Associated provider account id", "example": 1},
						"zoneId":            map[string]any{"type": "string", "example": "zone_id_from_dns_provider"},
						"note":              map[string]any{"type": "string"},
						"forMail":           map[string]any{"type": "boolean", "description": "Enable inbound temporary email collection on this zone", "example": true},
						"forLink":           map[string]any{"type": "boolean", "description": "Serve short links on this zone", "example": true},
						"linkHosts":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Host"}},
						"mailHosts":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/Host"}},
						"createdAt":         map[string]any{"type": "string", "format": "date-time"},
						"updatedAt":         map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"Host": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host":    map[string]any{"type": "string", "example": "go.example.com"},
						"enabled": map[string]any{"type": "boolean", "example": true},
					},
				},
				"DomainDTO": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":              map[string]any{"type": "string", "example": "example.com"},
						"providerAccountId": map[string]any{"type": "integer", "example": 1},
						"zoneId":            map[string]any{"type": "string", "example": "zone_id_from_dns_provider"},
						"note":              map[string]any{"type": "string"},
						"forMail":           map[string]any{"type": "boolean"},
						"forLink":           map[string]any{"type": "boolean"},
						"linkHosts":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/HostEntry"}},
						"mailHosts":         map[string]any{"type": "array", "items": map[string]any{"$ref": "#/components/schemas/HostEntry"}},
					},
				},
				"HostEntry": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"host":    map[string]any{"type": "string", "example": "go.example.com"},
						"enabled": map[string]any{"type": "boolean", "example": true},
					},
				},
				"DNSRecord": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":       map[string]any{"type": "string", "example": "rec_123"},
						"type":     map[string]any{"type": "string", "example": "A"},
						"name":     map[string]any{"type": "string", "example": "go.example.com"},
						"content":  map[string]any{"type": "string", "example": "192.0.2.1"},
						"priority": map[string]any{"type": "integer", "nullable": true, "example": nil},
						"ttl":      map[string]any{"type": "integer", "example": 3600},
						"proxied":  map[string]any{"type": "boolean", "example": false},
						"comment":  map[string]any{"type": "string", "example": "Pointing to Led server"},
					},
				},
				"DNSRecordInput": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"type":     map[string]any{"type": "string", "example": "MX"},
						"name":     map[string]any{"type": "string", "example": "example.com"},
						"content":  map[string]any{"type": "string", "example": "mail.example.com"},
						"priority": map[string]any{"type": "integer", "nullable": true, "example": 10},
						"ttl":      map[string]any{"type": "integer", "example": 3600},
						"proxied":  map[string]any{"type": "boolean", "example": false},
						"comment":  map[string]any{"type": "string"},
					},
					"required": []string{"type", "content"},
				},
				"RecordStatus": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"set":     map[string]any{"type": "boolean", "example": true},
						"healthy": map[string]any{"type": "boolean", "example": true},
						"value":   map[string]any{"type": "string", "example": "v=spf1 ip4:192.0.2.0/24 -all"},
					},
				},
				"DKIMStatus": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"set":      map[string]any{"type": "boolean", "example": true},
						"healthy":  map[string]any{"type": "boolean", "example": true},
						"value":    map[string]any{"type": "string", "example": "v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA..."},
						"selector": map[string]any{"type": "string", "example": "default"},
					},
				},
				"Mailbox": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":        map[string]any{"type": "integer", "example": 5},
						"address":   map[string]any{"type": "string", "example": "temp@example.com"},
						"note":      map[string]any{"type": "string", "example": "Temporary test box"},
						"enabled":   map[string]any{"type": "boolean", "example": true},
						"unread":    map[string]any{"type": "integer", "description": "Number of unread emails.", "example": 2},
						"createdAt": map[string]any{"type": "string", "format": "date-time"},
						"updatedAt": map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"MailboxDTO": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"address": map[string]any{"type": "string", "example": "temp@example.com"},
						"note":    map[string]any{"type": "string", "example": "Temporary test box"},
						"enabled": map[string]any{"type": "boolean", "example": true},
					},
				},
				"EmailListEntry": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "integer", "example": 104},
						"mailboxId":   map[string]any{"type": "integer", "example": 5},
						"messageId":   map[string]any{"type": "string", "example": "<unique-id@sender.domain>"},
						"from":        map[string]any{"type": "string", "example": "service@newsletter.com"},
						"to":          map[string]any{"type": "string", "example": "temp@example.com"},
						"subject":     map[string]any{"type": "string", "example": "Verify your account"},
						"text":        map[string]any{"type": "string", "description": "Plain text preview content of the email.", "example": "Please click the link to verify..."},
						"read":        map[string]any{"type": "boolean", "example": false},
						"note":        map[string]any{"type": "string", "example": "Registration flow"},
						"attachments": map[string]any{"type": "string", "description": "JSON array metadata of attachments.", "example": "[]"},
						"authSpf":     map[string]any{"type": "string", "enum": []string{"pass", "fail", "softfail", "neutral", "none"}, "example": "pass"},
						"authDkim":    map[string]any{"type": "string", "enum": []string{"pass", "fail", "none"}, "example": "pass"},
						"authDmarc":   map[string]any{"type": "string", "enum": []string{"pass", "fail", "none"}, "example": "pass"},
						"receivedAt":  map[string]any{"type": "string", "format": "date-time"},
					},
				},
				"Email": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":          map[string]any{"type": "integer", "example": 104},
						"mailboxId":   map[string]any{"type": "integer", "example": 5},
						"messageId":   map[string]any{"type": "string", "example": "<unique-id@sender.domain>"},
						"from":        map[string]any{"type": "string", "example": "service@newsletter.com"},
						"to":          map[string]any{"type": "string", "example": "temp@example.com"},
						"subject":     map[string]any{"type": "string", "example": "Verify your account"},
						"text":        map[string]any{"type": "string", "example": "Please click the link to verify..."},
						"html":        map[string]any{"type": "string", "example": "<html><body>Please click <a href='#'>here</a> to verify...</body></html>"},
						"read":        map[string]any{"type": "boolean", "example": true},
						"note":        map[string]any{"type": "string", "example": "Registration flow"},
						"attachments": map[string]any{"type": "string", "description": "JSON array metadata of attachments.", "example": "[]"},
						"authSpf":     map[string]any{"type": "string", "example": "pass"},
						"authDkim":    map[string]any{"type": "string", "example": "pass"},
						"authDmarc":   map[string]any{"type": "string", "example": "pass"},
						"receivedAt":  map[string]any{"type": "string", "format": "date-time"},
					},
				},
			},
		},
	}

	b, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling spec: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, b, 0644); err != nil {
		fmt.Printf("Error writing openapi.json: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OpenAPI specification written successfully to: %s\n", outPath)
}

func errorResponseContent() map[string]any {
	return map[string]any{
		"application/json": map[string]any{
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"error": map[string]any{"type": "string", "example": "invalid parameter"},
				},
			},
		},
	}
}

func unauthorizedResponse() map[string]any {
	return map[string]any{
		"description": "Unauthorized / Session invalid",
		"content": map[string]any{
			"application/json": map[string]any{
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"error": map[string]any{"type": "string", "example": "unauthorized"},
					},
				},
			},
		},
	}
}
