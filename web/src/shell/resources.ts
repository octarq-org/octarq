// octarq-provided resources — the same product links a standard SaaS keeps
// within reach. These are octarq's, not the org's, so they surface in the
// sidebar footer and the topbar help menu, kept strictly apart from the org's
// own business nav.
//
// One table, imported by both. TopBar and AreaPanel each carried their own copy
// and they had already drifted — only one listed `contact`, and both pointed
// `docs` at a URL that 404s.
//
// The hosts are split: octarq.org is the marketing site (landing, pricing,
// contact, legal) and docs.octarq.org is the documentation. A docs path on the
// apex is a 404 — that is what these links used to be.
export const RESOURCES = {
  docs: "https://docs.octarq.org/what-is-octarq/",
  about: "https://octarq.org",
  github: "https://github.com/octarq-org/octarq",
  // Trailing slash: the page is built as contact/index.html, so the bare path
  // only works by way of a redirect.
  contact: "https://octarq.org/contact/",
};
