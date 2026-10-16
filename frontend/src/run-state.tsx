import {
  Check,
  CircleAlert,
  CircleHelp,
  CirclePause,
  ExternalLink,
  LoaderCircle,
  RotateCw,
  Unplug,
  type LucideIcon,
} from "lucide-react";
import type { Messages } from "./messages";
import type { Run } from "./model";
import { RabbitActivity, type RabbitActivityState } from "./rabbit-activity";
import { useEffect, useRef, useState } from "react";

export type RunState =
  | Run["status"]
  | "starting"
  | "restarting"
  | "stopping"
  | "reconnecting"
  | "unknown"
  | "failed";
const appearance: Record<
  RunState,
  { color: string } & ({ activity: RabbitActivityState; icon?: never } | { icon: LucideIcon; activity?: never })
> = {
  starting: {
    icon: LoaderCircle,
    color: "text-muted-foreground",
  },
  running: { activity: "running", color: "text-success" },
  restarting: { icon: RotateCw, color: "text-warning" },
  stopping: { icon: LoaderCircle, color: "text-warning" },
  reconnecting: { icon: Unplug, color: "text-warning" },
  unknown: { icon: CircleHelp, color: "text-muted-foreground" },
  success: { icon: Check, color: "text-success" },
  stopped: { icon: CirclePause, color: "text-muted-foreground" },
  handedOff: { icon: ExternalLink, color: "text-muted-foreground" },
  failed: { icon: CircleAlert, color: "text-destructive" },
};

export function RunStatus({
  state,
  m,
  iconOnly = false,
}: {
  state: RunState;
  m: Pick<Messages, RunState>;
  iconOnly?: boolean;
}) {
  const { icon: Icon, color, activity } = appearance[state];
  const previous = useRef(state);
  const [hop, setHop] = useState(false);
  useEffect(() => {
    setHop(state === "running" && (previous.current === "starting" || previous.current === "restarting"));
    previous.current = state;
  }, [state]);
  const pending = state === "starting" || state === "restarting" || state === "stopping";
  return (
    <span
      className={`inline-flex shrink-0 items-center gap-1.5 whitespace-nowrap text-xs ${color}`}
      title={iconOnly ? m[state] : undefined}
      data-state={state}
    >
      {activity ? <RabbitActivity state={activity} size={14} animate={hop} /> : <Icon aria-hidden="true" className={`size-3.5 ${pending ? "motion-safe:animate-spin" : ""}`} />}
      <span className={iconOnly ? "sr-only" : undefined}>{m[state]}</span>
    </span>
  );
}
