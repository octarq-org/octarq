# Publishing `@octarq/plugin-sdk`

The frontend plugin SDK is published from `packages/plugin-sdk/` using
[Changesets](https://github.com/changesets/changesets) to **npmjs** (public,
scope `@octarq`).

---

## 1. Release flow (Changesets)

Releases are driven by changeset files committed alongside code changes.

### Step 1 — record a change

After changing the SDK, from the repo root run:

```bash
pnpm changeset
```

Pick the affected package (`@octarq/plugin-sdk`), the bump type
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
itself (CI runs `pnpm --filter @octarq/plugin-sdk build` first) and publishes the
newly-versioned package to the registry, then pushes the git tag
`@octarq/plugin-sdk@x.y.z`.

This is loop-safe: publishing removes the changesets, so the next `main` push
has nothing to release.

### Escape hatch — tag publish

Pushing a tag matching `sdk-v*` (e.g. `sdk-v1.2.3`) triggers a direct
`pnpm publish --filter @octarq/plugin-sdk`. Use this only for manual/out-of-band
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
`packages/plugin-sdk/package.json` includes the fields below:

```json
{
  "name": "@octarq/plugin-sdk",
  "version": "0.8.0",
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "git+https://github.com/octarq-org/octarq.git",
    "directory": "packages/plugin-sdk"
  },
  "publishConfig": {
    "registry": "https://registry.npmjs.org",
    "access": "public"
  }
}
```

Notes:

- `publishConfig.registry` is `https://registry.npmjs.org` for public npmjs publishing.
- `publishConfig.access: "public"` makes `@octarq/plugin-sdk` publicly accessible without authentication.
- `license` and `repository` are required for a clean public listing; keep `"private"` **out** of this package's `package.json`.

---

## 3. Consuming `@octarq/plugin-sdk`

Plugin authors can install `@octarq/plugin-sdk` directly from npmjs without authentication or `.npmrc`:

```bash
pnpm add @octarq/plugin-sdk
```

---

## 4. Pro Private Packages (GitHub Packages)

Internal commercial packages (such as `@octarq-org/plugin-issuer` and `@octarq-org/api-client`) are published to **GitHub Packages** under the `@octarq-org` scope.

Consumer projects that depend on Pro private packages route `@octarq-org` to GitHub Packages in `.npmrc`:

```ini
@octarq-org:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

- The first line sends every `@octarq-org/*` install to GitHub Packages.
- The second line supplies authentication via `GITHUB_TOKEN`.

---

## 5. Secrets & org settings needed to publish

- **npmjs (`@octarq/plugin-sdk`):** uses `NPM_TOKEN` repo secret wired to `NODE_AUTH_TOKEN` in `.github/workflows/publish-sdk.yml`.
- **GitHub Packages (`@octarq-org/*` Pro packages):** uses built-in `${{ secrets.GITHUB_TOKEN }}` with `permissions: packages: write`.
- **Version PR:** the `release` job needs `pull-requests: write` and `contents: write`.
