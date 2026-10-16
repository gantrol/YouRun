import { useEffect, useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { FieldRow, Picker } from "./controls";
import { projectAPI } from "./project-api";
import { alreadySaved, type ImportCandidate, type ImportResult } from "./project-import-data";
import type { Command } from "./model";
import type { Messages } from "./messages";
import { ScanIssues } from "./scan-issues";

export function ProjectCommandPicker({
  projectId, commands, value, onChange, m,
}: {
  projectId: string;
  commands: Command[];
  value?: string;
  onChange: (candidate?: ImportCandidate) => void;
  m: Messages;
}) {
  const [result, setResult] = useState<ImportResult>();
  const [busy, setBusy] = useState(true);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  useEffect(() => {
    const controller = new AbortController();
    setBusy(true);
    setError("");
    void projectAPI<ImportResult>("discover", "POST", { projectId }, controller.signal)
      .then((next) => {
        if (!controller.signal.aborted) setResult(next);
      })
      .catch((cause) => {
        if (!controller.signal.aborted)
          setError(cause instanceof Error ? cause.message : "scanFailed");
      })
      .finally(() => {
        if (!controller.signal.aborted) setBusy(false);
      });
    return () => controller.abort();
  }, [projectId, refresh]);
  const candidates = result?.candidates.filter((item) => !alreadySaved(item, commands)) ?? [];
  return (
    <FieldRow label={m.projectCommands}>
      <div className="flex items-center gap-2">
        <div className="min-w-0 flex-1">
          <Picker
            label={m.projectCommands}
            value={candidates.some((item) => item.id === value) ? value! : "manual"}
            disabled={busy}
            onChange={(id) => onChange(candidates.find((item) => item.id === id))}
            options={[
              ["manual", busy ? m.scanning : m.manualCommand],
              ...candidates.map((item): [string, string] => [
                item.id, `${item.title} · ${item.text}${item.review ? ` · ${m.reviewRequired}` : ""}`,
              ]),
            ]}
          />
        </div>
        <Button type="button" size="sm" variant="ghost" disabled={busy} onClick={() => setRefresh((n) => n + 1)}>
          {error ? m.retry : m.rescanProject}
        </Button>
      </div>
      {error && <span role="alert" className="text-xs text-destructive">{m[error as keyof Messages] ?? m.scanFailed}</span>}
      <ScanIssues issues={result?.issues} m={m} />
      {!busy && !error && result && !candidates.length && (
        <span role="status" className="text-xs text-muted-foreground">
          {result.candidates.length ? m.alreadyImported : m.noProjectCommands}
        </span>
      )}
    </FieldRow>
  );
}
