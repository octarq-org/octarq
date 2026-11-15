# @octarq/plugin-sdk

Frontend plugin SDK for Octarq. This package defines the `UIPlugin` contract, the plugin registry, and exports the shared glass-themed UI component library.

## Installation

```bash
pnpm add @octarq/plugin-sdk
```

## Features

- **Contracts**: Defines typescript interfaces for custom plugins (`UIPlugin`, `PluginMenuItem`, `UIWidget`, `UIArea`).
- **Registry**: In-memory registry to manage active plugins, routing injection, sidebar merging, and slot widgets.
- **Shared Primitives**: Accessible Tailwind/Base-UI wrappers matching the Octarq "glassmorphism" style.
- **I18n Utilities**: Translation catalogs merging with core namespaces.

## Data and hooks

`req` is the one request function for the host and every plugin. It sends the interface language, folds the backend's field details into the message, and throws `ApiError` with the HTTP status, so a page can branch on 402, 403 or 404.

```ts
import { req, ApiError, usePluginData, usePluginMutation } from "@octarq/plugin-sdk";

const domains = await req<Domain[]>("GET", "/api/x/octarq/dns/domains");

// The host app owns the react-query client; these hooks use it.
const { data } = usePluginData<Domain[]>(["domains"], "/api/x/octarq/dns/domains");
const remove = usePluginMutation<{ id: number }>({
  method: "DELETE",
  path: (vars) => `/api/x/octarq/dns/domains/${vars.id}`,
  invalidate: [["domains"]],
});
```

`configurePluginHttp({ basePath, language })` sets a path prefix and a language source. `usePluginStream` subscribes to an SSE route and aborts on unmount.

## Testing a plugin

`@octarq/plugin-sdk/testing` mounts a page under the same providers the host uses, and answers the routes a test declares.

```tsx
import { TestHarness, mockPluginApi } from "@octarq/plugin-sdk/testing";

const api = mockPluginApi([{ path: "/api/x/octarq/dns/domains", body: [] }]);
render(<TestHarness><DomainsPage /></TestHarness>);
expect(api.calls).toHaveLength(1);
api.restore();
```

## Embedding an nsgo workbench

`NsgoMount` runs an nsgo web build (`nsgo build -target web`) inside a page. It fetches the app's `nsgo.json`, imports its loader, compiles the WebAssembly once per page, and passes the host's parameters, appearance and language.

```tsx
import { NsgoMount } from "@octarq/plugin-sdk";

<NsgoMount
  src="/console/nsgo.json"
  params={{ orgId, embedded: true }}
  appearance={theme}
  locale="en"
  handlers={{ call: (name, args) => askHost(name, args) }}
  onOpenURL={(url) => navigate(url)}
/>
```

A load failure renders `fallback` (or an alert). It never renders a blank frame.
