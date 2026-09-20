# Octarq Engineering Invariants & Trinity Law

This document defines the normative engineering invariants for the Octarq ecosystem. These rules are non-negotiable and serve as the primary gate for all code reviews and architectural decisions.

---

## 1. The Trinity Interface Law (三位一体契约定律)

Every business module must simultaneously satisfy three distinct interface contracts. A feature is considered "incomplete" and will be rejected if any of these three planes are missing or ad-hoc.

1.  **Human Plane (Web UI)**: Standardized via `@octarq/plugin-sdk`. No ad-hoc native forms or inconsistent UI patterns.
2.  **Program Plane (REST API)**: Pure RESTful OpenAPI with unified Bearer Token authentication and strict HTTP status codes.
3.  **Agent Plane (AI/MCP)**: Type-complete MCP Tools and Resources with strict JSON Schema validation and mandatory `org_id` tenant isolation.

:::important[Agent-First Requirement]
No feature may be implemented for the Web UI without a corresponding Agent-callable MCP tool. MCP tools must include schema guardrails and strict tenant isolation.
:::

---

## 2. The Five Invariants (防劣化五大铁律)

### Invariant I: Trinity Interface Law
As defined above. Systems must be built for humans, programs, and AI agents simultaneously.

### Invariant II: Zero-Bloat & Middleware Independence
*   **Pure Go 1.25**: Zero CGO, standard library `http.ServeMux`, and static resource embedding.
*   **SQLite-First**: The system must be fully functional and pass all tests using only SQLite.
*   **Redis as Enhancement**: Redis is allowed only as an optional enhancement for `asynq` background queues. The system must not require Redis to function.
*   **Decoupled AI**: LLM capabilities must be accessed via the `llmprovider` protocol. No embedded Python runtimes or external vector databases.

### Invariant III: Sovereignty & Strict Fail-Closed
*   **Pre-v1.0 Iron Law**: No fallbacks, no soft degradation, and no dual-track shims.
*   **Fail-Closed**: If a credential is missing, an input is invalid, or a precondition is not met, the system must refuse to start or return an error immediately.
*   **Conflict Prevention**: MCP Tool registration must include name collision detection. Registering a duplicate tool name must trigger a `Fail-Closed` panic.

### Invariant IV: Human-in-the-Loop (HITL) Circuit Breaker
*   **High-Risk Operations**: Any operation that modifies the real world (e.g., DNS changes, VPS reboots, refunds) must be intercepted and pushed to an administrator for manual approval.
*   **Security Contract**:
    1.  **Single-Use Tokens**: HMAC-SHA256 signed tokens that are invalidated immediately after use.
    2.  **Atomic State Machine**: `pending → approved/rejected/expired` transitions using atomic CAS (Compare-And-Swap) to prevent race conditions or replay attacks.
    3.  **Encrypted Secrets**: All bot tokens and webhook secrets must be stored encrypted using AES-GCM.

### Invariant V: Grounding Law (Anti-Toy Rule)
*   **Real-World Delivery**: Features are only considered delivered when verified against real-world I/O: public network communication, real OCR processing, or real SSH handshakes.
*   **Clock-Skew Protection**: License verification must detect and reject system clock rollbacks (Fail-Closed) by comparing against the `last_verified_at` timestamp.
