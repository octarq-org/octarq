package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

type testMockCronService struct {
	jobs []plugin.CronJob
	run  func(name string) error
	set  func(name string, enabled bool) error
	spec func(name string, spec string) error
}

func (m *testMockCronService) Register(name string, spec string, handler func(ctx context.Context) error) error {
	return nil
}

func (m *testMockCronService) Unregister(name string) error {
	return nil
}

func (m *testMockCronService) List() []plugin.CronJob {
	return m.jobs
}

func (m *testMockCronService) StartJob(name string) error {
	if m.run != nil {
		return m.run(name)
	}
	return nil
}

func (m *testMockCronService) SetEnabled(name string, enabled bool) error {
	if m.set != nil {
		return m.set(name, enabled)
	}
	return nil
}

func (m *testMockCronService) SetSpec(name string, spec string) error {
	if m.spec != nil {
		return m.spec(name, spec)
	}
	return nil
}

func TestListCronJobs_Unauthenticated(t *testing.T) {
	_, srv, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/api/cron/jobs", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListCronJobs_Authenticated_NoService(t *testing.T) {
	_, srv, _ := newTestHandlerRaw(t)

	req := httptest.NewRequest(http.MethodGet, "/api/cron/jobs", nil)
	for _, c := range loginCookies(t, srv) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated request, got %d: %s", rec.Code, rec.Body.String())
	}

	var jobs []plugin.CronJob
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(jobs))
	}
}

func TestListCronJobs_Authenticated_WithCronService(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)

	mock := &testMockCronService{
		jobs: []plugin.CronJob{
			{
				Name:    "session_cleanup",
				Spec:    "0 * * * *",
				Status:  "idle",
				LastRun: time.Now(),
				NextRun: time.Now().Add(time.Hour),
			},
			{
				Name:    "log_rotation",
				Spec:    "0 0 * * *",
				Status:  "idle",
				LastRun: time.Now(),
				NextRun: time.Now().Add(24 * time.Hour),
			},
		},
	}
	h.SetCronService(mock)

	req := httptest.NewRequest(http.MethodGet, "/api/cron/jobs", nil)
	for _, c := range loginCookies(t, srv) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for authenticated request, got %d: %s", rec.Code, rec.Body.String())
	}

	var jobs []plugin.CronJob
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(jobs))
	}
	if jobs[0].Name != "session_cleanup" || jobs[1].Name != "log_rotation" {
		t.Fatalf("unexpected jobs returned: %+v", jobs)
	}
}

func TestListCronJobs_LookupServiceFallback(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)

	mock := &testMockCronService{
		jobs: []plugin.CronJob{
			{
				Name:   "custom_job",
				Spec:   "@daily",
				Status: "idle",
			},
		},
	}
	h.SetServiceLookup(func(name string) (any, bool) {
		if name == plugin.ServiceCron {
			return plugin.CronService(mock), true
		}
		return nil, false
	})

	req := httptest.NewRequest(http.MethodGet, "/api/cron/jobs", nil)
	for _, c := range loginCookies(t, srv) {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var jobs []plugin.CronJob
	if err := json.Unmarshal(rec.Body.Bytes(), &jobs); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Name != "custom_job" {
		t.Fatalf("expected custom_job, got %+v", jobs)
	}
}

func TestListCronJobs_NilContext(t *testing.T) {
	h, _, _ := newTestHandlerRaw(t)
	_, err := h.listCronJobs(context.Background(), &ListCronJobsInput{Ctx: nil})
	if err == nil {
		t.Fatal("expected error when huma context is nil")
	}

	input := &ListCronJobsInput{}
	errs := input.Resolve(nil)
	if len(errs) != 0 {
		t.Fatalf("expected empty resolve errs, got %v", errs)
	}
}

func TestRunCronJob_RequiresInstanceAdmin(t *testing.T) {
	_, srv, db := newTestHandlerRaw(t)
	cookies := loginCookies(t, srv)
	if err := db.Model(&models.User{}).Where("1 = 1").Update("is_instance_admin", false).Error; err != nil {
		t.Fatalf("demote admin: %v", err)
	}
	rec := do(srv, "POST", "/api/cron/jobs/session_cleanup/run", cookies, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRunCronJob_StartsJob(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	cookies := loginCookies(t, srv)
	ran := ""
	h.SetCronService(&testMockCronService{run: func(name string) error {
		ran = name
		return nil
	}})

	rec := do(srv, "POST", "/api/cron/jobs/session_cleanup/run", cookies, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ran != "session_cleanup" {
		t.Fatalf("job was not started: %q", ran)
	}
}

func TestRunCronJob_Errors(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	cookies := loginCookies(t, srv)

	h.SetCronService(&testMockCronService{run: func(name string) error { return plugin.ErrJobBusy }})
	rec := do(srv, "POST", "/api/cron/jobs/session_cleanup/run", cookies, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("busy job: expected 409, got %d: %s", rec.Code, rec.Body.String())
	}

	h.SetCronService(&testMockCronService{run: func(name string) error { return plugin.ErrJobNotFound }})
	rec = do(srv, "POST", "/api/cron/jobs/missing/run", cookies, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing job: expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateCronJob_ValidatesAndApplies(t *testing.T) {
	h, srv, _ := newTestHandlerRaw(t)
	cookies := loginCookies(t, srv)
	mock := &testMockCronService{
		jobs: []plugin.CronJob{{Name: "pt.tick", Spec: "*/5 * * * *", DefaultSpec: "*/5 * * * *", Enabled: true}},
	}
	h.SetCronService(mock)

	rec := do(srv, "PATCH", "/api/cron/jobs/pt.tick", cookies, `{"spec":"not a cron"}`)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid spec: expected 422, got %d: %s", rec.Code, rec.Body.String())
	}

	var enabled bool
	mock.set = func(name string, value bool) error {
		enabled = value
		return nil
	}
	rec = do(srv, "PATCH", "/api/cron/jobs/pt.tick", cookies, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if enabled {
		t.Fatal("expected enabled=false to reach the service")
	}
}
