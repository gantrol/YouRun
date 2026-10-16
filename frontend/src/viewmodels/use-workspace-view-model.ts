import { useEffect, useRef, useState, useSyncExternalStore, type SetStateAction } from "react";
import { clearCommandBindings, maxLaunchButtons, type Command, type Project, type Collection } from "../model";
import { validateCollectionTree } from "../collections";
import { projectAPI } from "../project-api";
import { applyLaunchRecommendations, type LaunchRecommendation } from "../launch-recommendations";
import { alreadySaved, ImportMetadataError, type ImportResult, type ImportLogoChoice } from "../project-import-data";
import type { ProjectDetails } from "../project-details";
import { onThisDevice, type Snapshot } from "../api";
import type { Workspace } from "../models/workspace";
import { WorkspaceViewModel } from "./workspace-view-model";

type PendingImport = { projectId: string; directory: string; commandIds: string[]; ready: boolean; logoChoice?: ImportLogoChoice | null };
const pendingKey = "pendingProjectImports";

function restoredImports(projects: Project[], commands: Command[]): PendingImport[] {
  try {
    const stored = JSON.parse(sessionStorage.getItem(pendingKey) ?? "null");
    if (stored?.host !== sessionStorage.getItem("hostToken") || !Array.isArray(stored.imports)) return [];
    return stored.imports.filter((item: PendingImport) =>
      item && typeof item.directory === "string" && Array.isArray(item.commandIds)
        && projects.some((project) => project.id === item.projectId)
        && (item.ready || item.commandIds.every((id) => commands.some((command) => command.id === id && command.project === item.projectId))),
    ).map((item: PendingImport) => ({ ...item, ready: true }));
  } catch {
    return [];
  }
}

export function useWorkspaceViewModel(initial: Snapshot) {
  const [viewModel] = useState(() => new WorkspaceViewModel(initial));
  const state = useSyncExternalStore(viewModel.subscribe, viewModel.getSnapshot);
  const { projects, commands, locale, collections = [] } = state.definitions;
  const { runs, issue, saveError, saving, metadataRevision, settingsRevision } = state;
  const { save: saveDefinitions, flush, retrySave, setIssue } = viewModel;
  const importing = useRef(false);
  const importIntent = useRef<{ key: string; projectId: string } | null>(null);
  const [pendingImports, setPendingImports] = useState(() => restoredImports(projects, commands));
  const pending = useRef(pendingImports);
  function setPart<K extends keyof Workspace>(key: K, value: SetStateAction<Workspace[K]>) {
    void saveDefinitions((current) => ({ ...current, [key]: typeof value === "function"
      ? (value as (previous: Workspace[K]) => Workspace[K])(current[key]) : value })).catch(() => {});
  }
  useEffect(() => {
    const controller = new AbortController();
    const foreground = () => { if (!document.hidden && viewModel.getSnapshot().issue === "connectionLost") void viewModel.refresh(controller.signal); };
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (viewModel.getSnapshot().saving) { event.preventDefault(); event.returnValue = ""; }
    };
    void viewModel.connect(controller.signal);
    window.addEventListener("focus", foreground);
    document.addEventListener("visibilitychange", foreground);
    window.addEventListener("beforeunload", beforeUnload);
    return () => {
      controller.abort();
      window.removeEventListener("focus", foreground);
      document.removeEventListener("visibilitychange", foreground);
      window.removeEventListener("beforeunload", beforeUnload);
    };
  }, [viewModel]);

  function rememberImports(next: PendingImport[]) {
    pending.current = next;
    setPendingImports(next);
    try {
      sessionStorage.setItem(pendingKey, JSON.stringify({ host: sessionStorage.getItem("hostToken"), imports: next }));
    } catch {
      // Navigation recovery still works if the webview disables session storage.
    }
  }
  async function retryImportMetadata(projectId: string, directory: string, choice?: ImportLogoChoice | null) {
    await retrySave();
    const item = pending.current.find((item) => item.projectId === projectId && item.directory === directory);
    const logoChoice = choice === undefined ? item?.logoChoice : choice;
    if (item && choice !== undefined) rememberImports(pending.current.map((entry) => entry === item ? { ...entry, logoChoice } : entry));
    try {
      try {
        await projectAPI("project-metadata", "POST", { id: projectId, directory, ...logoChoice });
      } catch (cause) {
        if (!(cause instanceof Error) || cause.message !== "conflict" || !logoChoice) throw cause;
        // A successful save can lose its response. Confirm the choice before
        // retrying only the idempotent root binding; never overwrite a new choice.
        const details = await projectAPI<Record<string, ProjectDetails>>("project-details", "POST", { ids: [projectId] });
        if (details[projectId]?.selectedLogo !== logoChoice.logoId || details[projectId]?.customAvatar) throw cause;
        await projectAPI("project-metadata", "POST", { id: projectId, directory });
      }
      rememberImports(pending.current.filter((item) => item.projectId !== projectId || item.directory !== directory));
    } catch (cause) {
      throw new ImportMetadataError(projectId, cause instanceof Error && cause.message === "conflict" ? "logoConflict" : undefined);
    }
  }
  async function importProject(result: ImportResult, ids: Set<string>, name: string, projectId: string, buttons: LaunchRecommendation[], logoChoice?: ImportLogoChoice | null) {
    if (importing.current) throw new Error("scanBusy");
    importing.current = true;
    try {
      const key = JSON.stringify([result.directory, projectId]);
      if (importIntent.current?.key !== key) {
        importIntent.current = { key, projectId: projectId === "new" ? crypto.randomUUID() : projectId };
      }
      const targetId = importIntent.current.projectId;
      await saveDefinitions((current) => {
        const candidates = result.candidates.filter((c) => ids.has(c.id) && !alreadySaved(c, current.commands));
        const existing = current.projects.find((p) => p.id === targetId);
        const selectedButtons = buttons.filter((button, index) =>
          (candidates.some((c) => c.id === button.commandId)
            || (ids.has(button.commandId) && current.commands.some((c) => c.id === button.commandId && c.project === targetId)))
          && buttons.findIndex((item) => item.commandId === button.commandId) === index,
        ).slice(0, maxLaunchButtons);
        const recommended = projectId === "new" ? applyLaunchRecommendations(selectedButtons, existing?.buttons) : [];
        const target = existing ?? (projectId === "new"
          ? { id: targetId, name, buttons: recommended } : undefined);
        if (!target || !target.name.trim()) throw new Error("invalidDefinition");
        const added: Command[] = candidates.map((c) => onThisDevice({
          id: c.id, title: c.title, text: c.text, source: c.source, directory: c.directory,
          project: target.id, shell: c.shell, platform: c.platform, fields: [], favorite: false, icon: c.icon,
        }, initial.platform));
        const previous = pending.current.find((item) => item.projectId === targetId && item.directory === result.directory);
        rememberImports([
          ...pending.current.filter((item) => item !== previous),
          { projectId: targetId, directory: result.directory, commandIds: candidates.length ? candidates.map((c) => c.id) : previous?.commandIds ?? [], ready: false, logoChoice },
        ]);
        return {
          ...current,
          projects: existing ? current.projects.map((p) => p.id === targetId ? {
            ...p, name: projectId === "new" ? name : p.name,
            buttons: projectId === "new" ? recommended : p.buttons, hidden: false,
          } : p) : [...current.projects, target],
          commands: [...current.commands, ...added],
        };
      });
      rememberImports(pending.current.map((item) => item.projectId === targetId && item.directory === result.directory
        ? { ...item, ready: true } : item));
      await retryImportMetadata(targetId, result.directory);
      importIntent.current = null;
      return targetId;
    } finally {
      importing.current = false;
    }
  }
  return {
    saveCommand: (command: Command, collectionId?: string) => saveDefinitions((current) => {
      if (collectionId && !current.collections?.some(item => item.id === collectionId)) throw new Error("collectionMissing");
      return { ...current,
      commands: current.commands.some((item) => item.id === command.id)
        ? current.commands.map((item) => item.id === command.id ? command : item) : [...current.commands, command],
      projects: clearCommandBindings(current.projects, command.id, command.project),
      collections: (current.collections ?? []).map(item => item.id === collectionId ? { ...item, commandIds: [...new Set([...item.commandIds, command.id])] } : item),
    }; }),
    deleteCommand: (id: string) => saveDefinitions((current) => ({ ...current,
      commands: current.commands.filter((item) => item.id !== id), projects: clearCommandBindings(current.projects, id),
    })),
    deleteProject: (id: string) => saveDefinitions((current) => ({ ...current,
      commands: current.commands.filter((command) => command.project !== id), projects: current.projects.filter((item) => item.id !== id),
    })),
    renameProject: (id: string, name: string) => saveDefinitions((current) => ({ ...current,
      projects: current.projects.map((item) => item.id === id ? { ...item, name } : item),
    })),
    updateProject: (next: Project) => setPart("projects", (previous) => previous.map((item) => item.id === next.id ? next : item)),
    hideProject: (id: string) => setPart("projects", (previous) => previous.map((item) => item.id === id ? { ...item, hidden: true } : item)),
    toggleProject: (id: string) => setPart("projects", (previous) => previous.map((item) => item.id === id ? { ...item, hidden: !item.hidden } : item)),
    toggleCommand: (id: string) => setPart("commands", (previous) => previous.map((item) => item.id === id ? { ...item, hidden: !item.hidden } : item)),
    collections,
    saveCollection: (collection: Collection, existing: boolean) => saveDefinitions(current => {
      if (existing && !current.collections?.some(item => item.id === collection.id)) throw new Error("collectionMissing");
      if (collection.commandIds.some(id => !current.commands.some(command => command.id === id))) throw new Error("commandMissing");
      const next = existing ? (current.collections ?? []).map(item => item.id === collection.id ? { ...item, name: collection.name, parent: collection.parent } : item)
        : [...(current.collections ?? []), collection];
      validateCollectionTree(next);
      return { ...current, collections: next };
    }),
    deleteCollection: (id: string) => saveDefinitions(current => {
      const removed = current.collections?.find(item => item.id === id);
      return { ...current, collections: (current.collections ?? []).filter(item => item.id !== id)
        .map(item => item.parent === id ? { ...item, parent: removed?.parent ?? null } : item) };
    }),
    addToCollections: (commandIds: string[], collectionIds: string[]) => saveDefinitions(current => {
      if (commandIds.some(id => !current.commands.some(c => c.id === id))) throw new Error("commandMissing");
      if (collectionIds.some(id => !current.collections?.some(c => c.id === id))) throw new Error("collectionMissing");
      return { ...current, collections: (current.collections ?? []).map(item => collectionIds.includes(item.id)
        ? { ...item, commandIds: [...new Set([...item.commandIds, ...commandIds])] } : item) };
    }),
    removeFromCollection: (id: string, commandId: string) => saveDefinitions(current => ({ ...current,
      collections: (current.collections ?? []).map(item => item.id === id ? { ...item, commandIds: item.commandIds.filter(value => value !== commandId) } : item),
    })),
    reorderCollection: (id: string, direction: number, commandId?: string) => saveDefinitions(current => {
      const next = [...(current.collections ?? [])], collection = next.find(item => item.id === id);
      if (!collection) throw new Error("collectionMissing");
      const order = commandId ? [...collection.commandIds] : next.filter(item => item.parent === collection.parent).map(item => item.id);
      const index = order.indexOf(commandId ?? id), target = index + direction;
      if (index < 0 || target < 0 || target >= order.length) return current;
      [order[index], order[target]] = [order[target], order[index]];
      if (commandId) return { ...current, collections: next.map(item => item.id === id ? { ...item, commandIds: order } : item) };
      let position = 0;
      return { ...current, collections: next.map(item => item.parent === collection.parent ? next.find(value => value.id === order[position++])! : item) };
    }),
    commands, setCommands: (value: SetStateAction<Command[]>) => setPart("commands", value),
    projects, setProjects: (value: SetStateAction<Project[]>) => setPart("projects", value),
    locale, setLocale: (value: SetStateAction<string>) => setPart("locale", value),
    runs, issue, setIssue, saveError, saving, metadataRevision, settingsRevision,
    definitionsSaved: !saving,
    pendingImports: pendingImports.filter((item) => projects.some((project) => project.id === item.projectId)
      && (item.ready || item.commandIds.every((id) => commands.some((command) => command.id === id && command.project === item.projectId)))),
    flush, retrySave, saveDefinitions, importProject, retryImportMetadata,
    refresh: viewModel.refresh,
    metadataKeys: state.metadataKeys,
  };
}
