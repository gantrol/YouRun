import {
  ArrowDown,
  ArrowUp,
  ChevronRight,
  Minus,
  Eye,
  EyeOff,
  FolderOpen,
  Plus,
  ListChecks,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/renderer/components/ui/collapsible";
import { Input } from "@/renderer/components/ui/input";
import { useMemo, useState, type ReactNode } from "react";
import { FieldRow, Picker } from "./controls";
import { HighlightText, rankByScore, scoreMultiTerm, searchTerms } from "./collections";
import { commandIcons, IconPicker } from "./command-icons";
import {
  commandTitle,
  maxLaunchButtons,
  newLaunchButton,
  type Command,
  type Project,
} from "./model";
import type { Messages } from "./messages";
import { skinOptions, useAppearance } from "./appearance";
import { applyLaunchRecommendations, recommendLaunchButtons } from "./launch-recommendations";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { useViewToggle } from "@/renderer/components/ui/view-state";
import { ExpandableList } from "./expandable-list";

type ManagedItem = {
  id: string;
  name: string;
  description?: string;
  keywords?: string[];
  hidden?: boolean;
};
function ManagementList({
  items,
  m,
  onEdit,
  onToggle,
  onDelete,
  empty,
  stateKey,
}: {
  items: ManagedItem[];
  m: Messages;
  empty: string;
  onEdit: (id: string) => void;
  onToggle: (id: string) => void;
  onDelete: (id: string) => void;
  stateKey: string;
}) {
  const [query, setQuery] = useState("");
  const hasQuery = searchTerms(query).length > 0;
  const filtered = useMemo(() => {
    if (!hasQuery) return items;
    return rankByScore(items, query, (item) =>
      scoreMultiTerm(query, [
        { value: item.name, weight: 4.0, subsequence: true },
        ...(item.keywords ?? []).map((kw) => ({ value: kw, weight: 3.0, subsequence: true })),
        { value: item.description ?? "", weight: 2.0, subsequence: true },
      ]),
    );
  }, [items, query, hasQuery]);

  if (!items.length)
    return <p className="py-4 text-sm text-muted-foreground">{empty}</p>;
  return (
    <div className="space-y-2">
      {(items.length > 4 || hasQuery) && (
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
          <Input
            type="search"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            onKeyDown={(event) => {
              if (!event.nativeEvent.isComposing && event.key === "Escape" && query) {
                event.preventDefault();
                event.stopPropagation();
                setQuery("");
              }
            }}
            aria-label={m.search}
            placeholder={m.search}
            autoComplete="off"
            spellCheck={false}
            className="pl-9 pr-8 [&::-webkit-search-cancel-button]:appearance-none"
          />
          {query && (
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={m.clear}
              title={m.clear}
              onClick={() => setQuery("")}
              className="absolute right-1 top-1/2 -translate-y-1/2"
            >
              <X className="size-4" aria-hidden="true" />
            </Button>
          )}
        </div>
      )}
      {filtered.length > 0 ? (
        <ExpandableList items={filtered} m={m} stateKey={stateKey} renderItem={(item) => {
          const VisibilityIcon = item.hidden ? EyeOff : Eye;
          return (
            <li
              key={item.id}
              className="flex min-h-12 items-center gap-1 border-b border-border last:border-0"
            >
              <Button
                variant="ghost"
                className="h-auto min-h-12 min-w-0 flex-1 justify-start px-2 py-2 text-left font-normal"
                aria-label={`${m.edit} · ${item.name}`}
                title={`${m.edit} · ${item.name}`}
                onClick={() => onEdit(item.id)}
                data-navigation-key={`manage:${item.id}:edit`}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <span className="truncate text-base"><HighlightText text={item.name} query={query} /></span>
                  {item.description && (
                    <span className="hidden min-w-0 truncate font-mono text-xs text-muted-foreground @min-[36rem]/workspace:inline">
                      <HighlightText text={item.description} query={query} />
                    </span>
                  )}
                  {item.hidden && (
                    <span className="text-xs text-muted-foreground">
                      {m.hidden}
                    </span>
                  )}
                </span>
              </Button>

              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`${item.hidden ? m.show : m.hide} · ${item.name}`}
                className="text-muted-foreground"
                title={item.hidden ? m.show : m.hide}
                onClick={() => onToggle(item.id)}
                data-navigation-key={`manage:${item.id}:visibility`}
              >
                <VisibilityIcon className="size-4" aria-hidden="true" />
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label={`${m.remove} · ${item.name}`}
                className="text-muted-foreground hover:text-destructive"
                title={m.remove}
                onClick={() => onDelete(item.id)}
                data-navigation-key={`manage:${item.id}:delete`}
              >
                <Trash2 className="size-4" aria-hidden="true" />
              </Button>
            </li>
          );
        }} />
      ) : (
        <div className="space-y-2 py-3">
          <p role="status" className="text-sm text-muted-foreground">{m.emptyResults}</p>
          <Button variant="outline" size="sm" onClick={() => setQuery("")}>{m.clear}</Button>
        </div>
      )}
    </div>
  );
}

type CommandManagement = {
  onEditCommand: (id: string) => void;
  onToggleCommand: (id: string) => void;
  onDeleteCommand: (id: string) => void;
};
const commandItems = (commands: Command[], m: Messages) =>
  commands.map((command) => ({
    id: command.id,
    name: commandTitle(command, m),
    description: command.text,
    keywords: [...(command.aliases ?? []), command.source ?? "", command.directory].filter(Boolean),
    hidden: command.hidden,
  }));


export function ProjectSettings({
  propertiesOnly = false,
  project,
  commands,
  platform,
  directory,
  m,
  onChange,
  metadata,
  onDelete,
  onNewCommand,
  onImportCommands,
  onEditCommand,
  onToggleCommand,
  onDeleteCommand,
}: {
  propertiesOnly?: boolean;
  project: Project;
  commands: Command[];
  platform: string;
  directory?: string;
  m: Messages;
  onChange: (project: Project) => void;
  metadata: ReactNode;
  onDelete: () => void;
  onNewCommand: () => void;
  onImportCommands: () => void;
} & CommandManagement) {
  const recommendations = recommendLaunchButtons(commands, platform, directory);
  function moveButton(index: number, offset: number) {
    const buttons = [...project.buttons];
    const target = index + offset;
    if (target < 0 || target >= buttons.length) return;
    [buttons[index], buttons[target]] = [buttons[target], buttons[index]];
    onChange({ ...project, buttons });
  }
  return (
    <div className="space-y-6">
      {metadata}
      <section className="space-y-3 border-t border-border pt-4">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-base font-semibold">{m.launchButtons}</h2>
          <div className="flex flex-wrap items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={!recommendations.length}
              title={recommendations.length ? m.recommendedSettings : m.noRecommendedButtons}
              onClick={() => onChange({
                ...project,
                buttons: applyLaunchRecommendations(recommendations, project.buttons),
              })}
            >
              <ListChecks className="size-4" aria-hidden="true" />
              {m.recommendedSettings}
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={project.buttons.length >= maxLaunchButtons}
              title={
                project.buttons.length >= maxLaunchButtons
                  ? m.buttonLimit
                  : m.addButton
              }
              onClick={() => {
                if (project.buttons.length < maxLaunchButtons)
                  onChange({
                    ...project,
                    buttons: [...project.buttons, newLaunchButton()],
                  });
              }}
            >
              <Plus className="size-4" aria-hidden="true" />
              {m.addButton}
            </Button>
          </div>
        </div>
        <ol className="divide-y divide-border">
          {project.buttons.map((button, index) => {
            const boundCommand = commands.find((command) => command.id === button.commandId);
            const VisibilityIcon = button.hidden ? EyeOff : Eye;
            const PreviewIcon = commandIcons[button.icon];
            const change = (patch: Partial<typeof button>) =>
              onChange({
                ...project,
                buttons: project.buttons.map((item) =>
                  item.id === button.id ? { ...item, ...patch } : item,
                ),
              });
            const options: [string, string][] = [
              ["none", m.notAssigned],
              ...commands
                .filter(
                  (command) =>
                    !project.buttons.some(
                      (item) =>
                        item.id !== button.id && item.commandId === command.id,
                    ),
                )
                .map((command): [string, string] => [
                  command.id,
                  `${commandTitle(command, m)} · ${command.source ?? m.manualSource}${command.hidden ? ` · ${m.hidden}` : ""}`,
                ]),
            ];
            return (
              <li key={button.id} className="space-y-3 py-3">
                <div className="flex flex-wrap items-center gap-1">
                  <span
                    aria-hidden="true"
                    className="mr-2 flex size-8 items-center justify-center rounded-md border border-border bg-background text-foreground-secondary"
                  >
                    <PreviewIcon className="size-4" />
                  </span>
                  <span className="mr-auto text-xs tabular-nums text-muted-foreground">
                    {new Intl.NumberFormat().format(index + 1)} /{" "}
                    {new Intl.NumberFormat().format(maxLaunchButtons)}
                    {button.hidden ? ` · ${m.hidden}` : ""}
                  </span>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    disabled={index === 0}
                    aria-label={m.moveEarlier}
                    title={m.moveEarlier}
                    onClick={() => moveButton(index, -1)}
                  >
                    <ArrowUp className="size-4" aria-hidden="true" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    disabled={index === project.buttons.length - 1}
                    aria-label={m.moveLater}
                    title={m.moveLater}
                    onClick={() => moveButton(index, 1)}
                  >
                    <ArrowDown className="size-4" aria-hidden="true" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={button.hidden ? m.show : m.hide}
                    title={button.hidden ? m.show : m.hide}
                    onClick={() => change({ hidden: !button.hidden })}
                  >
                    <VisibilityIcon className="size-4" aria-hidden="true" />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label={m.removeButton}
                    title={m.removeButton}
                    onClick={() =>
                      onChange({
                        ...project,
                        buttons: project.buttons.filter(
                          (item) => item.id !== button.id,
                        ),
                      })
                    }
                  >
                    <Trash2 className="size-4" aria-hidden="true" />
                  </Button>
                </div>
                <Disclosure stateKey={`button:${button.id}`} defaultOpen={!button.commandId}
                  label={boundCommand ? commandTitle(boundCommand, m) : m.notAssigned}>
                <div className="space-y-3">
                <FieldRow label={m.buttonCommand}>
                  <Picker
                    label={m.buttonCommand}
                    value={button.commandId || "none"}
                    options={options}
                    onChange={(value) =>
                      change({ commandId: value === "none" ? "" : value })
                    }
                  />
                </FieldRow>
                <FieldRow label={m.buttonIcon}>
                  <IconPicker
                    m={m}
                    value={button.icon}
                    onChange={(icon) => change({ icon })}
                  />
                </FieldRow>
                </div>
                </Disclosure>
              </li>
            );
          })}
        </ol>
        {project.buttons.length >= maxLaunchButtons && (
          <p role="status" className="text-xs text-muted-foreground">
            {m.buttonLimit}
          </p>
        )}
      </section>
      {!propertiesOnly && <section className="border-t border-border pt-4">
        <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h2 className="text-base font-semibold">{m.allCommands}</h2>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="outline" size="sm" onClick={onImportCommands} data-navigation-key="project:import">
              <Plus className="size-4" aria-hidden="true" />
              {m.fromProject}
            </Button>
            <Button variant="ghost" size="sm" onClick={onNewCommand} data-navigation-key="project:new-command">
              <Plus className="size-4" aria-hidden="true" />
              {m.manualCommand}
            </Button>
          </div>
        </div>
        <ManagementList
          stateKey="project-commands"
          m={m}
          items={commandItems(commands, m)}
          empty={m.noCommands}
          onEdit={onEditCommand}
          onToggle={onToggleCommand}
          onDelete={onDeleteCommand}
        />
      </section>}
      <div className="flex flex-wrap items-center justify-between gap-2 border-t border-border pt-4">
        <Button variant="ghost" size="sm" onClick={() => onChange({ ...project, hidden: !project.hidden })}>
          {project.hidden ? <Eye className="size-4" aria-hidden="true" /> : <EyeOff className="size-4" aria-hidden="true" />}
          {project.hidden ? m.show : m.hide}
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="text-destructive hover:text-destructive"
          onClick={onDelete}
        >
          <Trash2 className="size-4" aria-hidden="true" />
          {m.deleteProject}
        </Button>
      </div>
    </div>
  );
}

function SettingsRow({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div
      role="group"
      aria-label={label}
      className="grid min-h-14 gap-2 py-3 text-sm @min-[24rem]/settings:grid-cols-[minmax(0,1fr)_minmax(0,14rem)] @min-[24rem]/settings:items-center @min-[24rem]/settings:gap-6"
    >
      <span className="text-foreground-secondary">{label}</span>
      <div className="min-w-0 space-y-2">{children}</div>
    </div>
  );
}

function SettingsSection({
  title,
  count,
  action,
  children,
  stateKey,
}: {
  title: string;
  count: string;
  action: ReactNode;
  children: ReactNode;
  stateKey: string;
}) {
  const [open, setOpen] = useViewToggle(stateKey, false);
  return (
    <Collapsible open={open} onOpenChange={setOpen} asChild>
      <section className="py-3">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="min-w-0 flex-1">
            <CollapsibleTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-auto min-h-9 w-full justify-start px-1 text-left text-sm font-semibold whitespace-normal"
                data-navigation-key={`settings:${stateKey}`}
              >
                {open ? <Minus className="size-4 text-muted-foreground" aria-hidden="true"/> : <Plus className="size-4 text-muted-foreground" aria-hidden="true"/>}
                <span className="min-w-0 break-words">{title}</span>
                <span className="shrink-0 text-xs font-normal tabular-nums text-muted-foreground">{count}</span>
              </Button>
            </CollapsibleTrigger>
          </h2>
          {action}
        </div>
        <CollapsibleContent>
          <div className="pt-2">{children}</div>
        </CollapsibleContent>
      </section>
    </Collapsible>
  );
}

export function GlobalSettings({
  startup,
  agentIntegration,
  onUpdates,
  updateReady,
  projects,
  commands,
  m,
  locale,
  onLocale,
  onOpenProject,
  onNewCommand,
  onProject,
  onToggleProject,
  onDeleteProject,
  onEditCommand,
  onToggleCommand,
  onDeleteCommand,
}: {
  startup?: ReactNode;
  agentIntegration?: ReactNode;
  onUpdates?: () => void;
  updateReady: boolean;
  projects: Project[];
  commands: Command[];
  m: Messages;
  locale: string;
  onLocale: (value: string) => void;
  onOpenProject: () => void;
  onNewCommand: () => void;
  onProject: (id: string) => void;
  onToggleProject: (id: string) => void;
  onDeleteProject: (id: string) => void;
} & CommandManagement) {
  const appearance = useAppearance();
  const independentCommands = commands.filter((command) => !command.project);
  const numberFormat = new Intl.NumberFormat(locale);
  return (
    <div className="@container/settings min-w-0 space-y-6">
      <div className="divide-y divide-border">
        <SettingsRow label={m.skin}>
          <fieldset
            disabled={appearance.saving}
            className="min-w-0"
            aria-busy={appearance.saving}
          >
            <Picker
              label={m.skin}
              value={appearance.skin}
              options={skinOptions.map(({ id, label }) => [id, m[label]])}
              onChange={appearance.change}
            />
          </fieldset>
          {appearance.saving && (
            <span role="status" className="block text-xs text-muted-foreground">
              {m.saving}
            </span>
          )}
          {appearance.error && (
            <span role="alert" className="block text-xs text-destructive">
              {m[appearance.error as keyof Messages]}
            </span>
          )}
        </SettingsRow>
        <SettingsRow label={m.language}>
          <Picker
            label={m.language}
            value={locale}
            options={[["zh", "简体中文"], ["en", "English"]]}
            onChange={onLocale}
          />
        </SettingsRow>
        {startup && <SettingsRow label={m.startupAtLogin}>{startup}</SettingsRow>}
        {onUpdates && <div className="py-1">
          <Button
            variant="ghost"
            onClick={onUpdates}
            data-navigation-key="settings:updates"
            className="h-auto min-h-12 w-full justify-between gap-3 px-0 py-2 text-left font-normal whitespace-normal"
          >
            <span className="min-w-0 text-foreground-secondary">{m.updateTitle}</span>
            <span className="flex min-w-0 items-center gap-2">
              {updateReady && <span className="text-xs text-success">{m.updateReady}</span>}
              <ChevronRight className="size-4 text-muted-foreground" aria-hidden="true" />
            </span>
          </Button>
        </div>}
      </div>
      <div className="divide-y divide-border border-t border-border">
        {agentIntegration}
        <SettingsSection
          stateKey="projects-section"
          title={m.projects}
          count={numberFormat.format(projects.length)}
          action={
            <Button variant="ghost" size="sm" onClick={onOpenProject} data-navigation-key="settings:open-project">
              <FolderOpen className="size-4" aria-hidden="true" />
              {m.openProject}
            </Button>
          }
        >
          <ManagementList
            stateKey="projects-list"
            m={m}
            items={projects}
            empty={m.noProjects}
            onEdit={onProject}
            onToggle={onToggleProject}
            onDelete={onDeleteProject}
          />
        </SettingsSection>
        <SettingsSection
          stateKey="independent-section"
          title={m.independentCommands}
          count={numberFormat.format(independentCommands.length)}
          action={
            <Button variant="ghost" size="sm" onClick={onNewCommand} data-navigation-key="settings:new-command">
              <Plus className="size-4" aria-hidden="true" />
              {m.newCommand}
            </Button>
          }
        >
          <ManagementList
            stateKey="independent-list"
            m={m}
            items={commandItems(independentCommands, m)}
            empty={m.noCommands}
            onEdit={onEditCommand}
            onToggle={onToggleCommand}
            onDelete={onDeleteCommand}
          />
        </SettingsSection>
      </div>
    </div>
  );
}

export function DeletePanel({
  name, collections = [],
  project,
  m,
  onCancel,
  onDelete,
}: {
  name: string; collections?: string[];
  project: boolean;
  m: Messages;
  onCancel: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="space-y-4">
      <p className="break-words text-base font-medium">{name}</p>
      <p className="text-sm text-muted-foreground">
        {project ? m.deleteProjectEffect : m.deleteCommandEffect}
      </p>
      {collections.length > 0 && <section aria-label={m.affectedCollections} className="space-y-2 text-sm">
        <h3 className="text-muted-foreground">{m.affectedCollections}</h3>
        <ul className="max-h-40 overflow-auto">{collections.map((name, index) => <li key={index} className="truncate" title={name}>{name}</li>)}</ul>
      </section>}
      <div className="flex gap-2">
        <Button autoFocus variant="outline" size="sm" onClick={onCancel}>
          {m.cancel}
        </Button>
        <Button variant="destructive" size="sm" onClick={onDelete}>
          {m.remove}
        </Button>
      </div>
    </div>
  );
}
