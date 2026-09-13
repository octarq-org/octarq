package plugin_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/octarq-org/octarq/plugin"
)

type mockCronService struct {
	jobs []plugin.CronJob
}

func (m *mockCronService) Register(name string, spec string, handler func(ctx context.Context) error) error {
	m.jobs = append(m.jobs, plugin.CronJob{
		Name:    name,
		Spec:    spec,
		Status:  "idle",
		NextRun: time.Now().Add(time.Hour),
	})
	return nil
}

func (m *mockCronService) Unregister(name string) error {
	for i, j := range m.jobs {
		if j.Name == name {
			m.jobs = append(m.jobs[:i], m.jobs[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockCronService) List() []plugin.CronJob {
	return m.jobs
}

var _ plugin.CronService = (*mockCronService)(nil)

func TestCronServiceSPI(t *testing.T) {
	svc := &mockCronService{}
	err := svc.Register("test_job", "0 * * * *", func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected register error: %v", err)
	}

	jobs := svc.List()
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Name != "test_job" || jobs[0].Spec != "0 * * * *" || jobs[0].Status != "idle" {
		t.Fatalf("unexpected job attributes: %+v", jobs[0])
	}

	data, err := json.Marshal(jobs[0])
	if err != nil {
		t.Fatalf("failed to marshal CronJob: %v", err)
	}
	var decoded plugin.CronJob
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal CronJob: %v", err)
	}
	if decoded.Name != "test_job" || decoded.Spec != "0 * * * *" {
		t.Fatalf("mismatched decoded job: %+v", decoded)
	}

	if err := svc.Unregister("test_job"); err != nil {
		t.Fatalf("unexpected unregister error: %v", err)
	}
	if len(svc.List()) != 0 {
		t.Fatalf("expected 0 jobs after unregister, got %d", len(svc.List()))
	}
}
