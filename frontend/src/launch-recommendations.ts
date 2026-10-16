import { maxLaunchButtons, type Command, type IconId, type LaunchButton } from "./model";
import type { Messages } from "./messages";

type Candidate = Pick<Command, "id" | "text" | "directory" | "platform"> &
  Partial<Pick<Command, "hidden" | "unavailable" | "fields">>;
export type LaunchRecommendation = {
  commandId: string;
  kind: "debug" | "run" | "release" | "build";
  icon: IconId;
};
export const launchKindMessages = {
  debug: "launchDebug",
  run: "launchRun",
  release: "launchRelease",
  build: "launchBuild",
} as const satisfies Record<LaunchRecommendation["kind"], keyof Messages>;
type Ranked = LaunchRecommendation & { score: number; directory: string; text: string };

// Recognize the invoked task, not incidental words in paths or package names.
function rank(command: Candidate): Omit<Ranked, "commandId" | "directory" | "text"> | undefined {
  if (/[;&|\r\n`]/.test(command.text)) return;
  const tokens = (command.text.match(/"[^"]*"|'[^']*'|[^\s]+/g) ?? [])
    .map((token) => token.replace(/^(['"])(.*)\1$/, "$2"));
  const executable = (token: string) => token.split(/[\\/]/).at(-1)!.replace(/\.(exe|cmd|bat|ps1)$/i, "").toLowerCase();
  const invoked = tokens.shift() ?? "";
  let tool = executable(invoked);
  if (tool === "npx" || (["npm", "pnpm", "yarn", "bun"].includes(tool) && ["exec", "x"].includes(tokens[0]))) {
    if (tool !== "npx") tokens.shift();
    const separator = tokens.indexOf("--");
    if (separator >= 0) tokens.splice(0, separator + 1);
    else {
      while (tokens[0]?.startsWith("-")) {
        const flag = tokens.shift();
        if (flag === "--package" || flag === "-p") tokens.shift();
        else if (flag !== "--no-install" && flag !== "--offline" && flag !== "--no" && !flag?.startsWith("--package=")) return;
      }
    }
    tool = executable(tokens.shift() ?? "");
  }
  const args = tokens.map((token) => token.toLowerCase());
  let action = tokens[0] ?? "";
  let script = false;
  let entrypoint = false;
  if (/^\.[\\/](run|start|debug|build|release|package)\.(cmd|bat|ps1)$/i.test(invoked) && !tokens.length) {
    action = tool;
    entrypoint = true;
  } else if (tool === "bash" && tokens.length === 1 && /^\.\/(run|start|debug|build|release|package)\.sh$/i.test(tokens[0])) {
    action = executable(tokens[0]).replace(/\.sh$/, "");
    entrypoint = true;
  } else if (["npm", "pnpm", "yarn", "bun"].includes(tool)) {
    script = true;
    if (["run", "run-script"].includes(args[0])) action = tokens[1] ?? "";
    else if (["exec", "dlx"].includes(args[0])) return;
  } else if (["go", "cargo", "swift", "dotnet", "flutter", "wails", "tauri", "vite", "next", "nuxt", "astro", "nest", "expo", "react-native", "ng"].includes(tool)) {
    if (["vite", "next", "nuxt", "astro", "expo"].includes(tool) && !action) action = "dev";
    if (tool === "dotnet" && args[0] === "watch" && args.includes("test")) return;
  } else if (["gradle", "gradlew", "mvn", "mvnw"].includes(tool)) {
    const tasks = tokens.filter((token) => !token.startsWith("-"));
    if (tasks.length !== 1) return;
    action = tasks[0];
  } else if (tool === "cmake" && args[0] === "--build") {
    action = "build";
  } else if (tool === "msbuild") {
    const target = args.find((arg) => /^[-/]t(arget)?:/.test(arg));
    if (target && !/^[-/]t(arget)?:(re)?build$/.test(target)) return;
    action = "build";
  } else if (tool === "xcodebuild" && args.includes("build") && !args.includes("test")) {
    action = "build";
  } else {
    return;
  }
  const words = action.replace(/([a-z])([A-Z])/g, "$1 $2").toLowerCase().split(/[\s:._-]+/);
  if (words.some((word) => ["test", "lint", "check", "typecheck", "format", "clean", "install", "deploy", "publish", "upload", "migrate", "seed", "delete", "remove"].includes(word))) return;
  const has = (...values: string[]) => values.some((value) => words.includes(value));
  const build = has("build", "rebuild", "assemble", "bundle", "package", "pack");
  const run = has("run", "start", "serve");
  const debug = has("debug", "dev", "develop", "development", "watch");
  const release = has("release", "prod", "production") || (entrypoint && has("package"));
  if (!build && !run && !debug && !release) return;
  const flags = args.join(" ");
  const configuration = flags.match(/(?:^|\s)(?:--configuration[= ]|--config[= ]|-configuration |-c |[-/]p:configuration=)(debug|release)(?:\s|$)/)?.[1]
    ?? (tool === "cmake" ? flags.match(/(?:^|\s)--preset(?:=|\s)(debug|release)(?:\s|$)/)?.[1] : undefined);
  const exact = words.length === 1 ? 20 : 0;
  const configured = entrypoint ? 30 : script ? 10 : 0;
  // A native build's configuration does not turn it into a launch command.
  if (build && !script && !entrypoint)
    return { kind: "build", icon: "package", score: 50 + exact + configured };
  if (release || args.includes("--release") || configuration === "release")
    return { kind: "release", icon: "package", score: 150 + exact + configured };
  if (debug || args.includes("--debug") || args.includes("--watch") || args.some((arg) => /^--inspect(?:-brk)?(?:=|$)/.test(arg)) || configuration === "debug")
    return { kind: "debug", icon: "bolt", score: 100 + exact + configured + (has("debug") ? 10 : has("dev") ? 5 : 0) };
  if (run)
    return { kind: "run", icon: "play", score: 100 + exact + configured + (has("run") ? 10 : has("start") ? 5 : 0) + (args[1] === "." ? 5 : 0) };
  if (build)
    return { kind: "build", icon: "package", score: 50 + exact + configured };
}

export function recommendLaunchButtons(
  commands: Candidate[],
  platform: string,
  preferredDirectory = "",
): LaunchRecommendation[] {
  const pathKey = (path: string) => {
    const normalized = path.replaceAll("\\", "/").replace(/\/+$/, "");
    return platform === "windows" ? normalized.toLowerCase() : normalized;
  };
  const groups = new Map<string, Ranked[]>();
  for (const command of commands) {
    if (command.hidden || command.unavailable || command.fields?.length || !command.directory.trim()) continue;
    if ((command.platform === "Windows" && platform !== "windows") || (command.platform === "macOS" && platform !== "darwin")) continue;
    const match = rank(command);
    if (!match) continue;
    const directory = pathKey(command.directory);
    const group = groups.get(directory) ?? [];
    if (!group.some((item) => item.commandId === command.id || item.text === command.text))
      group.push({ ...match, commandId: command.id, directory, text: command.text });
    groups.set(directory, group);
  }
  // Keep one application's shortcuts together in a multi-project import.
  const recommendations = [...groups.entries()].map(([directory, candidates]) => {
    const buttons: Ranked[] = [];
    for (const kinds of [["debug"], ["run"], ["release", "build"]]) {
      const ranked = candidates.filter((item) => kinds.includes(item.kind)).sort((a, b) => b.score - a.score);
      // Equal candidates need a manual choice; traversal order is not a preference.
      if (ranked[0] && ranked[0].score !== ranked[1]?.score) buttons.push(ranked[0]);
    }
    return { buttons, score: buttons.length * 1000 + buttons.reduce((sum, item) => sum + item.score, 0) + (directory === pathKey(preferredDirectory) ? 100 : 0) };
  }).sort((a, b) => b.score - a.score);
  if (!recommendations[0] || recommendations[0].score === recommendations[1]?.score) return [];
  return recommendations[0].buttons.slice(0, maxLaunchButtons).map(({ commandId, kind, icon }) => ({ commandId, kind, icon }));
}

export function applyLaunchRecommendations(
  recommendations: LaunchRecommendation[],
  previous: LaunchButton[] = [],
): LaunchButton[] {
  return recommendations.map(({ commandId, icon }) => ({
    commandId,
    id: previous.find((button) => button.commandId === commandId)?.id ?? crypto.randomUUID(),
    icon,
  }));
}
