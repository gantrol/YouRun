import { onThisDevice, fingerprint, mergeWorkspace, persistedWorkspace, type Definitions, type Snapshot, type Workspace } from "../models/workspace";
import { workspaceRepository, type WorkspaceRepository, type WorkspaceEvent } from "../repositories/workspace-repository";

type State = Snapshot & { saving: boolean; saveError: string; issue: string };
function shareRows<T extends { id: string }>(previous: T[], next: T[]): T[] {
  if (previous === next) return previous;
  const byId = new Map(previous.map(row => [row.id, row]));
  const rows = next.map(row => { const old = byId.get(row.id); return old && fingerprint(old) === fingerprint(row) ? old : row; });
  return rows.length === previous.length && rows.every((row, i) => row === previous[i]) ? previous : rows;
}
type Intent = { base: Workspace; desired: Workspace };

// UI-independent presentation state. SQLite snapshots are the only published
// domain state; drafts and pending operations never masquerade as saved data.
export class WorkspaceViewModel {
  private state: State;
  private listeners = new Set<() => void>();
  private queue: Promise<void> = Promise.resolve();
  private pending = 0;
  private generation = 0;
  private readSequence = 0;
  private failed?: Intent;

  constructor(initial: Snapshot, private repository: WorkspaceRepository = workspaceRepository) {
    this.state = { ...initial, definitions: this.present(initial.definitions, initial.platform), saving: false, saveError: "", issue: initial.storageError ? "storageFailed" : "" };
  }
  private present(data: Definitions, platform = this.state.platform): Definitions {
    const old = this.state?.definitions;
    if (old && data.revision === old.revision && platform === this.state.platform) return old;
    const commands = data.commands.map(command => onThisDevice(command, platform));
    return { ...data, collections: old ? shareRows(old.collections ?? [], data.collections ?? []) : data.collections ?? [], commands: old ? shareRows(old.commands, commands) : commands, projects: old ? shareRows(old.projects, data.projects) : data.projects };
  }
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private publish(change: Partial<State>) {
    if (Object.entries(change).every(([key, value]) => Object.is(this.state[key as keyof State], value))) return;
    this.state = { ...this.state, ...change };
    for (const listener of this.listeners) listener();
  }
  private accept(snapshot: Snapshot) {
    const current = this.state;
    this.publish({ ...snapshot,
      definitions: snapshot.definitions.revision < current.definitions.revision ? current.definitions : this.present(snapshot.definitions, snapshot.platform),
      runs: (snapshot.runsRevision ?? 0) < (current.runsRevision ?? 0) ? current.runs : shareRows(current.runs, snapshot.runs),
      runsRevision: Math.max(snapshot.runsRevision ?? 0, current.runsRevision ?? 0),
      metadataRevision: Math.max(snapshot.metadataRevision, current.metadataRevision),
      settingsRevision: Math.max(snapshot.settingsRevision, current.settingsRevision),
      metadataKeys: snapshot.metadataRevision < current.metadataRevision || fingerprint(snapshot.metadataKeys) === fingerprint(current.metadataKeys) ? current.metadataKeys : snapshot.metadataKeys,
      issue: snapshot.storageError ? "storageFailed" : ["connectionLost", "storageFailed"].includes(this.state.issue) ? "" : this.state.issue });
  }
  connect = (signal: AbortSignal) => this.repository.watch(event => this.receive(event), () => this.setIssue("connectionLost"), signal);
  private receive(event: WorkspaceEvent) {
    if (event.error) { this.setIssue(event.error); return; }
    if (event.snapshot) {
      if (this.pending) { this.reconcileAfterSave = true; return; }
      this.readSequence++;
      this.accept(event.snapshot); return;
    }
    if (!event.patch) return;
    this.readSequence++;
    const { runs, definitions, ...rest } = event.patch;
    const change: Partial<State> = { ...rest };
    if (rest.metadataRevision !== undefined && rest.metadataRevision < this.state.metadataRevision) { delete change.metadataRevision; delete change.metadataKeys; }
    if (rest.settingsRevision !== undefined && rest.settingsRevision < this.state.settingsRevision) delete change.settingsRevision;
    if (definitions && definitions.revision >= this.state.definitions.revision) {
      if (this.pending) this.reconcileAfterSave = true;
      else { this.readSequence++; change.definitions = this.present(definitions); }
    }
    if (runs && runs.revision >= (this.state.runsRevision ?? 0)) {
      const rows = new Map(this.state.runs.map(run => [run.id, run]));
      for (const id of runs.removed) rows.delete(id);
      for (const run of runs.upserts) rows.set(run.id, run);
      const next = runs.order.map(id => rows.get(id));
      if (next.some(run => !run)) { if (this.pending) this.reconcileAfterSave = true; else void this.refresh(); return; }
      change.runs = shareRows(this.state.runs, next as typeof this.state.runs);
      change.runsRevision = runs.revision;
    }
    if (this.state.issue === "connectionLost") change.issue = "";
    this.publish(change);
  }
  private reconcileAfterSave = false;
  setIssue = (value: string | ((old: string) => string)) => this.publish({ issue: typeof value === "function" ? value(this.state.issue) : value });
  refresh = async (signal?: AbortSignal) => {
    const generation = this.generation;
    const sequence = ++this.readSequence;
    try {
      const snapshot = await this.repository.read(signal);
      if (signal?.aborted || this.pending || generation !== this.generation || sequence !== this.readSequence) return;
      this.accept(snapshot);
    } catch (error) {
      if (!signal?.aborted && generation === this.generation && sequence === this.readSequence) this.setIssue(error instanceof Error ? error.message : "connectionLost");
    }
  };
  save = (change: (current: Workspace) => Workspace): Promise<void> => {
    const base = persistedWorkspace(this.state.definitions);
    const intent = { base, desired: persistedWorkspace(change(this.state.definitions)) };
    return this.enqueue(intent);
  };
  private enqueue(intent: Intent): Promise<void> {
    this.pending++;
    this.generation++;
    this.publish({ saving: true, saveError: "" });
    const task = this.queue.then(async () => {
      try {
        for (let attempt = 0; attempt < 3; attempt++) {
          const remote = await this.repository.read();
          this.accept(remote);
          if (remote.storageError) throw new Error("storageFailed");
          const next = mergeWorkspace(intent.base, intent.desired, persistedWorkspace(remote.definitions));
          if (fingerprint(next) === fingerprint(persistedWorkspace(remote.definitions))) { this.failed = undefined; return; }
          try {
            const saved = await this.repository.save(next, remote.definitions.revision);
            this.publish({ definitions: this.present(saved), saveError: "" });
            this.failed = undefined;
            return;
          } catch (error) {
            if (!(error instanceof Error) || error.message !== "conflict" || attempt === 2) throw error;
          }
        }
      } catch (error) {
        this.failed = intent;
        // A conflict or lost response always reconciles against the database.
        try { this.accept(await this.repository.read()); } catch { /* Keep the last confirmed snapshot. */ }
        this.publish({ saveError: error instanceof Error ? error.message : "storageFailed" });
        throw error;
      } finally {
        this.pending--;
        this.generation++;
        this.publish({ saving: this.pending > 0 });
        if (!this.pending && this.reconcileAfterSave) { this.reconcileAfterSave = false; await this.refresh(); }
      }
    });
    this.queue = task.catch(() => {});
    return task;
  }
  flush = async () => {
    await this.queue;
    if (this.state.saveError) throw new Error(this.state.saveError);
  };
  retrySave = async () => {
    await this.queue;
    if (this.failed && this.state.saveError !== "conflict") return this.enqueue(this.failed);
    await this.refresh();
    this.failed = undefined;
    this.publish({ saveError: "" });
  };
}
