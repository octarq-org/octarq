// releaseVersion matches a tagged release, such as v0.9.1. Anything else is a
// pseudo-version from a working directory, which repeats the revision and is
// too long to show next to it.
const releaseVersion = /^v\d+\.\d+\.\d+$/;

// buildLabel names the running build for the status page. A build that the
// toolchain did not stamp reports neither field, and the page shows no line
// instead of a guess.
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
