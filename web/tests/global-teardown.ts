import { rmSync } from "node:fs";

export default function globalTeardown() {
  const dir = process.env.OCTARQ_E2E_TMPDIR;
  if (dir) {
    rmSync(dir, { recursive: true, force: true });
  }
}
