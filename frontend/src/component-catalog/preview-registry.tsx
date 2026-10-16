import { useState, type ComponentType } from "react";
import { Button } from "@/renderer/components/ui/button";
import { ProjectLogo } from "../project-logo";
import { ProjectGroup } from "../project-group";
import { SearchBar } from "../search-bar";
import { TitleBar } from "../title-bar";
import { WindowControls } from "../window-controls";
import { CommandMenu, ProjectMenu, type LauncherMenuActions } from "../launcher-menu";
import { YouRunLogo } from "../yourun-logo";
import { ButtonsPreview, FieldsPreview, FeedbackPreview, OverlaysPreview, type PreviewProps } from "./component-previews";
import { PanelPreview } from "./panel-previews";
import { LayoutPreview } from "./layout-preview";
import { RabbitActivityPreview } from "./rabbit-activity-preview";
import { ActionMenuPreview } from "./action-menu-preview";
import { sampleCommands, sampleProjects, sampleRuns } from "./samples";

export function FoundationsPreview({ c }: PreviewProps) {
  return <div className="space-y-6">
    <div className="grid grid-cols-2 gap-4 @min-[32rem]/gallery:grid-cols-4">
      {["background", "surface", "surface-sunken", "foreground", "muted-foreground", "border", "selected", "button-primary", "success", "warning", "destructive", "ring"].map((color) => <div key={color} className="min-w-0 space-y-2">
        <div className="h-10 border border-border" style={{ backgroundColor: `var(--${color})` }} />
        <code className="block break-words text-xs text-muted-foreground">{color}</code>
      </div>)}
    </div>
    <div className="flex flex-wrap items-baseline gap-6 border-t border-border pt-5">
      <span className="text-md font-semibold">YouRun · Aa 字</span><span className="text-base">{c.typography} · 14</span>
      <span className="text-sm">{c.typography} · 13</span><span className="text-xs">{c.typography} · 12</span><code className="text-xs">npm run build</code>
    </div>
  </div>;
}

function ChromePreview({ m, only }: PreviewProps) {
  const [query, setQuery] = useState("");
  const [expanded, setExpanded] = useState(false);
  const [maximized, setMaximized] = useState(false);
  const [result, setResult] = useState("");
  const controls = <WindowControls m={m} maximized={maximized} onAction={(action) => {
    if (action === "toggleMaximize") setMaximized((value) => !value);
    setResult(action === "close" ? m.windowClose : action === "minimize" ? m.windowMinimize : m.windowMaximize);
  }} />;
  return <div className="space-y-4">
    {only === "SearchBar" && <SearchBar m={m} value={query} onChange={setQuery} />}
    {only === "WindowControls" && <div className="flex justify-end">{controls}</div>}
    {only === "TitleBar" && <TitleBar m={m} title="YouRun" controls={controls} />}
    {only === "ProjectLogo" && <div className="flex flex-wrap gap-6">{["YouRun", "AIY", "模型纪年"].map((name) => <div key={name} className="flex items-center gap-2"><ProjectLogo name={name} /><span>{name}</span></div>)}</div>}
    {only === "YouRunLogo" && <div className="flex items-end gap-6">{[16, 24, 32, 48].map((size) => <YouRunLogo key={size} size={size} />)}</div>}
    {["ProjectGroup", "Collapsible"].includes(only) && <ul><ProjectGroup projectId="preview" name="YouRun" open={expanded} onOpenChange={() => setExpanded((value) => !value)}
      settingsLabel={m.projectSettings} onSettings={() => setResult(m.projectSettings)} busyLabel={m.scanning} menu={(trigger) => trigger} actions={null}>
      <Button variant="ghost" onClick={() => setResult(m.start)}>npm run build</Button>
    </ProjectGroup></ul>}
    {result && <output className="block text-xs text-muted-foreground">{result}</output>}
  </div>;
}

function MenusPreview({ m, c, only }: PreviewProps) {
  const [result, setResult] = useState("");
  const actions: LauncherMenuActions = {
    editCommand: () => setResult(m.editCommand), hideCommand: () => setResult(m.hide), deleteCommand: () => setResult(m.remove),
    renameProject: () => setResult(m.renameProject), hideProject: () => setResult(m.hide), deleteProject: () => setResult(m.deleteProject),
    openDirectory: () => setResult(m.openProjectDirectory), openWith: () => setResult(m.openProjectWith), customEditor: () => setResult(m.otherProjectEditor),
    projectOpeners: { platform: "windows", items: [{ id: "editor", name: "Visual Studio Code" }], loading: false, error: "", refresh: () => setResult(m.refresh) },
    copy: () => setResult(m.copyCommand), refreshLogo: () => setResult(m.refreshProjectLogo), logoBusy: () => false, savingLogo: false,
    selectLogo: () => setResult(m.projectIcon), chooseAvatar: () => setResult(m.chooseProjectAvatar),
  };
  const common = { m, actions, onRun: () => setResult(m.start), onStop: () => setResult(m.stop), onReplay: () => setResult(m.restart) };
  return <div className="space-y-4">
    {only === "ProjectMenu" ? <ProjectMenu {...common} project={sampleProjects[0]} commands={sampleCommands} runs={sampleRuns}
      details={{ directory: "/workspace/aiy", logo: null }} onSettings={() => setResult(m.projectSettings)}>
      <Button variant="outline">AIY · {c.contextMenu}</Button>
    </ProjectMenu> : <CommandMenu {...common} command={sampleCommands[0]} run={sampleRuns[0]} onOpen={() => setResult(m.viewCommand)}>
      <Button variant="outline">preview · {c.contextMenu}</Button>
    </CommandMenu>}
    {result && <output className="block text-xs text-muted-foreground">{result}</output>}
  </div>;
}

function AppearancePreview() {
  return <div className="flex flex-wrap items-center gap-4"><YouRunLogo size={32} /><Button>YouRun</Button><code className="text-xs text-muted-foreground">var(--background)</code></div>;
}

type Preview = { render: ComponentType<PreviewProps>; wide?: boolean };
export const previews: Record<string, Preview> = {
  Button: { render: ButtonsPreview }, Tooltip: { render: ButtonsPreview }, IconPicker: { render: ButtonsPreview }, CommandAction: { render: ButtonsPreview },
  Input: { render: FieldsPreview }, Textarea: { render: FieldsPreview }, Checkbox: { render: FieldsPreview }, Select: { render: FieldsPreview },
  TextField: { render: FieldsPreview }, Picker: { render: FieldsPreview }, FieldRow: { render: FieldsPreview },
  ProjectNameField: { render: FieldsPreview }, Disclosure: { render: FieldsPreview },
  ImportLaunchButtons: { render: FieldsPreview, wide: true }, LaunchRecommendationPreview: { render: FieldsPreview },
  RunStatus: { render: FeedbackPreview }, RunHistory: { render: FeedbackPreview }, CommandFailure: { render: FeedbackPreview },
  RabbitActivity: { render: RabbitActivityPreview },
  ActionMenu: { render: ActionMenuPreview },
  ProjectActivity: { render: LayoutPreview, wide: true },
  ImportRecovery: { render: FeedbackPreview }, ProjectMetadataError: { render: FeedbackPreview }, ScanIssues: { render: FeedbackPreview },
  Dialog: { render: OverlaysPreview }, ContextMenu: { render: OverlaysPreview }, DropdownMenu: { render: OverlaysPreview }, ModalOverlayScope: { render: OverlaysPreview },
  ProjectSettings: { render: PanelPreview, wide: true }, GlobalSettings: { render: PanelPreview }, DeletePanel: { render: PanelPreview },
  ProjectMetadataControls: { render: PanelPreview, wide: true }, ProjectNameEditor: { render: PanelPreview },
  ProjectOpenersMenu: { render: PanelPreview }, ProjectOpenersDropdown: { render: PanelPreview }, CustomProjectOpenerDialog: { render: PanelPreview },
  SearchBar: { render: ChromePreview }, TitleBar: { render: ChromePreview }, WindowControls: { render: ChromePreview },
  ProjectGroup: { render: ChromePreview }, ProjectLogo: { render: ChromePreview }, Collapsible: { render: ChromePreview }, YouRunLogo: { render: ChromePreview },
  Launcher: { render: LayoutPreview, wide: true }, WorkspaceFrame: { render: LayoutPreview, wide: true },
  ProjectMenu: { render: MenusPreview }, CommandMenu: { render: MenusPreview },
  AppearancePreviewProvider: { render: AppearancePreview },
};
