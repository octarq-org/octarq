export function isNavItemActive(itemPath: string, currentPath: string): boolean {
  if (itemPath.includes("?")) return currentPath === itemPath;
  if (itemPath === "/") return currentPath === "/";
  return currentPath === itemPath || currentPath.startsWith(itemPath + "/");
}
