import { NavLink } from "react-router-dom";
import { Menu } from "@base-ui/react/menu";
import {
  BookOpen,
  CheckIcon,
  ExternalLink,
  Globe,
  Info,
  LogOut,
  MessageCircle,
  Moon,
  Sun,
  User,
} from "lucide-react";
import { cn } from "@octarq/plugin-sdk";
import { LANGS, useTranslation } from "../i18n";
import { toggleTheme, useTheme } from "../theme";
import { MENU_ITEM, MENU_POPUP } from "./menuStyles";
import { RESOURCES } from "./resources";

function GithubIcon({ className }: { className?: string }) {
  return (
    <svg className={className} viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
      <path d="M12 2C6.477 2 2 6.484 2 12.017c0 4.425 2.865 8.18 6.839 9.504.5.092.682-.217.682-.483 0-.237-.008-.868-.013-1.703-2.782.605-3.369-1.343-3.369-1.343-.454-1.158-1.11-1.466-1.11-1.466-.908-.62.069-.608.069-.608 1.003.07 1.531 1.032 1.531 1.032.892 1.53 2.341 1.088 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.113-4.555-4.951 0-1.093.39-1.988 1.029-2.688-.103-.253-.446-1.272.098-2.65 0 0 .84-.27 2.75 1.026A9.564 9.564 0 0112 6.844c.85.004 1.705.115 2.504.337 1.909-1.296 2.747-1.027 2.747-1.027.546 1.379.202 2.398.1 2.651.64.7 1.028 1.595 1.028 2.688 0 3.848-2.339 4.695-4.566 4.943.359.309.678.92.678 1.855 0 1.338-.012 2.419-.012 2.747 0 .268.18.58.688.482A10.019 10.019 0 0022 12.017C22 6.484 17.522 2 12 2z" />
    </svg>
  );
}

export function AccountMenu({
  user,
  onNavigate,
  onLogout,
}: {
  user: string;
  onNavigate?: () => void;
  onLogout: () => void;
}) {
  const { t, lang, setLang } = useTranslation();
  const theme = useTheme();
  const userInitials = (user || "U").slice(0, 2).toUpperCase();

  return (
    <Menu.Root>
      <Menu.Trigger
        aria-label={t("topbar.account")}
        title={user || t("topbar.account")}
        className="flex h-9 w-9 cursor-pointer items-center justify-center rounded-md text-muted-foreground transition-colors duration-100 hover:bg-sidebar-accent/70 hover:text-sidebar-accent-foreground data-[popup-open]:bg-sidebar-accent data-[popup-open]:text-sidebar-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sidebar-ring pointer-coarse:h-11 pointer-coarse:w-11"
      >
        <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full brand-gradient text-[10px] font-semibold text-white">
          {userInitials}
        </span>
      </Menu.Trigger>
      <Menu.Portal>
        <Menu.Positioner side="right" align="end" sideOffset={8} className="z-50 outline-none">
          <Menu.Popup className={cn(MENU_POPUP, "w-60")}>
            <div className="flex items-center gap-2.5 px-2.5 py-2">
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full brand-gradient text-[11px] font-semibold text-white">
                {userInitials}
              </span>
              <span className="block min-w-0 truncate text-[13px] font-medium text-foreground">{user}</span>
            </div>
            <Menu.Separator className="my-1 h-px bg-border" />
            <Menu.Item render={<NavLink to="/settings/profile" />} onClick={onNavigate} className={cn(MENU_ITEM, "text-foreground")}>
              <User className="h-4 w-4" />
              {t("topbar.personalSettings")}
            </Menu.Item>
            <Menu.Item onClick={toggleTheme} className={cn(MENU_ITEM, "text-foreground")}>
              {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
              {theme === "dark" ? t("topbar.lightMode", "Light mode") : t("topbar.darkMode", "Dark mode")}
            </Menu.Item>
            <Menu.SubmenuRoot>
              <Menu.SubmenuTrigger className={cn(MENU_ITEM, "text-foreground")}>
                <Globe className="h-4 w-4" />
                <span className="flex-1">{t("common.language")}</span>
                <span className="text-[11px] uppercase text-muted-foreground">{lang}</span>
              </Menu.SubmenuTrigger>
              <Menu.Portal>
                <Menu.Positioner side="right" align="start" sideOffset={6} className="z-50 outline-none">
                  <Menu.Popup className={cn(MENU_POPUP, "w-40")}>
                    <Menu.RadioGroup value={lang} onValueChange={(v) => setLang(v as typeof lang)}>
                      {LANGS.map((l) => (
                        <Menu.RadioItem
                          key={l.code}
                          value={l.code}
                          closeOnClick={true}
                          className="flex cursor-pointer items-center justify-between rounded-md px-2.5 py-1.5 text-xs font-medium text-muted-foreground outline-none transition-colors data-[highlighted]:bg-surface-hover data-[highlighted]:text-foreground data-[checked]:bg-accent-soft data-[checked]:text-accent-fg"
                        >
                          <span>{l.label}</span>
                          {lang === l.code && <CheckIcon className="h-3.5 w-3.5" />}
                        </Menu.RadioItem>
                      ))}
                    </Menu.RadioGroup>
                  </Menu.Popup>
                </Menu.Positioner>
              </Menu.Portal>
            </Menu.SubmenuRoot>
            <Menu.Separator className="my-1 h-px bg-border" />
            <Menu.Item render={<a href={RESOURCES.docs} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
              <BookOpen className="h-4 w-4" strokeWidth={1.75} />
              <span className="flex-1">{t("footer.docs", "Developer Docs")}</span>
              <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            </Menu.Item>
            <Menu.Item render={<a href={RESOURCES.github} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
              <GithubIcon className="h-4 w-4" />
              <span className="flex-1">{t("footer.github", "GitHub")}</span>
              <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            </Menu.Item>
            <Menu.Item render={<a href={RESOURCES.about} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
              <Info className="h-4 w-4" strokeWidth={1.75} />
              <span className="flex-1">{t("footer.about", "Website")}</span>
              <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            </Menu.Item>
            <Menu.Item render={<a href={RESOURCES.contact} target="_blank" rel="noreferrer" />} className={MENU_ITEM}>
              <MessageCircle className="h-4 w-4" strokeWidth={1.75} />
              <span className="flex-1">{t("footer.contact", "Contact")}</span>
              <ExternalLink className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            </Menu.Item>
            <Menu.Separator className="my-1 h-px bg-border" />
            <Menu.Item
              onClick={onLogout}
              className="flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2.5 py-1.5 text-left text-[13px] text-danger-fg outline-none transition-colors data-[highlighted]:bg-danger-bg data-[highlighted]:text-danger-fg"
            >
              <LogOut className="h-4 w-4" />
              {t("common.signOut")}
            </Menu.Item>
          </Menu.Popup>
        </Menu.Positioner>
      </Menu.Portal>
    </Menu.Root>
  );
}
