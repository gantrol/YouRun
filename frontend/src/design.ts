import type { SkinId } from "./appearance";

// Skins change presentation, never command identity, layout or run behavior.
export const launcherSkins: Record<
  SkinId,
  { title: string; badge: string; action: string }
> = {
  classic: {
    title: "font-semibold",
    badge: "rounded-sm bg-surface-sunken text-foreground-secondary",
    action: "rounded-sm border-border bg-surface text-foreground-secondary",
  },
  comic: {
    title: "font-extrabold tracking-tight",
    badge:
      "-rotate-3 rounded-sm border-2 border-border-strong bg-surface font-black shadow-[2px_2px_0_var(--border-strong)]",
    action:
      "rounded-sm border-2 border-border-strong bg-selected text-selected-foreground shadow-[2px_2px_0_var(--border-strong)] active:shadow-none",
  },
  graphic: {
    title: "font-bold tracking-tight",
    badge:
      "rounded-none border-b-2 border-border-strong bg-surface font-bold text-foreground",
    action: "rounded-none border-border-strong bg-surface text-foreground",
  },
  pixel: {
    title: "font-mono font-semibold",
    badge:
      "rounded-none border-2 border-border-strong bg-selected font-mono shadow-[2px_2px_0_var(--border-strong)]",
    action:
      "rounded-none border-2 border-border-strong bg-selected text-selected-foreground shadow-[2px_2px_0_var(--border-strong)] active:shadow-none",
  },
};
