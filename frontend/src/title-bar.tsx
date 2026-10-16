import type { ReactNode, Ref } from "react";
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
  actions?: ReactNode;
  controls?: ReactNode;
  mac?: boolean;
  fullscreen?: boolean;
}) {
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
      {brand}
      <h1
        ref={headingRef}
        tabIndex={-1}
        title={title}
        className="min-w-0 flex-1 truncate text-base font-semibold outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        {title}
      </h1>
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
