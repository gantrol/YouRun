import { useRef } from "react";
import { Button } from "@/renderer/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/renderer/components/ui/dialog";
import type { Messages } from "./messages";

export function UnsavedChangesDialog({ open, m, onContinue, onDiscard, onRestoreFocus }: {
  open: boolean;
  m: Messages;
  onContinue: () => void;
  onDiscard: () => void;
  onRestoreFocus: () => void;
}) {
  const continueButton = useRef<HTMLButtonElement>(null);
  return (
    <Dialog open={open} onOpenChange={(value) => { if (!value) onContinue(); }}>
      <DialogContent
        role="alertdialog"
        aria-describedby={undefined}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          continueButton.current?.focus({ preventScroll: true });
        }}
        onCloseAutoFocus={(event) => { event.preventDefault(); onRestoreFocus(); }}
        onEscapeKeyDown={(event) => event.stopPropagation()}
        onPointerDownOutside={(event) => event.preventDefault()}
      >
        <DialogTitle className="text-base font-semibold">{m.unsaved}</DialogTitle>
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="outline" onClick={onDiscard}>{m.discard}</Button>
          <Button ref={continueButton} onClick={onContinue}>{m.continueEditing}</Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
