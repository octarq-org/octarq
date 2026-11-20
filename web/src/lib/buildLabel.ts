const releaseVersion = /^v\d+\.\d+\.\d+$/;

export function buildLabel(version?: string, revision?: string): string | null {
  const tag = (version ?? "").trim();
  const rev = (revision ?? "").trim();
  const release = releaseVersion.test(tag) ? tag : "";

  if (release !== "" && rev !== "") {
    return `${release} · ${rev}`;
  }
  if (release !== "") {
    return release;
  }
  if (rev !== "") {
    return rev;
  }
  return tag !== "" ? tag : null;
}
