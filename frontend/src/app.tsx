import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { TitleBar } from "./title-bar";
import { WorkspaceFrame, WorkspaceOverview, WorkspaceDetail } from "./workspace-layout";
import {
  WindowControls,
  type WindowAction,
  type useWindowControls,
} from "./window-controls";
import { SearchBar } from "./search-bar";
import { RunHistory, type HistoryFilters } from "./run-history";
import { catalogs } from "./messages";
import {
  commandTitle,
  type Command,
  type Run,
} from "./model";
import { WorkspaceResizer } from "./workspace-resizer";
import { CollectionEditor } from "./collection-editor";
import { CollectionCommands } from "./collection-commands";
import { CollectionDialog, type CollectionDialogTarget } from "./collection-dialog";
import type { CollectionActions } from "./collection-tree";
import { collectionPath } from "./collections";
import { ProjectCommands } from "./project-commands";
import { ProjectEditor } from "./project-editor";
import { AddCommandPage } from "./add-command-page";
import { Launcher } from "./launcher";
import { ProjectImport } from "./project-import";
import { ImportRecovery } from "./import-recovery";
import { alreadySaved } from "./project-import-data";
import { CommandPanel, CommandEditor, ProjectNameEditor } from "./panels";
import {
  DeletePanel,
  ProjectSettings,
  GlobalSettings,
} from "./settings-panels";
import { AgentIntegrationSetting, StartupSetting } from "./startup-setting";
import { RunOutput } from "./run-output";
import "./style.css";
import { api, type Snapshot } from "./api";
import { useWorkspaceViewModel } from "./viewmodels/use-workspace-view-model";
import { useRunViewModel } from "./viewmodels/use-run-view-model";
import type { Messages } from "./messages";
import { UpdatesPanel, useUpdates } from "./updates";
import type { Panel, HostSession, UpdateSession } from "./update-session";
import { useAppearance } from "./appearance";
import { useProjectDetails } from "./project-details";
import { ProjectMetadataControls } from "./project-metadata-controls";
import { QuitDialog, useQuitDialog } from "./quit-dialog";
import { CustomProjectOpenerDialog, useProjectOpeners } from "./project-openers";
import { UnsavedChangesDialog } from "./unsaved-changes-dialog";
import { ViewStateProvider } from "@/renderer/components/ui/view-state";
import {
  ProModeWorkspace,
  createInitialProModeState,
  type ProModeState,
  type ResourceOriginContext,
} from "./pro-mode";
import {
  isSubOrSamePath,
  resolveCommandWorkspaceDirectory,
  samePathNormalized,
} from "./project-workspaces";

export function App({
  initial,
  windowControls,
  session,
}: {
  initial: Snapshot;
  windowControls: ReturnType<typeof useWindowControls>;
  session: HostSession;
}) {
  const {
    commands, collections, saveCollection, deleteCollection, addToCollections, removeFromCollection, reorderCollection,
    projects,
    locale,
    setLocale,
    runs,
    refresh: refreshWorkspace,
    metadataRevision, metadataKeys,
    settingsRevision,
    issue,
    setIssue,
    saveError,
    saving,
    definitionsSaved,
    flush: flushWorkspace,
    importProject,
    retryImportMetadata,
    retrySave,
    saveCommand: persistCommand, deleteCommand, deleteProject, renameProject,
    updateProject, hideProject, toggleProject, toggleCommand,
    pendingImports,
  } = useWorkspaceViewModel(initial);
  const appearance = useAppearance();
  const appearanceRevision = useRef(initial.settingsRevision);
  useEffect(() => {
    if (appearanceRevision.current === settingsRevision && !appearance.error) return;
    appearanceRevision.current = settingsRevision;
    void appearance.refresh();
  }, [settingsRevision]);
  const projectOpeners = useProjectOpeners(initial.platform);
  const collectionViews = useRef<Record<string, { query: string; showHidden: boolean }>>({});
  const [collectionDialog, setCollectionDialog] = useState<CollectionDialogTarget>();
  const [customEditorProject, setCustomEditorProject] = useState<string>();
  const [menuStatus, setMenuStatus] = useState("");
  const [appMode, setAppMode] = useState<"launch" | "pro">("launch");
  const [proState, setProState] = useState<ProModeState>(createInitialProModeState);
  const [proRecheckTrigger, setProRecheckTrigger] = useState(0);
  const [runResourceOrigin, setRunResourceOrigin] = useState<ResourceOriginContext>();
  useEffect(() => {
    if (!menuStatus) return;
    const timeout = setTimeout(() => setMenuStatus(""), 3000);
    return () => clearTimeout(timeout);
  }, [menuStatus]);
  async function flush() {
    await Promise.all([flushWorkspace(), appearance.flush()]);
  }
  const m = catalogs[locale] ?? catalogs.zh;
  const updates = useUpdates();
  const [switching, setSwitching] = useState(session.candidate);
  const [query, setQuery] = useState(session.resume?.query ?? "");
  const [historyFilters, setHistoryFilters] = useState<HistoryFilters>({ query: "", project: "all" });
  const [panels, setPanels] = useState<Panel[]>(() => (session.resume?.panels ?? [])
    .filter((panel) => panel.kind !== "newProject"
      && !(panel.kind === "import" && !panel.projectId && !panel.directory)));
  const [inputs, setInputs] = useState<Record<string, Record<string, string>>>(
    session.resume?.inputs ?? {},
  );
  const [directories, setDirectories] = useState<Record<string, string>>(
    session.resume?.directories ?? {},
  );
  const [expanded, setExpanded] = useState<Record<string, boolean>>(
    session.resume?.expanded ?? {},
  );
  const scroll = useRef(session.resume?.scroll ?? { home: 0, panel: 0 });
  const homeRef = useRef<HTMLDivElement>(null);
  const homeOffsets = useRef({ projects: 0, commands: 0 });
  const projectScrolls = useRef<Record<string, number>>({});
  const [revealCommand, setRevealCommand] = useState<string>();
  const [revealProject, setRevealProject] = useState<string>();
  const contentRef = useRef<HTMLElement>(null);
  const [dirty, setDirty] = useState(false);
  const dirtyRef = useRef(dirty);
  dirtyRef.current = dirty;
  const [formSaving, setFormSaving] = useState(false);
  const [choosingImport, setChoosingImport] = useState(false);
  const submitting = useRef(false);
  const [leaving, setLeaving] = useState(false);
  const pendingPanel = useRef<Panel | null>(null);
  const replacePendingPanel = useRef<boolean | "home" | "project">(false);
  const reloadPending = useRef(false);
  const allowUnload = useRef(false);
  const closing = useRef<"close" | "quit" | null>(null);
  const quit = useQuitDialog(() => void requestQuit());
  const [preparingQuit, setPreparingQuit] = useState(false);
  const searchRef = useRef<HTMLInputElement>(null);
  const projectSearchRef = useRef<HTMLInputElement>(null);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const leavingOrigin = useRef<HTMLElement | null>(null);
  const restoreLeavingFocus = useRef(false);
  const triggers = useRef<(HTMLElement | null)[]>(panels.map(() => null));
  const panelScrolls = useRef<number[]>(panels.map(() => 0));
  const restorePanelScroll = useRef<number | null>(null);
  const { runOperations, launchErrors, launch, stopRun, replayRun, clearFailure } = useRunViewModel({
    runs, flush, refresh: refreshWorkspace, setIssue,
    beforeReplay: () => {
      if (dirtyRef.current || submitting.current) throw new Error("unsaved");
    },
    onSelfRestart: async () => {
      if (dirtyRef.current || submitting.current) throw new Error("updateUnsaved");
      open({ kind: "updates" });
      await rebuildSelf();
    },
    onReplay: (id, nextId) => {
      setPanels((previous) => previous.map((item) =>
        item.kind === "run" && item.id === id ? { ...item, id: nextId } : item.kind === "command" && item.runId === id ? { ...item, runId: nextId } : item));
      setRunResourceOrigin(previous => previous?.runId === id ? { ...previous, runId: nextId } : previous);
      setProState(previous => previous.origin?.runId === id ? { ...previous, origin: { ...previous.origin, runId: nextId } } : previous);
    },
  });
  const editBaseline = useRef<Command | undefined>(undefined);
  const projectBaseline = useRef<import("./model").Project | undefined>(undefined);
  const panel = panels.at(-1);
  const formPanel = !!panel && ["import", "newCommand", "editCommand", "renameProject", "project", "newCollection", "editCollection"].includes(panel.kind);
  const projectDetails = useProjectDetails(
    projects, commands, definitionsSaved, panel?.kind === "import", metadataRevision, metadataKeys,
  );
  const detailPanel = panel?.kind === "command" || panel?.kind === "run";
  const selected =
    panel?.kind === "command"
      ? commands.find((command) => command.id === panel.id)
      : undefined;
  const savedProject =
    panel &&
    ["project", "projectCommands", "renameProject", "deleteProject"].includes(panel.kind) &&
    "id" in panel
      ? projects.find((item) => item.id === panel.id)
      : undefined;
  if (savedProject) projectBaseline.current = savedProject;
  const project = savedProject ?? (panel?.kind === "project" && projectBaseline.current?.id === panel.id ? projectBaseline.current : undefined);
  const savedEditing =
    panel?.kind === "editCommand"
      ? commands.find((item) => item.id === panel.id)
      : undefined;
  if (savedEditing) editBaseline.current = savedEditing;
  const editing = savedEditing ?? (panel?.kind === "editCommand" && editBaseline.current?.id === panel.id ? editBaseline.current : undefined);
  const deleting =
    panel?.kind === "deleteCommand"
      ? commands.find((item) => item.id === panel.id)
      : undefined;
  const selectedRun =
    panel?.kind === "run" ? runs.find((run) => run.id === panel.id) : undefined;
  const running = runs.filter((run) => run.status === "running");
  const sourceContext = [...panels].reverse().find(item => item.kind === "projectCommands" || item.kind === "collectionCommands");
  const sourceProject = sourceContext?.kind === "projectCommands" ? sourceContext : undefined;
  const sourceCollection = sourceContext?.kind === "collectionCommands" ? sourceContext : undefined;
  const contextCollection = collections.find(c => c.id === (sourceCollection?.id ?? (panel?.kind === "newCommand" ? panel.collectionId : undefined)));
  const savedCollection = panel && ["editCollection", "deleteCollection"].includes(panel.kind) && "id" in panel ? collections.find(c => c.id === panel.id) : undefined;
  const collectionBaseline = useRef<import("./model").Collection | undefined>(undefined);
  if (savedCollection) collectionBaseline.current = savedCollection;
  const editedCollection = savedCollection ?? (panel?.kind === "editCollection" && collectionBaseline.current?.id === panel.id ? collectionBaseline.current : undefined);
  const contextProjectId = sourceProject && "id" in sourceProject ? sourceProject.id
    : project?.id ?? editing?.project ?? selected?.project
      ?? commands.find(c => c.id === selectedRun?.commandId)?.project
      ?? (panel && "projectId" in panel ? panel.projectId : undefined);
  const contextProject = !contextCollection && !sourceCollection ? projects.find(p => p.id === contextProjectId) : undefined;
  const leaf = !!panel && panel.kind !== "projectCommands" && panel.kind !== "collectionCommands";
  const pageKey = panel ? `${panel.kind}:${"id" in panel ? panel.id : "projectId" in panel ? panel.projectId ?? "" : ""}` : "home";
  const fullPage = !!panel && ["import", "newCommand", "editCommand", "project", "command", "run", "history"].includes(panel.kind);

  useLayoutEffect(() => {
    if (homeRef.current) homeRef.current.scrollTop = scroll.current.home;
    if (contentRef.current) contentRef.current.scrollTop = scroll.current.panel;
  }, []);
  useLayoutEffect(() => {
    if (restorePanelScroll.current === null || !contentRef.current) return;
    contentRef.current.scrollTop = restorePanelScroll.current;
    scroll.current.panel = contentRef.current.scrollTop;
    restorePanelScroll.current = null;
  }, [panels]);
  useLayoutEffect(() => {
    if (panel || formSaving || !revealProject) return;
    const group = Array.from(homeRef.current?.querySelectorAll<HTMLElement>("[data-project-id]") ?? [])
      .find((element) => element.dataset.projectId === revealProject);
    if (!group) return;
    group.scrollIntoView({ block: "start" });
    group.querySelector<HTMLElement>("[data-project-heading]")?.focus({ preventScroll: true });
    setRevealProject(undefined);
  }, [panel, formSaving, revealProject, projects]);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    async function ready(attempt = 0) {
      try {
        await api("updates/ready", "POST");
        if (!disposed) setSwitching(false);
      } catch {
        if (disposed) return;
        if (attempt < 2)
          timer = setTimeout(() => void ready(attempt + 1), 1000);
        else setIssue("updateHandoffFailed");
      }
    }
    void ready();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, []);
  useEffect(() => {
    if (updates.state?.phase === "failed") {
      setSwitching(false);
      if (updates.state.buildLog && updates.state.error) setIssue(updates.state.error);
    }
    if (
      updates.state?.phase === "switching" ||
      updates.state?.phase === "complete"
    )
      setSwitching(true);
  }, [updates.state?.phase]);
  async function applyUpdate() {
    if (dirtyRef.current || submitting.current) throw new Error("updateUnsaved");
    await flush();
    if (dirtyRef.current || submitting.current) throw new Error("updateUnsaved");
    const resume: UpdateSession = {
      query,
      panels: panels.filter((item) => item.kind !== "updates"),
      inputs,
      directories,
      expanded,
      scroll: scroll.current,
    };
    setSwitching(true);
    try {
      await api("updates/apply", "POST", resume);
      await updates.reload();
    } catch (cause) {
      // The install request may have succeeded even if its response was lost.
      try {
        const current = await api<{ phase: string }>("updates");
        setSwitching(["switching", "complete"].includes(current.phase));
      } catch {
        setSwitching(false);
      }
      throw cause;
    }
  }
  async function rebuildSelf() {
    if (dirtyRef.current || submitting.current) throw new Error("updateUnsaved");
    await flush();
    if (dirtyRef.current || submitting.current) throw new Error("updateUnsaved");
    await api("updates/rebuild", "POST");
    await updates.reload();
  }
  useEffect(() => {
    if (updates.state?.phase !== "ready" || !updates.state.localRestart) return;
    if (dirty) {
      setIssue("updateUnsaved");
      return;
    }
    if (switching || preparingQuit || quit.prompt || formSaving || choosingImport) return;
    void applyUpdate().catch((cause) =>
      setIssue(cause instanceof Error ? cause.message : "updateFailed"),
    );
  }, [updates.state?.phase, updates.state?.localRestart, dirty, preparingQuit, quit.prompt, formSaving, choosingImport]);
  const title = panel?.kind === "newCollection" ? m.newCollection
    : panel?.kind === "editCollection" ? m.editCollection
    : panel?.kind === "deleteCollection" ? m.deleteCollection
    : panel?.kind === "collectionCommands" ? contextCollection?.name ?? m.collections
    :
    panel?.kind === "updates"
      ? m.updateTitle
      : panel?.kind === "import"
        ? panel.projectId ? m.addProjectCommands : m.openProject
        : panel?.kind === "editCommand"
          ? m.editCommand
          : panel?.kind === "renameProject"
            ? m.renameProject
            : panel?.kind === "deleteCommand"
              ? m.deleteCommand
              : panel?.kind === "deleteProject"
                ? m.deleteProject
                : selectedRun
                  ? selectedRun.title
                  : selected
                    ? [
                        projects.find((item) => item.id === selected.project)
                          ?.name,
                        commandTitle(selected, m),
                      ]
                        .filter(Boolean)
                        .join(" · ")
                    : project
                      ? panel?.kind === "project" ? `${project.name} · ${m.projectSettings}` : project.name
                      : panel?.kind === "newCommand"
                        ? m.newCommand
                        : panel?.kind === "history"
                          ? m.history
                          : m.settings;

  useEffect(() => {
    document.documentElement.lang = locale;
    document.title = appMode === "pro" ? "iRun" : m.title;
  }, [locale, m.title, appMode]);
  useEffect(() => {
    const guard = (event: BeforeUnloadEvent) => {
      if (!allowUnload.current && (dirty || saving || saveError || appearance.saving || formSaving || choosingImport)) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [dirty, saving, saveError, appearance.saving, formSaving, choosingImport]);
  useEffect(() => {
    if (
      panel &&
      ![
        "newCollection",
        "editCollection",
        "newCommand",
        "editCommand",
        "renameProject",
        "deleteCommand",
        "deleteProject",
      ].includes(panel.kind)
    )
      headingRef.current?.focus({ preventScroll: true });
  }, [panel]);

  function confirmLeaving() {
    if (!leaving) leavingOrigin.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    restoreLeavingFocus.current = true;
    setLeaving(true);
  }
  function continueEditing() {
    pendingPanel.current = null;
    closing.current = null;
    reloadPending.current = false;
    setLeaving(false);
  }
  function open(next: Panel, replace: boolean | "home" | "project" = false, origin?: HTMLElement | null) {
    if (submitting.current || formSaving) return;
    if (next.kind === "import" && !next.projectId && !next.directory) {
      void chooseProjectDirectory(replace, origin);
      return;
    }
    if (dirty) {
      closing.current = null;
      reloadPending.current = false;
      pendingPanel.current = next;
      replacePendingPanel.current = replace;
      confirmLeaving();
      if (origin) leavingOrigin.current = origin;
      return;
    }
    showPanel(next, replace, origin);
  }
  function showPanel(next: Panel, replace: boolean | "home" | "project" = false, origin?: HTMLElement | null) {
    if (JSON.stringify(panel) === JSON.stringify(next)) return;
    const trigger = origin ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
    if (replace === "home") {
      triggers.current = [trigger]; panelScrolls.current = [0]; restorePanelScroll.current = 0;
      setPanels([next]); return;
    }
    if (replace === "project" && contextCollection) {
      let source = -1;
      panels.forEach((item, index) => { if (item.kind === "collectionCommands" && item.id === contextCollection.id) source = index; });
      const prefix = source >= 0 ? panels.slice(0, source + 1) : [{ kind: "collectionCommands" as const, id: contextCollection.id }];
      triggers.current = [...triggers.current.slice(0, prefix.length), trigger];
      panelScrolls.current = [...panelScrolls.current.slice(0, prefix.length), 0];
      restorePanelScroll.current = 0; setPanels([...prefix, next]); return;
    }
    if (replace === "project" && contextProject) {
      let source = -1;
      panels.forEach((item, index) => { if (item.kind === "projectCommands" && item.id === contextProject.id) source = index; });
      const prefix = source >= 0 ? panels.slice(0, source + 1) : [...panels.slice(0, -1), { kind: "projectCommands" as const, id: contextProject.id }];
      triggers.current = [...triggers.current.slice(0, prefix.length), trigger];
      panelScrolls.current = [...panelScrolls.current.slice(0, prefix.length), 0];
      restorePanelScroll.current = 0; setPanels([...prefix, next]); return;
    }
    const existing = panels.findIndex(item => item.kind === next.kind && "id" in item && "id" in next && item.id === next.id);
    if (existing >= 0 && existing < panels.length - 1) {
      triggers.current = triggers.current.slice(0, existing + 1);
      restorePanelScroll.current = panelScrolls.current[existing + 1] ?? 0;
      panelScrolls.current = panelScrolls.current.slice(0, existing + 1);
      setPanels(previous => [...previous.slice(0, existing), next]);
      return;
    }
    if (replace) {
      restorePanelScroll.current = 0;
      setPanels((previous) => [...previous.slice(0, -1), next]);
      return;
    }
    triggers.current.push(
      origin ?? (document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null),
    );
    panelScrolls.current.push(contentRef.current?.scrollTop ?? 0);
    restorePanelScroll.current = 0;
    setPanels((previous) => [...previous, next]);
  }
  async function chooseProjectDirectory(replace: boolean | "home" | "project" = false, origin?: HTMLElement | null) {
    if (submitting.current || formSaving) return;
    const trigger = origin ?? (document.activeElement instanceof HTMLElement ? document.activeElement : null);
    submitting.current = true;
    setChoosingImport(true);
    let directory = "";
    try {
      const chosen = await api<{ directory: string }>("choose-directory", "POST");
      directory = chosen.directory;
    } catch (cause) {
      setIssue(cause instanceof Error ? cause.message : "directoryPickerFailed");
    } finally {
      submitting.current = false;
      setChoosingImport(false);
    }
    if (directory) open({ kind: "import", directory }, dirtyRef.current ? "home" : replace, trigger);
    else restoreNavigationFocus(trigger, !!panel);
  }
  function back(discard = false) {
    if ((submitting.current || formSaving) && !discard) return;
    closing.current = null;
    pendingPanel.current = null;
    reloadPending.current = false;
    if (dirty && !discard) {
      confirmLeaving();
      return;
    }
    setDirty(false);
    setLeaving(false);
    const returningToResources =
      runResourceOrigin &&
      ((panel?.kind === "run" && panel.id === runResourceOrigin.runId) ||
        (panel?.kind === "command" && panel.id === runResourceOrigin.commandId));
    restorePanelScroll.current = panelScrolls.current.pop() ?? 0;
    setPanels((previous) => previous.slice(0, -1));
    const trigger = triggers.current.pop();
    if (returningToResources) {
      const origin = runResourceOrigin;
      const originRun = runs.find(run => run.id === origin.runId);
      const stopped = !originRun || (originRun.status !== "running" && originRun.status !== "stopping");
      setProState((prev) => ({
        ...prev,
        subview: origin.subview,
        origin: stopped ? { ...origin, reviewStatus: "stale" } : undefined,
      }));
      setRunResourceOrigin(undefined);
      setAppMode("pro");
      return;
    }
    restoreNavigationFocus(trigger, panels.length > 1);
  }
  function restoreNavigationFocus(trigger: HTMLElement | null | undefined, hasPanel: boolean) {
    requestAnimationFrame(() => {
      const key = trigger?.dataset.navigationKey;
      const target = trigger?.isConnected ? trigger : key
        ? Array.from(document.querySelectorAll<HTMLElement>("[data-navigation-key]"))
          .find((element) => element.dataset.navigationKey === key) : null;
      if (target) target.focus({ preventScroll: true });
      else if (hasPanel) headingRef.current?.focus({ preventScroll: true });
      else searchRef.current?.focus({ preventScroll: true });
    });
  }
  function returnAfterRemoval(commandIds: Set<string>, projectId?: string) {
    const retained = panels.map((item, index) => ({ item, index })).filter(({ item }) => {
      if (["command", "editCommand", "deleteCommand"].includes(item.kind) && "id" in item)
        return !commandIds.has(item.id);
      if (["project", "projectCommands", "renameProject", "deleteProject"].includes(item.kind) && "id" in item)
        return item.id !== projectId;
      if ((item.kind === "newCommand" || item.kind === "import") && projectId)
        return item.projectId !== projectId;
      return true;
    });
    const returnIndex = (retained.at(-1)?.index ?? -1) + 1;
    const trigger = triggers.current[returnIndex];
    restorePanelScroll.current = panelScrolls.current[returnIndex] ?? 0;
    triggers.current = retained.map(({ index }) => triggers.current[index] ?? null);
    panelScrolls.current = retained.map(({ index }) => panelScrolls.current[index] ?? 0);
    setDirty(false);
    setPanels(retained.map(({ item }) => item));
    restoreNavigationFocus(trigger, retained.length > 0);
  }
  async function windowAction(action: WindowAction) {
    if (action === "close" && (submitting.current || formSaving)) return;
    if (action === "close" && windowControls.platform !== "darwin") {
      await requestQuit();
      return;
    }
    if (action === "close" && dirty) {
      pendingPanel.current = null;
      closing.current = "close";
      reloadPending.current = false;
      confirmLeaving();
      return;
    }
    try {
      if (action === "close") await flush();
      await windowControls.perform(action);
    } catch (error) {
      setIssue(error instanceof Error ? error.message : "windowUnavailable");
    }
  }
  async function requestQuit() {
    if (switching || preparingQuit || quit.prompt || quit.busy || submitting.current || formSaving) return;
    if (dirty) {
      pendingPanel.current = null;
      closing.current = "quit";
      reloadPending.current = false;
      confirmLeaving();
      return;
    }
    const origin =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    setPreparingQuit(true);
    try {
      await flush();
      await quit.prepare(origin);
    } catch (error) {
      setIssue(error instanceof Error ? error.message : "windowUnavailable");
    } finally {
      setPreparingQuit(false);
    }
  }
  function discardChanges() {
    restoreLeavingFocus.current = false;
    if (reloadPending.current) {
      allowUnload.current = true;
      setLeaving(false);
      location.reload();
      return;
    }
    const next = pendingPanel.current;
    if (next) {
      pendingPanel.current = null;
      setDirty(false);
      setLeaving(false);
      showPanel(next, replacePendingPanel.current, leavingOrigin.current);
      return;
    }
    const shouldClose = closing.current;
    back(true);
    if (!shouldClose) return;
    if (shouldClose === "quit") setPreparingQuit(true);
    void flush()
      .then(() =>
        shouldClose === "quit"
          ? quit.prepare(headingRef.current ?? searchRef.current)
          : windowControls.perform("close"),
      )
      .catch((error) =>
        setIssue(error instanceof Error ? error.message : "windowUnavailable"),
      )
      .finally(() => setPreparingQuit(false));
  }
  async function submit<T>(task: () => Promise<T>): Promise<T> {
    if (submitting.current) throw new Error("saveBlocked");
    submitting.current = true;
    setFormSaving(true);
    try {
      return await task();
    } finally {
      submitting.current = false;
      setFormSaving(false);
    }
  }
  async function saveImport(task: () => Promise<string>, returnToProject = false, revealId?: string) {
    await submit(async () => {
      const id = await task();
      if (revealId) setRevealCommand(revealId);
      projectDetails.rescan(id);
      setIssue((old) => old === "importProjectMetadataFailed" ? "" : old);
      if (returnToProject) {
        back(true);
        return;
      }
      setQuery("");
      setExpanded((previous) => ({ ...previous, [id]: true }));
      setDirty(false);
      setLeaving(false);
      closing.current = null;
      triggers.current = [];
      panelScrolls.current = [];
      restorePanelScroll.current = null;
      setPanels([{ kind: "projectCommands", id }]);
      triggers.current = [null];
      panelScrolls.current = [0];
    });
  }
  function saveProjectCommands(...args: Parameters<typeof importProject>) {
    const first = args[0].candidates.find(candidate => args[1].has(candidate.id) && !alreadySaved(candidate, commands));
    return saveImport(() => importProject(...args), true, first?.id);
  }
  function retryPendingImport(projectId: string, directory: string) {
    void submit(async () => {
      await retryImportMetadata(projectId, directory);
      projectDetails.rescan(projectId);
      setIssue((old) => old === "importProjectMetadataFailed" ? "" : old);
    }).catch((cause) => setIssue(cause instanceof Error ? cause.message : "storageFailed"));
  }
  function quickRun(command: Command, origin: boolean | "home" | "project" = false) {
    const projectWs = command.project ? projectDetails.details[command.project]?.workspaces : undefined;
    const activeWsId = command.project ? projectDetails.details[command.project]?.activeWorkspace : undefined;
    const resolvedDir = resolveCommandWorkspaceDirectory(command, projectWs, activeWsId);
    const directory = directories[command.id] ?? resolvedDir;
    const active = runs.find(
      (run) =>
        run.commandId === command.id &&
        ["running", "stopping"].includes(run.status) &&
        (!projectWs || projectWs.length <= 1 || samePathNormalized(run.directory, directory)),
    );
    if (active) {
      open({ kind: "run", id: active.id }, origin);
      return;
    }
    if (
      active ||
      command.fields.length ||
      !directory ||
      command.unavailable ||
      command.external
    ) {
      open({ kind: "command", id: command.id }, origin);
      return;
    }
    launch(command, directory, inputs[command.id] ?? {});
  }
  async function saveCommand(command: Command, showOnHome?: boolean) {
    if (panel?.kind === "editCommand" && !commands.some(c => c.id === command.id)) throw new Error("commandMissing");
    const previous = commands.find((item) => item.id === command.id);
    await submit(async () => {
      await persistCommand(command, panel?.kind === "newCommand" ? panel.collectionId : undefined, showOnHome);
      clearFailure(command.id);
      if (previous?.directory !== command.directory)
        setDirectories((values) => {
          const next = { ...values };
          delete next[command.id];
          return next;
        });
      back(true);
      if (panel?.kind === "newCommand" && !panel.projectId) setQuery("");
      setRevealCommand(command.id);
    });
  }
  const commandManagement = {
    onEditCommand: (id: string) => open({ kind: "editCommand", id }),
    onToggleCommand: toggleCommand,
    onDeleteCommand: (id: string) => open({ kind: "deleteCommand", id }),
  };
  function openProjectDirectory(id: string, opener?: string) {
    void api("open-project-directory", "POST", { id, opener }).catch((cause) =>
      setIssue(cause instanceof Error ? cause.message : "projectOpenFailed"));
  }
  function openDirectoryPath(directory: string) {
    void api("open-project-directory", "POST", { directory }).catch((cause) =>
      setIssue(cause instanceof Error ? cause.message : "projectOpenFailed"));
  }
  function copy(text: string) {
    void navigator.clipboard.writeText(text).then(
      () => setMenuStatus("copied"), () => setIssue("copyFailed"),
    );
  }
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (
        !!collectionDialog ||
        event.isComposing ||
        event.defaultPrevented ||
        switching ||
        preparingQuit ||
        quit.prompt ||
        quit.busy
      )
        return;
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        const leafSearch = Array.from(
          contentRef.current?.querySelectorAll<HTMLInputElement>('input[type="search"]') ?? [],
        ).find((input) => input.getClientRects().length > 0 && !input.disabled);
        const projectSearch = projectSearchRef.current;
        const homeSearch = searchRef.current;
        const target =
          leafSearch ??
          (projectSearch?.getClientRects().length ? projectSearch : undefined) ??
          (!panel || !!contextProject || !!contextCollection
            ? homeSearch
            : homeSearch?.getClientRects().length
              ? homeSearch
              : undefined);
        if (target) {
          event.preventDefault();
          target.focus();
          target.select();
        }
      }
      if (event.key === "Escape") {
        if (leaving) {
          continueEditing();
        } else if (panel) back();
        else setQuery("");
      }
    };
    window.addEventListener("keydown", keydown);
    return () => window.removeEventListener("keydown", keydown);
  });

  const collectionActions: CollectionActions = {
    open: id => open({ kind: "collectionCommands", id }),
    create: parentId => open({ kind: "newCollection", parentId }),
    edit: id => open({ kind: "editCollection", id }),
    remove: id => open({ kind: "deleteCollection", id }),
    reorder: (id, direction) => { void reorderCollection(id, direction).catch(cause => setIssue(cause instanceof Error ? cause.message : "storageFailed")); },
  };
  const launcherMenus: import("./launcher-menu").LauncherMenuActions = {
                  operation: id => runOperations[id]?.state,
                  addToCollections: id => setCollectionDialog({ kind: "collections", id }),
                  editCommand: commandManagement.onEditCommand,
                  hideCommand: commandManagement.onToggleCommand,
                  deleteCommand: commandManagement.onDeleteCommand,
                  renameProject: (id) => open({ kind: "renameProject", id }),
                  hideProject,
                  deleteProject: (id) => open({ kind: "deleteProject", id }),
                  openDirectory: openProjectDirectory,
                  projectOpeners,
                  openWith: openProjectDirectory,
                  customEditor: setCustomEditorProject,
                  copy,
                  refreshLogo: projectDetails.rescan,
                  logoBusy: projectDetails.busy,
                  savingLogo: projectDetails.savingLogo,
                  selectLogo: (id, logoId) => {
                    void projectDetails.selectLogo(id, logoId);
                  },
                  chooseAvatar: (id) => {
                    void projectDetails.chooseAvatar(id);
                  },
                };

  return (
    <div
      lang={locale}
      className="@container/desktop h-full bg-surface font-sans text-foreground"
    >
      <QuitDialog quit={quit} m={m} />
      <UnsavedChangesDialog open={leaving} m={m} onContinue={continueEditing} onDiscard={discardChanges}
        onRestoreFocus={() => {
          if (restoreLeavingFocus.current) leavingOrigin.current?.focus({ preventScroll: true });
        }} />
      {switching && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-surface/95 text-sm"
          role="status"
        >
          {m.updateSwitching}
        </div>
      )}
      <div className="flex h-full flex-col" inert={switching || preparingQuit}>
        <TitleBar
          title={appMode === "pro" ? "iRun" : panel ? title : m.productName}
          m={m}
          onBack={appMode === "pro" ? undefined : panel ? () => back() : undefined}
          backDisabled={formSaving || choosingImport}
          headingRef={headingRef}
          mac={windowControls.platform === "darwin"}
          fullscreen={windowControls.fullscreen}
          mode={appMode}
          modeDisabled={formSaving || choosingImport || dirty}
          onModeChange={(nextMode) => setAppMode(nextMode)}
          controls={
            windowControls.platform !== "darwin" ? (
              <WindowControls
                m={m}
                maximized={windowControls.maximized}
                onAction={windowAction}
              />
            ) : undefined
          }
          actions={
            <>
              {(saving || formSaving) && (
                <span className="text-xs text-muted-foreground" role="status">
                  {m.saving}
                </span>
              )}
              {choosingImport && <span className="text-xs text-muted-foreground" role="status">{m.chooseDirectory}</span>}
            </>
          }
        />
        {(saveError || issue) && (
          <div
            role="alert"
            className="flex flex-wrap items-center gap-2 border-b border-border px-4 py-2 text-xs text-destructive"
          >
            <span>{m[(saveError || issue) as keyof Messages] ?? m.requestFailed}</span>
            {saveError && saveError !== "conflict" && !formPanel && !dirty && (
              <Button variant="outline" size="xs" disabled={saving || formSaving} onClick={() => void submit(retrySave).catch(() => {})}>
                {saving ? m.saving : m.retry}
              </Button>
            )}
            {(saveError || issue) === "conflict" && (
              <Button
                variant="outline"
                size="xs"
                disabled={formSaving || choosingImport}
                onClick={() => {
                  if (!dirty && !saveError && !saving) { location.reload(); return; }
                  pendingPanel.current = null;
                  closing.current = null;
                  reloadPending.current = true;
                  confirmLeaving();
                }}
              >
                {m.reload}
              </Button>
            )}
            {!saveError && <Button variant="ghost" size="xs" onClick={() => setIssue("")}>{m.close}</Button>}
          </div>
        )}
        {menuStatus && (
          <div
            role="status"
            className="px-4 py-1 text-xs text-muted-foreground"
          >
            {m[menuStatus as keyof Messages] ?? m.requestFailed}
          </div>
        )}
        <ViewStateProvider scope={appMode === "pro" ? `pro:${proState.subview}` : pageKey}>
        {appMode === "pro" ? (
          <ProModeWorkspace
            state={proState}
            onStateChange={setProState}
            projects={projects}
            projectDetails={projectDetails.details}
            runs={runs}
            commands={commands}
            runOperations={runOperations}
            launchErrors={launchErrors}
            onReplay={replayRun}
            onStop={stopRun}
            locale={locale}
            m={m}
            recheckTrigger={proRecheckTrigger}
            onOpenRun={(runId, origin) => {
              if (origin) {
                setRunResourceOrigin(origin);
                setProState((prev) => ({ ...prev, origin }));
              }
              setAppMode("launch");
              open({ kind: "run", id: runId }, "home");
            }}
            onOpenWorkspace={(projectId, workspaceId) => {
              if (workspaceId) {
                void projectDetails.mutateWorkspaces(projectId, {
                  action: "select",
                  locationId: workspaceId,
                });
              }
              setAppMode("launch");
              open({ kind: "projectCommands", id: projectId }, "home");
            }}
            onOpenCommand={(commandId, origin) => {
              setRunResourceOrigin(origin);
              setProState(prev => ({ ...prev, origin }));
              setAppMode("launch");
              open({ kind: "command", id: commandId, view: "launch" }, "home");
            }}
            onCopy={copy}
            onOpenDirectoryPath={openDirectoryPath}
          />
        ) : (
        <WorkspaceFrame className="workspace-rewrite" data-project={!!contextProject || !!contextCollection} data-leaf={leaf} data-panel={!!panel} inert={formSaving || choosingImport}>
          <WorkspaceOverview className="workspace-home" hasDetail={!!panel} split={detailPanel}>
            <div
              ref={homeRef}
              onScroll={(event) => {
                scroll.current.home = event.currentTarget.scrollTop;
              }}
              className="flex min-h-0 w-full flex-1 flex-col"
            >
              <Launcher
                projectOffset={homeOffsets.current.projects} onProjectScroll={offset => { homeOffsets.current.projects = offset; }}
                independentOffset={homeOffsets.current.commands} onIndependentScroll={offset => { homeOffsets.current.commands = offset; }}
                onOpenProject={() => open({ kind: "import" })}
                onHistory={() => open({ kind: "history" }, "home")}
                onGlobalSettings={() => open({ kind: "settings" }, "home")}
                runningCount={running.length > 0 ? new Intl.NumberFormat(locale).format(running.length) : undefined}
                onManage={() => open({ kind: "settings" }, "home")}
                onClearSearch={() => { setQuery(""); searchRef.current?.focus(); }}
                pendingImports={pendingImports}
                onRetryImport={retryPendingImport}
                savingImport={formSaving}
                projectErrors={projectDetails.errors}
                onRetryProject={projectDetails.retry}
                runOperations={runOperations}
                launchErrors={launchErrors}
                onOutput={(run) => open({ kind: "run", id: run.id }, "home")}
                details={projectDetails.details}
                menus={launcherMenus}
                collections={collections} selectedCollectionId={contextCollection?.id}
                collectionActions={{ ...collectionActions, open: id => open({ kind: "collectionCommands", id }, "home"), create: parentId => open({ kind: "newCollection", parentId }, "home") }}
                projects={projects}
                commands={commands}
                runs={runs}
                directories={directories}
                query={query}
                m={m}
                expanded={expanded}
                onExpanded={setExpanded}
                onRun={command => quickRun(command, "home")}
                onStop={stopRun}
                onReplay={replayRun}
                selectedCommandId={selected?.id ?? selectedRun?.commandId}
                revealId={!panel ? revealCommand : undefined} onRevealed={() => setRevealCommand(undefined)}
                onOpen={(command) => open({ kind: "command", id: command.id }, "home")}
                selectedProjectId={contextProject?.id}
                onProject={item => open({ kind: "projectCommands", id: item.id }, "home")}
                onNewCommand={() => open({ kind: "newCommand" }, "home")}
                onSettings={(item) => open({ kind: "project", id: item.id }, "home")}
              />
            </div>
            <SearchBar
              m={m}
              value={query}
              onChange={setQuery}
              inputRef={searchRef}
            />
          </WorkspaceOverview>
          <WorkspaceResizer pane="navigation" label={m.resizeNavigation}/>
          {contextCollection && <CollectionCommands key={contextCollection.id} collection={contextCollection} collections={collections} projects={projects}
            commands={commands} runs={runs} m={m} directories={directories} searchRef={projectSearchRef} actions={collectionActions}
            expanded={expanded} onExpanded={setExpanded} initialView={collectionViews.current[contextCollection.id]}
            onViewChange={value => { collectionViews.current[contextCollection.id] = value; }}
            menus={{ ...launcherMenus,
              removeFromCollection: id => { void removeFromCollection(contextCollection.id, id).catch(cause => setIssue(cause instanceof Error ? cause.message : "storageFailed")); },
              moveCommand: (id, direction) => { void reorderCollection(contextCollection.id, direction, id).catch(cause => setIssue(cause instanceof Error ? cause.message : "storageFailed")); },
            }} runOperations={runOperations} launchErrors={launchErrors}
            selectedId={selected?.id ?? editing?.id ?? selectedRun?.commandId} revealId={!leaf ? revealCommand : undefined} onRevealed={() => setRevealCommand(undefined)}
            initialOffset={projectScrolls.current[`collection:${contextCollection.id}`] ?? 0} onScroll={offset => { projectScrolls.current[`collection:${contextCollection.id}`] = offset; }}
            onOpen={command => open({ kind: "command", id: command.id }, "project")} onRun={command => quickRun(command, "project")}
            onStop={stopRun} onReplay={replayRun} onOutput={run => open({ kind: "run", id: run.id }, "project")}
            onAdd={() => setCollectionDialog({ kind: "commands", id: contextCollection.id })}
            onNew={() => open({ kind: "newCommand", collectionId: contextCollection.id }, "project")}/>}
          {panel?.kind === "collectionCommands" && !contextCollection && <div className="flex-1 p-5" role="status">{m.collectionMissing}</div>}
          {contextProject && <ProjectCommands key={contextProject.id} project={contextProject} commands={commands} directories={directories} runs={runs} m={m}
            searchRef={projectSearchRef}
            workspaces={projectDetails.details[contextProject.id]?.workspaces}
            activeWorkspaceId={projectDetails.details[contextProject.id]?.activeWorkspace}
            onMutateWorkspaces={(payload) => projectDetails.mutateWorkspaces(contextProject.id, payload)}
            onOpenPortForRun={(run) => {
              setProState((prev) => ({
                ...prev,
                subview: "ports",
                ports: { ...prev.ports, query: run.values?.port || "" },
              }));
              setAppMode("pro");
            }}
            menus={launcherMenus} runOperations={runOperations} launchErrors={launchErrors}
            selectedId={selected?.id ?? editing?.id ?? selectedRun?.commandId} revealId={!leaf ? revealCommand : undefined}
            onRevealed={() => setRevealCommand(undefined)} initialOffset={projectScrolls.current[contextProject.id] ?? 0}
            onScroll={offset => { projectScrolls.current[contextProject.id] = offset; }}
            onOpen={command => open({ kind: "command", id: command.id }, "project")}
            onRun={command => quickRun(command, "project")} onStop={stopRun} onReplay={replayRun} onOutput={run => open({ kind: "run", id: run.id }, "project")}
            onSettings={() => open({ kind: "project", id: contextProject.id }, "project")}
            onAdd={() => open({ kind: "newCommand", projectId: contextProject.id }, "project")}/>}
          {(contextProject || contextCollection) && <WorkspaceResizer pane="commands" label={m.resizeCommandList}/>}
          {panel && leaf && (
            <WorkspaceDetail
              key={pageKey}
              split={detailPanel}
              ref={panel.kind === "import" ? undefined : contentRef}
              onScroll={panel.kind === "import" ? undefined : (event) => {
                scroll.current.panel = event.currentTarget.scrollTop;
              }}
              className={`workspace-leaf ${fullPage ? "flex flex-col overflow-hidden" : "overflow-auto px-5 py-5"}`}
            >
              {((panel.kind === "command" && !selected) || (panel.kind === "run" && !selectedRun)) && <div className="space-y-3 p-5">
                <p role="status" className="text-sm text-muted-foreground">{panel.kind === "command" ? m.commandMissing : m.replayUnavailable}</p>
                <Button variant="outline" size="sm" onClick={() => back()}>{m.back}</Button>
              </div>}
              {(panel.kind === "newCollection" || panel.kind === "editCollection") && <CollectionEditor
                key={panel.kind === "editCollection" ? panel.id : `new:${panel.kind === "newCollection" ? panel.parentId ?? "root" : "root"}`} initial={editedCollection}
                deleted={panel.kind === "editCollection" && !savedCollection} parent={panel.kind === "newCollection" ? panel.parentId : undefined}
                collections={collections} m={m} onDirty={setDirty} onBusy={setFormSaving} onCancel={() => back()}
                onSave={collection => submit(async () => {
                  await saveCollection(panel.kind === "newCollection" && panel.commandId ? { ...collection, commandIds: [panel.commandId] } : collection, panel.kind === "editCollection");
                  if (panel.kind === "editCollection") back(true);
                  else { setDirty(false); showPanel({ kind: "collectionCommands", id: collection.id }, "home"); }
                })}/>}
              {panel.kind === "deleteCollection" && <div className="space-y-4">
                <p className="font-medium">{savedCollection?.name ?? m.collectionMissing}</p>
                <p className="text-sm text-muted-foreground">{m.deleteCollectionEffect}</p>
                <div className="flex gap-2"><Button size="sm" variant="destructive" disabled={!savedCollection} onClick={() => {
                  if (!savedCollection) return;
                  void submit(async () => {
                    await deleteCollection(savedCollection.id);
                    setDirty(false); triggers.current = []; panelScrolls.current = []; restorePanelScroll.current = 0;
                    setPanels(savedCollection.parent ? [{ kind: "collectionCommands", id: savedCollection.parent }] : []);
                  }).catch(cause => setIssue(cause instanceof Error ? cause.message : "storageFailed"));
                }}>{m.deleteCollection}</Button><Button size="sm" variant="outline" onClick={() => back()}>{m.cancel}</Button></div>
              </div>}
              {panel.kind === "updates" && (
                <UpdatesPanel
                  state={updates.state}
                  error={updates.error}
                  m={m}
                  locale={locale}
                  onDirty={setDirty}
                  onReload={updates.reload}
                  onApply={applyUpdate}
                  onRebuild={rebuildSelf}
                  onSaveConfig={(config) => submit(() => api<void>("updates/config", "POST", config))}
                />
              )}
              {selected && (
                <CommandPanel
                  key={selected.id}
                  view={panel.kind === "command" ? panel.view : undefined}
                  onView={(view, runId) => setPanels(previous => previous.map((item, i) => i === previous.length - 1 && item.kind === "command" ? { ...item, view, runId } : item))}
                  command={selected}
                  collections={collections.filter(c => c.commandIds.includes(selected.id)).map(c => ({ id: c.id, name: collectionPath(c, collections) }))}
                  onCollection={id => open({ kind: "collectionCommands", id })} onAddToCollections={() => setCollectionDialog({ kind: "collections", id: selected.id })}
                  operation={runOperations[selected.id]}
                  launchError={launchErrors[selected.id]}
                  onEdit={() => open({ kind: "editCommand", id: selected.id })}
                  m={m}
                  directory={
                    directories[selected.id] ??
                    resolveCommandWorkspaceDirectory(
                      selected,
                      selected.project ? projectDetails.details[selected.project]?.workspaces : undefined,
                      selected.project ? projectDetails.details[selected.project]?.activeWorkspace : undefined,
                    )
                  }
                  values={inputs[selected.id] ?? {}}
                  runs={runs.filter((run) => run.commandId === selected.id)}
                  initialRunId={
                    panel?.kind === "command" ? panel.runId : undefined
                  }
                  onDirectory={(value) =>
                    setDirectories((previous) => ({
                      ...previous,
                      [selected.id]: value,
                    }))
                  }
                  onValues={(value) =>
                    setInputs((previous) => ({
                      ...previous,
                      [selected.id]: value,
                    }))
                  }
                  onLaunch={(directory, values) =>
                    launch(selected, directory, values)
                  }
                  onStop={stopRun}
                  onReplay={replayRun}
                  onRecheck={refreshWorkspace}
                />
              )}
              {project && panel.kind === "project" && (
                <>
                {pendingImports.filter((item) => item.projectId === project.id).map((item) => (
                  <ImportRecovery key={item.directory} directory={item.directory} busy={formSaving} m={m}
                    onRetry={() => retryPendingImport(item.projectId, item.directory)} />
                ))}
                <ProjectEditor
                  key={project.id} project={project} deleted={!savedProject} platform={initial.platform} directory={projectDetails.details[project.id]?.directory}
                  commands={commands.filter(command => command.project === project.id)} runs={runs} m={m} onChange={updateProject} metadata={null}
                  details={projectDetails.details[project.id]} busy={projectDetails.busy(project.id)} error={projectDetails.errors[project.id]}
                  onMutateWorkspaces={(payload) => projectDetails.mutateWorkspaces(project.id, payload)}
                  onDirty={setDirty} onBusy={setFormSaving} onCancel={() => back()}
                  onSave={(base, next, logo) => submit(async () => {
                    await flush();
                    await api("project-settings", "PUT", { base, project: next, logo });
                    await refreshWorkspace();
                    projectDetails.retry(project.id);
                    back(true);
                  })}
                  onRescan={() => projectDetails.rescan(project.id)} onRetry={() => projectDetails.retry(project.id)}
                  onOpenDirectory={() => openProjectDirectory(project.id)} onCopyDirectory={() => copy(projectDetails.details[project.id]?.directory ?? "")}
                  openers={projectOpeners} onOpenWith={opener => openProjectDirectory(project.id, opener)} onCustomEditor={() => setCustomEditorProject(project.id)}
                  onDelete={() =>
                    open({ kind: "deleteProject", id: project.id })
                  }
                  onNewCommand={() =>
                    open({ kind: "newCommand", projectId: project.id })
                  }
                  onImportCommands={() => open({ kind: "import", projectId: project.id })}
                  {...commandManagement}
                />
                </>
              )}
              {panel.kind === "settings" && (
                <GlobalSettings
                  startup={<StartupSetting m={m} />}
                  agentIntegration={<AgentIntegrationSetting m={m} />}
                  onUpdates={updates.state && !updates.state.disabled ? () => open({ kind: "updates" }) : undefined}
                  updateReady={updates.state?.phase === "ready"}
                  projects={projects}
                  commands={commands}
                  m={m}
                  locale={locale}
                  onLocale={setLocale}
                  onOpenProject={() => open({ kind: "import" })}
                  onNewCommand={() => open({ kind: "newCommand" })}
                  onProject={(id) => open({ kind: "project", id })}
                  onToggleProject={toggleProject}
                  onDeleteProject={(id) => open({ kind: "deleteProject", id })}
                  {...commandManagement}
                />
              )}
              {panel.kind === "import" && (
                <ProjectImport
                  key={panel.projectId ?? "new-import"}
                  scrollRef={(element) => { contentRef.current = element; }}
                  onScroll={(event) => { scroll.current.panel = event.currentTarget.scrollTop; }}
                  initialDirectory={panel.directory}
                  lockedProject={projects.find((project) => project.id === panel.projectId)}
                  m={m}
                  platform={initial.platform}
                  projects={projects}
                  commands={commands}
                  onCancel={() => back()}
                  onDirty={setDirty}
                  recovery={pendingImports.map((item) => (
                    <ImportRecovery key={`${item.projectId}:${item.directory}`} directory={item.directory}
                      name={projects.find((project) => project.id === item.projectId)?.name} busy={formSaving} m={m}
                      onRetry={() => retryPendingImport(item.projectId, item.directory)} />
                  ))}
                  onSave={(...args) => saveImport(() => importProject(...args), !!panel.projectId)}
                  onManual={panel.projectId ? () => open({ kind: "newCommand", projectId: panel.projectId }, true) : undefined}
                  onRetryMetadata={(projectId, directory, logoChoice) => saveImport(async () => {
                    await retryImportMetadata(projectId, directory, logoChoice);
                    return projectId;
                  }, !!panel.projectId)}
                />
              )}
              {panel.kind === "newCommand" && <AddCommandPage onDirty={setDirty} onBusy={setFormSaving}
                manual={{ commands, projects, currentPlatform: initial.platform, defaultDirectory: contextProject ? projectDetails.details[contextProject.id]?.directory : undefined, projectId: panel.projectId, m, onDirty: setDirty, onSave: saveCommand, onCancel: () => back() }}
                scan={{ m, projects, commands, platform: initial.platform, lockedProject: projects.find(p => p.id === panel.projectId),
                  onCancel: () => back(), onSave: saveProjectCommands,
                  onRetryMetadata: (id, directory, choice) => saveImport(async () => { await retryImportMetadata(id, directory, choice); return id; }, true) }}/>
              }
              {editing && <CommandEditor key={editing.id} deleted={!savedEditing} initial={editing} currentPlatform={initial.platform} commands={commands} projects={projects} m={m}
                onDirty={setDirty} onSave={saveCommand} onCancel={() => back()}/>}
              {panel.kind === "renameProject" && project && (
                <ProjectNameEditor
                  key={project.id}
                  initialName={project.name}
                  names={projectDetails.details[project.id]?.names}
                  m={m}
                  onDirty={setDirty}
                  onCancel={() => back()}
                  onSave={(name) => submit(async () => {
                    await renameProject(project.id, name);
                    back(true);
                  })}
                />
              )}
              {(deleting || (panel.kind === "deleteProject" && project)) && (
                <DeletePanel
                  m={m}
                  name={deleting ? commandTitle(deleting, m) : project!.name}
                  project={!deleting}
                  collections={collections.filter(c => c.commandIds.some(id => deleting ? id === deleting.id : commands.some(command => command.id === id && command.project === project?.id))).map(c => collectionPath(c, collections))}
                  onCancel={() => back()}
                  onDelete={() => {
                    void submit(async () => {
                      if (deleting) {
                        await deleteCommand(deleting.id);
                        returnAfterRemoval(new Set([deleting.id]));
                      } else if (project) {
                        await deleteProject(project.id);
                        returnAfterRemoval(new Set(commands.filter((command) => command.project === project.id).map((command) => command.id)), project.id);
                      }
                    }).catch((cause) => setIssue(cause instanceof Error ? cause.message : "storageFailed"));
                  }}
                />
              )}
              {selectedRun && (
                <div className="flex min-h-0 flex-1 flex-col p-5"><RunOutput
                  run={selectedRun}
                  operation={runOperations[selectedRun.commandId]}
                  launchError={launchErrors[selectedRun.commandId]}
                  onEdit={commands.some((command) => command.id === selectedRun.commandId)
                    ? () => open({ kind: "editCommand", id: selectedRun.commandId }) : undefined}
                  m={m}
                  onStop={stopRun}
                  onReplay={replayRun}
                  onRecheck={refreshWorkspace}
                  resourceOrigin={runResourceOrigin?.runId === selectedRun.id ? runResourceOrigin : undefined}
                  onReturnAndRecheck={
                    runResourceOrigin?.runId === selectedRun.id
                      ? () => {
                          const origin = runResourceOrigin;
                          setProState((prev) => ({
                            ...prev,
                            subview: origin.subview,
                            origin: { ...origin, reviewStatus: "released" },
                          }));
                          setProRecheckTrigger((c) => c + 1);
                          setRunResourceOrigin(undefined);
                          restorePanelScroll.current = panelScrolls.current.pop() ?? 0;
                          setPanels((previous) => previous.slice(0, -1));
                          triggers.current.pop();
                          setAppMode("pro");
                        }
                      : undefined
                  }
                  onViewWorkspace={
                    (() => {
                      const runProjectId = commands.find((c) => c.id === selectedRun.commandId)?.project;
                      if (!runProjectId) return undefined;
                      return () => {
                        const wsList = projectDetails.details[runProjectId]?.workspaces;
                        const matchedWs = wsList?.find((w) => isSubOrSamePath(w.directory, selectedRun.directory));
                        if (matchedWs) {
                          void projectDetails.mutateWorkspaces(runProjectId, {
                            action: "select",
                            locationId: matchedWs.id,
                          });
                        }
                        open({ kind: "projectCommands", id: runProjectId }, "home");
                      };
                    })()
                  }
                  onViewPorts={() => {
                    setProState((prev) => ({
                      ...prev,
                      subview: "ports",
                      ports: { ...prev.ports, query: selectedRun.values?.port || "" },
                    }));
                    setAppMode("pro");
                  }}
                /></div>
              )}
              {panel.kind === "history" && (
                <RunHistory
                  runs={runs}
                  commands={commands}
                  projects={projects}
                  filters={historyFilters}
                  onFiltersChange={setHistoryFilters}
                  m={m}
                  locale={locale}
                  onOpen={(id) => open({ kind: "run", id })}
                />
              )}
            </WorkspaceDetail>
          )}
        </WorkspaceFrame>
        )}
        </ViewStateProvider>
        <span className="sr-only" role="status">
          {runs[0] ? `${runs[0].title} · ${m[runs[0].status]}` : ""}
        </span>
        {collectionDialog && <CollectionDialog target={collectionDialog} collections={collections} commands={commands} projects={projects} m={m}
          onSave={addToCollections} onClose={() => setCollectionDialog(undefined)}
          onCreate={() => { const commandId = collectionDialog.kind === "collections" ? collectionDialog.id : undefined; setCollectionDialog(undefined); open({ kind: "newCollection", commandId }); }}/>}
        {customEditorProject && <CustomProjectOpenerDialog m={m} onClose={() => setCustomEditorProject(undefined)}
          onOpen={async (executable) => { await api("open-project-directory", "POST", { id: customEditorProject, executable }); }} />}
      </div>
    </div>
  );
}
