import { useState } from "react";
import { Check, FolderPlus, GitBranch, Pencil, Star, Trash2, X } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Input } from "@/renderer/components/ui/input";
import { Picker } from "./controls";
import type { Command, Project, Run, SourceSnapshot } from "./model";
import type { ProjectWorkspaceDetail } from "./project-details";
import type { Messages } from "./messages";

export function samePathNormalized(a: string, b: string): boolean {
  if (!a || !b) return false;
  const norm = (v: string) => v.trim().replace(/[\\/]+$/, "").replace(/\\/g, "/").toLowerCase();
  return norm(a) === norm(b);
}

export function isSubOrSamePath(parentDir: string, childDir: string): boolean {
  if (!parentDir || !childDir) return false;
  const norm = (v: string) => v.trim().replace(/[\\/]+$/, "").replace(/\\/g, "/").toLowerCase();
  const p = norm(parentDir);
  const c = norm(childDir);
  return c === p || c.startsWith(p + "/");
}

export function resolveCommandWorkspaceDirectory(
  command: Command,
  workspaces?: ProjectWorkspaceDetail[],
  activeWorkspaceId?: string,
): string {
  if (!command.project || !workspaces || workspaces.length === 0) {
    return command.directory;
  }
  const active = workspaces.find((w) => w.id === activeWorkspaceId) ?? workspaces.find((w) => w.isDefault) ?? workspaces[0];
  if (!active) return command.directory;
  if (!command.directory.trim()) return active.directory;
  const followsWorkspaceRoot = workspaces.some((w) => samePathNormalized(w.directory, command.directory));
  return followsWorkspaceRoot ? active.directory : command.directory;
}

export function formatSourceSummary(source: SourceSnapshot | null | undefined, m: Messages): string {
  if (!source) return m.sourceUnrecorded;
  if (source.inaccessible) return m.dirInaccessible;
  if (source.nonGit || (!source.branch && !source.shortCommit && !source.commit)) {
    return m.sourceUnrecorded;
  }
  const ref = source.branch || (source.detached ? m.detachedHead : "") || source.tag || "";
  const commit = source.shortCommit || (source.commit ? source.commit.slice(0, 7) : "");
  const tree = source.dirty ? m.dirtyModified : m.cleanTree;
  return [ref, commit, tree].filter(Boolean).join(" · ");
}

export function formatSourceOptionLabel(ws: ProjectWorkspaceDetail, m: Messages): string {
  const name = ws.name === "主工作区" ? m.mainWorkspace : ws.name;
  const src = ws.source;
  if (!src || src.inaccessible || src.nonGit) {
    return name;
  }
  const ref = src.branch || (src.detached ? m.detachedHead : "");
  const commit = src.shortCommit || (src.commit ? src.commit.slice(0, 7) : "");
  const suffix = [ref, commit].filter(Boolean).join(" · ");
  return suffix ? `${name} · ${suffix}` : name;
}

export function ProjectWorkspaceBar({
  project,
  workspaces,
  activeWorkspaceId,
  runs,
  commands,
  m,
  onSelectWorkspace,
  onManageWorkspaces,
  onOpenRun,
  onOpenPortForRun,
}: {
  project: Project;
  workspaces: ProjectWorkspaceDetail[];
  activeWorkspaceId?: string;
  runs: Run[];
  commands: Command[];
  m: Messages;
  onSelectWorkspace: (locationId: string) => void;
  onManageWorkspaces: () => void;
  onOpenRun?: (runId: string) => void;
  onOpenPortForRun?: (run: Run) => void;
}) {
  if (workspaces.length <= 1) return null;
  const active = workspaces.find((w) => w.id === activeWorkspaceId) ?? workspaces.find((w) => w.isDefault) ?? workspaces[0];
  if (!active) return null;

  const projectCommandIds = new Set(commands.filter((c) => c.project === project.id).map((c) => c.id));
  const otherActiveRuns = runs.filter(
    (r) =>
      (r.status === "running" || r.status === "stopping") &&
      projectCommandIds.has(r.commandId) &&
      !isSubOrSamePath(active.directory, r.directory),
  );

  const options: [string, string][] = workspaces.map((ws) => [ws.id, formatSourceOptionLabel(ws, m)]);

  return (
    <div className="shrink-0 space-y-2 border-b border-border bg-surface-sunken/50 px-4 py-2.5 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className=" font-medium text-foreground-secondary">{m.workspace}</span>
        <div className="min-w-48 flex-1">
          <Picker
            label={m.workspace}
            value={active.id}
            options={options}
            onChange={onSelectWorkspace}
            className="h-8 text-xs"
          />
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={onManageWorkspaces}
          data-navigation-key={`project:${project.id}:workspaces`}
        >
          {m.manageWorkspaces}
        </Button>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-2 text-muted-foreground">
        <span className="inline-flex min-w-0 items-center gap-1.5 truncate font-mono">
          <GitBranch className="size-3.5 shrink-0" aria-hidden="true" />
          <span className="truncate">{formatSourceSummary(active.source, m)}</span>
        </span>
        <span className="truncate font-mono" title={active.directory}>
          {m.commandsLaunchInDir.replace("%s", active.directory)}
        </span>
      </div>
      {otherActiveRuns.length > 0 && (
        <div className="space-y-1.5 border-t border-border pt-2">
          <div className="font-medium text-foreground-secondary">{m.otherWorkspaceRuns}</div>
          {otherActiveRuns.map((run) => {
            const matchedWs = workspaces.find((w) => isSubOrSamePath(w.directory, run.directory));
            const wsDisplayName = matchedWs
              ? matchedWs.name === "主工作区"
                ? m.mainWorkspace
                : matchedWs.name
              : run.launchSource?.workspaceName || run.directory;
            const cmd = commands.find((c) => c.id === run.commandId);
            return (
              <div
                key={run.id}
                className="flex flex-wrap items-center justify-between gap-2 border border-border bg-background px-2.5 py-1.5"
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 truncate font-medium text-foreground">
                    <span className="truncate">{wsDisplayName}</span>
                    <span>·</span>
                    <span className="truncate font-mono">{cmd?.title ?? run.title}</span>
                  </div>
                  <div className="truncate font-mono text-[11px] text-muted-foreground">
                    {formatSourceSummary(run.launchSource ?? matchedWs?.source, m)} · {run.directory}
                  </div>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  {onOpenRun && (
                    <Button variant="outline" size="xs" onClick={() => onOpenRun(run.id)}>
                      {m.viewRun}
                    </Button>
                  )}
                  {onOpenPortForRun && (
                    <Button variant="ghost" size="xs" onClick={() => onOpenPortForRun(run)}>
                      {m.viewPorts}
                    </Button>
                  )}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function ProjectWorkspacesManager({
  project,
  workspaces = [],
  runs,
  commands,
  m,
  onMutate,
}: {
  project: Project;
  workspaces?: ProjectWorkspaceDetail[];
  runs: Run[];
  commands: Command[];
  m: Messages;
  onMutate: (payload: {
    action: "select" | "add" | "rename" | "setDefault" | "remove";
    locationId?: string;
    name?: string;
    directory?: string;
    chooseDirectory?: boolean;
  }) => Promise<unknown>;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [editingId, setEditingId] = useState<string | null>(null);
  const [editingName, setEditingName] = useState("");

  const projectCommandIds = new Set(commands.filter((c) => c.project === project.id).map((c) => c.id));
  const projectActiveRuns = runs.filter(
    (r) => (r.status === "running" || r.status === "stopping") && projectCommandIds.has(r.commandId),
  );

  async function runAction(payload: Parameters<typeof onMutate>[0]) {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await onMutate(payload);
      if (payload.action === "rename") {
        setEditingId(null);
      }
    } catch (cause) {
      const code = cause instanceof Error ? cause.message : "projectMetadataSaveFailed";
      setError(m[code as keyof Messages] ?? m.projectMetadataSaveFailed);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-sm font-semibold">{m.manageExistingWorkspaces}</h3>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy || workspaces.length >= 8}
          onClick={() => void runAction({ action: "add", chooseDirectory: true })}
        >
          <FolderPlus className="size-4" aria-hidden="true" />
          {m.associateExistingDir}
        </Button>
      </div>
      {error && (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
      <ol className="divide-y divide-border border border-border bg-background">
        {workspaces.map((ws) => {
          const activeCount = projectActiveRuns.filter((r) => isSubOrSamePath(ws.directory, r.directory)).length;
          const isEditing = editingId === ws.id;
          const displayName = ws.name === "主工作区" ? m.mainWorkspace : ws.name;
          return (
            <li key={ws.id} className="space-y-2 p-3">
              <div className="flex flex-wrap items-center justify-between gap-2">
                {isEditing ? (
                  <div className="flex min-w-0 flex-1 items-center gap-1.5">
                    <Input
                      value={editingName}
                      onChange={(e) => setEditingName(e.target.value)}
                      className="h-7 max-w-56 text-xs"
                      autoFocus
                      onKeyDown={(e) => {
                        if (e.key === "Enter" && editingName.trim()) {
                          e.preventDefault();
                          void runAction({ action: "rename", locationId: ws.id, name: editingName.trim() });
                        } else if (e.key === "Escape") {
                          setEditingId(null);
                        }
                      }}
                    />
                    <Button
                      type="button"
                      variant="outline"
                      size="icon-sm"
                      disabled={busy || !editingName.trim()}
                      onClick={() => void runAction({ action: "rename", locationId: ws.id, name: editingName.trim() })}
                      aria-label={m.save}
                    >
                      <Check className="size-3.5" />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon-sm"
                      onClick={() => setEditingId(null)}
                      aria-label={m.cancel}
                    >
                      <X className="size-3.5" />
                    </Button>
                  </div>
                ) : (
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="truncate text-sm font-medium">{displayName}</span>
                    {ws.isDefault && (
                      <span className="border border-border bg-surface-sunken px-1.5 py-0.5 text-[11px] text-foreground-secondary">
                        {m.defaultWorkspaceTag}
                      </span>
                    )}
                    <span className="text-xs text-muted-foreground">
                      {activeCount > 0 ? `${activeCount} ${m.workspaceActiveRunsCount}` : m.idleState}
                    </span>
                  </div>
                )}
                {!isEditing && (
                  <div className="flex shrink-0 items-center gap-1">
                    <Button
                      type="button"
                      variant="ghost"
                      size="xs"
                      disabled={busy}
                      onClick={() => {
                        setEditingId(ws.id);
                        setEditingName(displayName);
                      }}
                    >
                      <Pencil className="size-3.5" aria-hidden="true" />
                      {m.renameWorkspace}
                    </Button>
                    {!ws.isDefault && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        disabled={busy}
                        onClick={() => void runAction({ action: "setDefault", locationId: ws.id })}
                      >
                        <Star className="size-3.5" aria-hidden="true" />
                        {m.setAsDefault}
                      </Button>
                    )}
                    {workspaces.length > 1 && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="xs"
                        disabled={busy}
                        className="text-muted-foreground hover:text-destructive"
                        onClick={() => void runAction({ action: "remove", locationId: ws.id })}
                      >
                        <Trash2 className="size-3.5" aria-hidden="true" />
                        {m.removeAssociation}
                      </Button>
                    )}
                  </div>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs text-muted-foreground">
                <span className="break-all">{ws.directory}</span>
                <span>·</span>
                <span>{formatSourceSummary(ws.source, m)}</span>
              </div>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
