import { Button } from "@/renderer/components/ui/button";
import { FieldRow, Picker } from "./controls";
import { commandIcons } from "./command-icons";
import type { ImportCandidate } from "./project-import-data";
import { launchKindMessages, type LaunchRecommendation } from "./launch-recommendations";
import type { Messages } from "./messages";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { LaunchRecommendationPreview } from "./launch-recommendation-preview";

export function launchSlots(recommendations: LaunchRecommendation[]): LaunchRecommendation[] {
  return [
    recommendations.find((item) => item.kind === "debug") ?? { kind: "debug", icon: "bolt", commandId: "" },
    recommendations.find((item) => item.kind === "run") ?? { kind: "run", icon: "play", commandId: "" },
    recommendations.find((item) => item.kind === "release" || item.kind === "build") ?? { kind: "release", icon: "package", commandId: "" },
  ];
}

export function ImportLaunchButtons({ value, commands, disabled, m, onChange, onRecommend }: {
  value: LaunchRecommendation[];
  commands: ImportCandidate[];
  disabled: boolean;
  m: Messages;
  onChange: (value: LaunchRecommendation[]) => void;
  onRecommend: () => void;
}) {
  return (
    <Disclosure className="border-t border-border pt-3" label={m.launchButtons}
      defaultOpen={!value.some((button) => button.commandId)}
      summary={<LaunchRecommendationPreview recommendations={value} commands={commands} m={m} />}>
      <div className="space-y-3">
      {value.map((button, index) => {
        const label = m[launchKindMessages[button.kind]];
        const Icon = commandIcons[button.icon];
        return (
          <FieldRow key={index} label={label}>
            <div className="flex items-center gap-2">
              <Icon className="size-4 shrink-0" aria-hidden="true" />
              <Picker label={label} value={button.commandId || "none"} disabled={disabled}
                options={[["none", m.notAssigned], ...commands
                  .filter((command) => !value.some((other, i) => i !== index && other.commandId === command.id))
                  .map((command): [string, string] => [command.id, `${command.title} · ${command.text}${command.review ? ` · ${m.reviewRequired}` : ""}`])]}
                onChange={(id) => onChange(value.map((item, i) => i === index ? { ...item, commandId: id === "none" ? "" : id } : item))} />
            </div>
          </FieldRow>
        );
      })}
        <Button type="button" variant="ghost" size="sm" disabled={disabled} onClick={onRecommend}>{m.recommendedSettings}</Button>
      </div>
    </Disclosure>
  );
}
