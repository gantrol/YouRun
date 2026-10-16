import { YouRunLogo } from "./yourun-logo";
import "./rabbit-activity.css";

export type RabbitActivityState = "starting" | "running" | "stopping";

/** Decorative feedback; the owning status or button supplies the accessible name. */
export function RabbitActivity({
  state,
  size = 16,
  className = "",
  animate = state !== "running",
}: {
  state: RabbitActivityState;
  size?: number;
  className?: string;
  animate?: boolean;
}) {
  return (
    <span
      className={`yourun-rabbit-activity relative inline-flex shrink-0 ${className}`}
      data-state={state}
      data-animate={animate || undefined}
      style={{ width: size, height: size }}
      aria-hidden="true"
    >
      <YouRunLogo size={size} monochrome />
    </span>
  );
}
