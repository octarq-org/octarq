// Package llmgateway is the core model gateway: it owns the configured LLM
// backends of each workspace, their credentials, and the per-model price.
//
// Features do not hold their own provider keys or their own price table. They
// resolve a provider through the gateway, and a billing ledger asks the gateway
// what a turn costs, so the price a ceiling is enforced against and the price
// the ledger charges are the same number.
//
// A workspace with no configured provider has no AI. Nothing falls back to the
// instance environment: a tenant must never spend the operator's credentials.
//
// The gateway also serves the workspace's models to coding agents in the three
// wire formats they speak: /v1/models, /v1/chat/completions, /v1/responses, and
// /v1/messages. The agent asks for the model list, so nobody types a model
// name, and the provider key stays here.
//
// opencode configures itself from /.well-known/opencode: the discovery document
// points it at /v1/opencode/config, which builds the provider block from that
// same model list. A model added in the console reaches the agent on reload,
// with no config edit.
package llmgateway
