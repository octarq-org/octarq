---
title: Publishing the SDK
description: Versioning, building, and publishing the frontend plugin SDK (@octarq/plugin-sdk) with Changesets.
sidebar:
  order: 1
  group:
    label: "Guides"
---


The frontend plugin SDK is published from `packages/plugin-sdk/` using [Changesets](https://github.com/changesets/changesets) to **npmjs** (public, scope `@octarq`).

---

## Release Flow (Changesets)

Releases are driven by changeset files committed alongside code changes.

### Step 1: Record a Change

After making changes to the SDK, run the following command from the repository root:

```bash
pnpm changeset
```

1. Select the affected package (`@octarq/plugin-sdk`).
2. Choose the version bump type (`patch`, `minor`, or `major` according to semver).
3. Write a summary explaining the changes.

This command generates a markdown file inside the `.changeset/` directory. Commit this file as part of your pull request.

### Step 2: Merge to `main`

On a push to the `main` branch, the publishing workflow runs:
- If there are **unconsumed changesets**, it opens (or updates) a **"Version Packages"** pull request that bumps the version in `package.json`, updates `CHANGELOG.md`, and deletes the consumed changeset files.
- If there are no changesets, it takes no action.

### Step 3: Merge the "Version Packages" PR

When you merge the "Version Packages" pull request, the publishing workflow builds the package and runs `changeset publish` to publish the package to the registry and create a git tag (e.g., `@octarq/plugin-sdk@x.y.z`).

---

## Package Configuration

The `packages/plugin-sdk/package.json` file requires specific fields to publish:

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

- **`publishConfig.registry`**: Directs `pnpm publish` to publish to npmjs (`https://registry.npmjs.org`).
- **`publishConfig.access`**: Marked `public` so any plugin author can install it without authentication.

---

## Consuming `@octarq/plugin-sdk`

Because `@octarq/plugin-sdk` is published publicly on npmjs, plugin authors can install it directly without configuring `.npmrc` or authentication:

```bash
pnpm add @octarq/plugin-sdk
```

---

## Pro Private Packages (GitHub Packages)

Internal commercial packages (such as `@octarq-org/plugin-issuer` and `@octarq-org/api-client`) are published to **GitHub Packages** under the `@octarq-org` scope.

Consumer projects that depend on Pro private packages require a `.npmrc` file:

```ini
@octarq-org:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

- The first line routes `@octarq-org/*` packages to GitHub Packages.
- The second line provides authentication via `GITHUB_TOKEN`.


