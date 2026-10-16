import { useRef, useState } from "react";
import type { Command, Run } from "../model";
import { workspaceRepository } from "../repositories/workspace-repository";
import type { RunOperation, RunOperations, RunOperationState } from "../models/run-operation";

type Options = {
  runs: Run[];
  flush: () => Promise<void>;
  refresh: () => Promise<void>;
  setIssue: (issue: string) => void;
  beforeReplay: () => void;
  onSelfRestart: () => Promise<void>;
  onReplay: (previous: string, next: string) => void;
};

export function useRunViewModel(options: Options) {
  const latest = useRef(options);
  latest.current = options;
  const inFlight = useRef(new Map<string, RunOperation>());
  const [runOperations, setRunOperations] = useState<RunOperations>({});
  const [failures, setFailures] = useState<Record<string, { code: string; previousRunId?: string }>>({});
  const launchErrors = Object.fromEntries(Object.entries(failures)
    .filter(([id, failure]) => options.runs.find((run) => run.commandId === id)?.id === failure.previousRunId)
    .map(([id, failure]) => [id, failure.code]));
  function clearFailure(id: string) {
    setFailures((previous) => { const next = { ...previous }; delete next[id]; return next; });
  }
  async function perform(commandId: string, state: RunOperationState, action: () => Promise<void>, runId?: string, reuseActive = false) {
    if (inFlight.current.has(commandId)) return false;
    const previousRunId = latest.current.runs.find((run) => run.commandId === commandId)?.id;
    inFlight.current.set(commandId, { state, previousRunId, runId });
    setRunOperations(Object.fromEntries(inFlight.current));
    clearFailure(commandId);
    try {
      if (state !== "stopping") await latest.current.flush();
      await action();
      latest.current.setIssue("");
      await latest.current.refresh();
      return true;
    } catch (error) {
      const code = error instanceof Error ? error.message : state === "stopping" ? "stopFailed" : "startFailed";
      if (code === "alreadyRunning" && reuseActive) {
        // Another window (or a lost start response) can make the UI stale.
        // Keep the existing execution and reconcile its state without rerunning.
        await latest.current.refresh();
        return true;
      }
      if (code === "alreadyRunning") await latest.current.refresh();
      setFailures((previous) => ({ ...previous, [commandId]: { code, previousRunId } }));
      return false;
    } finally {
      inFlight.current.delete(commandId);
      setRunOperations(Object.fromEntries(inFlight.current));
    }
  }
  function launch(command: Command, directory: string, values: Record<string, string>) {
    if (command.unavailable || !directory.trim() || command.fields.some((field) => !values[field]?.trim())) return Promise.resolve(false);
    return perform(command.id, "starting", async () => { await workspaceRepository.launch(command.id, directory, values); }, undefined, true);
  }
  async function stopRun(id: string) {
    const run = latest.current.runs.find((run) => run.id === id);
    if (!run || run.status !== "running") return;
    await perform(run.commandId, "stopping", async () => { await workspaceRepository.stop(id); }, id);
  }
  async function replayRun(id: string) {
    const previous = latest.current.runs.find((run) => run.id === id);
    if (!previous || previous.status === "stopping") return;
    await perform(previous.commandId, previous.status === "running" ? "restarting" : "starting", async () => {
      latest.current.beforeReplay();
      const run = await workspaceRepository.replay(id);
      if ("selfRestart" in run) await latest.current.onSelfRestart();
      else latest.current.onReplay(id, run.id);
    }, id);
  }
  return { runOperations, launchErrors, launch, stopRun, replayRun, clearFailure };
}
