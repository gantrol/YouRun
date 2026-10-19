import { useCallback, useEffect, useRef, useState } from "react";
import { AppWindow, FolderOpen, GitBranch, LoaderCircle, RefreshCw, Terminal } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { ContextMenuItem, ContextMenuSeparator, ContextMenuSub, ContextMenuSubContent, ContextMenuSubTrigger } from "@/renderer/components/ui/context-menu";
import { Dialog, DialogContent, DialogTitle } from "@/renderer/components/ui/dialog";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/renderer/components/ui/dropdown-menu";
import { api } from "./api";
import { TextField } from "./controls";
import type { Messages } from "./messages";

type ProjectOpener = { id: string; name: string; icon?: string };

export function useProjectOpeners(platform: string) {
  const [items, setItems] = useState<ProjectOpener[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const request = useRef<AbortController | null>(null);
  const loadedAt = useRef(0);
  const refresh = useCallback((force = true) => {
    if (request.current || (!force && Date.now() - loadedAt.current < 60_000)) return;
    const controller = new AbortController();
    request.current = controller;
    setLoading(true);
    setError("");
    void api<ProjectOpener[]>(`project-openers${force ? "?refresh=1" : ""}`, "GET", undefined, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setItems(result);
        loadedAt.current = Date.now();
      })
      .catch((cause) => {
        if (!controller.signal.aborted) setError(cause instanceof Error ? cause.message : "editorDetectionFailed");
      })
      .finally(() => {
        if (request.current === controller) {
          request.current = null;
          setLoading(false);
        }
      });
  }, []);
  useEffect(() => {
    return () => { request.current?.abort(); request.current = null; };
  }, [refresh]);
  return { items, loading, error, refresh, platform };
}

export type ProjectOpeners = ReturnType<typeof useProjectOpeners>;

type ProjectOpenersProps = {
  openers: ProjectOpeners; m: Messages; disabled: boolean;
  onOpen: (id: string) => void; onCustom: () => void; onOpenDirectory: () => void;
};

function ProjectOpenerIcon({ opener }: { opener: ProjectOpener }) {
  const [failed, setFailed] = useState<string>();
  if (opener.icon && opener.icon !== failed) return <img src={opener.icon} alt="" draggable={false}
    className="size-4 shrink-0 object-contain" onError={() => setFailed(opener.icon)} />;
  const Icon = opener.id === "git-bash" ? GitBranch : ["powershell", "codex-cli", "terminal", "iterm", "warp", "ghostty"].includes(opener.id) ? Terminal : AppWindow;
  return <Icon aria-hidden="true" />;
}

function ProjectOpenerItems({ openers, m, onOpen, onCustom, onOpenDirectory, dropdown = false }: Omit<ProjectOpenersProps, "disabled"> & { dropdown?: boolean }) {
  const Item = dropdown ? DropdownMenuItem : ContextMenuItem;
  const Separator = dropdown ? DropdownMenuSeparator : ContextMenuSeparator;
  const fileManagerName = openers.platform === "darwin" ? m.finder
    : openers.platform === "windows" ? m.fileExplorer : m.fileManager;
  return <>
    <Item onSelect={onOpenDirectory}><FolderOpen aria-hidden="true" />{fileManagerName}</Item>
    <Separator />
    {openers.items.map((opener) => <Item key={opener.id} textValue={opener.name} onSelect={() => onOpen(opener.id)}>
      <ProjectOpenerIcon opener={opener} /><span>{opener.name}</span>
    </Item>)}
    {openers.loading && <Item disabled><LoaderCircle className="motion-safe:animate-spin" aria-hidden="true" />{m.scanning}</Item>}
    {openers.error && <Item disabled className="max-w-72 text-destructive">{m[openers.error as keyof Messages] ?? m.editorDetectionFailed}</Item>}
    {!openers.loading && !openers.error && !openers.items.length && <Item disabled>{m.noProjectEditors}</Item>}
    <Separator />
    <Item onSelect={onCustom}><AppWindow aria-hidden="true" />{m.otherProjectEditor}</Item>
    <Item disabled={openers.loading} onSelect={(event) => { event.preventDefault(); openers.refresh(); }}>
      <RefreshCw aria-hidden="true" />{m.refresh}
    </Item>
  </>;
}

export function ProjectOpenersDropdown(props: ProjectOpenersProps) {
  const customSelected = useRef(false);
  const trigger = useRef<HTMLButtonElement>(null);
  return <DropdownMenu modal={false} onOpenChange={(open) => {
    if (open) { customSelected.current = false; props.openers.refresh(false); }
  }}>
    <DropdownMenuTrigger asChild>
      <Button ref={trigger} type="button" variant="outline" size="icon-sm" aria-label={props.m.openProjectWith} title={props.m.openProjectWith} disabled={props.disabled}>
        <FolderOpen className="size-4" aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent align="end" onCloseAutoFocus={(event) => {
      if (!customSelected.current) return;
      event.preventDefault();
      customSelected.current = false;
      trigger.current?.focus();
      props.onCustom();
    }}>
      <ProjectOpenerItems {...props} dropdown onCustom={() => { customSelected.current = true; }} />
    </DropdownMenuContent>
  </DropdownMenu>;
}

export function ProjectOpenersMenu({ disabled, ...props }: ProjectOpenersProps) {
  const { openers, m } = props;
  return <ContextMenuSub onOpenChange={(open) => { if (open) openers.refresh(false); }}>
    <ContextMenuSubTrigger disabled={disabled}><AppWindow />{m.openProjectWith}</ContextMenuSubTrigger>
    <ContextMenuSubContent>
      <ProjectOpenerItems {...props} />
    </ContextMenuSubContent>
  </ContextMenuSub>;
}

export function CustomProjectOpenerDialog({ m, onClose, onOpen }: {
  m: Messages; onClose: () => void; onOpen: (executable: string) => Promise<void>;
}) {
  const [path, setPath] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef(false);
  return <Dialog open onOpenChange={(open) => { if (!open && !pending.current) onClose(); }}>
    <DialogContent aria-describedby={undefined}>
      <DialogTitle className="text-base font-semibold">{m.otherProjectEditor}</DialogTitle>
      <form className="grid gap-4" onSubmit={async (event) => {
        event.preventDefault();
        if (pending.current || !path.trim()) return;
        pending.current = true;
        setBusy(true);
        setError("");
        try { await onOpen(path.trim()); onClose(); }
        catch (cause) { setError(cause instanceof Error ? cause.message : "editorOpenFailed"); }
        finally { pending.current = false; setBusy(false); }
      }}>
        <TextField label={m.editorExecutable} value={path} disabled={busy} required onChange={(event) => setPath(event.target.value)} error={error ? m[error as keyof Messages] ?? m.editorOpenFailed : undefined} />
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" disabled={busy} onClick={onClose}>{m.cancel}</Button>
          <Button type="submit" disabled={busy || !path.trim()}>{m.openProjectWith}</Button>
        </div>
      </form>
    </DialogContent>
  </Dialog>;
}
