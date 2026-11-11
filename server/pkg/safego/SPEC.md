# safego Specification

## Package Overview
`github.com/octarq-org/octarq/server/pkg/safego` provides panic-recovery primitives for goroutines and HTTP handlers across Octarq core, extensions, and external plugins.

## Architecture & Constraints
- Zero internal dependencies: standard library only (`log/slog`, `net/http`, `runtime/debug`).
- Safe execution: swallows unexpected panics and logs them with stack traces.
- `http.ErrAbortHandler` transparency: re-panics `http.ErrAbortHandler` to allow `net/http` to perform normal client connection teardown without logging false-positive 500 errors.

## API Contracts
```go
func Recover(name string)
func Go(name string, fn func())
func RecoverHandler(name string, next http.Handler) http.Handler
```
