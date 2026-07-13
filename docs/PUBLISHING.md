# Publishing `@octarq-org/plugin-sdk`

The frontend plugin SDK is published from `packages/plugin-sdk/` using
[Changesets](https://github.com/changesets/changesets). The default registry is
**GitHub Packages** (private, org-scoped `@octarq-org`). Switching to public npm is a
one-line change (see the end of this doc).

---

## 1. Release flow (Changesets)

Releases are driven by changeset files committed alongside code changes.

### Step 1 — record a change

After changing the SDK, from the repo root run:

```bash
pnpm changeset
```

Pick the affected package (`@octarq-org/plugin-sdk`), the bump type
(`patch` / `minor` / `major` — follow semver), and write a one-line summary.
This writes a markdown file under `.changeset/`. Commit it with your PR.

> No changeset = no release. PRs that only touch docs/CI don't need one.

### Step 2 — merge to `main`

On push to `main`, `.github/workflows/publish-sdk.yml` runs
[`changesets/action`](https://github.com/changesets/changesets/tree/main/packages/action):

- If there are **unconsumed changesets**, it opens (or updates) a
  **"Version Packages"** PR that bumps `packages/plugin-sdk/package.json`,
  updates its `CHANGELOG.md`, and deletes the consumed changeset files.
- If there are **no changesets**, it does nothing.

### Step 3 — merge the "Version Packages" PR

Merging that PR pushes to `main` again. This time there are no changesets to
consume, so the action runs `pnpm changeset publish`, which builds nothing
itself (CI runs `pnpm --filter @octarq-org/plugin-sdk build` first) and publishes the
newly-versioned package to the registry, then pushes the git tag
`@octarq-org/plugin-sdk@x.y.z`.

This is loop-safe: publishing removes the changesets, so the next `main` push
has nothing to release.

### Escape hatch — tag publish

Pushing a tag matching `sdk-v*` (e.g. `sdk-v1.2.3`) triggers a direct
`pnpm publish --filter @octarq-org/plugin-sdk`. Use this only for manual/out-of-band
releases; the changesets flow above is the normal path.

### What actually publishes it

`scripts` in the **root** `package.json`:

```jsonc
{
  "scripts": {
    "changeset": "changeset",              // add a changeset
    "version-packages": "changeset version", // apply bumps (the Version PR)
    "release": "changeset publish"          // publish + tag
  }
}
```

---

## 2. Publish fields the SDK package needs

The pipeline is agnostic to package internals, but publishing will only work if
`packages/plugin-sdk/package.json` includes the fields below. Whoever finalizes
the package should paste these in (adjust `main`/`types`/`exports`/`files` to
match the tsup output — those are the package owner's call, the block below is
the publish-relevant subset):

```json
{
  "name": "@octarq-org/plugin-sdk",
  "version": "0.0.0",
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/octarq-org/octarq.git",
    "directory": "packages/plugin-sdk"
  },
  "publishConfig": {
    "registry": "https://npm.pkg.github.com",
    "access": "restricted"
  }
}
```

Notes:

- `publishConfig.registry` must be `https://npm.pkg.github.com` for GitHub
  Packages. This is what makes `pnpm publish` / `changeset publish` push to the
  right place regardless of the consumer's global registry.
- `publishConfig.access: "restricted"` keeps it private to the org. GitHub
  Packages ignores `public` for scoped packages unless the repo/package is
  public, so `restricted` is the safe default here. (It mirrors `access` in
  `.changeset/config.json`.)
- `repository.url` must point at the `octarq` repo and the package must be scoped
  `@octarq-org` so GitHub Packages links it to the repository. GitHub Packages requires
  the scope to match the owner (`@octarq-org` maps to the owning org/user configured
  for the repo — confirm the org name matches; see "Secrets & org settings").
- `license` and `repository` are required for a clean public listing; keep
  `"private"` **out** of this package's `package.json` (a private package cannot
  be published).

---

## 3. Consuming `@octarq-org/plugin-sdk` from GitHub Packages

Consumers (octarq-pro, community plugin authors) must route the `@octarq-org` scope to
GitHub Packages and authenticate.

### `.npmrc` in the consumer project

```ini
@octarq-org:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

- The first line sends every `@octarq-org/*` install to GitHub Packages; all other
  packages continue to resolve from the public npm registry.
- The second line supplies auth. Use an env var (as shown) rather than hardcoding
  the token, and export it before installing:

```bash
export GITHUB_TOKEN=ghp_xxx   # a PAT (classic) with read:packages, or a fine-grained
                              # token with "Packages: read" for the org
pnpm install                  # or npm/yarn in the consumer's toolchain
```

- In CI, `secrets.GITHUB_TOKEN` already has `read:packages` for the same repo/org;
  set `NODE_AUTH_TOKEN` / `GITHUB_TOKEN` from it and add the two `.npmrc` lines.

### Community plugin authors

Public (non-org) authors cannot read a `restricted` GitHub Packages package
without being granted access. Two options:

1. Grant them read access to the package/org (fine-grained PAT with
   `Packages: read`), or
2. Publish publicly to npm instead (below) so `pnpm add @octarq-org/plugin-sdk` works
   with no `.npmrc` at all.

---

## 4. Switching to public npm

To distribute the SDK on the public npm registry instead of GitHub Packages:

1. `.changeset/config.json`: set `"access": "public"`.
2. `packages/plugin-sdk/package.json` `publishConfig`:
   ```json
   "publishConfig": { "registry": "https://registry.npmjs.org", "access": "public" }
   ```
   (or remove `publishConfig.registry` to use the default public registry).
3. In `.github/workflows/publish-sdk.yml`, set
   `setup-node`'s `registry-url: https://registry.npmjs.org` and provide an npm
   automation token as `NODE_AUTH_TOKEN` from a new secret (e.g. `NPM_TOKEN`)
   instead of `GITHUB_TOKEN`.
4. Consumers drop the `.npmrc` scope line entirely.

You can also publish to **both**: keep GitHub Packages for octarq-pro and add a
public npm mirror — but that means two publish steps/tokens; keep it simple
unless there's demand.

---

## 5. Secrets & org settings needed to actually publish

- **GitHub Packages (default):** no extra secret — the workflow uses the
  built-in `${{ secrets.GITHUB_TOKEN }}` with `permissions: packages: write`.
  Confirm the repo's **owner/org** matches the `@octarq-org` scope; if the GitHub
  org/user is not literally `octarq`, GitHub Packages will reject the scope and the
  package name/scope (and `repository.url`) must be reconciled with the actual
  owner, or an org named `octarq` must own the repo.
- **Version PR:** the `release` job needs `pull-requests: write` and
  `contents: write` (already set at job level). If the org restricts
  Actions-created PRs, enable "Allow GitHub Actions to create and approve pull
  requests" in **Settings → Actions → General**.
- **Public npm (optional):** add an `NPM_TOKEN` repo secret (npm automation
  token) and wire it as shown in section 4.
