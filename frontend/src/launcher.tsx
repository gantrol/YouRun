import { useEffect, useMemo, useState } from "react";
import { CollectionTree, type CollectionActions } from "./collection-tree";
import {
  HighlightText,
  collectionPath,
  collectionSearchScore,
  commandContexts,
  commandSearchScore,
  projectSearchScore,
  rankByScore,
  searchTerms,
} from "./collections";
import { Eye, EyeOff, FolderOpen, History, Plus, Settings } from "lucide-react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/renderer/components/ui/dropdown-menu";
import { Button } from "@/renderer/components/ui/button";
import { CommandList } from "./command-list";
import { ImportRecovery } from "./import-recovery";
import { ProjectMetadataError } from "./project-metadata-controls";
import { ProjectLogo } from "./project-logo";
import { CommandFailure } from "./command-failure";
import { CommandAction } from "./command-action";
import { ProjectActivity } from "./project-activity";
import { commandTitle, isActiveRun, visibleButtons, type Collection, type Command, type Project, type Run } from "./model";
import type { Messages } from "./messages";
import { ProjectMenu, type LauncherMenuActions } from "./launcher-menu";
import type { ProjectDetails } from "./project-details";
import type { RunOperations } from "./models/run-operation";
export function Launcher({
  projects, collections = [], collectionActions, selectedCollectionId,
  commands,
  runs,
  directories = {},
  query,
  m,
  onRun,
  onOpen,
  onSettings,
  onProject,
  selectedProjectId,
  onNewCommand,
  projectOffset = 0,
  onProjectScroll,
  independentOffset = 0,
  onIndependentScroll,
  expanded,
  onExpanded,
  selectedCommandId,
  onStop,
  onReplay,
  menus,
  details,
  onOpenProject,
  onHistory,
  onGlobalSettings,
  runningCount,
  onManage,
  onClearSearch,
  pendingImports,
  onRetryImport,
  savingImport,
  projectErrors,
  onRetryProject,
  runOperations,
  launchErrors,
  onOutput,
  revealId,
  onRevealed,
}: {
  collections?: Collection[]; collectionActions?: CollectionActions; selectedCollectionId?: string;
  revealId?: string;
  onRevealed?: () => void;
  projectOffset?: number;
  onProjectScroll?: (offset: number) => void;
  independentOffset?: number;
  onIndependentScroll?: (offset: number) => void;
  onProject?: (project: Project) => void;
  selectedProjectId?: string;
  onNewCommand?: () => void;
  expanded: Record<string, boolean>;
  onExpanded: (value: Record<string, boolean>) => void;
  selectedCommandId?: string;
  projects: Project[];
  commands: Command[];
  runs: Run[];
  directories?: Record<string, string>;
  query: string;
  m: Messages;
  onRun: (command: Command) => void;
  onOpen: (command: Command) => void;
  onSettings: (project: Project) => void;
  onStop: (runId: string) => void;
  onReplay: (runId: string) => void;
  menus: LauncherMenuActions;
  details: Record<string, ProjectDetails>;
  onOpenProject: () => void;
  onHistory?: () => void;
  onGlobalSettings?: () => void;
  runningCount?: string;
  onManage: () => void;
  onClearSearch: () => void;
  pendingImports: { projectId: string; directory: string }[];
  onRetryImport: (projectId: string, directory: string) => void;
  savingImport: boolean;
  projectErrors: Record<string, string>;
  onRetryProject: (id: string) => void;
  runOperations: RunOperations;
  launchErrors: Record<string, string>;
  onOutput: (run: Run) => void;
}) {
  const term = query.trim().toLocaleLowerCase();
  const hasQuery = searchTerms(query).length > 0;
  const [showHidden, setShowHidden] = useState(false);
  const [revealedId, setRevealedId] = useState<string>();
  useEffect(() => {
    if (!revealId) return;
    setRevealedId(revealId);
    const target = commands.find(c => c.id === revealId);
    if (target && (target.hidden || (target.project && projects.find(p => p.id === target.project)?.hidden))) {
      setShowHidden(true);
    }
  }, [revealId, commands, projects]);
  useEffect(() => { setRevealedId(revealId); }, [term]);
  const indexed = useMemo(() => {
    const byId = new Map(commands.map(c => [c.id, c]));
    const projectById = new Map(projects.map(p => [p.id, p]));
    const collectionPaths = new Map(collections.map(c => [c.id, collectionPath(c, collections)]));
    const collectionsByCommandId = new Map<string, string[]>();
    const collectionIdsByCommandId = new Map<string, string[]>();
    for (const col of collections) {
      const path = collectionPaths.get(col.id) ?? col.name;
      for (const cmdId of col.commandIds) {
        const names = collectionsByCommandId.get(cmdId) ?? [];
        names.push(path);
        collectionsByCommandId.set(cmdId, names);
        const ids = collectionIdsByCommandId.get(cmdId) ?? [];
        ids.push(col.id);
        collectionIdsByCommandId.set(cmdId, ids);
      }
    }
    const contexts = commandContexts(commands, projects, m, query);
    const grouped = new Map<string, Command[]>();
    for (const c of commands) if (c.project) { const rows = grouped.get(c.project) ?? []; rows.push(c); grouped.set(c.project, rows); }
    const bestCommandScoreByProject = new Map<string, number>();
    const matchingCollections = new Set<string>();
    const scoredResults: { command: Command; score: number; index: number }[] = [];
    commands.forEach((c, index) => {
      const revealed = c.id === revealId || c.id === revealedId;
      const hidden = c.hidden || !!(c.project && projectById.get(c.project)?.hidden);
      if (!revealed && !showHidden && hidden) return;
      const score = commandSearchScore(
        c,
        query,
        m,
        projectById.get(c.project ?? "")?.name,
        collectionsByCommandId.get(c.id),
      );
      if (score > 0) {
        if (c.project) {
          const prev = bestCommandScoreByProject.get(c.project) ?? 0;
          if (score > prev) bestCommandScoreByProject.set(c.project, score);
        }
        for (const colId of collectionIdsByCommandId.get(c.id) ?? []) {
          matchingCollections.add(colId);
        }
      }
      if (revealed || (score > 0 && (!c.project || hasQuery))) {
        scoredResults.push({ command: c, score: revealed && score <= 0 ? 1 : score, index });
      }
    });
    if (hasQuery) {
      scoredResults.sort((a, b) => b.score - a.score || a.index - b.index);
    }
    return {
      byId,
      grouped,
      contexts,
      collectionPaths,
      bestCommandScoreByProject,
      matchingCollections,
      results: scoredResults.map(entry => entry.command),
    };
  }, [commands, projects, collections, query, hasQuery, showHidden, m, revealId, revealedId]);
  const visibleProjects = useMemo(() => {
    const candidates = projects.filter(p => showHidden || !p.hidden);
    if (!hasQuery) return candidates;
    return rankByScore(candidates, query, p =>
      projectSearchScore(
        p,
        query,
        details[p.id]?.directory,
        indexed.bestCommandScoreByProject.get(p.id) ?? 0,
      ),
    );
  }, [projects, showHidden, hasQuery, query, details, indexed.bestCommandScoreByProject]);
  const hasMatchingCollections = useMemo(() => {
    if (!collections.length) return false;
    if (!hasQuery) return true;
    return collections.some(c =>
      indexed.matchingCollections.has(c.id) ||
      collectionSearchScore(c, collections, query, indexed.collectionPaths.get(c.id) ?? c.name) > 0,
    );
  }, [collections, hasQuery, query, indexed.matchingCollections, indexed.collectionPaths]);
  const runIndex = useMemo(() => {
    const active = new Map<string, Run>(), latest = new Map<string, Run>();
    for (const run of runs) { if (!latest.has(run.commandId)) latest.set(run.commandId, run); if (isActiveRun(run) && !active.has(run.commandId)) active.set(run.commandId, run); }
    return { active, latest };
  }, [runs]);
  const hasHiddenItems = useMemo(
    () => projects.some(p => p.hidden) || commands.some(c => c.hidden),
    [projects, commands],
  );
  return <div className="flex min-h-0 flex-1 flex-col">
    <header className="flex h-14 shrink-0 items-center gap-1 border-b border-border px-3">
      <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">{m.projects}</h2>
      <Button variant="ghost" size="icon-sm" title={m.openProject} aria-label={m.openProject} onClick={onOpenProject}><FolderOpen className="size-4"/></Button>
      {onNewCommand && (collectionActions ? <DropdownMenu><DropdownMenuTrigger asChild><Button variant="ghost" size="icon-sm" title={m.add} aria-label={m.add}><Plus className="size-4"/></Button></DropdownMenuTrigger>
        <DropdownMenuContent align="end"><DropdownMenuItem onSelect={onNewCommand}>{m.newCommand}</DropdownMenuItem><DropdownMenuItem onSelect={() => collectionActions.create()}>{m.newCollection}</DropdownMenuItem></DropdownMenuContent>
      </DropdownMenu> : <Button variant="ghost" size="icon-sm" title={m.newCommand} aria-label={m.newCommand} onClick={onNewCommand}><Plus className="size-4"/></Button>)}
      {(hasHiddenItems || showHidden) && <Button variant="ghost" size="icon-sm" title={m.hidden} aria-label={m.hidden} aria-pressed={showHidden} onClick={() => setShowHidden(value => !value)}>{showHidden ? <Eye className="size-4"/> : <EyeOff className="size-4"/>}</Button>}
      {onHistory && <Button variant="ghost" size="icon-sm" title={m.history} aria-label={runningCount ? `${m.history} · ${m.running} ${runningCount}` : m.history} onClick={onHistory}><History className="size-4"/></Button>}
      {onGlobalSettings && <Button variant="ghost" size="icon-sm" title={m.settings} aria-label={m.settings} onClick={onGlobalSettings}><Settings className="size-4"/></Button>}
    </header>
    <CommandList revealId={revealId} onRevealed={onRevealed} initialOffset={independentOffset || projectOffset}
      onScroll={offset => { onIndependentScroll?.(offset); onProjectScroll?.(offset); }} commands={indexed.results} contexts={indexed.contexts}
      query={query} onClearSearch={onClearSearch}
      directories={directories} runs={runs} m={m} menus={menus} selectedId={selectedCommandId}
      runOperations={runOperations} launchErrors={launchErrors} onOpen={onOpen} onRun={onRun} onStop={onStop} onReplay={onReplay} onOutput={onOutput}
      header={<>
      {visibleProjects.length > 0 && <ul aria-label={m.projects} onKeyDown={event => {
        const target = event.target as HTMLElement;
        if (!target.hasAttribute("data-project-heading") || !["ArrowUp", "ArrowDown", "Home", "End"].includes(event.key)) return;
        const rows = Array.from(event.currentTarget.querySelectorAll<HTMLElement>("[data-project-heading]"));
        const current = rows.indexOf(target);
        const next = event.key === "Home" ? 0 : event.key === "End" ? rows.length - 1 : Math.max(0, Math.min(rows.length - 1, current + (event.key === "ArrowDown" ? 1 : -1)));
        event.preventDefault(); rows[next]?.focus();
      }}>
      {visibleProjects.map(project => { const children = indexed.grouped.get(project.id) ?? []; const buttons = visibleButtons(project, children); return <li key={project.id} data-project-id={project.id}>
        <ProjectMenu project={project} commands={children} runs={runs} details={details[project.id]} m={m} actions={menus}
          onRun={onRun} onStop={onStop} onReplay={onReplay} onSettings={() => onSettings(project)}>
          <div className={`flex min-h-14 min-w-0 flex-wrap items-center gap-x-2 gap-y-1 px-3 py-1 ${selectedProjectId === project.id ? "bg-selected" : "hover:bg-hover"}`}>
            <Button variant="ghost" className="h-11 min-w-0 flex-1 basis-24 justify-start gap-2 px-0 hover:bg-transparent"
              data-project-heading data-navigation-key={`project:${project.id}`} aria-current={selectedProjectId === project.id ? "page" : undefined}
              title={project.name} onClick={() => (onProject ?? onSettings)(project)}>
              <ProjectLogo name={project.name} {...details[project.id]}/><span className="truncate"><HighlightText text={project.name} query={query} /></span>
              {project.hidden && <span className="shrink-0 text-xs font-normal text-muted-foreground">{m.hidden}</span>}
            </Button>
            <div className="ml-auto flex max-w-full shrink-0 items-center">
              <ProjectActivity project={project} commands={children} activeRuns={runIndex.active} latestRuns={runIndex.latest}
                operations={runOperations} m={m} onStop={onStop} onOutput={onOutput} onOpen={onOpen} separated={buttons.length > 0}/>
              <div className="flex flex-wrap items-center justify-end gap-2">{buttons.map(button => {
                const command = button.commandId ? indexed.byId.get(button.commandId) : undefined;
                const run = command ? runIndex.active.get(command.id) : undefined;
                const output = run ?? (command ? runIndex.latest.get(command.id) : undefined);
                return <CommandAction key={button.id} m={m} name={command ? `${project.name} · ${commandTitle(command, m)}` : project.name}
                  icon={button.icon} unassigned={!command} running={!!run} stopping={run?.status === "stopping"}
                  onStop={run ? () => onStop(run.id) : undefined} onReplay={run ? () => onReplay(run.id) : undefined}
                  onOpen={command ? () => onOpen(command) : undefined} onOutput={output ? () => onOutput(output) : undefined}
                  operation={command ? runOperations[command.id]?.state : undefined} onClick={() => command ? onRun(command) : onSettings(project)}/>;
              })}</div>
            </div>
          </div>
        </ProjectMenu>
        {children.filter(c => !runOperations[c.id] && (launchErrors[c.id] || runIndex.latest.get(c.id)?.error || runIndex.latest.get(c.id)?.status === "failed")).slice(0, 3).map(c => <div key={c.id} className="px-3">
          <CommandFailure name={commandTitle(c, m)} m={m} error={launchErrors[c.id]} run={runIndex.latest.get(c.id)} onEdit={() => menus.editCommand(c.id)} onOutput={() => { const run = runIndex.latest.get(c.id); if (run) onOutput(run); }}/>
        </div>)}
        <ProjectMetadataError error={projectErrors[project.id]} busy={menus.logoBusy(project.id)} m={m} onRetry={() => onRetryProject(project.id)}/>
        {pendingImports.filter(item => item.projectId === project.id).map(item => <ImportRecovery key={item.directory} directory={item.directory} busy={savingImport} m={m} onRetry={() => onRetryImport(item.projectId, item.directory)}/>)}
      </li>; })}
    </ul>}
    {collectionActions && collections.length > 0 && <CollectionTree collections={collections} m={m} actions={collectionActions} selectedId={selectedCollectionId} query={query} matchingCollections={indexed.matchingCollections} expanded={expanded} onExpanded={onExpanded}/>}
    {indexed.results.length > 0 && <h2 className="border-t border-border px-3 py-2 text-xs text-muted-foreground">{hasQuery ? m.commandsList : m.independentCommands}</h2>}
    {!visibleProjects.length && !indexed.results.length && !hasMatchingCollections && <div className="space-y-3 px-4 py-8">
      <p role="status" className="text-sm text-muted-foreground">{hasQuery ? m.emptyResults : (projects.length > 0 || commands.length > 0) && !showHidden ? m.allItemsHidden : m.noCommands}</p>
      <div className="flex flex-wrap gap-2">
        {hasQuery ? (
          <Button variant="outline" size="sm" onClick={onClearSearch}>{m.clear}</Button>
        ) : (projects.length > 0 || commands.length > 0) && !showHidden ? (
          <>
            <Button variant="outline" size="sm" onClick={() => setShowHidden(true)}>{m.show}</Button>
            <Button variant="ghost" size="sm" onClick={onManage}>{m.settings}</Button>
          </>
        ) : (
          <Button variant="outline" size="sm" onClick={onOpenProject}>{m.openProject}</Button>
        )}
      </div>
    </div>}
    </>}/>
  </div>;
}
