import { Package, Play, Rocket, Terminal, Upload, Zap } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { iconIds, type IconId } from "./model";
import type { Messages } from "./messages";

export const commandIcons = {
  play: Play,
  bolt: Zap,
  terminal: Terminal,
  package: Package,
  upload: Upload,
  rocket: Rocket,
};
const labels = {
  play: "iconPlay",
  bolt: "iconBolt",
  terminal: "iconTerminal",
  package: "iconPackage",
  upload: "iconUpload",
  rocket: "iconRocket",
} as const;

export function IconPicker({
  value,
  onChange,
  m,
}: {
  value: IconId;
  onChange: (value: IconId) => void;
  m: Messages;
}) {
  return (
    <div
      role="group"
      aria-label={m.buttonIcon}
      className="flex flex-wrap gap-1"
    >
      {iconIds.map((id) => {
        const Icon = commandIcons[id];
        return (
          <Button
            key={id}
            type="button"
            variant="ghost"
            size="icon-sm"
            aria-label={m[labels[id]]}
            title={m[labels[id]]}
            aria-pressed={id === value}
            className={
              id === value
                ? "border border-selected-border bg-selected text-selected-foreground hover:bg-selected"
                : "border border-transparent text-muted-foreground hover:border-border"
            }
            onClick={() => onChange(id)}
          >
            <Icon className="size-4" aria-hidden="true" />
          </Button>
        );
      })}
    </div>
  );
}
