import { useEffect, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import type { Messages } from "./messages";
import { api } from "./api";
import { installWailsDrag } from "./window-drag";

type WindowState = {
  framework: string;
  platform: string;
  maximized: boolean;
  fullscreen: boolean;
};
export type WindowAction = "minimize" | "toggleMaximize" | "close";

// Codicons chrome-* glyphs, CC BY 4.0; attribution in THIRD-PARTY-NOTICES.txt.
// Preserve their individual optical proportions; filled paths avoid skin stroke overrides.
const windowIconPaths = {
  minimize: "M3 7.5C3 7.22386 3.22386 7 3.5 7H12.5C12.7761 7 13 7.22386 13 7.5C13 7.77614 12.7761 8 12.5 8H3.5C3.22386 8 3 7.77614 3 7.5Z",
  maximize: "M2 4.5C2 3.11929 3.11929 2 4.5 2H11.5C12.8807 2 14 3.11929 14 4.5V11.5C14 12.8807 12.8807 14 11.5 14H4.5C3.11929 14 2 12.8807 2 11.5V4.5ZM4.5 3C3.67157 3 3 3.67157 3 4.5V11.5C3 12.3284 3.67157 13 4.5 13H11.5C12.3284 13 13 12.3284 13 11.5V4.5C13 3.67157 12.3284 3 11.5 3H4.5Z",
  restore: "M5.08496 4C5.29088 3.4174 5.8465 3 6.49961 3H9.99961C11.6565 3 12.9996 4.34315 12.9996 6V9.5C12.9996 10.1531 12.5822 10.7087 11.9996 10.9146V6C11.9996 4.89543 11.1042 4 9.99961 4H5.08496ZM4.5 5H9.5C10.3284 5 11 5.67157 11 6.5V11.5C11 12.3284 10.3284 13 9.5 13H4.5C3.67157 13 3 12.3284 3 11.5V6.5C3 5.67157 3.67157 5 4.5 5ZM4.5 6C4.22386 6 4 6.22386 4 6.5V11.5C4 11.7761 4.22386 12 4.5 12H9.5C9.77614 12 10 11.7761 10 11.5V6.5C10 6.22386 9.77614 6 9.5 6H4.5Z",
  close: "M2.58859 2.71569L2.64645 2.64645C2.82001 2.47288 3.08944 2.4536 3.28431 2.58859L3.35355 2.64645L8 7.293L12.6464 2.64645C12.8417 2.45118 13.1583 2.45118 13.3536 2.64645C13.5488 2.84171 13.5488 3.15829 13.3536 3.35355L8.707 8L13.3536 12.6464C13.5271 12.82 13.5464 13.0894 13.4114 13.2843L13.3536 13.3536C13.18 13.5271 12.9106 13.5464 12.7157 13.4114L12.6464 13.3536L8 8.707L3.35355 13.3536C3.15829 13.5488 2.84171 13.5488 2.64645 13.3536C2.45118 13.1583 2.45118 12.8417 2.64645 12.6464L7.293 8L2.64645 3.35355C2.47288 3.17999 2.4536 2.91056 2.58859 2.71569L2.64645 2.64645L2.58859 2.71569Z",
} as const;

function WindowControlIcon({ action, maximized }: { action: WindowAction; maximized: boolean }) {
  const icon = action === "toggleMaximize" ? maximized ? "restore" : "maximize" : action;
  return (
    <svg viewBox="0 0 16 16" className="size-4" fill="currentColor" stroke="none"
      aria-hidden="true" focusable="false">
      <path d={windowIconPaths[icon]} />
    </svg>
  );
}

export function useWindowControls() {
  const [state, setState] = useState<WindowState>({
    framework: "",
    platform: /Mac/.test(navigator.platform) ? "darwin" : "windows",
    maximized: false,
    fullscreen: false,
  });
  const refreshRef = useRef<() => void>(() => {});
  useEffect(() => {
    if (state.framework === "wails") return installWailsDrag();
  }, [state.framework]);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    let pending = false;
    async function refresh() {
      if (pending) return;
      pending = true;
      try {
        const next = await api<WindowState>("window");
        if (!disposed) setState(next);
      } catch {
        /* Keep native controls available during a transient read failure. */
      } finally {
        pending = false;
      }
    }
    const schedule = () => {
      clearTimeout(timer);
      timer = setTimeout(refresh, 100);
    };
    refreshRef.current = schedule;
    void refresh();
    window.addEventListener("resize", schedule);
    window.addEventListener("focus", schedule);
    return () => {
      disposed = true;
      clearTimeout(timer);
      window.removeEventListener("resize", schedule);
      window.removeEventListener("focus", schedule);
    };
  }, []);
  async function perform(action: WindowAction) {
    await api("window", "POST", { action });
    refreshRef.current();
  }
  return { ...state, perform };
}

export function WindowControls({
  m,
  maximized,
  onAction,
}: {
  m: Messages;
  maximized: boolean;
  onAction: (action: WindowAction) => void;
}) {
  const entries = [
    { action: "minimize", label: m.windowMinimize },
    {
      action: "toggleMaximize",
      label: maximized ? m.windowRestore : m.windowMaximize,
    },
    { action: "close", label: m.windowClose },
  ] as const;
  return (
    <div
      data-window-no-drag
      className="flex h-full shrink-0"
      role="group"
      aria-label={m.windowControls}
    >
      {entries.map(({ action, label }) => (
        <Button
          key={action}
          variant="ghost"
          size="icon"
          aria-label={label}
          title={label}
          className={`size-11 rounded-none text-muted-foreground focus-visible:ring-inset focus-visible:ring-offset-0 ${action === "close" ? "hover:bg-destructive hover:text-destructive-foreground active:bg-destructive-hover" : "hover:text-foreground"}`}
          onClick={() => onAction(action)}
        >
          <WindowControlIcon action={action} maximized={maximized} />
        </Button>
      ))}
    </div>
  );
}
