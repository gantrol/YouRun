import { useState } from "react";
import { FileText, LoaderCircle, RotateCw, Settings2, Square } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { ActionMenu, ActionMenuItem, ActionMenuLabel, ActionMenuSeparator } from "@/renderer/components/ui/action-menu";
import { commandIcons } from "./command-icons";
import { launcherSkins } from "./design";
import { useAppearance } from "./appearance";
import type { IconId } from "./model";
import type { Messages } from "./messages";
import type { RunOperationState } from "./models/run-operation";
import "./command-action.css";

export function CommandAction({
  icon = "play", m, name, running = false, unassigned = false,
  onClick, stopping = false, onStop, onReplay, onOutput, onOpen, operation,
}: {
  icon?: IconId;
  m: Messages;
  name: string;
  running?: boolean;
  stopping?: boolean;
  operation?: RunOperationState;
  onStop?: () => void;
  onReplay?: () => void;
  onOutput?: () => void;
  onOpen?: () => void;
  unassigned?: boolean;
  onClick: () => void;
}) {
  const { skin } = useAppearance();
  const pending = operation ?? (stopping ? "stopping" : undefined);
  const controls = (running && !!onStop) || pending === "restarting" || pending === "stopping";
  const condition = pending ? m[pending] : running ? m.running : unassigned ? m.notAssigned : "";
  const primaryAction = controls ? "stop" : running ? "output" : "start";
  const label = controls ? m.stop : running ? m.viewRun : unassigned ? m.configure : m.start;
  const Icon = controls ? Square : running ? FileText : commandIcons[icon];
  const primary = controls ? onStop : running ? onOutput ?? onClick : onClick;
  const hasMenu = !!((running && onReplay) || onOutput || onOpen || unassigned);
  const phase = pending ?? primaryAction;
  const [feedback, setFeedback] = useState({ phase, changed: false });
  if (feedback.phase !== phase) setFeedback({ phase, changed: true });
  const buttonLabel = [name, label, condition].filter(Boolean).join(" · ");
  return <div className="inline-flex shrink-0" role="group" aria-label={name}>
    <ActionMenu label={`${name} · ${m.commandActions}`} enabled={hasMenu} onAction={primary} actionDisabled={!!pending}
      trigger={props => <Button {...props} data-launch="" data-command-control={primaryAction} data-pending={pending} data-unassigned={unassigned || undefined}
        variant="ghost" size="icon-sm" aria-disabled={!!pending} aria-busy={!!pending}
        className={`command-action border ${unassigned ? "rounded-sm border-dashed border-input text-muted-foreground" : launcherSkins[skin].action} ${controls ? "bg-destructive-surface text-destructive hover:bg-destructive-surface hover:text-destructive" : running ? "bg-success-surface text-success hover:bg-success/15" : "hover:bg-hover"}`}
        aria-label={buttonLabel} title={hasMenu ? undefined : buttonLabel}>
        <span key={phase} className="command-action-symbol inline-flex" data-changed={feedback.changed || undefined} aria-hidden="true">
          {pending ? pending === "restarting"
            ? <RotateCw className="size-4 motion-safe:animate-spin" />
            : <LoaderCircle className="size-4 motion-safe:animate-spin" />
            : <Icon className="size-4" />}
        </span>
      </Button>}>
      <ActionMenuLabel>{name}</ActionMenuLabel>
      {onOutput && <ActionMenuItem onSelect={onOutput}><FileText aria-hidden="true" />{m.viewRun}</ActionMenuItem>}
      {running && onReplay && <ActionMenuItem disabled={!!pending} onSelect={onReplay}><RotateCw aria-hidden="true" />{m.restart}</ActionMenuItem>}
      {!running && <ActionMenuItem disabled={!!pending} onSelect={onClick}>
        {unassigned ? <Settings2 aria-hidden="true" /> : <Icon aria-hidden="true" />}{unassigned ? m.configure : m.start}
      </ActionMenuItem>}
      {onOpen && <ActionMenuItem onSelect={onOpen}><FileText aria-hidden="true" />{m.viewCommand}</ActionMenuItem>}
      {running && onStop && <><ActionMenuSeparator /><ActionMenuItem disabled={!!pending} className="text-destructive" onSelect={onStop}>
        <Square aria-hidden="true" />{m.stop}
      </ActionMenuItem></>}
    </ActionMenu>
    <span role="status" className="sr-only">{condition ? `${name} · ${condition}` : ""}</span>
  </div>;
}
