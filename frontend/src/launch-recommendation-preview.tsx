import { commandIcons } from "./command-icons";
import { launchKindMessages, type LaunchRecommendation } from "./launch-recommendations";
import type { Command } from "./model";
import type { Messages } from "./messages";

export function LaunchRecommendationPreview({
  recommendations,
  commands,
  m,
}: {
  recommendations: LaunchRecommendation[];
  commands: Pick<Command, "id" | "title" | "text" | "directory">[];
  m: Messages;
}) {
  return (
    <ul className="flex flex-wrap gap-x-5 gap-y-2 text-xs">
      {recommendations.map((item) => {
        const command = commands.find((command) => command.id === item.commandId);
        if (!command) return null;
        const Icon = commandIcons[item.icon];
        const kind = m[launchKindMessages[item.kind]];
        return (
          <li key={item.commandId} className="flex min-w-0 items-center gap-2" title={`${command.text}\n${command.directory}`}>
            <Icon className="size-4 shrink-0" aria-hidden="true" />
            <span className="truncate">{kind} · {command.title ?? command.text}</span>
          </li>
        );
      })}
    </ul>
  );
}
