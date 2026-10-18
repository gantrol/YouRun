import { useCallback, useEffect, useId, useRef, useState } from "react";
import { Minus, Plus } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/renderer/components/ui/collapsible";
import { useViewToggle } from "@/renderer/components/ui/view-state";
import { api } from "./api";
import type { Messages } from "./messages";

type StartupSettings = { enabled: boolean; supported: boolean; reason?: string };

export type AgentIntegrationTarget = {
  id: string;
  name: string;
  kind: "path" | "skill" | "mcp";
  path: string;
  detected: boolean;
  configured: boolean;
  note?: string;
};

export type AgentIntegrationSettings = {
  enabled: boolean;
  supported: boolean;
  reason?: string;
  executable?: string;
  cliWrapper?: string;
  targets?: AgentIntegrationTarget[];
};

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

export function AgentIntegrationSetting({ m }: { m: Messages }) {
  const baseId = useId();
  const [open, setOpen] = useViewToggle("agent-integration-section", true);
  const [state, setState] = useState<AgentIntegrationSettings>();
  const [loading, setLoading] = useState(true);
  const [savingTarget, setSavingTarget] = useState<string>("");
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
      const next = await api<AgentIntegrationSettings>("agent-integration", "GET", undefined, controller.signal);
      if (!controller.signal.aborted && mounted.current) {
        setState(next);
        setError("");
      }
    } catch {
      if (!controller.signal.aborted && mounted.current) {
        setState(undefined);
        setError("agentIntegrationLoadFailed");
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

  async function mutateTargets(targetIds: string[], enabled: boolean, busyKey: string) {
    if (busy.current || loading || (enabled && !state?.supported)) return;
    busy.current = true;
    setSavingTarget(busyKey);
    setError("");
    try {
      const next = await api<AgentIntegrationSettings>("agent-integration", "PUT", {
        enabled,
        targets: targetIds,
      });
      if (mounted.current) setState(next);
    } catch (cause) {
      try {
        const next = await api<AgentIntegrationSettings>("agent-integration");
        if (mounted.current) setState(next);
      } catch {
        if (mounted.current) setState(undefined);
      }
      if (mounted.current) setError(cause instanceof Error ? cause.message : "agentIntegrationSaveFailed");
    } finally {
      busy.current = false;
      if (mounted.current) setSavingTarget("");
    }
  }

  const targets = state?.targets ?? [];
  const configuredCount = targets.filter((t) => t.configured).length;
  const saving = savingTarget !== "";
  const message = error || state?.reason;

  return (
    <Collapsible open={open} onOpenChange={setOpen} asChild>
      <section className="py-3" aria-busy={loading || saving}>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="min-w-0 flex-1">
            <CollapsibleTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-auto min-h-9 w-full justify-start px-1 text-left text-sm font-semibold whitespace-normal"
                data-navigation-key="settings:agent-integration-section"
              >
                {open ? (
                  <Minus className="size-4 text-muted-foreground" aria-hidden="true" />
                ) : (
                  <Plus className="size-4 text-muted-foreground" aria-hidden="true" />
                )}
                <span className="min-w-0 break-words">{m.agentIntegration}</span>
                <span className="shrink-0 text-xs font-normal tabular-nums text-muted-foreground">
                  {loading ? m.loading : `${configuredCount}/${targets.length}`}
                </span>
              </Button>
            </CollapsibleTrigger>
          </h2>
          {configuredCount > 0 && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              disabled={loading || saving}
              onClick={() => { void mutateTargets(["all"], false, "all"); }}
              className="text-muted-foreground hover:text-destructive"
              data-navigation-key="settings:agent-integration-revoke-all"
            >
              {savingTarget === "all" ? m.saving : m.agentIntegrationRevokeAll}
            </Button>
          )}
        </div>
        <CollapsibleContent>
          <div className="space-y-2 pt-2">
            {message && (
              <span
                id={`${baseId}-message`}
                role={error ? "alert" : "status"}
                className={error ? "block px-2 text-xs text-destructive" : "block px-2 text-xs text-muted-foreground"}
              >
                {m[message as keyof Messages] ?? m.agentIntegrationSaveFailed}
              </span>
            )}
            {error && !loading && !saving && (
              <div className="px-2">
                <Button variant="outline" size="sm" onClick={() => { void refresh(); }}>
                  {m.retry}
                </Button>
              </div>
            )}
            {targets.length > 0 && (
              <ul className="divide-y divide-border">
                {targets.map((target) => {
                  const checkboxId = `${baseId}-${target.id}`;
                  const itemBusy = savingTarget === target.id || savingTarget === "all";
                  return (
                    <li
                      key={target.id}
                      className="flex min-h-11 items-center justify-between gap-2 px-2 py-2"
                    >
                      <label
                        htmlFor={checkboxId}
                        className="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5"
                      >
                        <Checkbox
                          id={checkboxId}
                          checked={target.configured}
                          aria-label={target.name}
                          disabled={loading || saving || (!target.configured && !state?.supported)}
                          onCheckedChange={(checked) => {
                            void mutateTargets([target.id], checked === true, target.id);
                          }}
                        />
                        <span className="min-w-0 flex-1">
                          <span className="flex min-w-0 items-center gap-2">
                            <span className="truncate text-sm">{target.name}</span>
                            <span
                              className="hidden min-w-0 truncate font-mono text-xs text-muted-foreground @min-[36rem]/workspace:inline"
                              title={target.path}
                            >
                              {target.path}
                            </span>
                          </span>
                        </span>
                      </label>
                      {target.configured ? (
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={loading || saving}
                          onClick={() => { void mutateTargets([target.id], false, target.id); }}
                          className="shrink-0 text-xs text-muted-foreground hover:text-destructive"
                        >
                          {itemBusy ? m.saving : m.agentIntegrationRevoke}
                        </Button>
                      ) : (
                        <span className="shrink-0 px-2 text-xs text-muted-foreground">
                          {itemBusy ? m.saving : m.agentIntegrationOff}
                        </span>
                      )}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </CollapsibleContent>
      </section>
    </Collapsible>
  );
}
