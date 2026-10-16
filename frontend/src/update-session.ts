export type Panel =
  | { kind: "command"; id: string; runId?: string; view?: "run" | "launch" }
  | {
      kind:
        | "project"
        | "projectCommands"
        | "collectionCommands"
        | "editCollection"
        | "deleteCollection"
        | "editCommand"
        | "renameProject"
        | "deleteCommand"
        | "deleteProject"
        | "run";
      id: string;
    }
  | { kind: "newCommand"; projectId?: string; collectionId?: string }
  | { kind: "newCollection"; parentId?: string; commandId?: string }
  | { kind: "import"; projectId?: string; directory?: string }
  | { kind: "settings" | "newProject" | "history" | "updates" };
export type UpdateSession = {
  query: string;
  panels: Panel[];
  inputs: Record<string, Record<string, string>>;
  directories: Record<string, string>;
  expanded: Record<string, boolean>;
  scroll: { home: number; panel: number };
};
export type HostSession = { candidate: boolean; resume: UpdateSession | null };
