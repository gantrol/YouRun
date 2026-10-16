import { useState } from "react";
import { Button } from "@/renderer/components/ui/button";
import { ContextMenu, ContextMenuContent, ContextMenuTrigger } from "@/renderer/components/ui/context-menu";
import { ProjectSettings, GlobalSettings, DeletePanel } from "../settings-panels";
import { ProjectNameEditor } from "../panels";
import { ProjectMetadataControls } from "../project-metadata-controls";
import { CustomProjectOpenerDialog, ProjectOpenersDropdown, ProjectOpenersMenu } from "../project-openers";
import type { PreviewProps } from "./component-previews";
import { sampleCommands, sampleProjects } from "./samples";

export function PanelPreview({ m, only, locale, onLocale }: PreviewProps) {
  const [projects, setProjects] = useState(sampleProjects);
  const [commands, setCommands] = useState(() => [...sampleCommands, { ...sampleCommands[6], id: "independent-check", project: null }]);
  const [project, setProject] = useState(sampleProjects[2]);
  const [result, setResult] = useState("");
  const [dirty, setDirty] = useState(false);
  const [custom, setCustom] = useState(false);
  const [deleted, setDeleted] = useState(false);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const report = (value: string) => setResult(value);
  const openers = {
    platform: "windows",
    items: [{ id: "editor", name: "Visual Studio Code" }, { id: "powershell", name: "PowerShell" }],
    loading: false, error: "", refresh: () => report(m.refresh),
  };
  const openerProps = { openers, m, disabled: false, onOpen: report, onCustom: () => setCustom(true), onOpenDirectory: () => report(m.openProjectDirectory) };
  const commandActions = {
    onEditCommand: (id: string) => report(`${m.editCommand} · ${id}`),
    onToggleCommand: (id: string) => setCommands((items) => items.map((item) => item.id === id ? { ...item, hidden: !item.hidden } : item)),
    onDeleteCommand: (id: string) => setCommands((items) => items.filter((item) => item.id !== id)),
  };
  const metadata = <ProjectMetadataControls project={project} details={{ directory: "/workspace/yourun", logo: null, names: [{ name: "YouRun", source: "package.json" }] }}
    m={m} busy={false} savingLogo={false} onDirty={setDirty} onSaveName={async (name) => setProject((item) => ({ ...item, name }))}
    onSelectLogo={() => report(m.projectIcon)} onChooseAvatar={() => report(m.chooseProjectAvatar)} onRescan={() => report(m.refreshProjectLogo)}
    onRetry={() => report(m.retry)} onCopyDirectory={() => report(m.copyProjectPath)} onOpenDirectory={openerProps.onOpenDirectory}
    openers={openers} onOpenWith={report} onCustomEditor={openerProps.onCustom} />;
  return <div className="space-y-5">
    {only === "ProjectMetadataControls" && metadata}
    {only === "ProjectSettings" && <ProjectSettings project={project} commands={commands.filter((item) => item.project === project.id)} m={m}
      platform="windows" directory="/workspace/yourun" metadata={metadata} onChange={setProject}
      onDelete={() => report(m.deleteProject)} onNewCommand={() => report(m.newCommand)} onImportCommands={() => report(m.addProjectCommands)} {...commandActions} />}
    {only === "GlobalSettings" && <GlobalSettings m={m} locale={locale} onLocale={onLocale} projects={projects} commands={commands} updateReady={false}
      onUpdates={() => report(m.updateTitle)} onOpenProject={() => report(m.openProject)} onNewCommand={() => report(m.newCommand)}
      onProject={(id) => report(`${m.projectSettings} · ${id}`)}
      onToggleProject={(id) => setProjects((items) => items.map((item) => item.id === id ? { ...item, hidden: !item.hidden } : item))}
      onDeleteProject={(id) => setProjects((items) => items.filter((item) => item.id !== id))} {...commandActions} />}
    {only === "ProjectNameEditor" && (editing ? <ProjectNameEditor m={m} initialName={project.name} names={[{ name: "command-manager", source: "go.mod" }]}
      onDirty={setDirty} onSave={async (name) => { setProject((item) => ({ ...item, name })); report(m.saved); }} />
      : <Button variant="outline" onClick={() => setEditing(true)}>{m.edit} · {project.name}</Button>)}
    {only === "DeletePanel" && (confirming && !deleted ? <DeletePanel name="YouRun" project m={m}
      onCancel={() => { setConfirming(false); report(m.cancel); }} onDelete={() => { setDeleted(true); report(m.remove); }} />
      : <Button variant="outline" onClick={() => { setDeleted(false); setConfirming(true); }}>{m.deleteProject}</Button>)}
    {only === "ProjectOpenersDropdown" && <ProjectOpenersDropdown {...openerProps} />}
    {only === "ProjectOpenersMenu" && <ContextMenu><ContextMenuTrigger asChild><Button variant="outline">{m.openProjectWith}</Button></ContextMenuTrigger>
      <ContextMenuContent><ProjectOpenersMenu {...openerProps} /></ContextMenuContent></ContextMenu>}
    {only === "CustomProjectOpenerDialog" && <Button variant="outline" onClick={() => setCustom(true)}>{m.otherProjectEditor}</Button>}
    {custom && <CustomProjectOpenerDialog m={m} onClose={() => setCustom(false)} onOpen={async (path) => report(path)} />}
    {dirty && <span className="text-xs text-muted-foreground">{m.unsaved}</span>}
    {result && <output className="block text-xs text-muted-foreground">{result}</output>}
  </div>;
}
