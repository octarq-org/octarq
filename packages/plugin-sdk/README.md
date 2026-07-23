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
