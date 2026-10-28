import { useEffect, useRef, useState } from "react";
import { useDraftViewModel } from "./viewmodels/use-draft-view-model";
import { Download, LoaderCircle, RefreshCw } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Textarea } from "@/renderer/components/ui/textarea";
import { TextField } from "./controls";
import { api } from "./api";
import type { Messages } from "./messages";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { stripAnsiAndControl } from "./terminal-output";

export type UpdateState = {
  disabled?: boolean;
  storeManaged?: boolean;
  phase:
    | "idle"
    | "checking"
    | "downloading"
    | "building"
    | "ready"
    | "switching"
    | "complete"
    | "failed"
    | "current";
  version: string;
  build: string;
  targetVersion?: string;
  downloaded: number;
  total: number;
  error?: string;
  localSource?: string;
  localRestart?: boolean;
  buildLog?: string;
  config: { repository: string; publicKey: string };
};
export function useUpdates() {
  const [state, setState] = useState<UpdateState>();
  const [error, setError] = useState("");
  const reload = async () => {
    try {
      const next = await api<UpdateState>("updates");
      setState(previous => JSON.stringify(previous) === JSON.stringify(next) ? previous : next);
      setError("");
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "updateFailed");
    }
  };
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const next = await api<UpdateState>("updates");
        if (disposed) return;
        setState(previous => JSON.stringify(previous) === JSON.stringify(next) ? previous : next);
        setError("");
        if (next.disabled) return;
        timer = setTimeout(
          poll,
          ["checking", "downloading", "building", "switching"].includes(next.phase)
            ? 1000
            : 5000,
        );
      } catch {
        if (!disposed) {
          setError("updateFailed");
          timer = setTimeout(poll, 5000);
        }
      }
    };
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, []);
  return { state, error, reload };
}
export function UpdatesPanel({
  state,
  error,
  m,
  locale,
  onDirty,
  onReload,
  onApply,
  onRebuild,
  onSaveConfig,
}: {
  state?: UpdateState;
  error: string;
  m: Messages;
  locale: string;
  onDirty: (dirty: boolean) => void;
  onReload: () => Promise<void>;
  onApply: () => Promise<void>;
  onRebuild: () => Promise<void>;
  onSaveConfig?: (config: UpdateState["config"]) => Promise<void>;
}) {
  const editor = useDraftViewModel(
    state?.config ?? { repository: "", publicKey: "" },
  );
  const { draft: config, setDraft: setConfig, dirty } = editor;
  const [pending, setPending] = useState(false);
  const [failure, setFailure] = useState("");
  useEffect(() => {
    onDirty(dirty);
  }, [dirty, onDirty]);
  const busy =
    pending ||
    ["checking", "downloading", "building", "switching", "complete"].includes(
      state?.phase ?? "",
    );
  async function perform(action: string) {
    if (pending) return;
    setPending(true);
    setFailure("");
    try {
      if (action === "config" && editor.conflict) throw new Error("conflict");
      if (action === "apply") await onApply();
      else if (action === "rebuild") await onRebuild();
      else {
        if (action === "config" && onSaveConfig) await onSaveConfig(config);
        else await api(
          `updates/${action}`,
          "POST",
          action === "config" ? config : undefined,
        );
        if (action === "config") {
          editor.accept(config);
          onDirty(false);
        }
        await onReload();
      }
    } catch (cause) {
      setFailure(cause instanceof Error ? cause.message : "updateFailed");
    } finally {
      setPending(false);
    }
  }
  const phaseLabels = {
    idle: m.updateIdle,
    checking: m.updateChecking,
    downloading: m.updateDownloading,
    building: m.updateBuilding,
    ready: m.updateReady,
    switching: m.updateSwitching,
    complete: m.updateSwitching,
    failed: m.updateFailed,
    current: m.updateCurrent,
  };
  const message = failure || state?.error || error;
  if (state?.disabled) return <span role="status" className="text-sm text-muted-foreground">{m.updateDisabled}</span>;
  if (state?.storeManaged) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-sm tabular-nums">{state.version}</span>
        <span className="text-xs text-muted-foreground" role="status">
          {m.updateStoreManaged}
        </span>
      </div>
    );
  }
  return (
    <div className="space-y-6">
      {message && <p role="alert" className="sticky top-0 z-10 bg-surface py-2 text-sm text-destructive">
        {m[message as keyof Messages] ?? m.updateFailed}
      </p>}
      <section className="space-y-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="text-sm tabular-nums">
            {state?.version ?? "—"}{" "}
            <span className="text-xs text-muted-foreground">
              {state?.build}
            </span>
          </span>
          <span
            className="inline-flex items-center gap-2 text-xs text-muted-foreground"
            role="status"
          >
            {busy && (
              <LoaderCircle
                className="size-3.5 motion-safe:animate-spin"
                aria-hidden
              />
            )}
            {state ? phaseLabels[state.phase] : m.loading}
          </span>
        </div>
        {state?.targetVersion && (
          <p className="text-sm">
            {m.updateNewVersion} {state.targetVersion}
          </p>
        )}
        {state?.phase === "downloading" && (
          <progress
            className="h-1 w-full accent-primary"
            aria-label={m.updateDownloading}
            max={state.total || 1}
            value={state.downloaded}
          />
        )}
        {state?.phase === "downloading" && (
          <p className="text-xs tabular-nums text-muted-foreground">
            {new Intl.NumberFormat(locale, {
              style: "unit",
              unit: "megabyte",
              maximumFractionDigits: 1,
            }).format(state.downloaded / 1048576)}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          {state?.phase === "ready" ? (
            <Button
              size="sm"
              disabled={busy || dirty}
              onClick={() => void perform("apply")}
            >
              <RefreshCw className="size-4" aria-hidden />
              {m.updateApply}
            </Button>
          ) : (
            <Button
              variant="outline"
              size="sm"
              disabled={
                busy ||
                dirty ||
                !state?.config.repository ||
                !state?.config.publicKey
              }
              onClick={() => void perform("check")}
            >
              <Download className="size-4" aria-hidden />
              {m.updateCheck}
            </Button>
          )}
          <Button
            variant="ghost"
            size="sm"
            disabled={busy || dirty}
            onClick={() => void perform("local")}
          >
            {m.updateLocal}
          </Button>
          {["checking", "downloading", "building"].includes(state?.phase ?? "") && (
            <Button
              variant="ghost"
              size="sm"
              disabled={pending}
              onClick={() => void perform("cancel")}
            >
              {m.cancel}
            </Button>
          )}
        </div>
      </section>
      <Disclosure stateKey="local-build" label={m.updateRebuild} className="border-t border-border pt-4">
      <section className="space-y-3">
        <TextField
          label={m.updateLocalSource}
          value={state?.localSource ?? ""}
          readOnly
        />
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            disabled={busy || dirty || !state?.localSource}
            onClick={() => void perform("rebuild")}
          >
            <RefreshCw className="size-4" aria-hidden />
            {m.updateRebuild}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={busy || dirty}
            onClick={() => void perform("source")}
          >
            {m.chooseDirectory}
          </Button>
        </div>
        {state?.buildLog && (
          <Textarea
            readOnly
            aria-label={m.updateBuildLog}
            value={stripAnsiAndControl(state.buildLog)}
            className="h-48 font-mono text-xs"
          />
        )}
      </section>
      </Disclosure>
      <Disclosure stateKey="update-source" label={m.updateSourceSettings}
        defaultOpen={!state?.config.repository || !state?.config.publicKey} className="border-t border-border pt-4">
      <form
        className="space-y-4"
        onSubmit={(event) => {
          event.preventDefault();
          void perform("config");
        }}
      >
        <TextField
          label={m.updateRepository}
          value={config.repository}
          placeholder="owner/repository"
          disabled={busy}
          onChange={(event) =>
            setConfig({ ...config, repository: event.target.value })
          }
        />
        <TextField
          label={m.updatePublicKey}
          value={config.publicKey}
          autoComplete="off"
          spellCheck={false}
          disabled={busy}
          onChange={(event) =>
            setConfig({ ...config, publicKey: event.target.value })
          }
        />
        <Button
          size="sm"
          variant="outline"
          type="submit"
          disabled={busy || !dirty || editor.conflict}
        >
          {m.save}
        </Button>
        {editor.conflict && <div role="alert" className="space-y-2 text-sm text-destructive">
          <p>{m.conflict}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => { editor.reload(); setFailure(""); }}>{m.reload}</Button>
        </div>}
      </form>
      </Disclosure>
    </div>
  );
}
