import type { Messages } from "./messages";

export type Field = "input" | "output" | "port";
export const iconIds = [
  "play",
  "bolt",
  "terminal",
  "package",
  "upload",
  "rocket",
] as const;
export type IconId = (typeof iconIds)[number];
export const maxLaunchButtons = 3;
export type LaunchButton = {
  id: string;
  commandId: string;
  icon: IconId;
  hidden?: boolean;
};
export function newLaunchButton(commandId = ""): LaunchButton {
  return { id: crypto.randomUUID(), commandId, icon: "play" };
}
export type Project = {
  id: string;
  name: string;
  buttons: LaunchButton[];
  hidden?: boolean;
};
export type Command = {
  id: string;
  titleKey?: keyof Messages;
  title?: string;
  text: string;
  project: string | null;
  source: string | null;
  shell: string;
  platform: string;
  directory: string;
  fields: Field[];
  favorite: boolean;
  icon?: IconId;
  hidden?: boolean;
  persistent?: boolean;
  omitOutput?: boolean;
  external?: boolean;
  unavailable?: boolean;
  aliases?: string[];
};
export type Collection = { id: string; name: string; parent: string | null; commandIds: string[] };
export type Run = {
  id: string;
  commandId: string;
  title: string;
  text: string;
  directory: string;
  values: Record<string, string>;
  shell: string;
  time: string;
  status:
    | "running"
    | "stopping"
    | "success"
    | "stopped"
    | "handedOff"
    | "failed"
    | "unknown";
  pid?: number;
  exitCode?: number;
  error?: string;
  omitOutput?: boolean;
};
export function commandTitle(command: Command, messages: Messages) {
  return (
    command.title ??
    (command.titleKey ? messages[command.titleKey] : command.id)
  );
}

export function isActiveRun(run: Run) {
  return run.status === "running" || run.status === "stopping";
}

export function visibleButtons(project: Project, commands: Command[]) {
  return project.buttons.slice(0, maxLaunchButtons).filter((button) => {
    const command = commands.find((item) => item.id === button.commandId);
    return (
      !button.hidden &&
      (!command || (!command.hidden && command.project === project.id))
    );
  });
}
export function clearCommandBindings(
  projects: Project[],
  commandId: string,
  projectId: string | null = null,
) {
  return projects.map((project) =>
    project.id === projectId
      ? project
      : {
          ...project,
          buttons: project.buttons.map((button) =>
            button.commandId === commandId
              ? { ...button, commandId: "" }
              : button,
          ),
        },
  );
}
