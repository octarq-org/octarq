package plugin

import (
	"context"
	"errors"
	"testing"
)

type fakeQuotaChecker struct {
	err error
}

func (f fakeQuotaChecker) Check(_ context.Context, _ uint, _ string, _ int64) error {
	return f.err
}

func ctxWithChecker(err error) *Context {
	return &Context{
		Lookup: func(name string) (any, bool) {
			if name == ServiceQuotaChecker {
				return QuotaChecker(fakeQuotaChecker{err: err}), true
			}
			return nil, false
		},
	}
}

func TestCheckQuotaNoCheckerAllows(t *testing.T) {
	t.Parallel()
	if err := CheckQuota(&Context{}, context.Background(), 1, "links", 1); err != nil {
		t.Errorf("no checker registered: want nil, got %v", err)
	}
}

func TestCheckQuotaNilContextAllows(t *testing.T) {
	t.Parallel()
	if err := CheckQuota(nil, context.Background(), 1, "links", 1); err != nil {
		t.Errorf("nil ctx: want nil, got %v", err)
	}
}

func TestCheckQuotaNilLookupAllows(t *testing.T) {
	t.Parallel()
	if err := CheckQuota(&Context{Lookup: nil}, context.Background(), 1, "links", 1); err != nil {
		t.Errorf("nil Lookup: want nil, got %v", err)
	}
}

func TestCheckQuotaExceeded(t *testing.T) {
	t.Parallel()
	err := CheckQuota(ctxWithChecker(ErrQuotaExceeded), context.Background(), 1, "links", 1)
	if !errors.Is(err, ErrQuotaExceeded) {
		t.Errorf("want ErrQuotaExceeded, got %v", err)
	}
}

func TestCheckQuotaUnavailable(t *testing.T) {
	t.Parallel()
	err := CheckQuota(ctxWithChecker(ErrQuotaUnavailable), context.Background(), 1, "mailOutPerMonth", 1)
	if !errors.Is(err, ErrQuotaUnavailable) {
		t.Errorf("want ErrQuotaUnavailable, got %v", err)
	}
}

func TestCheckQuotaPassesArguments(t *testing.T) {
	t.Parallel()
	var gotOrg uint
	var gotMetric string
	var gotN int64
	rec := &Context{
		Lookup: func(name string) (any, bool) {
			return QuotaChecker(recordingChecker{onCheck: func(orgID uint, metric string, n int64) {
				gotOrg, gotMetric, gotN = orgID, metric, n
			}}), true
		},
	}
	if err := CheckQuota(rec, context.Background(), 42, "customDomains", 3); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
	if gotOrg != 42 || gotMetric != "customDomains" || gotN != 3 {
		t.Errorf("arguments not passed through: org=%d metric=%q n=%d", gotOrg, gotMetric, gotN)
	}
}

type recordingChecker struct {
	onCheck func(orgID uint, metric string, n int64)
}

func (r recordingChecker) Check(_ context.Context, orgID uint, metric string, n int64) error {
	r.onCheck(orgID, metric, n)
	return nil
}
