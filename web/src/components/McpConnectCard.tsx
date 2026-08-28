import { useState, useMemo } from "react";
import { Link } from "react-router-dom";
import { Token } from "../api";
import { useTranslation } from "../i18n";
import { GlassCard, Badge, Button, toast } from "../ui";
import { Bot, Copy, Check, Terminal, Code2, ExternalLink, Sparkles } from "lucide-react";

export type McpClientType = "cursor" | "claude-desktop" | "claude-code" | "http";

export interface McpConnectCardProps {
  tokens?: Token[];
  className?: string;
}

export function McpConnectCard({ tokens = [], className = "" }: McpConnectCardProps) {
  const { t } = useTranslation();
  const [selectedClient, setSelectedClient] = useState<McpClientType>("cursor");
  const [selectedTokenPrefix, setSelectedTokenPrefix] = useState<string>("");
  const [copied, setCopied] = useState(false);

  const origin = typeof window !== "undefined" ? window.location.origin : "http://localhost:8080";
  const tokenPlaceholder = selectedTokenPrefix ? `${selectedTokenPrefix}…` : t("personal.mcpPlaceholderToken");

  const configSnippet = useMemo(() => {
    const sseUrl = `${origin}/api/mcp/sse`;

    switch (selectedClient) {
      case "cursor":
        return JSON.stringify(
          {
            mcpServers: {
              octarq: {
                url: sseUrl,
                headers: {
                  Authorization: `Bearer ${tokenPlaceholder}`,
                },
              },
            },
          },
          null,
          2
        );

      case "claude-desktop":
        return JSON.stringify(
          {
            mcpServers: {
              octarq: {
                command: "octarq",
                args: ["mcp"],
                env: {
                  OCTARQ_DB_DSN: "/path/to/octarq.db",
                },
              },
            },
          },
          null,
          2
        );

      case "claude-code":
        return `claude mcp add --transport sse octarq ${sseUrl} --header "Authorization: Bearer ${tokenPlaceholder}"`;

      case "http":
        return `# Remote SSE Endpoint:\ncurl -N -H "Authorization: Bearer ${tokenPlaceholder}" ${sseUrl}`;
    }
  }, [selectedClient, origin, tokenPlaceholder]);

  async function copyConfig() {
    try {
      await navigator.clipboard.writeText(configSnippet);
      setCopied(true);
      toast.success(t("personal.mcpCopied"));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Failed to copy");
    }
  }

  async function copyPrompt(prompt: string) {
    try {
      await navigator.clipboard.writeText(prompt);
      toast.success(t("personal.tokenCopied"));
    } catch {
      toast.error("Failed to copy");
    }
  }

  const sampleQueries = [
    t("personal.mcpQuery1"),
    t("personal.mcpQuery2"),
    t("personal.mcpQuery3"),
  ];

  return (
    <GlassCard className={`p-6 space-y-6 ${className}`}>
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <div className="p-2 rounded-xl bg-accent-soft text-accent-fg">
            <Bot className="h-5 w-5" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h3 className="font-bold text-base text-foreground">{t("personal.mcpTitle")}</h3>
              <Badge>{t("personal.mcpBadge")}</Badge>
            </div>
            <p className="text-xs text-foreground/60 mt-0.5 max-w-xl leading-relaxed">
              {t("personal.mcpDesc")}
            </p>
          </div>
        </div>

        <Link
          to="/help/settings/mcp"
          className="inline-flex items-center gap-1 text-xs text-accent-fg hover:underline shrink-0"
        >
          {t("personal.mcpViewDocs")}
          <ExternalLink className="h-3.5 w-3.5" />
        </Link>
      </div>

      {/* Tabs & Token Selector */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-2 border-t border-foreground/[0.06]">
        {/* Client Tabs */}
        <div className="flex flex-wrap items-center gap-1.5 p-1 rounded-xl bg-foreground/[0.04] border border-foreground/[0.06]">
          {(
            [
              { id: "cursor", label: t("personal.mcpTabCursor"), icon: Code2 },
              { id: "claude-desktop", label: t("personal.mcpTabClaudeDesktop"), icon: Bot },
              { id: "claude-code", label: t("personal.mcpTabClaudeCode"), icon: Terminal },
              { id: "http", label: t("personal.mcpTabHttp"), icon: Code2 },
            ] as const
          ).map((tab) => {
            const Icon = tab.icon;
            const active = selectedClient === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => setSelectedClient(tab.id)}
                className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-xs font-medium transition-all ${
                  active
                    ? "bg-surface text-foreground shadow-xs"
                    : "text-foreground/60 hover:text-foreground hover:bg-foreground/[0.04]"
                }`}
              >
                <Icon className="h-3.5 w-3.5" />
                {tab.label}
              </button>
            );
          })}
        </div>

        {/* Token Selector (if tokens available) */}
        {tokens.length > 0 && selectedClient !== "claude-desktop" && (
          <div className="flex items-center gap-2 text-xs">
            <span className="text-foreground/50 shrink-0">{t("personal.mcpSelectToken")}:</span>
            <select
              className="input py-1 px-2 text-xs"
              value={selectedTokenPrefix}
              onChange={(e) => setSelectedTokenPrefix(e.target.value)}
            >
              <option value="">{t("personal.mcpSelectTokenDefault")}</option>
              {tokens.map((tok) => (
                <option key={tok.id} value={tok.prefix}>
                  {`${tok.name} (${tok.prefix}…)`}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {/* Code Snippet */}
      <div className="relative rounded-xl bg-foreground/[0.04] border border-foreground/[0.06] overflow-hidden">
        <div className="flex items-center justify-between px-4 py-2 bg-foreground/[0.02] border-b border-foreground/[0.04] text-[11px] text-foreground/50">
          <span className="font-mono">
            {selectedClient === "cursor" && ".cursor/mcp.json"}
            {selectedClient === "claude-desktop" && "claude_desktop_config.json"}
            {selectedClient === "claude-code" && "Terminal Command"}
            {selectedClient === "http" && "cURL / HTTP Endpoint"}
          </span>
          <Button
            variant="secondary"
            onClick={copyConfig}
            className="text-xs py-1 px-2.5 h-auto min-h-0 gap-1 border-0"
          >
            {copied ? <Check className="h-3.5 w-3.5 text-success-fg" /> : <Copy className="h-3.5 w-3.5" />}
            {copied ? "Copied" : t("personal.mcpCopyConfig")}
          </Button>
        </div>
        <pre className="p-4 font-mono text-xs text-foreground overflow-x-auto select-all leading-relaxed whitespace-pre-wrap">
          {configSnippet}
        </pre>
      </div>

      {/* Example Queries */}
      <div className="space-y-2 pt-2">
        <div className="flex items-center gap-1.5 text-xs font-semibold text-foreground/80">
          <Sparkles className="h-3.5 w-3.5 text-accent-fg" />
          <span>{t("personal.mcpExampleQueries")}</span>
        </div>
        <div className="grid gap-2 sm:grid-cols-3">
          {sampleQueries.map((query, i) => (
            <button
              key={i}
              type="button"
              onClick={() => copyPrompt(query)}
              className="flex items-center justify-between p-2.5 rounded-lg text-left text-xs bg-foreground/[0.02] border border-foreground/[0.06] hover:bg-foreground/[0.05] hover:border-accent-border transition-all text-foreground/70 group"
            >
              <span className="truncate pr-2">"{query}"</span>
              <Copy className="h-3 w-3 shrink-0 opacity-0 group-hover:opacity-100 transition-opacity text-accent-fg" />
            </button>
          ))}
        </div>
      </div>
    </GlassCard>
  );
}
