import { useEffect, useMemo, useRef, useState, type ReactNode, type RefCallback, type UIEventHandler } from "react";
import { FolderOpen, LoaderCircle, Search, X } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { Input } from "@/renderer/components/ui/input";
import { FieldRow, Picker } from "./controls";
import { HighlightText, rankByScore, scoreMultiTerm, searchTerms } from "./collections";
import { ProjectNameField } from "./project-name-field";
import { ProjectIconPicker } from "./project-icon-picker";
import type { ProjectDetails } from "./project-details";
import { projectAPI } from "./project-api";
import { recommendLaunchButtons, type LaunchRecommendation } from "./launch-recommendations";
import { ImportLaunchButtons, launchSlots } from "./import-launch-buttons";
import { ScanIssues } from "./scan-issues";
import { api } from "./api";
import type { Command, Project } from "./model";
import type { Messages } from "./messages";
import { alreadySaved, ImportMetadataError, type ImportResult, type ImportLogoChoice } from "./project-import-data";
import { ExpandableList } from "./expandable-list";
import { CommandText } from "./command-text";

type ImportDraft = {
  result: ImportResult;
  name: string;
  projectId: string;
  selected: Set<string>;
  buttons: LaunchRecommendation[] | null;
  logoChoice: ImportLogoChoice | null;
};

export function ProjectImport({
  m, projects, commands, platform, lockedProject, initialDirectory, onSave, onRetryMetadata, onCancel, onDirty, recovery, onManual, scrollRef, onScroll, onBusy, onPartial,
}: {
  onBusy?: (busy: boolean) => void;
  onPartial?: (partial: boolean) => void;
  m: Messages;
  projects: Project[];
  commands: Command[];
  platform: string;
  lockedProject?: Project;
  initialDirectory?: string;
  onSave: (result: ImportResult, ids: Set<string>, name: string, projectId: string, buttons: LaunchRecommendation[], logoChoice: ImportLogoChoice | null) => Promise<void>;
  onCancel: () => void;
  onDirty?: (dirty: boolean) => void;
  onRetryMetadata: (projectId: string, directory: string, logoChoice: ImportLogoChoice | null) => Promise<void>;
  recovery?: ReactNode;
  onManual?: () => void;
  scrollRef?: RefCallback<HTMLDivElement>;
  onScroll?: UIEventHandler<HTMLDivElement>;
}) {
  const [draft, setDraft] = useState<ImportDraft>();
  const currentDraft = useRef(draft);
  const drafts = useRef(new Map<string, ImportDraft>());
  const [busy, setBusy] = useState(false);
  const [picking, setPicking] = useState(false);
  const [scanDirectory, setScanDirectory] = useState("");
  const [saving, setSaving] = useState(false);
  const [draftChanged, setDraftChanged] = useState(false);
  const [error, setError] = useState("");
  const [candidateQuery, setCandidateQuery] = useState("");
  const [metadataPending, setMetadataPending] = useState<{ projectId: string; directory: string }>();
  const [iconDetails, setIconDetails] = useState<{ projectId: string; details: ProjectDetails }>();
  const [iconLoading, setIconLoading] = useState(false);
  const [iconError, setIconError] = useState("");
  const [iconRefresh, setIconRefresh] = useState(0);
  const controller = useRef<AbortController | null>(null);
  const generation = useRef(0);
  const operation = useRef(false);
  const mounted = useRef(true);
  const scanSource = useRef<{ directory?: string; projectId?: string } | undefined>(undefined);
  const commandsRef = useRef(commands);
  commandsRef.current = commands;
  const draftKey = (directory: string) => lockedProject ? lockedProject.id
    : platform === "windows" ? directory.replaceAll("\\", "/").toLowerCase() : directory;

  function replace(next: ImportDraft | undefined) {
    currentDraft.current = next;
    if (next) drafts.current.set(draftKey(next.result.directory), next);
    setDraft(next);
  }
  function change(patch: Partial<ImportDraft>) {
    if (currentDraft.current) {
      replace({ ...currentDraft.current, ...patch });
      setDraftChanged(true);
    }
  }
  useEffect(() => { onDirty?.(draftChanged); }, [draftChanged, onDirty]);

  useEffect(() => { onBusy?.(saving || picking); }, [saving, picking, onBusy]);
  useEffect(() => { onPartial?.(!!metadataPending); }, [metadataPending, onPartial]);

  async function scan(source?: { directory?: string; projectId?: string }) {
    if (operation.current || metadataPending) return;
    operation.current = true;
    const ticket = ++generation.current;
    setBusy(true);
    setPicking(!source);
    setError("");
    try {
      if (!source) {
        const chosen = await api<{ directory: string }>("choose-directory", "POST");
        if (!chosen.directory) return;
        source = { directory: chosen.directory };
      }
      if (!mounted.current || ticket !== generation.current) return;
      setPicking(false);
      scanSource.current = source;
      setScanDirectory(source.directory ?? "");
      const previous = source.projectId ? currentDraft.current : drafts.current.get(draftKey(source.directory!));
      replace(previous);
      controller.current = new AbortController();
      const next = await projectAPI<ImportResult>("discover", "POST", source, controller.current.signal);
      if (!mounted.current || ticket !== generation.current) return;
      const saved = commandsRef.current;
      const known = new Set(previous?.result.candidates.map((c) => c.id));
      const existing = next.projectId ?? next.candidates.map((c) => alreadySaved(c, saved)?.project).find(Boolean);
      replace({
        result: next,
        name: previous?.name ?? next.name,
        projectId: lockedProject?.id ?? previous?.projectId ?? existing ?? "new",
        selected: new Set(next.candidates.filter((c) => !alreadySaved(c, saved)
          && (known.has(c.id) ? previous!.selected.has(c.id) : !c.review)).map((c) => c.id)),
        buttons: previous?.buttons ?? null,
        logoChoice: previous?.logoChoice ?? null,
      });
    } catch (cause) {
      if (mounted.current && ticket === generation.current) setError(cause instanceof Error ? cause.message : "scanFailed");
    } finally {
      if (mounted.current && ticket === generation.current) {
        operation.current = false;
        setBusy(false);
        setPicking(false);
      }
    }
  }
  useEffect(() => {
    mounted.current = true;
    if (lockedProject) void scan({ projectId: lockedProject.id });
    else if (initialDirectory) void scan({ directory: initialDirectory });
    return () => {
      mounted.current = false;
      generation.current++;
      operation.current = false;
      controller.current?.abort();
    };
  }, [lockedProject?.id, initialDirectory]);

  function cancelScan() {
    generation.current++;
    controller.current?.abort();
    operation.current = false;
    setBusy(false);
    setPicking(false);
  }
  const result = draft?.result;
  useEffect(() => { setCandidateQuery(""); }, [result?.directory]);
  const projectId = lockedProject?.id ?? draft?.projectId ?? "new";
  const iconProjectId = metadataPending?.projectId ?? projectId;
  useEffect(() => {
    setIconDetails(undefined);
    setIconError("");
    if (!result || iconProjectId === "new") { setIconLoading(false); return; }
    const controller = new AbortController();
    setIconLoading(true);
    void projectAPI<Record<string, ProjectDetails>>("project-details", "POST", {
      ids: [iconProjectId], refreshId: iconRefresh ? iconProjectId : undefined,
    }, controller.signal).then((details) => {
      if (!controller.signal.aborted) setIconDetails({ projectId: iconProjectId, details: details[iconProjectId] });
    }).catch((cause) => {
      if (!controller.signal.aborted) setIconError(cause instanceof Error ? cause.message : "logoScanFailed");
    }).finally(() => { if (!controller.signal.aborted) setIconLoading(false); });
    return () => controller.abort();
  }, [iconProjectId, result, iconRefresh]);
  const savedIcons = iconDetails?.projectId === iconProjectId ? iconDetails.details : undefined;
  const logos = [...new Map([...(savedIcons?.logos ?? []), ...(result?.logos ?? [])].map((logo) => [logo.id, logo])).values()];
  const name = projectId === "new" ? draft?.name ?? "" : projects.find((project) => project.id === iconProjectId)?.name ?? draft?.name ?? "";
  const namedLogos = logos.filter((logo) => logo.name.trim().toLocaleLowerCase() === name.trim().toLocaleLowerCase());
  const automaticLogo = logos.length === 1 ? logos[0] : namedLogos.length === 1 ? namedLogos[0] : null;
  const logoChoice = draft?.logoChoice;
  const selectedLogoMissing = !!logoChoice?.logoId && !logos.some((logo) => logo.id === logoChoice.logoId);
  const iconPreview = {
    ...savedIcons, logos,
    logo: logoChoice ? (logoChoice.logoId ? logos.find((logo) => logo.id === logoChoice.logoId) ?? null : automaticLogo) : savedIcons?.logo ?? automaticLogo,
    selectedLogo: logoChoice?.logoId ?? savedIcons?.selectedLogo,
    customAvatar: !logoChoice && savedIcons?.customAvatar,
    scannedAt: savedIcons?.scannedAt ?? result?.directory,
  };
  const available = result?.candidates.filter((c) => !alreadySaved(c, commands)) ?? [];
  const chosen = available.filter((c) => draft?.selected.has(c.id));
  const eligible = available.filter((c) => (c.platform !== "Windows" || platform === "windows") && (c.platform !== "macOS" || platform !== "windows"));
  const hasCandidateQuery = searchTerms(candidateQuery).length > 0;
  const visibleCandidates = useMemo(() => {
    if (!result?.candidates) return [];
    return rankByScore(result.candidates, candidateQuery, (c) =>
      scoreMultiTerm(candidateQuery, [
        { value: c.title, weight: 4.0, subsequence: true },
        { value: c.text, weight: 2.0, subsequence: true },
        { value: c.source, weight: 1.4, subsequence: false },
        { value: c.directory, weight: 1.0, subsequence: false },
        { value: c.platform, weight: 0.8, subsequence: false },
      ]),
    );
  }, [result?.candidates, candidateQuery]);
  const recommendations = recommendLaunchButtons(chosen, platform, result?.directory);
  const buttons = launchSlots(draft?.buttons ?? recommendations).map((button) => ({
    ...button, commandId: eligible.some((c) => c.id === button.commandId) && draft?.selected.has(button.commandId) ? button.commandId : "",
  }));
  const disabled = busy || saving || !!metadataPending;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {error && !draft && <p role="alert" className="shrink-0 px-5 pt-3 text-sm text-destructive">{m[error as keyof Messages] ?? m.scanFailed}</p>}
      <div ref={scrollRef} onScroll={onScroll} className="min-h-0 flex-1 space-y-5 overflow-y-auto overscroll-contain px-5 py-5">
        {!metadataPending && recovery}
        <div className="flex flex-wrap items-center gap-2">
          {(!lockedProject || (!draft && !busy)) && (
            <Button variant="outline" size="sm" onClick={() => void scan()} disabled={disabled}>
              <FolderOpen className="size-4" aria-hidden="true" />{draft || initialDirectory || scanDirectory ? m.changeImportDirectory : m.chooseDirectory}
            </Button>
          )}
          {busy && <span role="status" className="flex items-center gap-2 text-sm">
            <LoaderCircle className="size-4 motion-safe:animate-spin" aria-hidden="true" />
            {picking ? m.chooseDirectory : m.scanning}
          </span>}
          {busy && !picking && <Button variant="ghost" size="sm" onClick={cancelScan}>{m.cancel}</Button>}
          {!busy && scanSource.current && !metadataPending && (
            <Button variant="ghost" size="sm" disabled={saving} onClick={() => void scan(scanSource.current)}>
              {error ? m.retry : m.rescanProject}
            </Button>
          )}
          {lockedProject && onManual && <Button variant="ghost" size="sm" disabled={disabled} onClick={onManual}>{m.manualCommand}</Button>}
        </div>
        {!result && scanDirectory && <p className="break-all font-mono text-xs text-muted-foreground">{scanDirectory}</p>}
        {draft && result && (
          <>
            <p className="break-all font-mono text-xs text-muted-foreground">{result.directory}</p>
            <FieldRow label={m.project}>
              {lockedProject || metadataPending ? (
                <span className="text-sm">{lockedProject?.name ?? projects.find((p) => p.id === metadataPending?.projectId)?.name ?? draft.name}</span>
              ) : (
                <Picker label={m.project} value={projectId} disabled={disabled}
                  onChange={(id) => change({ projectId: id, logoChoice: null })}
                  options={[["new", m.newProject], ...projects.map((p): [string, string] => [p.id, p.name])]} />
              )}
            </FieldRow>
            {projectId === "new" && !metadataPending && (
              <ProjectNameField label={m.chooseProject} names={result.names} value={draft.name} disabled={disabled} onChange={(name) => change({ name })} />
            )}
            <ProjectIconPicker name={name} details={iconPreview} m={m}
              disabled={busy || saving || iconLoading || (iconProjectId !== "new" && !savedIcons)}
              onChange={(logoId) => change({ logoChoice: { logoId, revision: savedIcons?.revision ?? 0 } })}>
              {iconLoading && <LoaderCircle className="size-4 shrink-0 motion-safe:animate-spin" aria-label={m.scanning} />}
            </ProjectIconPicker>
            {iconError && <div role="alert" className="flex items-center gap-2 text-sm text-destructive">
              <span>{m[iconError as keyof Messages] ?? m.logoScanFailed}</span>
              <Button variant="outline" size="sm" disabled={iconLoading || saving} onClick={() => setIconRefresh((value) => value + 1)}>{m.retry}</Button>
            </div>}
            <ScanIssues issues={[...result.issues, ...(savedIcons?.issues ?? [])]} m={m} />
            {result.candidates.length > 0 && <Disclosure key={result.directory}
              defaultOpen={!chosen.length || result.candidates.some((candidate) => candidate.review)}
              label={`${m.projectCommands} · ${new Intl.NumberFormat().format(result.candidates.length)}`}>
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                {result.candidates.length > 3 && (
                  <div className="relative min-w-44 flex-1">
                    <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                    <Input
                      type="search"
                      value={candidateQuery}
                      onChange={(event) => setCandidateQuery(event.target.value)}
                      onKeyDown={(event) => {
                        if (!event.nativeEvent.isComposing && event.key === "Escape" && candidateQuery) {
                          event.preventDefault();
                          event.stopPropagation();
                          setCandidateQuery("");
                        }
                      }}
                      aria-label={m.searchCommands}
                      placeholder={m.searchCommands}
                      autoComplete="off"
                      spellCheck={false}
                      disabled={disabled}
                      className="pl-9 pr-8 [&::-webkit-search-cancel-button]:appearance-none"
                    />
                    {candidateQuery && (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon-sm"
                        aria-label={m.clear}
                        title={m.clear}
                        onClick={() => setCandidateQuery("")}
                        className="absolute right-1 top-1/2 -translate-y-1/2"
                      >
                        <X className="size-4" aria-hidden="true" />
                      </Button>
                    )}
                  </div>
                )}
                <label className="ml-auto flex items-center gap-3 text-sm">
                  <Checkbox className="border-muted-foreground" aria-label={m.selectAll} disabled={!available.length || disabled}
                    checked={chosen.length === 0 ? false : chosen.length === available.length ? true : "indeterminate"}
                    onCheckedChange={(checked) => change({ selected: new Set(checked === true ? available.map((c) => c.id) : []) })} />
                  {m.selectAll}
                </label>
              </div>
              {visibleCandidates.length > 0 ? (
                <ExpandableList items={visibleCandidates} m={m} limit={12} className="divide-y divide-border"
                  renderItem={(candidate) => {
                    const exists = !!alreadySaved(candidate, commands);
                    return (
                      <li key={candidate.id} className="py-3">
                        <label className="flex cursor-pointer items-start gap-3">
                          <Checkbox className="mt-1 border-muted-foreground" disabled={exists || disabled}
                            checked={!exists && draft.selected.has(candidate.id)}
                            onCheckedChange={(checked) => {
                              const selected = new Set(draft.selected);
                              if (checked === true) selected.add(candidate.id); else selected.delete(candidate.id);
                              change({ selected });
                            }} />
                          <span className="min-w-0 flex-1 space-y-1">
                            <span className="block break-words text-base"><HighlightText text={candidate.title} query={candidateQuery} /></span>
                            <span className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
                              <span>{candidate.origin === "configuration" ? m.configuredCommand : m.commandTemplate}</span>
                              {candidate.platform !== "macOS / Windows" && <span>{candidate.platform}</span>}
                              {candidate.review && <span className="text-warning">{m.reviewRequired}</span>}
                              {exists && <span>{m.alreadyImported}</span>}
                            </span>
                          </span>
                        </label>
                        <div className="ml-7 mt-1 space-y-1 text-foreground-secondary">
                          <CommandText text={candidate.text} m={m} />
                          <Disclosure label={m.source}>
                            <p className="break-all text-xs text-muted-foreground"><HighlightText text={candidate.source} query={candidateQuery} /></p>
                            {candidate.directory !== result.directory && <p className="break-all text-xs text-muted-foreground">{m.directory} · <HighlightText text={candidate.directory} query={candidateQuery} /></p>}
                          </Disclosure>
                        </div>
                      </li>
                    );
                  }} />
              ) : (
                <div className="space-y-2 py-4">
                  <p role="status" className="text-sm text-muted-foreground">{m.emptyResults}</p>
                  {hasCandidateQuery && <Button variant="outline" size="sm" onClick={() => setCandidateQuery("")}>{m.clear}</Button>}
                </div>
              )}
            </Disclosure>}
            {!result.candidates.length && <p role="status" className="text-sm text-muted-foreground">{m.noProjectCommands}</p>}
            {projectId === "new" && eligible.length > 0 && !metadataPending && (
              <ImportLaunchButtons value={buttons} commands={eligible} disabled={disabled} m={m}
                onChange={(buttons) => change({ buttons, selected: new Set([...draft.selected, ...buttons.map((button) => button.commandId).filter(Boolean)]) })}
                onRecommend={() => {
                  const recommended = recommendLaunchButtons(eligible, platform, result.directory);
                  change({ buttons: recommended, selected: new Set([...draft.selected, ...recommended.map((button) => button.commandId)]) });
                }} />
            )}
          </>
        )}
      </div>

      {draft && result && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-t border-border bg-surface px-5 py-3">
          {(error || selectedLogoMissing) && <p role="alert" className="basis-full text-sm text-destructive">
            {error ? m[error as keyof Messages] ?? m.storageFailed : m.logoSelectionMissing}
          </p>}
          <Button size="sm" disabled={(!!lockedProject && !chosen.length && !metadataPending && !logoChoice) || busy || saving || selectedLogoMissing || (!!logoChoice && iconLoading) || (projectId === "new" && !draft.name.trim())}
            onClick={async () => {
              if (operation.current) return;
              operation.current = true;
              setSaving(true);
              setError("");
              try {
                if (metadataPending) await onRetryMetadata(metadataPending.projectId, metadataPending.directory, draft.logoChoice);
                else await onSave(result, draft.selected, draft.name.trim(), projectId, buttons.filter((button) => button.commandId), draft.logoChoice);
              } catch (cause) {
                if (mounted.current) {
                  if (cause instanceof ImportMetadataError) {
                    setMetadataPending({ projectId: cause.projectId, directory: result.directory });
                    if (cause.message === "logoConflict") {
                      change({ logoChoice: null });
                      setIconRefresh((value) => value + 1);
                    }
                    setDraftChanged(false);
                  }
                  setError(cause instanceof Error ? cause.message : "storageFailed");
                }
              } finally {
                operation.current = false;
                if (mounted.current) setSaving(false);
              }
            }}>
            {saving ? m.saving : metadataPending ? m.retry : lockedProject ? m.importSelected : m.openProject}
          </Button>
          <Button variant="outline" size="sm" disabled={saving} onClick={onCancel}>{metadataPending ? m.back : m.cancel}</Button>
          <span className="ml-auto text-sm text-muted-foreground" role="status">{m.selectedCommands} · {new Intl.NumberFormat().format(chosen.length)}</span>
        </div>
      )}
    </div>
  );
}
