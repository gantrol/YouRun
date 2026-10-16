import { api, streamAPI, type Definitions, type Snapshot } from "../api";
import type { Run } from "../model";
import { persistedWorkspace, type Workspace } from "../models/workspace";

export type WorkspaceEvent = { snapshot?: Snapshot; error?: string; patch?: Partial<Omit<Snapshot, "runs">> & {
  runs?: { upserts: Run[]; removed: string[]; order: string[]; revision: number };
} };

export const workspaceRepository = {
  watch: (receive: (event: WorkspaceEvent) => void, failed: () => void, signal: AbortSignal) => streamAPI("events", receive, failed, signal),
  read: (signal?: AbortSignal) => api<Snapshot>("state", "GET", undefined, signal),
  save: (data: Workspace, revision: number) => api<Definitions>("definitions", "PUT", { ...persistedWorkspace(data), revision }),
  launch: (commandId: string, directory: string, values: Record<string, string>) => api<Run>("run", "POST", { id: crypto.randomUUID(), commandId, directory, values }),
  stop: (id: string) => api("stop", "POST", { id }),
  replay: (runId: string) => api<Run | { selfRestart: true }>("replay", "POST", { id: crypto.randomUUID(), runId }),
};

export type WorkspaceRepository = Pick<typeof workspaceRepository, "read" | "save" | "watch">;
