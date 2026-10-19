import { useEffect, useMemo, useState, type ComponentProps, type Ref } from "react";
import { Plus, Settings2, Eye, EyeOff, X } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { CommandList } from "./command-list";
import { commandSearchScore, rankByScore, searchTerms } from "./collections";
import { type Project, type Run } from "./model";
import { SearchBar } from "./search-bar";
import type { ProjectWorkspaceDetail } from "./project-details";
import {
  ProjectWorkspaceBar,
  ProjectWorkspacesManager,
  isSubOrSamePath,
  resolveCommandWorkspaceDirectory,
} from "./project-workspaces";

export function ProjectCommands({
  project,
  onSettings,
  onAdd,
  searchRef,
  workspaces,
  activeWorkspaceId,
  onMutateWorkspaces,
  onOpenPortForRun,
  ...props
}: ComponentProps<typeof CommandList> & {
  project: Project;
  onSettings: () => void;
  onAdd: () => void;
  searchRef?: Ref<HTMLInputElement>;
  workspaces?: ProjectWorkspaceDetail[];
  activeWorkspaceId?: string;
  onMutateWorkspaces?: (payload: {
    action: "select" | "add" | "rename" | "setDefault" | "remove";
    locationId?: string;
    name?: string;
    directory?: string;
    chooseDirectory?: boolean;
  }) => Promise<unknown>;
  onOpenPortForRun?: (run: Run) => void;
}) {
  const [showHidden, setShowHidden] = useState(false);
  const [query, setQuery] = useState("");
  const [managingWorkspaces, setManagingWorkspaces] = useState(false);
  const hasQuery = searchTerms(query).length > 0;
  useEffect(() => {
    if (props.revealId && props.commands.some((c) => c.id === props.revealId && c.hidden))
      setShowHidden(true);
  }, [props.revealId, props.commands]);
  useEffect(() => {
    if (props.revealId) setQuery("");
  }, [props.revealId]);

  const activeWorkspace = useMemo(() => {
    if (!workspaces || workspaces.length === 0) return undefined;
    return (
      workspaces.find((w) => w.id === activeWorkspaceId) ??
      workspaces.find((w) => w.isDefault) ??
      workspaces[0]
    );
  }, [workspaces, activeWorkspaceId]);

  const visible = useMemo(() => {
    const candidates = props.commands
      .filter((c) => c.project === project.id && (c.id === props.revealId || showHidden || !c.hidden))
      .map((c) =>
        workspaces && workspaces.length > 1
          ? { ...c, directory: resolveCommandWorkspaceDirectory(c, workspaces, activeWorkspaceId) }
          : c,
      );
    return rankByScore(candidates, query, (c) => {
      const score = commandSearchScore(c, query, props.m, project.name);
      return c.id === props.revealId ? Math.max(1, score) : score;
    });
  }, [props.commands, project.id, project.name, showHidden, props.revealId, props.m, query, workspaces, activeWorkspaceId]);

  const workspaceScopedRuns = useMemo(() => {
    if (!workspaces || workspaces.length <= 1 || !activeWorkspace) return props.runs;
    return props.runs.filter(
      (r) =>
        !(r.status === "running" || r.status === "stopping") ||
        isSubOrSamePath(activeWorkspace.directory, r.directory),
    );
  }, [props.runs, workspaces, activeWorkspace]);

  return (
    <section
      className="workspace-project flex min-h-0 min-w-0 flex-1 flex-col bg-surface"
      aria-label={`${project.name} · ${props.m.commandsList}`}
    >
      <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-4">
        <h2 className="min-w-0 flex-1 truncate text-base font-semibold" title={project.name}>
          {project.name}
        </h2>
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={props.m.newCommand}
          title={props.m.newCommand}
          onClick={onAdd}
          data-navigation-key={`project:${project.id}:add`}
        >
          <Plus className="size-4" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={props.m.hidden}
          title={props.m.hidden}
          aria-pressed={showHidden}
          onClick={() => setShowHidden((value) => !value)}
        >
          {showHidden ? <Eye className="size-4" /> : <EyeOff className="size-4" />}
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          title={props.m.projectSettings}
          aria-label={props.m.projectSettings}
          onClick={onSettings}
          data-navigation-key={`project:${project.id}:settings`}
        >
          <Settings2 className="size-4" />
        </Button>
      </header>
      {workspaces && workspaces.length > 1 && onMutateWorkspaces && (
        <ProjectWorkspaceBar
          project={project}
          workspaces={workspaces}
          activeWorkspaceId={activeWorkspaceId}
          runs={props.runs}
          commands={props.commands}
          m={props.m}
          onSelectWorkspace={(locationId) => {
            void onMutateWorkspaces({ action: "select", locationId });
          }}
          onManageWorkspaces={() => setManagingWorkspaces((v) => !v)}
          onOpenRun={(runId) => {
            const target = props.runs.find((r) => r.id === runId);
            if (target) props.onOutput(target);
          }}
          onOpenPortForRun={onOpenPortForRun}
        />
      )}
      {managingWorkspaces && workspaces && onMutateWorkspaces && (
        <div className="shrink-0 border-b border-border bg-surface p-4">
          <div className="mb-2 flex items-center justify-end">
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => setManagingWorkspaces(false)}
              aria-label={props.m.close}
            >
              <X className="size-4" />
            </Button>
          </div>
          <ProjectWorkspacesManager
            project={project}
            workspaces={workspaces}
            runs={props.runs}
            commands={props.commands}
            m={props.m}
            onMutate={onMutateWorkspaces}
          />
        </div>
      )}
      <CommandList
        {...props}
        runs={workspaceScopedRuns}
        commands={visible}
        query={query}
        onClearSearch={() => setQuery("")}
      />
      <SearchBar
        m={props.m}
        value={query}
        onChange={setQuery}
        inputRef={searchRef}
        count={hasQuery ? visible.length : undefined}
      />
    </section>
  );
}


