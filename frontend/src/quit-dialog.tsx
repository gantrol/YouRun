import { useEffect, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/renderer/components/ui/dialog";
import { api, streamAPI } from "./api";
import type { Messages } from "./messages";

type QuitPrompt = {
  Title: string;
  Message: string;
  Keep: string;
  Stop: string;
  Cancel: string;
  CanStop: boolean;
  StopErrorBody: string;
};

export function useQuitDialog(onRequest: () => void) {
  const [prompt, setPrompt] = useState<QuitPrompt | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef(false);
  const request = useRef(onRequest);
  request.current = onRequest;
  const trigger = useRef<HTMLElement | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    void streamAPI<{ generation?: number }>(
      "quit",
      ({ generation }) => {
        if (generation === undefined) return;
        void api("quit", "POST", { action: "received", generation }, controller.signal).then(
          () => { if (!controller.signal.aborted) request.current(); },
          () => {},
        );
      },
      () => {},
      controller.signal,
    );
    return () => controller.abort();
  }, []);

  async function prepare(origin: HTMLElement | null) {
    if (pending.current || prompt) return;
    pending.current = true;
    trigger.current = origin;
    try {
      const result = await api<{ idle: boolean; prompt: QuitPrompt }>(
        "quit",
        "POST",
        { action: "prepare" },
      );
      if (result.idle) await api("quit", "POST", { action: "keep" });
      else {
        setError("");
        setPrompt(result.prompt);
      }
    } finally {
      pending.current = false;
    }
  }
  async function confirm(action: "keep" | "stop") {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError("");
    try {
      await api("quit", "POST", { action });
    } catch {
      setError(action === "stop" ? "stopFailed" : "windowUnavailable");
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  function cancel() {
    if (pending.current) return;
    setPrompt(null);
    setError("");
    void api("quit", "POST", { action: "cancel" }).catch(() => {});
  }
  return { prompt, busy, error, trigger, prepare, confirm, cancel };
}

export function QuitDialog({
  quit,
  m,
}: {
  quit: ReturnType<typeof useQuitDialog>;
  m: Messages;
}) {
  const keep = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!quit.busy && quit.error) keep.current?.focus();
  }, [quit.busy, quit.error]);
  return (
    <Dialog
      open={!!quit.prompt}
      onOpenChange={(open) => {
        if (!open) quit.cancel();
      }}
    >
      <DialogContent
        role="alertdialog"
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          keep.current?.focus();
        }}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          quit.trigger.current?.focus();
        }}
        onEscapeKeyDown={(event) => {
          event.stopPropagation();
          if (quit.busy) event.preventDefault();
        }}
        onPointerDownOutside={(event) => {
          if (quit.busy) event.preventDefault();
        }}
      >
        <DialogTitle className="text-md font-semibold">
          {quit.prompt?.Title}
        </DialogTitle>
        <DialogDescription className="text-sm text-foreground-secondary">
          {quit.prompt?.Message}
        </DialogDescription>
        {quit.error && (
          <p role="alert" className="text-sm text-destructive">
            {quit.error === "stopFailed"
              ? quit.prompt?.StopErrorBody
              : m.windowUnavailable}
          </p>
        )}
        <div className="flex flex-wrap justify-end gap-2" aria-busy={quit.busy}>
          <Button variant="ghost" disabled={quit.busy} onClick={quit.cancel}>
            {quit.prompt?.Cancel}
          </Button>
          {quit.prompt?.CanStop && (
            <Button
              variant="outline"
              className="text-destructive"
              disabled={quit.busy}
              onClick={() => void quit.confirm("stop")}
            >
              {quit.prompt.Stop}
            </Button>
          )}
          <Button
            ref={keep}
            disabled={quit.busy}
            onClick={() => void quit.confirm("keep")}
          >
            {quit.busy ? m.quitBusy : quit.prompt?.Keep}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
