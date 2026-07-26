import { useEffect, useState } from "react";
import { CheckCircle2, AlertTriangle, XCircle, MinusCircle, RefreshCw, Sun, Moon } from "lucide-react";
import { api, SubsystemStatusResponse } from "../api";
import { BrandMark } from "../shell/BrandMark";
import { useI18n } from "../i18n";
import { useTheme, toggleTheme } from "../theme";

import { Badge } from "../ui";

export default function StatusPage() {
  const { lang, setLang, t } = useI18n();
  const theme = useTheme();
  const [data, setData] = useState<SubsystemStatusResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fetchStatus = async (isManual = false) => {
    if (isManual) setRefreshing(true);
    try {
      const res = await api.subsystemStatus();
      setData(res);
      setError(null);
    } catch (e: any) {
      setError(e.message || "Failed to load subsystem status");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => {
    fetchStatus();
    const timer = setInterval(() => fetchStatus(), 15000);
    return () => clearInterval(timer);
  }, []);

  const overall = data?.overall || (error ? "down" : "ok");

  const getOverallBanner = () => {
    if (overall === "ok") {
      return {
        bg: "bg-success-bg border-success-border text-success-fg",
        icon: <CheckCircle2 className="h-8 w-8 text-success-fg animate-pulse" />,
        title: t("status.allSystemsOperational"),
      };
    }
    if (overall === "degraded") {
      return {
        bg: "bg-warning-bg border-warning-border text-warning-fg",
        icon: <AlertTriangle className="h-8 w-8 text-warning-fg animate-bounce" />,
        title: t("status.someSystemsDegraded"),
      };
    }
    return {
      bg: "bg-danger-bg border-danger-border text-danger-fg",
      icon: <XCircle className="h-8 w-8 text-danger-fg" />,
      title: t("status.majorOutage"),
    };
  };

  const banner = getOverallBanner();

  const renderStatusBadge = (status: string) => {
    switch (status) {
      case "ok":
        return (
          <div className="flex items-center gap-2">
            <span className="relative flex h-2.5 w-2.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-success-fg opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-success-fg"></span>
            </span>
            <Badge variant="success">{t("status.statusOk")}</Badge>
          </div>
        );
      case "degraded":
        return (
          <div className="flex items-center gap-2">
            <span className="relative flex h-2.5 w-2.5">
              <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-warning-fg opacity-75"></span>
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-warning-fg"></span>
            </span>
            <Badge variant="warning">{t("status.statusDegraded")}</Badge>
          </div>
        );
      case "down":
        return (
          <div className="flex items-center gap-2">
            <span className="relative flex h-2.5 w-2.5">
              <span className="relative inline-flex rounded-full h-2.5 w-2.5 bg-danger-fg"></span>
            </span>
            <Badge variant="danger">{t("status.statusDown")}</Badge>
          </div>
        );
      default:
        return (
          <div className="flex items-center gap-2">
            <MinusCircle className="h-4 w-4 text-foreground/40" />
            <Badge variant="default">{t("status.statusNa")}</Badge>
          </div>
        );
    }
  };

  const getSubsystemLabel = (name: string) => {
    switch (name) {
      case "database":
        return t("status.database");
      case "mail":
        return t("status.mail");
      case "queue":
        return t("status.queue");
      default:
        return name.charAt(0).toUpperCase() + name.slice(1);
    }
  };

  return (
    <div className="min-h-screen bg-background text-foreground transition-colors duration-200">
      {/* Header */}
      <header className="border-b border-foreground/10 bg-background/80 backdrop-blur-md sticky top-0 z-10">
        <div className="max-w-4xl mx-auto px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <BrandMark size="md" />
            <div>
              <h1 className="font-bold text-lg leading-tight tracking-tight">{t("status.title")}</h1>
              <p className="text-xs text-foreground/50">{t("status.desc")}</p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setLang(lang === "zh" ? "en" : "zh")}
              className="px-2.5 py-1.5 text-xs font-medium rounded-lg border border-foreground/10 hover:bg-foreground/5 transition-colors"
              title="Switch Language"
            >
              {lang === "zh" ? "EN" : "中文"}
            </button>
            <button
              onClick={toggleTheme}
              className="p-2 rounded-lg border border-foreground/10 hover:bg-foreground/5 transition-colors text-foreground/70"
              title="Toggle Theme"
            >
              {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-4xl mx-auto px-4 py-8 space-y-6">
        {/* Banner */}
        <div className={`p-6 rounded-2xl border flex items-center gap-4 shadow-sm ${banner.bg}`}>
          {banner.icon}
          <div>
            <h2 className="text-xl font-bold">{banner.title}</h2>
            <p className="text-xs opacity-80 mt-0.5">
              {data?.time
                ? t("status.lastChecked", { time: new Date(data.time).toLocaleTimeString() })
                : error || "Checking status..."}
            </p>
          </div>
        </div>

        {/* Subsystems Card */}
        <div className="bg-card border border-foreground/10 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between mb-6 pb-4 border-b border-foreground/10">
            <h3 className="font-semibold text-base text-foreground/90">{t("status.subsystems")}</h3>
            <button
              onClick={() => fetchStatus(true)}
              disabled={refreshing}
              className="flex items-center gap-1.5 text-xs font-medium text-foreground/60 hover:text-foreground px-3 py-1.5 rounded-lg border border-foreground/10 hover:bg-foreground/5 transition-all disabled:opacity-50"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${refreshing ? "animate-spin" : ""}`} />
              {refreshing ? t("status.refreshing") : t("status.refresh")}
            </button>
          </div>

          {loading ? (
            <div className="py-12 text-center text-foreground/40 text-sm">
              <RefreshCw className="h-6 w-6 animate-spin mx-auto mb-2 opacity-50" />
              Loading system status...
            </div>
          ) : (
            <div className="divide-y divide-foreground/5">
              {data?.subsystems.map((sub) => (
                <div key={sub.name} className="py-4 first:pt-0 last:pb-0 flex items-center justify-between">
                  <div>
                    <span className="font-medium text-sm text-foreground/90">
                      {getSubsystemLabel(sub.name)}
                    </span>
                    {sub.detail && (
                      <p className="text-xs text-foreground/50 mt-0.5">{sub.detail}</p>
                    )}
                  </div>
                  {renderStatusBadge(sub.status)}
                </div>
              ))}
            </div>
          )}
        </div>
      </main>

      {/* Footer */}
      <footer className="max-w-4xl mx-auto px-4 py-8 text-center text-xs text-foreground/40 border-t border-foreground/10 mt-12">
        <p>Octarq Status Page &bull; Powered by Octarq</p>
      </footer>
    </div>
  );
}
