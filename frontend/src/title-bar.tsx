import { useEffect, useRef, type ReactNode, type Ref } from "react";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import type { Messages } from "./messages";
import { YouRunLogo } from "./yourun-logo";
import "./title-bar.css";

export function TitleBar({
  title,
  m,
  brand = <YouRunLogo size={24} className="shrink-0" />,
  onBack,
  backDisabled = false,
  headingRef,
  mode,
  onModeChange,
  modeDisabled = false,
  actions,
  controls,
  mac = false,
  fullscreen = false,
}: {
  title: string;
  m: Messages;
  brand?: ReactNode;
  onBack?: () => void;
  backDisabled?: boolean;
  headingRef?: Ref<HTMLHeadingElement>;
  mode?: "launch" | "pro";
  onModeChange?: (mode: "launch" | "pro") => void;
  modeDisabled?: boolean;
  actions?: ReactNode;
  controls?: ReactNode;
  mac?: boolean;
  fullscreen?: boolean;
}) {
  const clickCountRef = useRef(0);
  const clickTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const switchedToProAtRef = useRef(0);

  useEffect(() => {
    clickCountRef.current = 0;
    clearTimeout(clickTimerRef.current);
  }, [mode, title]);

  useEffect(() => {
    return () => clearTimeout(clickTimerRef.current);
  }, []);

  const displayedTitle = mode === "pro" ? "iRun" : title;
  const canSwitchMode =
    !!onModeChange &&
    !modeDisabled &&
    (mode === "pro" || (!onBack && title === m.productName));

  function handleBrandTitleClick() {
    if (!canSwitchMode || !onModeChange) return;
    if (mode === "pro") {
      if (Date.now() - switchedToProAtRef.current < 350) return;
      clickCountRef.current = 0;
      clearTimeout(clickTimerRef.current);
      onModeChange("launch");
      return;
    }
    clearTimeout(clickTimerRef.current);
    const nextCount = clickCountRef.current + 1;
    if (nextCount >= 3) {
      clickCountRef.current = 0;
      switchedToProAtRef.current = Date.now();
      onModeChange("pro");
      return;
    }
    clickCountRef.current = nextCount;
    clickTimerRef.current = setTimeout(() => {
      clickCountRef.current = 0;
    }, 600);
  }

  return (
    <header
      data-window-drag
      className={`command-title-bar flex h-11 shrink-0 select-none items-center gap-2 border-b border-border bg-background ${mac && !fullscreen ? "pl-24" : "pl-3"}`}
    >
      {onBack && (
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={onBack}
          disabled={backDisabled}
          aria-label={m.back}
          title={m.back}
          className="text-muted-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
        </Button>
      )}
      <div
        data-window-no-drag={canSwitchMode ? "" : undefined}
        onClick={canSwitchMode ? handleBrandTitleClick : undefined}
        className={`flex min-w-0 items-center gap-2 ${canSwitchMode ? "cursor-pointer" : ""}`}
      >
        {brand}
        <h1
          ref={headingRef}
          tabIndex={-1}
          title={displayedTitle}
          className="min-w-0 truncate text-base font-semibold outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {displayedTitle}
        </h1>
      </div>
      <div className="min-w-0 flex-1" />
      <div
        data-window-no-drag
        className="flex shrink-0 items-center gap-1 pr-2"
      >
        {actions}
      </div>
      {controls}
    </header>
  );
}
