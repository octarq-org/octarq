import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  Button,
  PageCard,
  PageEmpty,
  PageShell,
  Input,
  StatusBadge,
  Switch,
  Table,
  TableSkeleton,
  TBody,
  TD,
  TH,
  THead,
  TR,
  timeAgo,
  useToast,
  type StatusTone,
} from "@octarq/plugin-sdk";
import { Play, RotateCw, ShieldAlert } from "lucide-react";
import { api, ApiError, type CronJobInfo } from "../../api";
import { useTranslation } from "../../i18n";

function hasRun(ts: string): boolean {
  return Boolean(ts) && !ts.startsWith("0001-01-01");
}

function formatNextRun(ts: string): string {
  return new Date(ts).toLocaleString();
}

function statusTone(status: string): { tone: StatusTone; key: string; fallback: string } {
  switch (status) {
    case "running":
      return { tone: "running", key: "jobs.statusRunning", fallback: "Running" };
    case "error":
      return { tone: "error", key: "jobs.statusError", fallback: "Failed" };
    default:
      return { tone: "idle", key: "jobs.statusIdle", fallback: "Idle" };
  }
}

export function ScheduledJobsSettings() {
  const { t } = useTranslation();
  const toast = useToast();
  const [jobs, setJobs] = useState<CronJobInfo[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [busyName, setBusyName] = useState("");
  const [savingName, setSavingName] = useState("");

  const load = useCallback(() => {
    setFailed(false);
    api
      .cronJobs()
      .then((rows) => {
        setJobs(rows);
        setDrafts((prev) => {
          const next: Record<string, string> = {};
          for (const row of rows) next[row.name] = prev[row.name] ?? row.spec;
          return next;
        });
      })
      .catch(() => {
        setFailed(true);
        setJobs([]);
      });
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  const running = useMemo(() => (jobs ?? []).some((job) => job.status === "running"), [jobs]);

  useEffect(() => {
    if (!running) return;
    const timer = setInterval(() => {
      api.cronJobs().then(setJobs).catch(() => {});
    }, 3000);
    return () => clearInterval(timer);
  }, [running]);

  const saveSpec = async (job: CronJobInfo) => {
    const spec = (drafts[job.name] ?? "").trim();
    if (spec === "" || spec === job.spec) return;
    setSavingName(job.name);
    try {
      await api.updateCronJob(job.name, { spec });
      toast.success(t("jobs.saved", "Schedule saved."));
      load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        toast.error(t("jobs.invalidSpec", "Use a valid cron expression with minutes, hours, day, month, and weekday."));
      } else {
        toast.error(t("jobs.saveFailed", "The schedule was not saved. Try again."));
      }
    } finally {
      setSavingName("");
    }
  };

  const toggle = async (job: CronJobInfo, enabled: boolean) => {
    setSavingName(job.name);
    try {
      await api.updateCronJob(job.name, { enabled });
      setJobs((rows) => (rows ?? []).map((row) => (row.name === job.name ? { ...row, enabled } : row)));
    } catch {
      toast.error(t("jobs.toggleFailed", "The change was not saved. Try again."));
    } finally {
      setSavingName("");
    }
  };

  const runNow = async (job: CronJobInfo) => {
    setBusyName(job.name);
    try {
      await api.runCronJob(job.name);
      toast.success(t("jobs.runStarted", "The task started."));
      load();
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        toast.warning(t("jobs.runBusy", "The task is already running."));
      } else {
        toast.error(t("jobs.runFailed", "The task did not start. Try again."));
      }
    } finally {
      setBusyName("");
    }
  };

  return (
    <PageShell title={t("jobs.title", "Scheduled tasks")} description={t("jobs.description", "Background jobs run by this instance. Instance administrators can change a schedule, pause a task, or run it now.")}>
      {failed ? (
        <Alert variant="danger" icon={<ShieldAlert className="h-4 w-4 shrink-0" />} className="mt-6 text-xs p-3 rounded-lg">
          <span className="mr-3">{t("jobs.loadFailed", "The scheduled tasks could not be loaded.")}</span>
          <Button variant="secondary" size="sm" onClick={load}>
            <RotateCw className="mr-1.5 h-3.5 w-3.5" />
            {t("jobs.retry", "Try again")}
          </Button>
        </Alert>
      ) : null}

      {jobs === null ? (
        <PageCard className="mt-6 overflow-hidden p-0">
          <TableSkeleton rowsCount={4} columnsCount={6} ariaLabel={t("jobs.loading", "Loading scheduled tasks…")} />
        </PageCard>
      ) : jobs.length === 0 ? (
        <PageCard className="mt-6 p-0">
          <PageEmpty title={t("jobs.empty", "No scheduled tasks are registered on this instance.")} />
        </PageCard>
      ) : (
        <PageCard className="mt-6 overflow-hidden p-0">
          <Table>
            <THead className="border-b border-border">
              <TR>
                <TH>{t("jobs.colName", "Task")}</TH>
                <TH>{t("jobs.colSchedule", "Schedule")}</TH>
                <TH>{t("jobs.colState", "State")}</TH>
                <TH>{t("jobs.colLastRun", "Last run")}</TH>
                <TH>{t("jobs.colNextRun", "Next run")}</TH>
                <TH className="text-right">{t("jobs.colActions", "Actions")}</TH>
              </TR>
            </THead>
            <TBody className="divide-y divide-border/40">
              {jobs.map((job) => {
                const status = statusTone(job.status);
                const draft = drafts[job.name] ?? job.spec;
                const dirty = draft.trim() !== "" && draft.trim() !== job.spec;
                return (
                  <TR key={job.name}>
                    <TD>
                      <div className="font-medium text-foreground">{job.name}</div>
                      <div className="mt-1 flex items-center gap-2 text-[11px] text-muted-foreground">
                        <Switch
                          checked={job.enabled}
                          onCheckedChange={(value) => void toggle(job, value)}
                          disabled={savingName === job.name}
                          aria-label={`${t("jobs.enabledLabel", "Enabled")} ${job.name}`}
                        />
                        <span>{t("jobs.enabledLabel", "Enabled")}</span>
                      </div>
                    </TD>
                    <TD className="min-w-[220px]">
                      <div className="flex items-center gap-2">
                        <Input
                          value={draft}
                          onChange={(event) => setDrafts((prev) => ({ ...prev, [job.name]: event.target.value }))}
                          onKeyDown={(event) => {
                            if (event.key === "Enter") void saveSpec(job);
                          }}
                          aria-label={t("jobs.editSchedule", { name: job.name })}
                          className="font-mono text-xs"
                        />
                        <Button
                          variant="secondary"
                          size="sm"
                          onClick={() => void saveSpec(job)}
                          disabled={!dirty || savingName === job.name}
                        >
                          {savingName === job.name ? t("jobs.saving", "Saving…") : t("jobs.save", "Save")}
                        </Button>
                      </div>
                      <div className="mt-1 font-mono text-[10px] text-muted-foreground">
                        {t("jobs.defaultSpec", { spec: job.defaultSpec })}
                      </div>
                    </TD>
                    <TD>
                      <StatusBadge tone={status.tone}>{t(status.key, status.fallback)}</StatusBadge>
                      {job.lastErr ? (
                        <div className="mt-1 max-w-[220px] truncate text-[11px] text-danger-fg" title={job.lastErr}>
                          {job.lastErr}
                        </div>
                      ) : null}
                    </TD>
                    <TD className="text-xs text-muted-foreground">{hasRun(job.lastRun) ? timeAgo(job.lastRun) : t("jobs.never", "Never")}</TD>
                    <TD className="text-xs text-muted-foreground">
                      {job.enabled && hasRun(job.nextRun) ? formatNextRun(job.nextRun) : t("jobs.never", "Never")}
                    </TD>
                    <TD className="text-right">
                      <Button size="sm" variant="secondary" onClick={() => void runNow(job)} disabled={busyName === job.name}>
                        <Play className="mr-1.5 h-3.5 w-3.5" />
                        {t("jobs.runNow", "Run now")}
                      </Button>
                    </TD>
                  </TR>
                );
              })}
            </TBody>
          </Table>
        </PageCard>
      )}
    </PageShell>
  );
}
