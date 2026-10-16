import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { api } from "./api";
import type { Messages } from "./messages";

type StartupSettings = { enabled: boolean; supported: boolean; reason?: string };

export function StartupSetting({ m }: { m: Messages }) {
  const id = useId();
  const [state, setState] = useState<StartupSettings>();
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const busy = useRef(false);
  const mounted = useRef(false);
  const read = useRef<AbortController | null>(null);

  const refresh = useCallback(async () => {
    if (busy.current) return;
    read.current?.abort();
    const controller = new AbortController();
    read.current = controller;
    setLoading(true);
    try {
      const next = await api<StartupSettings>("startup", "GET", undefined, controller.signal);
      if (!controller.signal.aborted && mounted.current) {
        setState(next);
        setError("");
      }
    } catch {
      if (!controller.signal.aborted && mounted.current) {
        setState(undefined);
        setError("startupLoadFailed");
      }
    } finally {
      if (!controller.signal.aborted && mounted.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    mounted.current = true;
    void refresh();
    const onFocus = () => { void refresh(); };
    const onVisible = () => { if (document.visibilityState === "visible") void refresh(); };
    window.addEventListener("focus", onFocus);
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      mounted.current = false;
      read.current?.abort();
      window.removeEventListener("focus", onFocus);
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [refresh]);

  async function change(enabled: boolean) {
    if (busy.current || loading || !state?.supported) return;
    busy.current = true;
    setSaving(true);
    setError("");
    try {
      const next = await api<StartupSettings>("startup", "PUT", { enabled });
      if (mounted.current) setState(next);
    } catch (cause) {
      // A response can be lost after the OS has committed. Reconcile without
      // displaying the requested value as a successful save.
      try {
        const next = await api<StartupSettings>("startup");
        if (mounted.current) setState(next);
      } catch {
        if (mounted.current) setState(undefined);
      }
      if (mounted.current) setError(cause instanceof Error ? cause.message : "startupSaveFailed");
    } finally {
      busy.current = false;
      if (mounted.current) setSaving(false);
    }
  }

  const message = error || state?.reason;
  return <div className="space-y-2" aria-busy={loading || saving}>
    <label htmlFor={id} className="flex min-h-9 items-center gap-2">
      <Checkbox id={id} checked={state?.enabled ?? false} aria-label={m.startupAtLogin}
        aria-describedby={message ? `${id}-message` : undefined}
        disabled={loading || saving || !state?.supported}
        onCheckedChange={(checked) => { void change(checked === true); }} />
      <span>{loading ? m.loading : saving ? m.saving : state?.enabled ? m.startupOn : m.startupOff}</span>
    </label>
    {message && <span id={`${id}-message`} role={error ? "alert" : "status"}
      className={error ? "block text-xs text-destructive" : "block text-xs text-muted-foreground"}>
      {m[message as keyof Messages] ?? m.startupSaveFailed}
    </span>}
    {error && !loading && !saving && <Button variant="outline" size="sm" onClick={() => { void refresh(); }}>{m.retry}</Button>}
  </div>;
}
