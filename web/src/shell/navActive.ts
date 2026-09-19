// isNavItemActive matches by whole path segment, so "/mail" does not swallow
// "/maillink". Query paths ("/links?create=1") match exactly; "/" matches only
// at the root.
export function isNavItemActive(itemPath: string, currentPath: string): boolean {
  if (itemPath.includes("?")) return currentPath === itemPath;
  if (itemPath === "/") return currentPath === "/";
  return currentPath === itemPath || currentPath.startsWith(itemPath + "/");
}
