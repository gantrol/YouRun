import type { ReactElement, ReactNode } from "react";
import { ChevronRight, LoaderCircle, Settings2 } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/renderer/components/ui/collapsible";
import { useAppearance } from "./appearance";
import { launcherSkins } from "./design";
import { ProjectLogo } from "./project-logo";
import type { ProjectDetails } from "./project-details";

export function ProjectGroup({
  projectId,
  name,
  open,
  onOpenChange,
  actions,
  settingsLabel,
  onSettings,
  children,
  logo,
  logos,
  scannedAt,
  busy,
  busyLabel,
  menu,
  status,
  expandable = true,
}: {
  projectId: string;
  name: string;
  open: boolean;
  onOpenChange: () => void;
  actions: ReactNode;
  settingsLabel: string;
  onSettings: () => void;
  children: ReactNode;
  logo?: ProjectDetails["logo"];
  logos?: ProjectDetails["logos"];
  scannedAt?: string;
  busy?: boolean;
  busyLabel: string;
  menu: (trigger: ReactElement) => ReactNode;
  status?: ReactNode;
  expandable?: boolean;
}) {
  const { skin } = useAppearance();
  const style = launcherSkins[skin];
  const headingClass = "flex h-auto min-h-10 min-w-0 flex-1 basis-48 items-center justify-start gap-2 rounded-sm px-0 py-1 text-left";
  const heading = <>
    {expandable ? <ChevronRight
      className={`size-4 shrink-0 text-muted-foreground motion-safe:transition-transform motion-safe:duration-fast ${open ? "rotate-90" : ""}`}
      aria-hidden="true"
    /> : <span className="size-4 shrink-0" aria-hidden="true" />}
    <ProjectLogo name={name} logo={logo} logos={logos} scannedAt={scannedAt} />
    <span className={`min-w-0 truncate text-base ${style.title}`}>{name}</span>
    {busy && <span role="status" aria-label={busyLabel}>
      <LoaderCircle className="size-3.5 shrink-0 motion-safe:animate-spin" aria-hidden="true" />
    </span>}
  </>;
  return (
    <Collapsible asChild open={expandable && open} onOpenChange={onOpenChange}>
      <li data-project-id={projectId}>
        {menu(
          <div
            className={`sticky top-0 z-20 flex min-h-12 flex-wrap items-center gap-x-2 gap-y-1 px-2 py-1 ${open ? "bg-surface-sunken" : "bg-surface hover:bg-hover focus-within:bg-hover"}`}
          >
            {expandable ? <CollapsibleTrigger asChild>
              <Button
                variant="ghost"
                className={`${headingClass} hover:bg-transparent`}
                title={name}
                data-project-heading
              >
                {heading}
              </Button>
            </CollapsibleTrigger> : <div className={`${headingClass} outline-none focus-visible:ring-2 focus-visible:ring-ring`}
              tabIndex={-1} title={name} data-project-heading>{heading}</div>}
            <div className="ml-auto flex max-w-full flex-wrap items-center justify-end gap-2">
              <Button
                variant="ghost"
                size="icon-sm"
                className="text-muted-foreground"
                aria-label={settingsLabel}
                title={settingsLabel}
                onClick={onSettings}
              >
                <Settings2 className="size-4" aria-hidden="true" />
              </Button>
              {actions}
            </div>
          </div>,
        )}
        {status}
        {expandable && <CollapsibleContent inert={!open}>
          <div className="ml-4 mt-1 border-l border-border pb-1 pl-3">
            {children}
          </div>
        </CollapsibleContent>}
      </li>
    </Collapsible>
  );
}
