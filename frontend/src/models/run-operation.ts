import type { Run } from "../model";

export type RunOperationState = "starting" | "restarting" | "stopping";
export type RunOperation = {
  state: RunOperationState;
  previousRunId?: string;
  runId?: string;
};
export type RunOperations = Readonly<Record<string, RunOperation | undefined>>;

/** Pending requests are presentation state; a confirmed result always takes priority. */
export function displayedRunState(run?: Run, operation?: RunOperation): Run["status"] | RunOperationState | undefined {
  if (!operation) return run?.status;
  if (operation.state === "stopping") {
    return run && (run.id !== operation.runId || run.status !== "running") ? run.status : "stopping";
  }
  return run && run.id !== operation.previousRunId && run.id !== operation.runId ? run.status : operation.state;
}
