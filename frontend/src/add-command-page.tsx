import { useEffect, useRef, useState, type ComponentProps } from "react";
import { Button } from "@/renderer/components/ui/button";
import { ProjectImport } from "./project-import";
import { CommandEditor } from "./panels";
import { UnsavedChangesDialog } from "./unsaved-changes-dialog";

export function AddCommandPage({ scan, manual, onDirty, onBusy }: {
  scan: ComponentProps<typeof ProjectImport>;
  manual: ComponentProps<typeof CommandEditor>;
  onDirty: (dirty: boolean) => void;
  onBusy?: (busy: boolean) => void;
}) {
  const [mode, setMode] = useState(scan.lockedProject ? "scan" : "manual");
  const [scanDirty, setScanDirty] = useState(false), [manualDirty, setManualDirty] = useState(false);
  const [scanBusy, setScanBusy] = useState(false), [manualBusy, setManualBusy] = useState(false);
  const [partial, setPartial] = useState(false);
  const [visitedScan, setVisitedScan] = useState(!!scan.lockedProject);
  const [discardMode, setDiscardMode] = useState<"scan" | "manual">();
  const confirm = useRef<((discard: boolean) => void) | undefined>(undefined);
  const trigger = useRef<HTMLElement | null>(null);
  useEffect(() => { onDirty(scanDirty || manualDirty); }, [scanDirty, manualDirty, onDirty]);
  useEffect(() => { onBusy?.(scanBusy || manualBusy); }, [scanBusy, manualBusy, onBusy]);
  useEffect(() => () => { confirm.current?.(false); }, []);
  async function saveWithOtherDraft(other: "scan" | "manual", save: () => Promise<void>) {
    if (other === "scan" ? scanDirty : manualDirty) {
      trigger.current = document.activeElement as HTMLElement | null;
      const discard = await new Promise<boolean>(resolve => { confirm.current = resolve; setDiscardMode(other); });
      if (!discard) return;
    }
    await save();
  }
  function resolveDiscard(discard: boolean) {
    setDiscardMode(undefined);
    confirm.current?.(discard);
    confirm.current = undefined;
  }
  return <section className="flex min-h-0 flex-1 flex-col">
    <UnsavedChangesDialog open={!!discardMode} m={{ ...scan.m, unsaved: `${discardMode === "scan" ? scan.m.fromProject : scan.m.manualCommand} · ${scan.m.unsaved}` }}
      onContinue={() => resolveDiscard(false)} onDiscard={() => resolveDiscard(true)}
      onRestoreFocus={() => trigger.current?.focus({ preventScroll: true })}/>
    {scan.lockedProject && <div className="flex shrink-0 gap-1 border-b border-border px-5 py-3" role="group" aria-label={scan.m.newCommand}>
      <Button size="sm" variant={mode === "scan" ? "secondary" : "ghost"} disabled={scanBusy || manualBusy || partial} aria-pressed={mode === "scan"}
        onClick={() => { setMode("scan"); setVisitedScan(true); }}>{scan.m.fromProject}</Button>
      <Button size="sm" variant={mode === "manual" ? "secondary" : "ghost"} disabled={scanBusy || manualBusy || partial} aria-pressed={mode === "manual"}
        onClick={() => setMode("manual")}>{scan.m.manualCommand}</Button>
    </div>}
    {visitedScan && <div className={mode === "scan" ? "flex min-h-0 flex-1 flex-col" : "hidden"} inert={mode !== "scan"}>
      <ProjectImport {...scan} onDirty={setScanDirty} onBusy={setScanBusy} onPartial={setPartial}
        onSave={(...args) => saveWithOtherDraft("manual", () => scan.onSave(...args))}/>
    </div>}
    <div className={mode === "manual" ? "flex min-h-0 flex-1 flex-col" : "hidden"} inert={mode !== "manual"}>
      <CommandEditor {...manual} autoFocus={!scan.lockedProject} onDirty={setManualDirty} onBusy={setManualBusy}
        onSave={(...args) => saveWithOtherDraft("scan", () => manual.onSave(...args))}/>
    </div>
  </section>;
}
