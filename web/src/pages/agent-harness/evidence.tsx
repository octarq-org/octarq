export function EvidenceViewer({ evidence }: { evidence: unknown }) {
  if (evidence === null || evidence === undefined) {
    return <span className="text-xs text-muted-foreground italic">—</span>;
  }
  if (typeof evidence !== "object") {
    return <span className="font-mono text-xs text-foreground/90">{String(evidence)}</span>;
  }
  if (Array.isArray(evidence)) {
    if (evidence.length === 0) {
      return <span className="text-xs text-muted-foreground italic">—</span>;
    }
    return (
      <div className="space-y-1 font-mono text-xs">
        {evidence.map((item, idx) => (
          <div key={idx} className="p-1.5 rounded bg-well/40 border border-border/40">
            {typeof item === "object" && item !== null ? (
              <EvidenceViewer evidence={item} />
            ) : (
              <span className="text-foreground/90">{String(item)}</span>
            )}
          </div>
        ))}
      </div>
    );
  }
  const entries = Object.entries(evidence as Record<string, unknown>);
  if (entries.length === 0) {
    return <span className="text-xs text-muted-foreground italic">—</span>;
  }
  return (
    <div className="space-y-1 font-mono text-xs">
      {entries.map(([k, v]) => (
        <div key={k} className="flex items-start gap-2">
          <span className="font-semibold text-foreground/70 shrink-0">{k}:</span>
          <span className="text-foreground/90 break-all">
            {typeof v === "object" && v !== null ? JSON.stringify(v) : String(v)}
          </span>
        </div>
      ))}
    </div>
  );
}
