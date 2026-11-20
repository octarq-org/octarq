import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

export const WEB_ROOT = dirname(fileURLToPath(import.meta.url));

export function resolveEntries() {
  const inline = process.env.OCTARQ_PLUGINS?.trim();
  if (inline) {
    if (inline.startsWith("[")) return JSON.parse(inline);
    return inline.split(",").map((s) => s.trim()).filter(Boolean);
  }
  const file = process.env.OCTARQ_PLUGINS_MANIFEST
    ? resolve(process.env.OCTARQ_PLUGINS_MANIFEST)
    : resolve(WEB_ROOT, "octarq.plugins.json");
  const parsed = JSON.parse(readFileSync(file, "utf8"));
  return parsed.plugins ?? [];
}

export function splitSpec(spec) {
  const at = spec.lastIndexOf("@");
  if (at > 0) return { name: spec.slice(0, at), range: spec.slice(at + 1) };
  return { name: spec, range: "" };
}

export function parseEntry(entry) {
  const raw = typeof entry === "string" ? entry : entry.from;
  const named = typeof entry === "string" ? undefined : entry.import;
  const version = typeof entry === "object" ? entry.version : undefined;
  if (raw.startsWith(".") || raw.startsWith("/")) {
    return { isLocal: true, importSpec: resolve(WEB_ROOT, raw), installSpec: null, named };
  }
  const { name, range } = splitSpec(raw);
  const wanted = version || range;
  return { isLocal: false, importSpec: name, installSpec: wanted ? `${name}@${wanted}` : name, named };
}
