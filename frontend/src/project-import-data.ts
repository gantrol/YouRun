import type { Command } from "./model";
import type { ProjectDetails } from "./project-details";

export type ImportCandidate = {
  id: string;
  title: string;
  text: string;
  source: string;
  directory: string;
  tool: string;
  origin: "configuration" | "template";
  review: boolean;
  shell: string;
  platform: string;
  icon: "play";
};
export type ImportResult = {
  directory: string;
  projectId?: string;
  name: string;
  names?: { name: string; source: string }[];
  logos?: ProjectDetails["logos"];
  candidates: ImportCandidate[];
  issues: { path: string; code: string }[];
};
export type ImportLogoChoice = { logoId: string; revision: number };
export class ImportMetadataError extends Error {
  constructor(readonly projectId: string, code = "importProjectMetadataFailed") {
    super(code);
  }
}
export function alreadySaved(candidate: ImportCandidate, commands: Command[]) {
  return commands.find(
    (command) =>
      command.id === candidate.id ||
      (command.source === candidate.source && command.text === candidate.text),
  );
}
