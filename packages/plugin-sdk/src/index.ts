// @octarq-org/plugin-sdk — package root (the `.` export).
//
// This entry carries ONLY the pure, app-independent plugin contract and its
// registry. The shared UI surface lives under the `./ui` subpath export so a
// consumer that only needs the contract (e.g. a build-time injection module)
// never pulls the component tree.
export * from "./contract";
