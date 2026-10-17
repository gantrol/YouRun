import { useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { useDraftViewModel } from "./viewmodels/use-draft-view-model";
import { Button } from "@/renderer/components/ui/button";
import { Textarea } from "@/renderer/components/ui/textarea";
import { FieldRow, Picker, TextField } from "./controls";
import {
  commandTitle,
  type Command,
  type Project,
  type Run,
  type Field,
  type IconId,
} from "./model";
import type { Messages } from "./messages";
import { FileText, LoaderCircle, Pencil, Play, RotateCw } from "lucide-react";
import type { RunOperation } from "./models/run-operation";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { IconPicker } from "./command-icons";
import { RunOutput } from "./run-output";
import { CommandFailure } from "./command-failure";
import { DirectoryField } from "./directory-field";
import { ProjectCommandPicker } from "./project-command-picker";
import { ProjectNameField } from "./project-name-field";
import { alreadySaved, type ImportCandidate, type ImportResult } from "./project-import-data";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { FormActions } from "./form-actions";
import { CommandText } from "./command-text";

import { useViewPosition } from "@/renderer/components/ui/view-state";
import { UnsavedChangesDialog } from "./unsaved-changes-dialog";
import { onThisDevice } from "./models/workspace";
export function CommandPanel({ command, collections = [], onCollection, onAddToCollections, m, directory, values, runs, initialRunId, view: requestedView, onView,
  onDirectory, onValues, onLaunch, onStop, onReplay, onRecheck, onEdit, operation, launchError }: {
  collections?: { id: string; name: string }[]; onCollection?: (id: string) => void; onAddToCollections?: () => void;
  command: Command; m: Messages; directory: string; values: Record<string, string>; runs: Run[];
  initialRunId?: string; view?: "run" | "launch"; onView?: (view: "run" | "launch", runId?: string) => void;
  onDirectory: (value: string) => void; onValues: (value: Record<string, string>) => void;
  onLaunch: (directory: string, values: Record<string, string>) => Promise<boolean>;
  onStop: (id: string) => void; onReplay: (id: string) => void; onEdit: () => void;
  onRecheck?: () => Promise<void>;
  operation?: RunOperation; launchError?: string;
}) {
  const running = runs.find(run => ["running", "stopping"].includes(run.status));
  const pending = !!operation;
  const [localView, setLocalView] = useState<"run" | "launch">(initialRunId || running ? "run" : "launch");
  const [runId, setRunId] = useState(initialRunId);
  const active = runs.find(run => run.id === (initialRunId ?? runId)) ?? running ?? runs[0];
  const view = requestedView ?? localView;
  function selectView(next: "run" | "launch", id?: string) { setLocalView(next); setRunId(id); onView?.(next, id); }
  const [submitted, setSubmitted] = useState(false);
  const [starting, setStarting] = useState(false);
  const directoryRef = useRef<HTMLInputElement>(null);
  const fieldRefs = useRef<Record<string, HTMLInputElement | null>>({});
  const launchScroll = useRef<HTMLDivElement>(null);
  const launchPosition = useViewPosition(`command:${command.id}:launch`);
  useLayoutEffect(() => {
    if (launchScroll.current) launchScroll.current.scrollTop = launchPosition.read() ?? 0;
  }, [command.id, view]);
  return <section className="flex min-h-0 flex-1 flex-col">
    <header className="flex shrink-0 items-center gap-2 border-b border-border px-5 py-3">
      <div className="flex min-w-0 flex-1 gap-1" role="group" aria-label={m.commandText}>
        <Button variant={view === "launch" ? "secondary" : "ghost"} size="sm" aria-pressed={view === "launch"} onClick={() => selectView("launch")}>{m.start}</Button>
        <Button variant={view === "run" ? "secondary" : "ghost"} size="sm" disabled={!active} aria-pressed={view === "run"} onClick={() => selectView("run", active?.id)}>{m.outputTab}</Button>
      </div>
      <Button variant="ghost" size="sm" onClick={onEdit} data-navigation-key={`command:${command.id}:edit`}><Pencil className="size-4"/>{m.edit}</Button>
    </header>
    {view === "run" && active ? <div className="flex min-h-0 flex-1 flex-col gap-3 p-5">
      {runs.length > 1 && <Picker value={active.id} label={m.history} options={runs.map(run => [run.id,
        `${new Intl.DateTimeFormat(document.documentElement.lang, { dateStyle: "short", timeStyle: "medium" }).format(new Date(run.time))} · ${m[run.status]}`])}
        onChange={id => selectView("run", id)}/>}
      <RunOutput run={active} m={m} onStop={onStop} onReplay={onReplay} onRecheck={onRecheck} onEdit={onEdit} operation={operation} launchError={launchError} showEditAction={false}/>
    </div> : <form className="flex min-h-0 flex-1 flex-col" noValidate onSubmit={async event => {
      event.preventDefault();
      if (starting || pending) return;
      if (running) { selectView("run", running.id); return; }
      setSubmitted(true);
      if (!directory.trim()) { directoryRef.current?.focus(); return; }
      const missing = command.fields.find(field => !values[field]?.trim());
      if (missing) { fieldRefs.current[missing]?.focus(); return; }
      setStarting(true);
      try { if (await onLaunch(directory, values)) { setSubmitted(false); selectView("run", undefined); } }
      finally { setStarting(false); }
    }} onKeyDown={event => {
      if (!event.nativeEvent.isComposing && (event.metaKey || event.ctrlKey) && event.key === "Enter") { event.preventDefault(); event.currentTarget.requestSubmit(); }
    }}>
      <div ref={launchScroll} onScroll={event => launchPosition.write(event.currentTarget.scrollTop)} className="min-h-0 flex-1 space-y-4 overflow-auto p-5">
        <div className="bg-surface-sunken p-3"><CommandText text={command.text} m={m}/></div>
        <p className="truncate text-xs text-muted-foreground">{command.source ?? m.manualSource} · {command.shell}</p>
        {onAddToCollections && <div className="flex flex-wrap items-center gap-1" role="group" aria-label={m.collections}>
          {collections.map(c => <Button key={c.id} variant="ghost" size="sm" className="max-w-full" title={c.name} onClick={() => onCollection?.(c.id)} type="button"><span className="truncate">{c.name}</span></Button>)}
          <Button variant="outline" size="sm" type="button" onClick={onAddToCollections}>{m.addToCollections}</Button>
        </div>}
        <DirectoryField m={m} value={directory} inputRef={directoryRef} required onChange={onDirectory} error={submitted && !directory.trim() ? m.missingDirectory : undefined}/>
        {command.fields.map(field => <TextField key={field} label={m[field]} required inputRef={el => { fieldRefs.current[field] = el; }}
          error={submitted && !values[field]?.trim() ? m.required : undefined} value={values[field] ?? ""} onChange={event => onValues({ ...values, [field]: event.target.value })}/>)}
        {command.unavailable && <p role="status" className="text-sm text-muted-foreground">{m.unavailable} · {command.platform === "macOS / Windows" ? m.allPlatforms : command.platform}</p>}
      </div>
      <FormActions contained>
        {!pending && <CommandFailure error={launchError} m={m} onEdit={onEdit}/>}
        {active?.status === "unknown" && <span role="status" className="text-sm text-muted-foreground">{m.unknown}</span>}
        <Button size="sm" type="submit" disabled={command.unavailable || starting || pending || running?.status === "stopping"} aria-busy={starting || pending}>
          {starting || pending ? operation?.state === "restarting" ? <RotateCw className="size-4 motion-safe:animate-spin" aria-hidden="true"/> : <LoaderCircle className="size-4 motion-safe:animate-spin" aria-hidden="true"/> : running ? <FileText className="size-4"/> : <Play className="size-4"/>}
          {operation ? m[operation.state] : starting ? m.starting : running ? m.viewRun : command.external ? m.external : m.start}
        </Button>
      </FormActions>
    </form>}
  </section>;
}

export function CommandEditor({
  deleted = false,
  initial,
  projectId,
  projects,
  commands,
  m,
  onDirty,
  onSave,
  onCancel,
  autoFocus = true,
  defaultDirectory = "",
  currentPlatform = /Mac/i.test(navigator.userAgent) ? "darwin" : /Linux/i.test(navigator.userAgent) ? "linux" : "windows",
  onBusy,
}: {
  autoFocus?: boolean;
  defaultDirectory?: string;
  currentPlatform?: string;
  onBusy?: (value: boolean) => void;
  deleted?: boolean;
  initial?: Command;
  projectId?: string;
  projects: Project[];
  commands: Command[];
  m: Messages;
  onDirty: (value: boolean) => void;
  onSave: (command: Command) => Promise<void>;
  onCancel?: () => void;
}) {
  const [startingDirectory] = useState(defaultDirectory);
  const editor = useDraftViewModel({
    aliases: (initial?.aliases ?? []).join(", "), name: initial ? commandTitle(initial, m) : "", text: initial?.text ?? "",
    directory: initial?.directory ?? startingDirectory, project: initial?.project ?? projectId ?? "independent",
    shell: initial?.shell ?? "default", platform: initial?.platform ?? "macOS / Windows",
    icon: (initial?.icon ?? "play") as IconId, hidden: !!initial?.hidden, fields: initial?.fields ?? [],
    omitOutput: !!initial?.omitOutput,
  });
  const { aliases, name, text, directory, project, shell, platform, icon, hidden, fields, omitOutput } = editor.draft;
  const searchAliases = [...new Map(aliases.split(/[,，;；\n]/).map(value => value.trim()).filter(Boolean).map(value => [value.toLocaleLowerCase(), value])).values()];
  const aliasesInvalid = searchAliases.length > 20 || searchAliases.some(value => new TextEncoder().encode(value).length > 200 || value.includes("\0"));
  const setName = (value: string) => editor.setField("name", value);
  const setText = (value: string) => editor.setField("text", value);
  const setDirectory = (value: string) => editor.setField("directory", value);
  const setProject = (value: string) => editor.setField("project", value);
  const setShell = (value: string) => editor.setField("shell", value);
  const setPlatform = (value: string) => editor.setField("platform", value);
  const setIcon = (value: IconId) => editor.setField("icon", value);
  const setHidden = (value: boolean) => editor.setField("hidden", value);
  const setFields = (value: Field[]) => editor.setField("fields", value);
  const [candidate, setCandidate] = useState<ImportCandidate>();
  const [draftId] = useState(() => crypto.randomUUID());
  const [reload, setReload] = useState(false);
  const reloadButton = useRef<HTMLButtonElement>(null);
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const submitting = useRef(false);
  const submittedId = useRef<string | undefined>(undefined);
  const duplicate = candidate && alreadySaved(candidate, commands.filter((command) => command.id !== submittedId.current));
  const [submitted, setSubmitted] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  const textRef = useRef<HTMLTextAreaElement>(null);
  const textErrorId = useId();
  useEffect(() => {
    onDirty(editor.dirty || !!candidate);
  }, [editor.dirty, candidate, onDirty]);
  useEffect(() => { onBusy?.(saving); }, [saving, onBusy]);
  return (
    <form
      className="flex min-h-0 flex-1 flex-col"
      noValidate
      inert={saving}
      aria-busy={saving}
      onSubmit={async (event) => {
        event.preventDefault();
        if (submitting.current) return;
        setSubmitted(true);
        if (!name.trim()) {
          nameRef.current?.focus();
          return;
        }
        if (!text.trim()) {
          textRef.current?.focus();
          return;
        }
        if (deleted || duplicate || editor.conflict || aliasesInvalid) return;
        submitting.current = true;
        setSaving(true);
        setSaveError("");
        submittedId.current = initial?.id ?? draftId;
        try {
          await onSave(onThisDevice({
            ...initial,
            id: submittedId.current,
            title:
              initial && name.trim() === commandTitle(initial, m)
                ? initial.title
                : name.trim(),
            aliases: searchAliases,
            text: text.trim(),
            directory: directory.trim(),
            project: project === "independent" ? null : project,
            source: initial?.source ?? candidate?.source ?? null,
            shell,
            platform,
            fields,
            icon,
            hidden,
            omitOutput,
            favorite: initial?.favorite ?? false,
          }, currentPlatform));
        } catch (cause) {
          setSaveError(cause instanceof Error ? cause.message : "storageFailed");
        } finally {
          submitting.current = false;
          setSaving(false);
        }
      }}
    >
      <UnsavedChangesDialog open={reload} m={m} onContinue={() => setReload(false)} onRestoreFocus={() => reloadButton.current?.focus({ preventScroll: true })}
        onDiscard={() => { editor.reload(); setCandidate(undefined); setSaveError(""); setReload(false); }}/>
      <div className="min-h-0 flex-1 space-y-4 overflow-auto p-5">
      <FieldRow label={m.project}>
        <Picker
          value={project}
          disabled={!initial && !!projectId}
          label={m.project}
          options={[
            ["independent", m.independent],
            ...projects.map((item) => [item.id, item.name] as [string, string]),
          ]}
          onChange={(id) => {
            setProject(id);
            setCandidate(undefined);
          }}
        />
      </FieldRow>
      {!initial && !projectId && project !== "independent" && (
        <ProjectCommandPicker
          key={project}
          projectId={project}
          commands={commands.filter((command) => command.id !== submittedId.current)}
          value={candidate?.id}
          m={m}
          onChange={(next) => {
            setCandidate(next);
            if (!next) return;
            setName(next.title);
            setText(next.text);
            setDirectory(next.directory);
            setShell(next.shell);
            setPlatform(next.platform);
            setIcon(next.icon);
            setFields([]);
            setSubmitted(false);
          }}
        />
      )}
      <TextField
        label={m.commandName}
        autoFocus={autoFocus}
        inputRef={nameRef}
        error={submitted && !name.trim() ? m.required : undefined}
        required
        value={name}
        onChange={(event) => setName(event.target.value)}
      />
      <TextField label={m.searchAliases} error={aliasesInvalid ? m.invalidAliases : undefined} value={aliases} maxLength={2000} onChange={event => editor.setField("aliases", event.target.value)}/>
      <FieldRow label={m.commandText}>
        <Textarea
          required
          ref={textRef}
          aria-label={m.commandText}
          aria-invalid={submitted && !text.trim()}
          aria-describedby={submitted && !text.trim() ? textErrorId : undefined}
          value={text}
          className={`min-h-24 border-muted-foreground/75 font-mono text-sm ${submitted && !text.trim() ? "border-destructive" : ""}`}
          onChange={(event) => setText(event.target.value)}
        />
        {submitted && !text.trim() && (
          <span id={textErrorId} className="text-xs text-destructive">
            {m.required}
          </span>
        )}
      </FieldRow>
      <FieldRow label={m.shell}>
        <Picker
          value={shell}
          label={m.shell}
          options={[
            ["default", m.defaultShell],
            ["PowerShell", "PowerShell"],
            ["zsh", "zsh"],
            ["bash", "bash"],
            ...((currentPlatform === "windows" || platform === "Windows" || shell === "cmd") && platform !== "macOS"
              ? ([["cmd", "cmd"]] as [string, string][])
              : []),
          ]}
          onChange={setShell}
        />
      </FieldRow>
      <DirectoryField m={m} value={directory} onChange={setDirectory} />
      <Disclosure label={m.advancedSettings} defaultOpen={!!initial && (initial.platform !== "macOS / Windows" || initial.fields.length > 0 || !!initial.hidden || !!initial.omitOutput)}>
      <div className="space-y-4">
      <FieldRow label={m.platform}>
        <Picker
          label={m.platform}
          value={platform}
          options={[
            ["macOS / Windows", m.allPlatforms],
            ["macOS", "macOS"],
            ["Windows", "Windows"],
          ]}
          onChange={setPlatform}
        />
      </FieldRow>
      <fieldset className="space-y-2">
        <legend className="mb-2 text-sm text-foreground-secondary">
          {m.buttonIcon}
        </legend>
        <IconPicker m={m} value={icon} onChange={setIcon} />
      </fieldset>
      <fieldset className="space-y-2">
        <legend className="mb-2 text-sm text-foreground-secondary">
          {m.inputFields}
        </legend>
        {(["input", "output", "port"] as Field[]).map((field) => (
          <BooleanField
            key={field}
            label={`${m[field]} · {{${field}}}`}
            checked={fields.includes(field)}
            onChange={(checked) =>
              setFields(
                checked
                  ? [...fields, field]
                  : fields.filter((item) => item !== field),
              )
            }
          />
        ))}
      </fieldset>
      <BooleanField
        label={m.showOnHome}
        checked={!hidden}
        onChange={(checked) => setHidden(!checked)}
      />
      <BooleanField label={m.recordOutput} checked={!omitOutput} onChange={checked => editor.setField("omitOutput", !checked)} />
      </div>
      </Disclosure>
      </div>
      <FormActions contained error={deleted ? m.commandMissing : editor.conflict ? m.conflict : duplicate ? m.alreadyImported : saveError ? m[saveError as keyof Messages] ?? m.storageFailed : undefined}>
      {editor.conflict && <Button type="button" variant="outline" size="sm" ref={reloadButton} onClick={() => setReload(true)}>{m.reload}</Button>}
      <Button size="sm" type="submit" disabled={saving || deleted || !!duplicate || editor.conflict}>
        {saving ? m.saving : saveError ? m.retry : m.save}
      </Button>
      {onCancel && <Button type="button" variant="outline" size="sm" disabled={saving} onClick={onCancel}>{m.cancel}</Button>}
      </FormActions>
    </form>
  );
}

export function ProjectNameEditor({
  initialName,
  names,
  m,
  onDirty,
  onSave,
  onCancel,
}: {
  initialName: string;
  names?: ImportResult["names"];
  m: Messages;
  onDirty: (value: boolean) => void;
  onSave: (name: string) => Promise<void>;
  onCancel?: () => void;
}) {
  const editor = useDraftViewModel(initialName);
  const { draft: name, setDraft: setName } = editor;
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const submitting = useRef(false);
  const [submitted, setSubmitted] = useState(false);
  const nameRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    onDirty(editor.dirty);
  }, [editor.dirty, onDirty]);
  return (
    <form
      className="space-y-4"
      noValidate
      inert={saving}
      aria-busy={saving}
      onSubmit={async (event) => {
        event.preventDefault();
        if (submitting.current) return;
        setSubmitted(true);
        if (!name.trim()) {
          nameRef.current?.focus();
          return;
        }
        submitting.current = true;
        setSaving(true);
        setSaveError("");
        try {
          if (editor.conflict) throw new Error("conflict");
          await onSave(name.trim());
        } catch (cause) {
          setSaveError(cause instanceof Error ? cause.message : "storageFailed");
        } finally {
          submitting.current = false;
          setSaving(false);
        }
      }}
    >
      <ProjectNameField
        names={names}
        label={m.chooseProject}
        autoFocus
        inputRef={nameRef}
        error={submitted && !name.trim() ? m.required : undefined}
        value={name}
        required
        onChange={setName}
      />
      <FormActions error={editor.conflict ? m.conflict : saveError ? m[saveError as keyof Messages] ?? m.storageFailed : undefined}>
      {editor.conflict && <Button type="button" variant="outline" size="sm" onClick={() => { editor.reload(); setSaveError(""); }}>{m.reload}</Button>}
      <Button size="sm" type="submit" disabled={saving || editor.conflict}>
        {saving ? m.saving : saveError ? m.retry : m.save}
      </Button>
      {onCancel && <Button type="button" variant="outline" size="sm" disabled={saving} onClick={onCancel}>{m.cancel}</Button>}
      </FormActions>
    </form>
  );
}

function BooleanField({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (value: boolean) => void;
}) {
  const id = useId();
  return (
    <label htmlFor={id} className="flex min-h-8 items-center gap-2 text-sm">
      <Checkbox
        id={id}
        checked={checked}
        onCheckedChange={(value) => onChange(value === true)}
        className="border-muted-foreground/75"
      />
      {label}
    </label>
  );
}
