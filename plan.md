1. Add `wg sync.WaitGroup` to the `Resolver` struct in `internal/geo/geo.go` using `replace_with_git_merge_diff`.
2. Use `run_in_bash_session` to `cat internal/geo/geo.go` and verify the struct was modified correctly.
3. In `internal/geo/geo.go`, modify `openAuto` (around line 92) to call `r.wg.Add(1)` and start a wrapper goroutine that calls `defer r.wg.Done()` before `r.autoDownload(...)` using `replace_with_git_merge_diff`.
4. Use `run_in_bash_session` to `cat internal/geo/geo.go` and verify `openAuto` was modified correctly.
5. In `internal/geo/geo.go`, modify `Close()` to restructure the mutex logic. Remove `defer r.mu.Unlock()` and add an explicit `r.mu.Unlock()` before returning (e.g. at line 195). Then call `r.wg.Wait()` after the lock has been released. This prevents a deadlock since `autoDownload` needs the lock to call `Load()`. Use `replace_with_git_merge_diff`.
6. Use `run_in_bash_session` to `cat internal/geo/geo.go` and verify `Close` was modified correctly.
7. Run `go test ./internal/geo/...` using `run_in_bash_session` to ensure the goroutine management changes are correct and have not broken any functionality.
8. Complete pre-commit steps to ensure proper testing, verification, review, and reflection are done.
