import type { ComponentProps } from "react";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { cn } from "@/renderer/lib/utils";
import { useOverlayPortalContainer } from "./overlay-layer";
import "./dialog.css";

export const Dialog = DialogPrimitive.Root;
export const DialogTitle = DialogPrimitive.Title;
export const DialogDescription = DialogPrimitive.Description;

export function DialogContent({
  className,
  ...props
}: ComponentProps<typeof DialogPrimitive.Content>) {
  const container = useOverlayPortalContainer();
  return (
    <DialogPrimitive.Portal container={container ?? undefined}>
      <DialogPrimitive.Overlay className="yourun-dialog-overlay fixed inset-0 z-50 bg-black/20" />
      <DialogPrimitive.Content
        className={cn(
          "yourun-dialog fixed left-1/2 top-1/2 z-50 grid max-h-[calc(100dvh-2rem)] w-[calc(100%-2rem)] max-w-md -translate-x-1/2 -translate-y-1/2 gap-4 overflow-auto rounded-md border border-border bg-surface p-5 text-foreground shadow-overlay outline-none",
          className,
        )}
        {...props}
      />
    </DialogPrimitive.Portal>
  );
}
