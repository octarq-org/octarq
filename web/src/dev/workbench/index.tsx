// Dev-only route entry. Default-exported so App.tsx can reach it with a dynamic
// import behind `import.meta.env.DEV`, which keeps the whole workbench out of a
// production build.
export { Workbench as default } from "./Workbench";
