import { Badge } from "../../../ui";

export function AuthBadges({
  spf,
  dkim,
  dmarc,
  compact = false,
}: {
  spf?: string;
  dkim?: string;
  dmarc?: string;
  compact?: boolean;
}) {
  const badge = (label: string, result: string | undefined) => {
    if (!result || result === "none" || result === "") return null;
    const pass = result === "pass";
    const warn = result === "softfail" || result === "neutral";
    if (compact && pass) return null; // only show problems in list view

    let tone: "green" | "amber" | "red" = "red";
    if (pass) tone = "green";
    else if (warn) tone = "amber";

    return (
      <Badge key={label} tone={tone} className="font-mono text-[9px] uppercase tracking-wider">
        {label}:{result}
      </Badge>
    );
  };
  const badges = [badge("SPF", spf), badge("DKIM", dkim), badge("DMARC", dmarc)].filter(Boolean);
  if (badges.length === 0) return null;
  return <div className="flex gap-1 items-center">{badges}</div>;
}
