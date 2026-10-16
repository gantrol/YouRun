import { useId, useState } from "react";
import { Check, Play, Settings2, Trash2, X } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { Textarea } from "@/renderer/components/ui/textarea";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator } from "@/renderer/components/ui/dropdown-menu";
import { Dialog, DialogContent, DialogTitle } from "@/renderer/components/ui/dialog";
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuSeparator, ContextMenuTrigger } from "@/renderer/components/ui/context-menu";
import { ModalOverlayScope } from "@/renderer/components/ui/overlay-layer";
import { CommandAction } from "../command-action";
import { IconPicker } from "../command-icons";
import { FieldRow, Picker, TextField } from "../controls";
import { ProjectNameField } from "../project-name-field";
import { RunStatus, type RunState } from "../run-state";
import { RunHistory } from "../run-history";
import { CommandFailure } from "../command-failure";
import { ImportRecovery } from "../import-recovery";
import { ProjectMetadataError } from "../project-metadata-controls";
import { ScanIssues } from "../scan-issues";
import { ImportLaunchButtons, launchSlots } from "../import-launch-buttons";
import { LaunchRecommendationPreview } from "../launch-recommendation-preview";
import type { ImportCandidate } from "../project-import-data";
import type { Messages } from "../messages";
import type { IconId } from "../model";
import type { CatalogMessages } from "./messages";
import { sampleCommands, sampleRun } from "./samples";
import { RabbitActivityPreview } from "./rabbit-activity-preview";

export type PreviewProps = { m: Messages; c: CatalogMessages; only: string; locale: string; onLocale: (locale: string) => void };
const states: RunState[] = ["starting", "running", "restarting", "stopping", "success", "failed", "stopped", "handedOff", "reconnecting", "unknown"];

const importCommands: ImportCandidate[] = sampleCommands.slice(1, 4).map((command) => ({
  id: command.id, title: command.title ?? command.id, text: command.text, directory: "/workspace/yourun",
  source: "package.json", shell: "default", platform: "macOS / Windows", tool: "npm", origin: "configuration", review: false, icon: "play",
}));

function ImportButtonsPreview({ m }: { m: Messages }) {
  const recommended = launchSlots([{ kind: "debug", icon: "bolt", commandId: importCommands[0].id }, { kind: "run", icon: "play", commandId: importCommands[2].id }]);
  const [buttons, setButtons] = useState(recommended);
  return <ImportLaunchButtons m={m} value={buttons} commands={importCommands} disabled={false}
    onChange={setButtons} onRecommend={() => setButtons(recommended)} />;
}

export function ButtonsPreview({ m, c, only }: PreviewProps) {
  const [icon, setIcon] = useState<IconId>("play");
  const [result, setResult] = useState("");
  const report = () => setResult(m.start);
  return (
    <div className="space-y-6">
      {only === "CommandAction" && <FieldRow label="CommandAction">
        <div className="flex flex-wrap gap-x-8 gap-y-5">
          {(["ready", "starting", "running", "restarting", "stopping", "notAssigned"] as const).map((state) => (
            <div key={state} className="grid gap-3">
              <span className="text-xs text-muted-foreground">{m[state]}</span>
              <CommandAction m={m} name="YouRun · Debug" icon="bolt" operation={state === "starting" || state === "restarting" || state === "stopping" ? state : undefined}
                running={state === "running" || state === "restarting" || state === "stopping"} stopping={state === "stopping"}
                unassigned={state === "notAssigned"} onClick={report}
                onOpen={() => setResult(m.viewCommand)}
                onOutput={state === "running" ? () => setResult(m.viewRun) : undefined}
                onStop={() => setResult(m.stop)} onReplay={() => setResult(m.restart)} />
            </div>
          ))}
        </div>
      </FieldRow>}
      {only === "Button" && <FieldRow label={c.variants}>
        <div className="flex flex-wrap gap-3">
          {(["default", "secondary", "outline", "ghost", "destructive", "link"] as const).map((variant) => (
            <div key={variant} className="grid justify-items-start gap-2">
              <code className="text-xs text-muted-foreground">{variant}</code>
              <Button variant={variant} onClick={() => setResult(variant)}>{m.save}</Button>
              <Button variant={variant} disabled>{c.disabled}</Button>
            </div>
          ))}
        </div>
      </FieldRow>}
      {["Button", "Tooltip"].includes(only) && <FieldRow label={c.sizes}>
        <div className="flex flex-wrap items-center gap-3">
          {only === "Button" && (["2xs", "xs", "sm", "default", "lg"] as const).map((size) => (
            <Button key={size} variant="outline" size={size} onClick={() => setResult(size)}>{size}</Button>
          ))}
          <Button size="icon" aria-label={m.start} title={m.start} onClick={report}><Play className="size-4" aria-hidden="true" /></Button>
          <Button size="icon-sm" disabled aria-label={m.start} title={`${m.start} · ${c.disabled}`}><Play className="size-4" aria-hidden="true" /></Button>
        </div>
      </FieldRow>}
      {only === "IconPicker" && <IconPicker m={m} value={icon} onChange={setIcon} />}
      {result && <output className="block text-xs text-muted-foreground">{c.preview} · {result}</output>}
    </div>
  );
}

export function FieldsPreview({ m, c, only }: PreviewProps) {
  const [name, setName] = useState("YouRun");
  const [required, setRequired] = useState("");
  const [shell, setShell] = useState("default");
  const textareaId = useId();
  return (
    <div className="space-y-5">
      {["Input", "TextField"].includes(only) && <>
      <TextField label={`TextField · ${c.default}`} defaultValue="YouRun" />
      <TextField label={`TextField · ${c.error}`} required value={required} onChange={(event) => setRequired(event.target.value)} error={!required.trim() ? m.required : undefined} />
      <TextField label={`Input · ${c.disabled}`} disabled defaultValue="YouRun" />
      </>}
      {only === "ProjectNameField" && <ProjectNameField label="ProjectNameField" value={name} onChange={setName} names={[{ name: "YouRun", source: "package.json" }, { name: "command-manager", source: "go.mod" }]} />}
      {["Select", "Picker", "FieldRow"].includes(only) && <>
      <FieldRow label="Picker / Select"><Picker label={m.shell} value={shell} onChange={setShell} options={[["default", m.defaultShell], ["pwsh", "PowerShell"], ["bash", "Bash"]]} /></FieldRow>
      <FieldRow label={`Select · ${c.disabled}`}><Picker label={m.shell} value="default" onChange={setShell} disabled options={[["default", m.defaultShell]]} /></FieldRow>
      </>}
      {only === "Textarea" && <div className="grid gap-2 text-sm">
        <label htmlFor={textareaId} className="text-foreground-secondary">Textarea</label>
        <Textarea id={textareaId} defaultValue="npm run build" className="font-mono" />
      </div>}
      {only === "Checkbox" && <FieldRow label="Checkbox">
        <div className="flex flex-wrap gap-5">
          <label className="flex items-center gap-2"><Checkbox />{c.default}</label>
          <label className="flex items-center gap-2"><Checkbox defaultChecked />{c.selected}</label>
          <label className="flex items-center gap-2"><Checkbox defaultChecked="indeterminate" />{m.selectAll}</label>
          <label className="flex items-center gap-2 text-disabled-foreground"><Checkbox disabled defaultChecked />{c.disabled}</label>
        </div>
      </FieldRow>}
      {only === "ImportLaunchButtons" && <ImportButtonsPreview m={m} />}
      {only === "Disclosure" && <Disclosure label={m.commandDetails} summary={<code className="text-xs">npm run build</code>}><TextField label={m.name} defaultValue="Build" /></Disclosure>}
      {only === "LaunchRecommendationPreview" && <LaunchRecommendationPreview m={m} commands={importCommands} recommendations={[{ kind: "debug", icon: "bolt", commandId: importCommands[0].id }]} />}
    </div>
  );
}

export function FeedbackPreview({ m, c, locale, only }: PreviewProps) {
  const [result, setResult] = useState("");
  return (
    <div className="space-y-6">
      {only === "RunStatus" && <>
        <RabbitActivityPreview m={m} c={c} artwork={false}/>
        <div className="flex flex-wrap gap-5 border-t border-border pt-5">{states.map((state) => <RunStatus key={state} state={state} m={m} />)}</div>
      </>}
      <div className="space-y-5">
        {only === "RunHistory" && <FieldRow label="RunHistory">
          <RunHistory locale={locale} m={m} runs={[sampleRun(sampleCommands[0]), sampleRun(sampleCommands[5], "success"), sampleRun(sampleCommands[6], "failed")]}
            onOpen={(id) => setResult(`${m.viewRun} · ${id}`)} />
          <RunHistory locale={locale} m={m} runs={[]} onOpen={setResult} />
        </FieldRow>}
        <div className="space-y-4">
          {only === "CommandFailure" && <CommandFailure m={m} error="startFailed" onEdit={() => setResult(m.editCommand)} />}
          {only === "ImportRecovery" && <ImportRecovery m={m} directory="/workspace/yourun" busy={false} onRetry={() => setResult(m.retry)} />}
          {only === "ProjectMetadataError" && <ProjectMetadataError m={m} error="logoScanFailed" busy={false} onRetry={() => setResult(m.retry)} />}
          {only === "ScanIssues" && <ScanIssues m={m} issues={[
            { code: "scanInvalidManifest", path: "/workspace/example/package.json" },
            { code: "scanInvalidManifest", path: "/workspace/example/app/package.json" },
            { code: "scanReadFailed", path: "/workspace/example/go.mod" },
          ]} />}
        </div>
      </div>
      {result && <output className="block text-xs text-muted-foreground">{c.preview} · {result}</output>}
    </div>
  );
}

export function OverlaysPreview({ m, c, only }: PreviewProps) {
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("default");
  const [result, setResult] = useState("");
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-4">
        {["Dialog", "ModalOverlayScope"].includes(only) && <Button variant="outline" onClick={() => setOpen(true)}>{c.dialog}</Button>}
        {only === "DropdownMenu" && <DropdownMenu>
          <DropdownMenuTrigger asChild><Button variant="outline">{m.settings}</Button></DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuItem onSelect={() => setResult(m.edit)}>{m.edit}</DropdownMenuItem>
            <DropdownMenuItem disabled>{m.unavailable}</DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => setResult(m.remove)}>{m.remove}</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>}
        {only === "ContextMenu" && <ContextMenu>
          <ContextMenuTrigger asChild><Button variant="secondary">{c.contextMenu}</Button></ContextMenuTrigger>
          <ContextMenuContent>
            <ContextMenuItem onSelect={() => setResult(m.start)}><Play aria-hidden="true" />{m.start}</ContextMenuItem>
            <ContextMenuItem onSelect={() => setResult(m.settings)}><Settings2 aria-hidden="true" />{m.settings}</ContextMenuItem>
            <ContextMenuItem disabled>{m.unavailable}</ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem destructive onSelect={() => setResult(m.remove)}><Trash2 aria-hidden="true" />{m.remove}</ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>}
      </div>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent aria-describedby={undefined}>
          <ModalOverlayScope>
            <div className="flex items-center justify-between gap-4">
              <DialogTitle className="text-md font-semibold">{c.dialog}</DialogTitle>
              <Button variant="ghost" size="icon-sm" title={m.close} aria-label={m.close} onClick={() => setOpen(false)}><X className="size-4" aria-hidden="true" /></Button>
            </div>
            <TextField label={m.name} defaultValue="YouRun" />
            <FieldRow label={m.shell}><Picker label={m.shell} value={value} onChange={setValue} options={[["default", m.defaultShell], ["pwsh", "PowerShell"], ["bash", "Bash"]]} /></FieldRow>
            <div className="flex justify-end gap-3">
              <Button variant="ghost" onClick={() => setOpen(false)}>{m.cancel}</Button>
              <Button onClick={() => { setResult(m.saved); setOpen(false); }}><Check className="size-4" aria-hidden="true" />{m.save}</Button>
            </div>
          </ModalOverlayScope>
        </DialogContent>
      </Dialog>
      {result && <output className="block text-xs text-muted-foreground">{c.preview} · {result}</output>}
    </div>
  );
}
