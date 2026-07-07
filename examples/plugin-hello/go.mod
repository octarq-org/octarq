// A standalone module so `go build ./...` / `go test ./...` in the led repo does
// NOT descend into this example (a nested module is excluded from the parent's
// package pattern). A third party copying this directory keeps exactly this
// shape: their own module that depends on led and implements plugin.Plugin.
//
// The replace directive points at the led checkout two levels up so the example
// builds in-tree without a published tag; a real third party would drop it and
// depend on a released version.
module example.com/plugin-hello

go 1.25

require (
	github.com/Jungley8/led v0.0.0
	gorm.io/gorm v1.25.12
)

replace github.com/Jungley8/led => ../..
