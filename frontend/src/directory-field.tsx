import { useState, type Ref } from "react";
import { FolderOpen } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { TextField } from "./controls";
import { api } from "./api";
import type { Messages } from "./messages";

export function DirectoryField({
  value,
  onChange,
  m,
  inputRef,
  error,
  required,
}: {
  value: string;
  onChange: (value: string) => void;
  m: Messages;
  inputRef?: Ref<HTMLInputElement>;
  error?: string;
  required?: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [failed, setFailed] = useState(false);
  async function choose() {
    setBusy(true);
    setFailed(false);
    try {
      const result = await api<{ directory: string }>(
        "choose-directory",
        "POST",
      );
      if (result.directory) onChange(result.directory);
    } catch {
      setFailed(true);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="flex items-start gap-2">
      <div className="min-w-0 flex-1">
        <TextField
          label={m.directory}
          value={value}
          onChange={(event) => onChange(event.target.value)}
          inputRef={inputRef}
          error={error || (failed ? m.directoryPickerFailed : undefined)}
          required={required}
        />
      </div>
      <Button
        type="button"
        variant="outline"
        size="icon"
        className="mt-7"
        title={m.chooseDirectory}
        aria-label={m.chooseDirectory}
        disabled={busy}
        onClick={choose}
      >
        <FolderOpen className="size-4" aria-hidden="true" />
      </Button>
    </div>
  );
}
