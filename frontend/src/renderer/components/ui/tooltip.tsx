import type { ComponentProps } from "react";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { cn } from "@/renderer/lib/utils";
import { useOverlayPortalContainer } from "./overlay-layer";
import "./context-menu.css";

export const TooltipProvider = TooltipPrimitive.Provider;
export const TooltipTrigger = TooltipPrimitive.Trigger;

export function Tooltip(props: ComponentProps<typeof TooltipPrimitive.Root>) {
  return (
    <TooltipProvider delayDuration={350}>
      <TooltipPrimitive.Root {...props} />
    </TooltipProvider>
  );
}

export function TooltipContent({
  className,
  sideOffset = 6,
  collisionPadding = 8,
  ...props
}: ComponentProps<typeof TooltipPrimitive.Content>) {
  const container = useOverlayPortalContainer();
  return (
    <TooltipPrimitive.Portal container={container ?? undefined}>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        collisionPadding={collisionPadding}
        className={cn(
          "runshelf-context-menu pointer-events-auto z-modal-nested max-w-[min(20rem,var(--radix-tooltip-content-available-width))] break-words rounded-sm border border-border bg-overlay px-2.5 py-1.5 text-xs text-foreground shadow-overlay",
          className,
        )}
        {...props}
      />
    </TooltipPrimitive.Portal>
  );
}
