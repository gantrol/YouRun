import { Button } from "@/renderer/components/ui/button";
import type { Messages } from "./messages";

export function ImportRecovery({ directory, name, busy, m, onRetry }: {
  directory: string;
  name?: string;
  busy: boolean;
  m: Messages;
  onRetry: () => void;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 px-2 py-2 text-xs">
      <span role="status" className={busy ? "text-muted-foreground" : "text-destructive"} title={directory}>
        {name && `${name} · `}{busy ? m.saving : m.importProjectMetadataFailed}
      </span>
      <Button variant="outline" size="xs" disabled={busy} onClick={onRetry}>
        {busy ? m.saving : m.retry}
      </Button>
    </div>
  );
}
