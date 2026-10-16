import type { ReactNode } from "react";
import { FieldRow, Picker } from "./controls";
import { ProjectLogo } from "./project-logo";
import type { ProjectDetails } from "./project-details";
import type { Messages } from "./messages";

export function ProjectIconPicker({ name, details, disabled, m, onChange, children }: {
  name: string;
  details?: Pick<ProjectDetails, "logo" | "logos" | "selectedLogo" | "customAvatar" | "scannedAt">;
  disabled: boolean;
  m: Messages;
  onChange: (id: string) => void;
  children?: ReactNode;
}) {
  return <FieldRow label={m.projectIcon}>
    <div className="flex min-w-0 items-center gap-2">
      <ProjectLogo name={name} logo={details?.logo} logos={details?.logos} scannedAt={details?.scannedAt} />
      <div className="min-w-0 flex-1">
        <Picker label={m.projectIcon} disabled={disabled}
          value={details?.customAvatar ? "custom" : details?.selectedLogo ? `logo:${details.selectedLogo}` : "automatic"}
          options={[
            ["automatic", m.automaticProjectLogo],
            ...(details?.customAvatar ? [["custom", m.customProjectIcon] as [string, string]] : []),
            ...(details?.selectedLogo && !details.logos?.some((logo) => logo.id === details.selectedLogo)
              ? [[`logo:${details.selectedLogo}`, m.logoSelectionMissing] as [string, string]] : []),
            ...(details?.logos ?? []).map((logo): [string, string] => [`logo:${logo.id}`, `${logo.name} · ${logo.source}`]),
          ]}
          onChange={(value) => { if (value !== "custom") onChange(value === "automatic" ? "" : value.slice(5)); }} />
      </div>
      {children}
    </div>
  </FieldRow>;
}
