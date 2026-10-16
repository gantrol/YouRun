import type { Command, Project, Run } from "../model";

export const sampleProjects: Project[] = [
  { id: "aiy", name: "AIY", buttons: [{ id: "aiy-preview", commandId: "preview", icon: "play" }, { id: "aiy-debug", commandId: "debug", icon: "terminal" }] },
  { id: "monitor", name: "Codex Micro Monitor", buttons: [{ id: "monitor-debug", commandId: "monitor", icon: "bolt" }] },
  { id: "yourun", name: "YouRun", buttons: [{ id: "yourun-run", commandId: "yourun", icon: "play" }] },
  { id: "timeline", name: "模型纪年", buttons: [{ id: "timeline-debug", commandId: "timeline", icon: "bolt" }, { id: "timeline-build", commandId: "build", icon: "package" }] },
];

export const sampleCommands: Command[] = [
  ["preview", "preview", "aiy", "npm run preview"],
  ["debug", "Debug", "aiy", "npm run dev"],
  ["monitor", "Debug", "monitor", "npm run dev"],
  ["yourun", "Run", "yourun", "npm start"],
  ["timeline", "Debug", "timeline", "npm run dev"],
  ["build", "Build", "timeline", "npm run build"],
  ["check", "Typecheck", "yourun", "npm run typecheck"],
].map(([id, title, project, text]) => ({
  id, title, project, text,
  source: "package.json",
  shell: "default",
  platform: "macOS / Windows",
  directory: `/workspace/${project}`,
  fields: [],
  favorite: false,
}));

export function sampleRun(command: Command, status: Run["status"] = "running"): Run {
  return {
    id: `run-${command.id}`, commandId: command.id, title: command.title ?? command.id,
    text: command.text, directory: command.directory, shell: command.shell,
    values: {}, time: "2026-10-11T09:20:00+08:00", status,
    ...(status === "failed" ? { exitCode: 1 } : {}),
  };
}

export const sampleRuns = [sampleCommands[0], sampleCommands[2], sampleCommands[3]].map((command) => sampleRun(command));
