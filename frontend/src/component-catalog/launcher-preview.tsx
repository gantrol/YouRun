import { useEffect, useRef, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { Dialog, DialogContent, DialogTitle } from "@/renderer/components/ui/dialog";
import { ModalOverlayScope } from "@/renderer/components/ui/overlay-layer";
import { Launcher } from "../launcher";
import { SearchBar } from "../search-bar";
import { TitleBar } from "../title-bar";
import { WindowControls } from "../window-controls";
import { ProjectSettings } from "../settings-panels";
import { WorkspaceFrame, WorkspaceOverview, WorkspaceDetail } from "../workspace-layout";
import { CommandAction } from "../command-action";
import { RunStatus } from "../run-state";
import type { Messages } from "../messages";
import type { LauncherMenuActions } from "../launcher-menu";
import type { Command } from "../model";
import type { RunOperations, RunOperationState } from "../models/run-operation";
import { sampleCommands, sampleProjects, sampleRun, sampleRuns } from "./samples";

export function LauncherPreview({ m, width = 520, height = 720, initialSelectedId = "" }: { m: Messages; width?: number; height?: number; initialSelectedId?: string }) {
  const [projects, setProjects] = useState(sampleProjects);
  const [commands, setCommands] = useState(sampleCommands);
  const [runs, setRuns] = useState(sampleRuns);
  const [operations, setOperations] = useState<RunOperations>({});
  const pending = useRef(new Map<string, number>());
  useEffect(() => () => { pending.current.forEach(timer => window.clearTimeout(timer)); }, []);
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [settings, setSettings] = useState("");
  const [result, setResult] = useState("");
  const [maximized, setMaximized] = useState(false);
  const [selectedId, setSelectedId] = useState(initialSelectedId);
  const searchRef = useRef<HTMLInputElement>(null);
  const selected = commands.find((item) => item.id === selectedId);
  const selectedRun = runs.find((item) => item.commandId === selectedId);
  const project = projects.find((item) => item.id === settings);
  const report = (action: string) => setResult(action);
  // Local presentation only: let the real rows demonstrate their complete state transitions.
  const begin = (command: Command, state: RunOperationState) => {
    if (pending.current.has(command.id)) return;
    const previous = runs.find(item => item.commandId === command.id);
    setOperations(items => ({ ...items, [command.id]: { state, previousRunId: previous?.id, runId: previous?.id } }));
    pending.current.set(command.id, window.setTimeout(() => {
      pending.current.delete(command.id);
      setRuns(items => [sampleRun(command, state === "stopping" ? "stopped" : "running"), ...items.filter(item => item.commandId !== command.id)]);
      setOperations(items => { const next = { ...items }; delete next[command.id]; return next; });
    }, state === "stopping" ? 600 : 900));
  };
  const run = (command: Command) => {
    if (runs.some((item) => item.commandId === command.id && item.status === "running")) {
      setSelectedId(command.id);
    } else {
      begin(command, "starting");
    }
  };
  const stop = (id: string) => {
    const command = commands.find(item => item.id === runs.find(run => run.id === id)?.commandId);
    if (command) begin(command, "stopping");
  };
  const replay = (id: string) => {
    const command = commands.find(item => item.id === runs.find(run => run.id === id)?.commandId);
    if (command) begin(command, "restarting");
  };
  const hideCommand = (id: string) => setCommands((items) => items.map((item) => item.id === id ? { ...item, hidden: !item.hidden } : item));
  const deleteCommand = (id: string) => {
    window.clearTimeout(pending.current.get(id)); pending.current.delete(id);
    setOperations(items => { const next = { ...items }; delete next[id]; return next; });
    setCommands(items => items.filter(item => item.id !== id));
  };
  const menus: LauncherMenuActions = {
    editCommand: () => report(m.editCommand), hideCommand, deleteCommand,
    renameProject: setSettings,
    hideProject: (id) => setProjects((items) => items.map((item) => item.id === id ? { ...item, hidden: !item.hidden } : item)),
    deleteProject: (id) => setProjects((items) => items.filter((item) => item.id !== id)),
    openDirectory: () => report(m.openProjectDirectory),
    projectOpeners: { platform: "windows", items: [{ id: "editor", name: "Visual Studio Code" }], loading: false, error: "", refresh: () => report(m.refresh) },
    openWith: () => report(m.openProjectWith), customEditor: () => report(m.otherProjectEditor),
    copy: () => report(m.copyCommand), refreshLogo: () => report(m.refreshProjectLogo),
    logoBusy: () => false, savingLogo: false, selectLogo: () => report(m.projectIcon), chooseAvatar: () => report(m.chooseProjectAvatar),
  };
  return (
    <div className="flex flex-col overflow-hidden bg-surface outline outline-1 -outline-offset-1 outline-border" style={{ width, height }}
      onKeyDown={(event) => {
        if (event.defaultPrevented || event.nativeEvent.isComposing) return;
        if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k" && !selected) {
          event.preventDefault(); searchRef.current?.focus();
        }
        if (event.key === "Escape") setSelectedId("");
      }}>
      <TitleBar m={m} title={selected?.title ?? m.productName} onBack={selected ? () => setSelectedId("") : undefined}
        controls={<WindowControls m={m} maximized={maximized} onAction={(action) => {
          if (action === "toggleMaximize") setMaximized((value) => !value);
          report(action === "close" ? m.windowClose : action === "minimize" ? m.windowMinimize : m.windowMaximize);
        }} />} />
      <WorkspaceFrame>
      <WorkspaceOverview hasDetail={!!selected} split={true}>
      <div className="mx-auto min-h-0 w-full max-w-xl flex-1 overflow-auto">
      <Launcher projects={projects} commands={commands} runs={runs} m={m} query={query}
        onRun={run} onOpen={(command) => setSelectedId(command.id)} selectedCommandId={selectedId}
        onSettings={(item) => setSettings(item.id)} expanded={expanded} onExpanded={setExpanded}
        onStop={stop} onReplay={replay} menus={menus}
        details={Object.fromEntries(projects.map((item) => [item.id, { directory: `/workspace/${item.id}`, logo: null }]))}
        onOpenProject={() => report(m.openProject)} onManage={() => report(m.manageProjects)} onClearSearch={() => setQuery("")}
        onHistory={!selected && runs.length > 0 ? () => report(m.history) : undefined}
        onGlobalSettings={!selected ? () => report(m.settings) : undefined}
        runningCount={runs.some((item) => item.status === "running") ? String(runs.filter((item) => item.status === "running").length) : undefined}
        pendingImports={[]} onRetryImport={() => report(m.retry)} savingImport={false}
        projectErrors={{}} onRetryProject={() => report(m.retry)} runOperations={operations}
        launchErrors={{}} onOutput={(item) => setSelectedId(item.commandId)} />
      </div>
      <SearchBar m={m} value={query} onChange={setQuery} inputRef={searchRef} />
      </WorkspaceOverview>
      {selected && <WorkspaceDetail as="section" split className="space-y-5 overflow-auto px-5 py-5" aria-label={m.commandDetails}>
        <code className="block break-all bg-surface-sunken p-3 text-sm">{selected.text}</code>
        <div className="flex flex-wrap items-center justify-between gap-4">
          <CommandAction m={m} name={selected.title ?? selected.id} icon={selected.icon}
            operation={operations[selected.id]?.state}
            running={selectedRun?.status === "running"} onClick={() => run(selected)}
            onStop={selectedRun ? () => stop(selectedRun.id) : undefined}
            onReplay={selectedRun ? () => replay(selectedRun.id) : undefined} />
          {selectedRun && <RunStatus state={selectedRun.status} m={m} />}
        </div>
        <dl className="space-y-3 text-xs">
          <div className="space-y-1"><dt className="text-muted-foreground">{m.directory}</dt><dd className="break-all font-mono">{selected.directory}</dd></div>
          <div className="space-y-1"><dt className="text-muted-foreground">{m.shell}</dt><dd>{m.defaultShell}</dd></div>
        </dl>
      </WorkspaceDetail>}
      </WorkspaceFrame>
      {result && <output className="block border-t border-border px-5 py-2 text-xs text-muted-foreground">{m.previewOutput} · {result}</output>}
      <Dialog open={!!project} onOpenChange={(open) => { if (!open) setSettings(""); }}>
        <DialogContent aria-describedby={undefined}>
          <ModalOverlayScope>
            <div className="flex items-center justify-between gap-4">
              <DialogTitle className="text-md font-semibold">{project?.name} · {m.projectSettings}</DialogTitle>
              <Button variant="ghost" size="sm" onClick={() => setSettings("")}>{m.close}</Button>
            </div>
            {project && <ProjectSettings project={project} commands={commands.filter((item) => item.project === project.id)}
              m={m} platform="windows" directory={`/workspace/${project.id}`} metadata={null}
              onChange={(next) => setProjects((items) => items.map((item) => item.id === next.id ? next : item))}
              onDelete={() => { menus.deleteProject(project.id); setSettings(""); }}
              onNewCommand={() => report(m.newCommand)} onImportCommands={() => report(m.addProjectCommands)}
              onEditCommand={menus.editCommand} onToggleCommand={hideCommand} onDeleteCommand={deleteCommand} />}
          </ModalOverlayScope>
        </DialogContent>
      </Dialog>
    </div>
  );
}
