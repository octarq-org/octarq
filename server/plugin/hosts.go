package plugin

import (
	"context"
	"errors"
)

var (
	ErrHostInvalid        = errors.New("host is not a valid bindable name")
	ErrHostTaken          = errors.New("host is already bound to another workspace")
	ErrHostOrgRequired    = errors.New("host binding requires a workspace")
	ErrHostSourceRequired = errors.New("host binding requires a source")
	ErrHostsUnavailable   = errors.New("host registry unavailable")
)

type HostRegistry interface {
	OwnerOf(host string) (uint, bool)
	ServesTraffic(host string) bool
	Any() bool
	Bind(ctx context.Context, orgID uint, host string, traffic bool, source string) error
	Unbind(ctx context.Context, orgID uint, host, source string) error
	ReplaceOwned(ctx context.Context, orgID uint, source string, hosts []string, traffic bool) error
}

type noopHostRegistry struct{}

func (noopHostRegistry) OwnerOf(string) (uint, bool) { return 0, false }
func (noopHostRegistry) ServesTraffic(string) bool   { return false }
func (noopHostRegistry) Any() bool                   { return false }
func (noopHostRegistry) Bind(context.Context, uint, string, bool, string) error {
	return ErrHostsUnavailable
}
func (noopHostRegistry) Unbind(context.Context, uint, string, string) error {
	return ErrHostsUnavailable
}
func (noopHostRegistry) ReplaceOwned(context.Context, uint, string, []string, bool) error {
	return ErrHostsUnavailable
}

func NoopHostRegistry() HostRegistry { return noopHostRegistry{} }
