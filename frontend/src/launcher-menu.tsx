import { useRef, type ReactElement } from "react";
import {
  Copy,
  Check,
  EyeOff,
  Pencil,
  Play,
  RefreshCw,
  RotateCw,
  Settings2,
  Square,
  Trash2,
  FileText,
  ImagePlus,
} from "lucide-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/renderer/components/ui/context-menu";
import type { Command, Project, Run } from "./model";
import { commandTitle, visibleButtons, isActiveRun } from "./model";
import type { Messages } from "./messages";
import type { ProjectDetails } from "./project-details";
import { ProjectOpenersMenu, type ProjectOpeners } from "./project-openers";
import type { RunOperationState } from "./models/run-operation";

export type LauncherMenuActions = {
  operation?: (commandId: string) => RunOperationState | undefined;
  addToCollections?: (id: string) => void;
  removeFromCollection?: (id: string) => void;
  moveCommand?: (id: string, direction: number) => void;
  editCommand: (id: string) => void;
  hideCommand: (id: string) => void;
  deleteCommand: (id: string) => void;
  renameProject: (id: string) => void;
  hideProject: (id: string) => void;
  deleteProject: (id: string) => void;
  openDirectory: (id: string) => void;
  projectOpeners: ProjectOpeners;
  openWith: (id: string, opener: string) => void;
  customEditor: (id: string) => void;
  copy: (text: string) => void;
  refreshLogo: (id: string) => void;
  logoBusy: (id: string) => boolean;
  savingLogo: boolean;
  selectLogo: (id: string, logoId: string) => void;
  chooseAvatar: (id: string) => void;
};
type Common = {
  m: Messages;
  actions: LauncherMenuActions;
  children: ReactElement;
  onRun: (command: Command) => void;
  onStop: (id: string) => void;
  onReplay: (id: string) => void;
};

function MenuShell({
  children,
  items,
}: {
  children: ReactElement;
  items: ReactElement;
}) {
  const trigger = useRef<HTMLElement | null>(null);
  return (
    <ContextMenu
      modal={false}
      onOpenChange={(open) => {
        if (open) trigger.current = document.activeElement as HTMLElement;
      }}
    >
      <ContextMenuTrigger
        asChild
        onContextMenu={(event) => {
          const target =
            (event.target as HTMLElement).closest("button") ??
            event.currentTarget.querySelector("button");
          target?.focus();
        }}
      >
        {children}
      </ContextMenuTrigger>
      <ContextMenuContent
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          // Navigation owns focus after a menu action; Escape restores the row.
          const active = document.activeElement;
          if (active === document.body || active?.closest('[role="menu"]'))
            trigger.current?.focus();
        }}
      >
        {items}
      </ContextMenuContent>
    </ContextMenu>
  );
}
function RunItems({
  command,
  run,
  m,
  onRun,
  onStop,
  onReplay,
  named = false,
  operation,
}: Omit<Common, "actions" | "children"> & {
  command: Command;
  run?: Run;
  named?: boolean;
  operation?: RunOperationState;
}) {
  const suffix = named ? ` · ${commandTitle(command, m)}` : "";
  return (
    <>
      <ContextMenuItem disabled={!run && !!operation} onSelect={() => onRun(command)}>
        {run ? <FileText /> : <Play />}
        {(run ? m.viewRun : operation ? m[operation] : m.start) + suffix}
      </ContextMenuItem>
      {run && (
        <>
          <ContextMenuItem
            disabled={!!operation || run.status === "stopping"}
            onSelect={() => onReplay(run.id)}
          >
            <RotateCw />
            {(operation === "restarting" ? m.restarting : m.restart) + suffix}
          </ContextMenuItem>
          <ContextMenuItem
            destructive
            disabled={!!operation || run.status === "stopping"}
            onSelect={() => onStop(run.id)}
          >
            <Square />
            {(operation === "stopping" || run.status === "stopping" ? m.stopping : m.stop) + suffix}
          </ContextMenuItem>
        </>
      )}
    </>
  );
}
export function CommandMenu({
  command,
  run,
  onOpen,
  ...props
}: Common & {
  command: Command;
  run?: Run;
  onOpen: (command: Command) => void;
}) {
  const { m, actions } = props;
  return (
    <MenuShell
      children={props.children}
      items={
        <>
          <RunItems {...props} command={command} run={run} operation={actions.operation?.(command.id)} />
          <ContextMenuItem onSelect={() => onOpen(command)}>
            <FileText />
            {m.viewCommand}
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem onSelect={() => actions.copy(command.text)}>
            <Copy />
            {m.copyCommand}
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => actions.editCommand(command.id)}>
            <Pencil />
            {m.edit}
          </ContextMenuItem>
          {actions.addToCollections && <ContextMenuItem onSelect={() => actions.addToCollections?.(command.id)}>{m.addToCollections}</ContextMenuItem>}
          {actions.removeFromCollection && <ContextMenuItem onSelect={() => actions.removeFromCollection?.(command.id)}>{m.removeFromCollection}</ContextMenuItem>}
          {actions.moveCommand && <>
            <ContextMenuItem onSelect={() => actions.moveCommand?.(command.id, -1)}>{m.moveUp}</ContextMenuItem>
            <ContextMenuItem onSelect={() => actions.moveCommand?.(command.id, 1)}>{m.moveDown}</ContextMenuItem>
          </>}
          <ContextMenuItem onSelect={() => actions.hideCommand(command.id)}>
            <EyeOff />
            {m.hide}
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem
            destructive
            onSelect={() => actions.deleteCommand(command.id)}
          >
            <Trash2 />
            {m.remove}
          </ContextMenuItem>
        </>
      }
    />
  );
}
export function ProjectMenu({
  project,
  commands,
  runs,
  details,
  onSettings,
  ...props
}: Common & {
  project: Project;
  commands: Command[];
  runs: Run[];
  details?: ProjectDetails;
  onSettings: () => void;
}) {
  const { m, actions } = props;
  const shortcuts = visibleButtons(project, commands)
    .map((button) =>
      commands.find(
        (command) =>
          command.id === button.commandId && command.project === project.id,
      ),
    )
    .filter((command): command is Command => !!command);
  return (
    <MenuShell
      children={props.children}
      items={
        <>
          {shortcuts.map((command) => (
            <RunItems
              key={command.id}
              {...props}
              command={command}
              operation={actions.operation?.(command.id)}
              run={runs.find(
                (run) => run.commandId === command.id && isActiveRun(run),
              )}
              named
            />
          ))}
          {shortcuts.length > 0 && <ContextMenuSeparator />}
          <ProjectOpenersMenu openers={actions.projectOpeners} m={m} disabled={!details?.directory}
            onOpen={(opener) => actions.openWith(project.id, opener)} onCustom={() => actions.customEditor(project.id)}
            onOpenDirectory={() => actions.openDirectory(project.id)} />
          <ContextMenuItem
            disabled={!details?.directory}
            onSelect={() => actions.copy(details!.directory)}
          >
            <Copy />
            {m.copyProjectPath}
          </ContextMenuItem>
          <ContextMenuItem disabled={actions.logoBusy(project.id)} onSelect={() => actions.refreshLogo(project.id)}>
            <RefreshCw className={actions.logoBusy(project.id) ? "motion-safe:animate-spin" : ""} />
            {actions.logoBusy(project.id) ? m.scanning : m.refreshProjectLogo}
          </ContextMenuItem>
          <ContextMenuItem disabled={actions.savingLogo} onSelect={() => actions.chooseAvatar(project.id)}>
            <ImagePlus />
            {m.chooseProjectAvatar}
          </ContextMenuItem>
          {details && (!!details.logos?.length || !!details.selectedLogo || !!details.customAvatar) && (
            <>
              <ContextMenuSeparator />
              {details.issues?.some((issue) => issue.code === "logoSelectionMissing") && (
                <ContextMenuItem disabled>{m.logoSelectionMissing}</ContextMenuItem>
              )}
              {details.issues?.some((issue) => issue.code === "avatarReadFailed") && (
                <ContextMenuItem disabled>{m.avatarReadFailed}</ContextMenuItem>
              )}
              <ContextMenuItem
                role="menuitemradio"
                disabled={actions.savingLogo}
                aria-checked={!details.selectedLogo && !details.customAvatar}
                onSelect={() => actions.selectLogo(project.id, "")}
              >
                <Check className={details.selectedLogo || details.customAvatar ? "invisible" : ""} />
                {m.automaticProjectLogo}
              </ContextMenuItem>
              {details.logos?.map((logo) => (
                <ContextMenuItem
                  key={logo.id}
                  title={logo.source}
                  role="menuitemradio"
                  disabled={actions.savingLogo || actions.logoBusy(project.id)}
                  aria-checked={details.selectedLogo === logo.id}
                  onSelect={() => actions.selectLogo(project.id, logo.id)}
                >
                  <Check
                    className={details.selectedLogo === logo.id ? "" : "invisible"}
                  />
                  <img
                    src={logo.data}
                    alt=""
                    className="size-4 shrink-0 object-contain"
                  />
                  <span className="max-w-64 truncate">{logo.name}</span>
                </ContextMenuItem>
              ))}
            </>
          )}
          <ContextMenuSeparator />
          <ContextMenuItem onSelect={onSettings}>
            <Settings2 />
            {m.projectSettings}
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => actions.renameProject(project.id)}>
            <Pencil />
            {m.renameProject}
          </ContextMenuItem>
          <ContextMenuItem onSelect={() => actions.hideProject(project.id)}>
            <EyeOff />
            {m.hide}
          </ContextMenuItem>
          <ContextMenuSeparator />
          <ContextMenuItem
            destructive
            onSelect={() => actions.deleteProject(project.id)}
          >
            <Trash2 />
            {m.deleteProject}
          </ContextMenuItem>
        </>
      }
    />
  );
}
