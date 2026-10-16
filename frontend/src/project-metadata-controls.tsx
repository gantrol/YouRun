import { useEffect, useRef, useState } from "react";
import { useDraftViewModel } from "./viewmodels/use-draft-view-model";
import { Copy, ImagePlus, RefreshCw } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { ProjectNameField } from "./project-name-field";
import { ProjectIconPicker } from "./project-icon-picker";
import { ScanIssues } from "./scan-issues";
import type { ProjectDetails } from "./project-details";
import type { Project } from "./model";
import type { Messages } from "./messages";
import { ProjectOpenersDropdown, type ProjectOpeners } from "./project-openers";

export function ProjectMetadataError({ error, busy, m, onRetry }: {
  error?: string; busy: boolean; m: Messages; onRetry: () => void;
}) {
  if (!error) return null;
  return (
    <div role="alert" className="flex flex-wrap items-center gap-2 px-2 py-2 text-xs text-destructive">
      <span>{m[error as keyof Messages] ?? m.logoScanFailed}</span>
      <Button variant="outline" size="xs" disabled={busy} onClick={onRetry}>{busy ? m.scanning : m.retry}</Button>
    </div>
  );
}

export function ProjectMetadataControls({ project, details, busy, savingLogo, error, m,
  onDirty, onSaveName, onSelectLogo, onChooseAvatar, onRescan, onRetry, onOpenDirectory, onCopyDirectory,
  openers, onOpenWith, onCustomEditor,
}: {
  project: Project;
  details?: ProjectDetails;
  busy: boolean;
  savingLogo: boolean;
  error?: string;
  m: Messages;
  onDirty: (value: boolean) => void;
  onSaveName: (name: string) => Promise<void>;
  onSelectLogo: (id: string) => void;
  onChooseAvatar: () => void;
  onRescan: () => void;
  onRetry: () => void;
  onOpenDirectory: () => void;
  onCopyDirectory: () => void;
  openers: ProjectOpeners;
  onOpenWith: (id: string) => void;
  onCustomEditor: () => void;
}) {
  const editor = useDraftViewModel(project.name);
  const { draft: name, setDraft: setName } = editor;
  const [saving, setSaving] = useState(false);
  const [nameError, setNameError] = useState("");
  const submitting = useRef(false);
  useEffect(() => { onDirty(editor.dirty); }, [editor.dirty, onDirty]);
  return (
    <div className="@container/metadata space-y-5">
      <div className="grid items-start gap-4 @min-[32rem]/metadata:grid-cols-2">
        <form className="min-w-0 space-y-2" onSubmit={async (event) => {
          event.preventDefault();
          if (!name.trim() || submitting.current) return;
          submitting.current = true;
          setSaving(true);
          setNameError("");
          try { if (editor.conflict) throw new Error("conflict"); await onSaveName(name.trim()); editor.accept(name.trim()); }
          catch (cause) { setNameError(cause instanceof Error ? cause.message : "storageFailed"); }
          finally { submitting.current = false; setSaving(false); }
        }}>
          <ProjectNameField label={m.chooseProject} names={details?.names} value={name} onChange={setName} disabled={saving} required />
          {(nameError || editor.conflict) && <p role="alert" className="text-xs text-destructive">{editor.conflict ? m.conflict : m[nameError as keyof Messages] ?? m.storageFailed}</p>}
          {editor.conflict && <Button type="button" variant="outline" size="sm" onClick={() => { editor.reload(); setNameError(""); }}>{m.reload}</Button>}
          {name !== project.name && <Button type="submit" variant="outline" size="sm" disabled={saving || !name.trim() || editor.conflict}>{saving ? m.saving : nameError ? m.retry : m.save}</Button>}
        </form>
        <div className="min-w-0">
          <ProjectIconPicker name={project.name} details={details} disabled={busy || savingLogo} m={m} onChange={onSelectLogo}>
            <Button variant="ghost" size="icon-sm" disabled={busy || savingLogo} onClick={onRescan}
              title={busy ? m.scanning : m.refreshProjectLogo} aria-label={busy ? m.scanning : m.refreshProjectLogo}>
              <RefreshCw className={`size-4 ${busy ? "motion-safe:animate-spin" : ""}`} aria-hidden="true" />
            </Button>
            <Button variant="ghost" size="icon-sm" disabled={busy || savingLogo} onClick={onChooseAvatar}
              title={m.chooseProjectAvatar} aria-label={m.chooseProjectAvatar}>
              <ImagePlus className="size-4" aria-hidden="true" />
            </Button>
          </ProjectIconPicker>
        </div>
      </div>
      {(error || !!details?.issues?.length) && <div>
        <ProjectMetadataError error={error} busy={busy || savingLogo} m={m} onRetry={onRetry} />
        <ScanIssues issues={details?.issues} m={m} />
      </div>}
      <div className="space-y-2" role="group" aria-label={m.projectDirectory}>
        <div className="flex flex-wrap items-center gap-2">
          <span className="mr-auto text-sm text-foreground-secondary">{m.projectDirectory}</span>
          <ProjectOpenersDropdown openers={openers} m={m} disabled={!details?.directory}
            onOpen={onOpenWith} onCustom={onCustomEditor} onOpenDirectory={onOpenDirectory} />
          <Button variant="ghost" size="icon-sm" disabled={!details?.directory} onClick={onCopyDirectory}
            title={m.copyProjectPath} aria-label={m.copyProjectPath}>
            <Copy className="size-4" aria-hidden="true" />
          </Button>
        </div>
        <code className="block select-text break-all text-xs text-muted-foreground">{details?.directory || m.unbound}</code>
      </div>
    </div>
  );
}
