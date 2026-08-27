import { KeyRound, SlidersHorizontal } from "lucide-react";
import { type ReactNode } from "react";

export interface SettingsNavItem {
  href: string;
  label: string;
  icon: ReactNode;
}

// Source of truth for the settings tabs that now live on the main sidebar.
// Labels match `Sidebar.PRIMARY_NAV` so the page header agrees with the nav
// item the user clicked. Each entry also backs the SettingsPage header via
// `settingsNavItem`.
export const SETTINGS_NAV: SettingsNavItem[] = [
  { href: "/settings", label: "API keys", icon: <KeyRound size={16} /> },
  { href: "/settings/models", label: "Routing", icon: <SlidersHorizontal size={16} /> },
];

export function settingsNavItem(href: string): SettingsNavItem {
  const item = SETTINGS_NAV.find(i => i.href === href);
  if (item == null) throw new Error(`unknown settings route: ${href}`);
  return item;
}
