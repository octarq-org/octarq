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
package llmgateway
