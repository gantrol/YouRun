import { useEffect, useRef, useState, type ComponentProps } from "react";
import { Copy, ImagePlus, RefreshCw } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { api } from "./api";
import { FormActions } from "./form-actions";
import { ProjectSettings } from "./settings-panels";
import { ProjectNameField } from "./project-name-field";
import { ProjectIconPicker } from "./project-icon-picker";
import { ProjectMetadataError, type ProjectMetadataControls } from "./project-metadata-controls";
import { ProjectOpenersDropdown } from "./project-openers";
import { ScanIssues } from "./scan-issues";
import { UnsavedChangesDialog } from "./unsaved-changes-dialog";
import { useDraftViewModel } from "./viewmodels/use-draft-view-model";
import { ProjectWorkspacesManager } from "./project-workspaces";
import type { Project, Run } from "./model";
import type { Messages } from "./messages";

export type ProjectLogoDraft = { revision: number; logoId: string; avatar: string };
type Props = ComponentProps<typeof ProjectSettings> & Pick<ComponentProps<typeof ProjectMetadataControls>,
  "details" | "busy" | "error" | "onDirty" | "onRescan" | "onRetry" | "onOpenDirectory" | "onCopyDirectory" | "openers" | "onOpenWith" | "onCustomEditor"> & {
  deleted?: boolean;
  runs?: Run[];
  onMutateWorkspaces?: (payload: {
    action: "select" | "add" | "rename" | "setDefault" | "remove";
    locationId?: string;
    name?: string;
    directory?: string;
    chooseDirectory?: boolean;
  }) => Promise<unknown>;
  onSave: (base: Project, project: Project, logo?: ProjectLogoDraft) => Promise<void>;
  onCancel: () => void;
  onBusy: (value: boolean) => void;
};

export function ProjectEditor(props: Props) {
  const { project, details, m, onDirty, onBusy } = props;
  const editor = useDraftViewModel(project);
  const [choice, setChoice] = useState<ProjectLogoDraft>();
  const [avatarLogo, setAvatarLogo] = useState<NonNullable<typeof details>["logo"]>();
  const [saving, setSaving] = useState(false), [picking, setPicking] = useState(false);
  const [error, setError] = useState("");
  const reloadTrigger = useRef<HTMLButtonElement>(null);
  const [reload, setReload] = useState(false);
  const operation = useRef(false);
  useEffect(() => { onDirty(editor.dirty || !!choice || picking); }, [editor.dirty, choice, picking, onDirty]);
  useEffect(() => { onBusy(saving || picking); }, [saving, picking, onBusy]);
  const iconChangedElsewhere = !!choice && choice.revision !== (details?.revision ?? 0);
  const conflict = editor.conflict || iconChangedElsewhere;
  const iconDetails = choice && details ? { ...details, selectedLogo: choice.logoId, customAvatar: !!choice.avatar,
    logo: choice.avatar ? avatarLogo ?? null : details.logos?.find(logo => logo.id === choice.logoId) ?? null } : details;
  async function save() {
    if (operation.current || props.deleted || conflict) return;
    if (!editor.draft.name.trim()) { setError("required"); return; }
    operation.current = true; setSaving(true); setError("");
    try { await props.onSave(editor.base, { ...editor.draft, name: editor.draft.name.trim() }, choice); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "storageFailed"); }
    finally { operation.current = false; setSaving(false); }
  }
  return <section className="flex min-h-0 flex-1 flex-col" aria-busy={saving || picking} onKeyDown={event => {
    if (!event.nativeEvent.isComposing && (event.ctrlKey || event.metaKey) && event.key === "Enter") { event.preventDefault(); void save(); }
  }}>
    <UnsavedChangesDialog open={reload} m={m} onRestoreFocus={() => reloadTrigger.current?.focus({ preventScroll: true })} onContinue={() => setReload(false)} onDiscard={() => {
      editor.reload(); setChoice(undefined); setAvatarLogo(undefined); setError(""); setReload(false); props.onRetry();
    }}/>
    <div className="min-h-0 flex-1 overflow-auto p-5" inert={saving || picking}>
      <ProjectSettings {...props} propertiesOnly project={editor.draft} onChange={editor.setDraft} metadata={<div className="space-y-5">
        <ProjectNameField label={m.chooseProject} names={details?.names} value={editor.draft.name} onChange={name => editor.setField("name", name)} required/>
        <ProjectIconPicker name={editor.draft.name} details={iconDetails} disabled={!details || props.busy || picking} m={m} onChange={logoId => {
          setChoice({ revision: choice?.revision ?? details?.revision ?? 0, logoId, avatar: "" }); setAvatarLogo(undefined);
        }}>
          <Button variant="ghost" size="icon-sm" disabled={props.busy} onClick={props.onRescan} title={m.refreshProjectLogo} aria-label={m.refreshProjectLogo}><RefreshCw className="size-4"/></Button>
          <Button variant="ghost" size="icon-sm" disabled={!details || props.busy || picking} title={m.chooseProjectAvatar} aria-label={m.chooseProjectAvatar} onClick={async () => {
            if (operation.current) return;
            operation.current = true; setPicking(true); setError("");
            try {
              const next = await api<{ cancelled?: boolean; avatar: string; logo: NonNullable<typeof details>["logo"] }>("project-avatar", "POST", { id: project.id, revision: details?.revision ?? 0, preview: true });
              if (!next.cancelled) { setChoice({ revision: choice?.revision ?? details?.revision ?? 0, logoId: "", avatar: next.avatar }); setAvatarLogo(next.logo); }
            } catch (cause) { setError(cause instanceof Error ? cause.message : "avatarPickerFailed"); }
            finally { operation.current = false; setPicking(false); }
          }}><ImagePlus className="size-4"/></Button>
        </ProjectIconPicker>
        <ProjectMetadataError error={props.error} busy={props.busy} m={m} onRetry={props.onRetry}/>
        <ScanIssues issues={details?.issues} m={m}/>
        <div className="space-y-2">
          <div className="flex items-center gap-2"><span className="mr-auto text-sm">{m.projectDirectory}</span>
            <ProjectOpenersDropdown openers={props.openers} m={m} disabled={!details?.directory} onOpen={props.onOpenWith} onCustom={props.onCustomEditor} onOpenDirectory={props.onOpenDirectory}/>
            <Button variant="ghost" size="icon-sm" disabled={!details?.directory} onClick={props.onCopyDirectory} title={m.copyProjectPath} aria-label={m.copyProjectPath}><Copy className="size-4"/></Button>
          </div>
          <code className="block break-all text-xs text-muted-foreground">{details?.directory || m.unbound}</code>
        </div>
        {details?.workspaces && props.onMutateWorkspaces && (
          <ProjectWorkspacesManager
            project={project}
            workspaces={details.workspaces}
            runs={props.runs ?? []}
            commands={props.commands}
            m={m}
            onMutate={props.onMutateWorkspaces}
          />
        )}
      </div>}/>
    </div>
    <FormActions contained error={props.deleted ? m.projectMissing : conflict ? m.conflict : error ? m[error as keyof Messages] ?? m.storageFailed : undefined}>
      <Button size="sm" disabled={saving || picking || props.deleted || conflict || (!editor.dirty && !choice)} onClick={() => void save()}>{saving ? m.saving : error ? m.retry : m.save}</Button>
      <Button size="sm" variant="outline" disabled={saving || picking} onClick={props.onCancel}>{m.cancel}</Button>
      {(conflict || error === "conflict") && <Button size="sm" variant="outline" ref={reloadTrigger} onClick={() => setReload(true)}>{m.reload}</Button>}
    </FormActions>
  </section>;
}
