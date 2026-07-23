---
title: Publishing the SDK
description: How to version, build, and publish the frontend plugin SDK (@octarq/plugin-sdk) using Changesets.
---

The frontend plugin SDK is published from `packages/plugin-sdk/` using [Changesets](https://github.com/changesets/changesets). The default registry is **GitHub Packages** (private, org-scoped `@octarq-org`).

---

## 1. Release Flow (Changesets)

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

## 2. Package Configuration

The `packages/plugin-sdk/package.json` file requires specific fields to successfully publish:

```json
{
  "name": "@octarq/plugin-sdk",
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

- **`publishConfig.registry`**: Directs `pnpm publish` to publish to GitHub Packages.
- **`publishConfig.access`**: Marked `restricted` to keep it private within the organization.

---

## 3. Consuming `@octarq/plugin-sdk`

To consume `@octarq/plugin-sdk` from GitHub Packages, target projects must define a `.npmrc` file:

```ini
@octarq-org:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=${GITHUB_TOKEN}
```

- The first line scopes `@octarq-org/*` package installations to GitHub Packages.
- The second line provides authentication using a `GITHUB_TOKEN` environment variable. Set it before running installations:

```bash
export GITHUB_TOKEN=your_personal_access_token
pnpm install
```

---

## 4. Switching to Public npm

To distribute the SDK on the public npm registry:

1. Update `.changeset/config.json`: set `"access": "public"`.
2. Update `packages/plugin-sdk/package.json` `publishConfig`:
   ```json
   "publishConfig": { "registry": "https://registry.npmjs.org", "access": "public" }
   ```
3. Update the publish workflow to point to `https://registry.npmjs.org` and provide an npm token (e.g. `NPM_TOKEN`) as `NODE_AUTH_TOKEN`.
4. Consumer projects can then remove the custom scope redirection from their `.npmrc` files.
