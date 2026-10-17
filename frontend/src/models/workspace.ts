import type { Collection, Command, Project, Run } from "../model";
import { validateCollectionTree } from "../collections";

export type Definitions = {
  revision: number;
  locale: string;
  projects: Project[];
  commands: Command[];
  collections?: Collection[];
};
export type Snapshot = {
  runsRevision?: number;
  metadataKeys?: Record<string, string>;
  protocol: number;
  platform: string;
  definitions: Definitions;
  runs: Run[];
  storageError: boolean;
  metadataRevision: number;
  settingsRevision: number;
};
export function onThisDevice(command: Command, platform: string): Command {
  return {
    ...command,
    unavailable:
      (command.platform === "macOS" && platform === "windows") ||
      (command.platform === "Windows" && platform !== "windows"),
  };
}

export function persistedCommand(command: Command) {
  const {
    unavailable: _unavailable,
    external: _external,
    titleKey: _titleKey,
    ...definition
  } = command;
  return definition;
}


export type Workspace = Omit<Definitions, "revision">;

export function persistedWorkspace(data: Workspace): Workspace {
  return { locale: data.locale, projects: data.projects, commands: data.commands.map(command => ({ ...persistedCommand(command), aliases: command.aliases ?? [] })), collections: data.collections ?? [] };
}

export function fingerprint(data: unknown): string {
  return JSON.stringify(data, (key, value) =>
    (key === "hidden" || key === "persistent" || key === "omitOutput") && value === false ? undefined :
    value && typeof value === "object" && !Array.isArray(value)
      ? Object.fromEntries(Object.keys(value).sort().map((key) => [key, value[key]])) : value,
  );
}

// Rebase only the user's changed fields. A concurrent change to the same field
// is a conflict; untouched rows/fields always come from SQLite.
function mergeRows<T extends { id: string }>(base: T[], desired: T[], remote: T[]): T[] {
  const before = new Map(base.map((row) => [row.id, row]));
  const after = new Map(desired.map((row) => [row.id, row]));
  const current = new Map(remote.map((row) => [row.id, row]));
  for (const old of base) {
    const next = after.get(old.id);
    const saved = current.get(old.id);
    if (!next) {
      if (saved && fingerprint(saved) !== fingerprint(old)) throw new Error("conflict");
      current.delete(old.id);
    } else if (fingerprint(next) !== fingerprint(old)) {
      if (!saved) throw new Error("conflict");
      const merged = { ...saved };
      for (const key of new Set([...Object.keys(old), ...Object.keys(next)]) as Set<keyof T>) {
        const field = (row: T) => fingerprint(key === "hidden" || key === "persistent" || key === "omitOutput" ? !!row[key] : row[key]);
        if (field(old) === field(next)) continue;
        if (field(saved) !== field(old) && field(saved) !== field(next)) throw new Error("conflict");
        merged[key] = next[key];
      }
      current.set(old.id, merged);
    }
  }
  for (const next of desired) {
    if (before.has(next.id)) continue;
    const saved = current.get(next.id);
    if (saved && fingerprint(saved) !== fingerprint(next)) throw new Error("conflict");
    current.set(next.id, next);
  }
  const shared = (rows: T[]) => rows.filter((row) => before.has(row.id) && after.has(row.id) && current.has(row.id)).map((row) => row.id);
  const baseOrder = shared(base), desiredOrder = shared(desired);
  const result = [...current.values()];
  if (fingerprint(baseOrder) !== fingerprint(desiredOrder)) {
    const remoteOrder = shared(remote);
    if (fingerprint(remoteOrder) !== fingerprint(baseOrder) && fingerprint(remoteOrder) !== fingerprint(desiredOrder)) throw new Error("conflict");
    const reordered = new Set(desiredOrder);
    let index = 0;
    return result.map((row) => reordered.has(row.id) ? current.get(desiredOrder[index++])! : row);
  }
  return result;
}

export function mergeWorkspace(base: Workspace, desired: Workspace, remote: Workspace): Workspace {
  if (base.locale !== desired.locale && remote.locale !== base.locale && remote.locale !== desired.locale) throw new Error("conflict");
  const projects = mergeRows(base.projects, desired.projects, remote.projects);
  const retainedProjects = new Set(projects.map((project) => project.id));
  const removedProjects = new Set(base.projects.filter((project) => !retainedProjects.has(project.id)).map((project) => project.id));
  const commands = mergeRows(base.commands, desired.commands, remote.commands)
    .filter((command) => command.project === null || !removedProjects.has(command.project));
  const commandIds = new Set(commands.map(command => command.id));
  const previousMembers = new Map((base.collections ?? []).map(c => [c.id, new Set(c.commandIds)]));
  for (const collection of desired.collections ?? []) {
    if (collection.commandIds.some(id => !previousMembers.get(collection.id)?.has(id) && !commandIds.has(id))) throw new Error("commandMissing");
  }
  const collections = mergeRows(base.collections ?? [], desired.collections ?? [], remote.collections ?? [])
    .map(collection => ({ ...collection, commandIds: collection.commandIds.filter(id => commandIds.has(id)) }));
  const collectionIds = new Set(collections.map(collection => collection.id));
  const oldCollections = new Map([...(remote.collections ?? []), ...(base.collections ?? [])].map(collection => [collection.id, collection]));
  for (const collection of collections) {
    const seen = new Set<string>();
    while (collection.parent && !collectionIds.has(collection.parent)) {
      if (seen.has(collection.parent) || !oldCollections.has(collection.parent)) throw new Error("conflict");
      seen.add(collection.parent);
      collection.parent = oldCollections.get(collection.parent)!.parent;
    }
  }
  validateCollectionTree(collections);
  return {
    locale: base.locale === desired.locale ? remote.locale : desired.locale,
    projects,
    // Deleting a project also removes commands added to it by another window
    // after the base snapshot, while preserving genuine standalone commands.
    commands, collections,
  };
}
