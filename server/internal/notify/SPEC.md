# SPEC: `internal/notify`

Thin adapter over `internal/notification.Router`. Registers channel providers
(best-effort, logged on failure) and delegates delivery.

## Refinement Notes
- `Register`/`RegisterWithDescriptor` now log registration failures via
  `slog.Warn` instead of ignoring them.
EOF
echo done