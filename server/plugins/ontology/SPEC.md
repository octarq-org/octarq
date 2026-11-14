# Ontology plugin — engineering specification

Tier 2 document. Not embedded in the binary. The user-facing behaviour lives in
`server/plugins/help/docs/data.mdx`; this file records the mechanism, the tables
and the failure modes.

## What this plugin is

`server/plugins/ontology` turns a declaration document into a working data
surface: one table per entity, six REST routes, MCP tools per exposed operation,
and a sidebar entry. It is a Core plugin — it is mounted by `builtin.Default()`
and appears in every build.

The declaration is the only definition store. There is no definition CRUD, no
version table and no optimistic locking; the platform never writes the
declaration back.

## Not a module API

A module cannot build on this package yet, and two things stand in the way. Both
are prerequisites, not polish:

1. **Nothing is public.** `internal/ontology` is an internal package, so code
   outside this module — `octarq-pro`, a third-party module — cannot import
   `Spec`, `Load`, `Apply` or the DDL generators. The only way to declare an
   entity is the process-wide document, read once at boot. There is no
   programmatic registration: a module cannot hand the plugin its own entity at
   mount time, per workspace or at all.
2. **Isolation is one column.** Every generated table is scoped by `org_id`,
   written from the tenant handle and enforced by `TableScoped`. There is no
   `scope_key` and no owner-level scope, so a table that belongs to one owner
   inside a workspace — a mailbox, a domain, a link, an order — has no way to say
   so. Filtering by the owner would be an ordinary column the caller could
   forget, which is the isolation model this plugin exists to avoid.

Until both exist, a module that adopted these tables would move them into a shell
that cannot express them: its own lifecycle, its own owner scoping and its own
ACLs would have to be rebuilt inside a declaration that has none of those
concepts. The eight tables that would be candidates for reuse stay Go modules.

This plugin is therefore an operator-facing feature: an installation declares the
records it needs, and gets a table, a page, an API and tools for them.

## Declaration sources

Resolution order, first match wins:

1. `OCTARQ_ENTITIES` — the document inline, `{"entities":[…]}`.
2. `OCTARQ_ENTITIES_FILE` — a path.
3. `./octarq.entities.json` — relative to the process working directory.

No source at all is a valid installation: the plugin publishes nothing, registers
no route, adds no menu. A source that exists but does not parse, or fails
validation, refuses the boot.

An inline document may be empty: `OCTARQ_ENTITIES={"entities":[]}` states that
this installation declares nothing. A file that declares nothing is refused
instead — the operator who wrote it meant to declare something. `openapi`
generation uses the inline form so that a declaration in the working directory
cannot put runtime routes into the published spec.

`Load` additionally refuses what a single-spec check cannot see: two entities with
the same slug, an unknown JSON key (`DisallowUnknownFields`), a key declared
twice, an icon outside `menuIcons`, a tier outside `{ "", pro, elite }`, and
`requireApproval: true`.

## Load and apply

`Mount` runs in this order, and the order is the fail-closed guarantee:

1. Read the whole document and decode it. Any error stops here.
2. Validate every entity. Any issue stops here.
3. Apply the declarations (HTTP composition only — see below).
4. Register routes and menus.

Nothing is executed before step 2 finishes, so an invalid declaration leaves the
database untouched. `Apply` plans every statement first and writes the
bookkeeping row only after the statements succeed.

Reading and checking happen in `ensureLoaded`, which both `Mount` and `Validate`
call. The order of those two differs by composition root: `app.Run` mounts first
and validates after, while `octarq mcp` validates before the stdio mount. Loading
only in `Mount` would leave the CLI running a declaration it never checked.

### Bookkeeping table

`ont_applied_specs` — `slug` (primary key), `spec_json`, `hash`, `applied_at`.

It stores the last successfully applied declaration and its sha256. It exists
because the physical table cannot answer what the declaration said: SQLite stores
`string`, `text`, `uuid` and `json` all as `TEXT`, and `required` / `default` /
`unique` cannot be recovered from a column at all.

`applied_at` is not rewritten on an unchanged boot. A repair (a manually dropped
index) leaves the row alone.

### Schema rules

`Plan` compares the stored declaration with the new one:

| Change | Result |
|---|---|
| New entity | `CREATE TABLE` + tenant index + declared indexes |
| New field | `ALTER TABLE … ADD COLUMN` + its index |
| New index / unique | `CREATE INDEX` |
| Unchanged | Nothing, but missing indexes are repaired |
| Slug changed | Refused (`SLUG_IMMUTABLE`) |
| Field type changed | Refused (`TYPE_CHANGE_UNSUPPORTED`) |
| Field removed | Refused (`COLUMN_DROP_UNSUPPORTED`), even on an empty table |
| `unique` / `indexed` dropped | Refused (`INDEX_REMOVED`) |
| `required` changed | Refused (`REQUIRED_CHANGE_UNSUPPORTED`) |
| `default` changed | Refused (`DEFAULT_CHANGE_UNSUPPORTED`) |
| New required field, table non-empty, no default | Refused (`REQUIRED_FIELD_NONEMPTY`) |

Platform columns are `id` (TEXT primary key, a UUID), `org_id` (integer, not
null), `created_at` and `updated_at` (timestamps, not null). A declared field may
not use those names.

Table name: `ont_<slug>`. Index names come from `TenantIndexName` and
`FieldIndexName`, which append a hash when the name would exceed the identifier
limit — creation and reconciliation call the same function, so a long slug never
re-creates its own index on every boot.

### DDL runs only in the HTTP composition

`Apply` runs when `ctx.Huma != nil`. The MCP-only composition (`octarq mcp`,
`mcp.RunWithPlugins`) has no `Huma`, no `Guard` and a nil `TenantDB`, so the
plugin registers no routes and runs no DDL there. Tools read the declaration and
answer `503 SCHEMA_NOT_APPLIED` when the bookkeeping row is missing.

Known limitation: this DDL runs after `RecordSchemaVersion`, so it is outside the
pre-migration backup and the schema-version check. The guard is "every statement
succeeds, then the bookkeeping row is written, otherwise the boot aborts".

## HTTP surface

| Method | Path | Operation |
|---|---|---|
| GET | `/api/data` | catalog, filtered by the caller's role |
| GET | `/api/data/{slug}` | list |
| POST | `/api/data/{slug}` | create |
| GET | `/api/data/{slug}/{id}` | get |
| PATCH | `/api/data/{slug}/{id}` | update |
| DELETE | `/api/data/{slug}/{id}` | delete |

Registered with `huma.Register` so they sit behind the platform middleware chain:
authentication, and any downstream tier gate reading `MetadataTier`. The path is
literal per entity and the `OperationID` carries the slug (`data_list_tickets`,
`data_get_tickets`, …). One wildcard route shared by every entity would carry a
single `OperationID` and a single tier, while a declaration sets both per
operation; the registry also refuses two declarations of the same METHOD+path, so
the literal form is the one that can express a declaration.

The request-body schema is generated from the declaration and set on the
operation before registration, so huma validates the body without a Go type per
entity. The handler decodes the raw body itself and coerces per declared type.

Body limit: 1 MiB per write.

### Values

Every value is checked against the declaration before it reaches SQL, on both
surfaces. Three rules are worth naming because the alternative is silent:

- A key sent twice in one body is refused (`422 VALUE_INVALID`). `encoding/json`
  keeps the last value and reports nothing, so the caller cannot tell which one
  was stored. The same scan runs over a declaration document.
- An `integer` beyond 2^53 is refused. An MCP client decodes arguments as a
  float64, which cannot tell two integers apart past that point; storing one
  would store a number the caller never sent.
- A `json` field accepts any JSON value. Its tool schema therefore declares no
  `type`: `object` would make the SDK refuse the array the server would store.

### Role resolution

The auth middleware authenticates the request and hands the resulting context to
the handler. The `*http.Request` the huma adapter returns still carries the
context it arrived with, so `roleForRequest` attaches the handler context before
asking the host. Without that step a bearer token — which has no cookie to fall
back to — reads as holding no role and every route answers 403.

### Tenancy

Every read, update and delete goes through `plugin.NewTenantDB(db, orgID)` with
`orgID` from the handler context. `TableScoped` adds the `org_id` predicate to
SELECT, UPDATE and DELETE; GORM drops `WHERE` on INSERT, so `create` writes
`org_id` from the handle itself.

A body that carries `id`, `org_id`, `created_at` or `updated_at` is refused with
`422 VALUE_RESERVED`. A cross-tenant read or delete answers `404`, never `403`,
so the response does not leak existence.

### Errors

| Condition | Status | Code |
|---|---|---|
| Value fails the declaration | 422 | `INVALID_VALUES` |
| Unknown `sort` field | 422 | `UNKNOWN_SORT` |
| Argument outside the platform's bounds | 422 | `INVALID_ARGUMENT` |
| Role not in the operation's ACL | 403 | `FORBIDDEN` |
| Transport carries no role | 403 | `FORBIDDEN` |
| No tenant scope | 401 | `UNAUTHENTICATED` |
| Record not found, or another tenant's record | 404 | `NOT_FOUND` |
| Unique constraint violated | 409 | `CONFLICT` |
| Bookkeeping row missing | 503 | `SCHEMA_NOT_APPLIED` |
| The host provided no database | 503 | `DATABASE_UNAVAILABLE` |

Query parameters are validated by huma before the handler runs, so a `limit`
outside 1–200 or an unknown parameter answers `400`, not `422`. That is the
platform's behaviour for every huma route, not a choice of this plugin.

A 5xx answers with a generic message and logs the underlying error. The message
is the same on both surfaces, so a driver error never becomes a response body.

## MCP surface

One tool per operation with `exposeMcp: true`. Names are
`octarq_data__<op>_<slug>` and are asserted to fit the 64-character tool-name
budget at load; a slug that does not fit refuses the boot.

The input schema is rendered separately from the OpenAPI one: MCP has no
`components` section, so the schema must be self-contained. Both renderers live
in `schema.go` so the required lists cannot drift. Defaults are typed JSON values
(`"2"` becomes `2`), because the SDK validates defaults against the schema and
panics on a mismatch.

`MCPTier()` is one tier for the whole plugin, so it reports the highest tier any
exposed operation declares. A free operation of an entity that also declares a
pro operation is therefore hidden on a free license — coarse on purpose: a
declared gate must not leak.

Roles come from `plugin.RoleFromContext(ctx)`, which answers with the bearer
token's role ceiling. An empty role is refused, and it is refused with its own
message: the stdio composition stamps a workspace but no identity, so `octarq mcp`
can never satisfy an ACL and must say that rather than blame the token.

Known limitations of this surface:

- **The stdio composition cannot use these tools.** `mcp.RunWithPlugins` stamps
  `plugin.WithOrgID` and no role, so every ACL-gated tool answers 403 there. The
  supported path for an agent is the HTTP MCP endpoint with an API token.
- **The ACL is compared with the token's cap, not the caller's live membership.**
  That is the platform's rule for every MCP tool (`requireMCPRole`), not this
  plugin's: a token minted for `admin` keeps working after its owner is demoted.
- **MCP writes are not audited.** The MCP composition has no `Context.Audit`, and
  the SPI signature takes an `*http.Request`.

## Menus

One entry per entity: id `data-<slug>`, path `/data/<slug>`, label
`pluralLabel`, icon the declared key or `database`, category `Workspace`, area
`operations`. `Menus()` runs per request and hides the entry when the caller's
role cannot `list` the entity.

No entry is published when the schema was not applied. The route behind the
entry is registered only after `Apply` succeeds, so an entry in that state would
open a 404.

The frontend serves `/data/:slug` from the shell route table; the entity's labels
are user data and are rendered literally.

## Fail-closed summary

The boot is refused when:

- the document does not parse, carries an unknown key, or declares a key twice;
- any declaration issue is reported;
- two entities share a slug;
- a declared icon is not a sidebar icon key;
- an operation's tier is not in the vocabulary;
- `requireApproval` is set;
- a tool name would exceed 64 characters;
- the declaration cannot be applied without losing data;
- a statement fails while applying.

At request time nothing is silently corrected: an unknown field, a reserved
field, a duplicate key, a wrong type, a missing required value, an unknown sort, a
page outside the bounds and a role outside the ACL all answer with an error.
