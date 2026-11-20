
import { mount } from "@cloudflare/nimbus-docs/client";

declare global {
  interface Window {
    __nbApplyTheme?: () => void;
  }
}

function initThemeToggle(button: HTMLElement): () => void {
  function handleClick() {
    const isDark = document.documentElement.getAttribute("data-mode") === "dark";
    try {
      localStorage.setItem("ui-mode", isDark ? "light" : "dark");
    } catch {
    }
    window.__nbApplyTheme?.();
  }

  window.__nbApplyTheme?.();
  button.addEventListener("click", handleClick);
  return () => button.removeEventListener("click", handleClick);
}

mount("[data-nb-theme-toggle]", initThemeToggle);
