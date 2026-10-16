import { Button } from "@/renderer/components/ui/button";
import type { Run } from "./model";
import type { Messages } from "./messages";

export function CommandFailure({ error, run, name, m, onEdit, onOutput }: {
  error?: string;
  run?: Run;
  name?: string;
  m: Messages;
  onEdit?: () => void;
  onOutput?: () => void;
}) {
  error ??= run?.error;
  if (!error && run?.status !== "failed") return null;
  return (
    <div className="flex w-full flex-wrap items-center gap-2 py-2 text-xs">
      <span role="alert" className="text-destructive">
        {name && `${name} · `}{error ? m[error as keyof Messages] ?? m.startFailed : m.failed}
        {!error && run?.exitCode !== undefined && ` · ${run.exitCode}`}
      </span>
      {run && (run.status === "failed" || run.status === "unknown" || error === "alreadyRunning") && onOutput && <Button variant="outline" size="xs" onClick={onOutput}>{m.viewRun}</Button>}
      {onEdit && error !== "alreadyRunning" && run?.status !== "unknown" && <Button variant="ghost" size="xs" onClick={onEdit}>{m.editCommand}</Button>}
    </div>
  );
}
