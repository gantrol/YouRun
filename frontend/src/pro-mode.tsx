import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AppWindow,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  Bell,
  Check,
  ChevronDown,
  ChevronRight,
  Copy,
  FileSearch,
  FileText,
  FolderOpen,
  ListFilter,
  Plus,
  RefreshCw,
  Search,
  Trash2,
  X,
} from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Input } from "@/renderer/components/ui/input";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/renderer/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/renderer/components/ui/dropdown-menu";
import { api } from "./api";
import {
  ResourceActionsProvider,
  ResourceActions,
  ResourceBatchActions,
  ResourceMenu,
  ResourceRow,
  ResourceSelectAll,
  type ResourceProcess,
} from "./resource-actions";
import type { Command, Project, Run, SourceSnapshot } from "./model";
import type { RunOperations } from "./models/run-operation";
import type { ProjectDetails } from "./project-details";
import type { Messages } from "./messages";
import { formatSourceSummary } from "./project-workspaces";
import { YouRunLogo } from "./yourun-logo";

export type ProSubview = "home" | "processes" | "ports" | "files";

export type BountyBoardMode = "live" | "peaks" | "watch";

export type BountyGroupSummary = {
  groupKey: string;
  windowCpu: number | null;
  windowMemory: number;
  windowGpu: number | null;
  windowGpuMemory: number;
  windowNetwork: number | null;
  cumulativeNetBytes: number;
  cpuDelta?: number;
  memoryDeltaBytes?: number;
  gpuDelta?: number;
  networkDeltaBps?: number;
  risingCategories?: string[];
  watched?: boolean;
  watchTriggered?: boolean;
};

export type BountyEvent = {
  id: string;
  groupKey: string;
  category: "cpu" | "memory" | "gpu" | "network";
  reasonKind: "rapid_rise" | "peak_high" | "watch_triggered" | string;
  watchId?: string;
  title: string;
  processName: string;
  icon?: string;
  pid: number;
  processIdentity?: string;
  controlRunId?: string;
  executable?: string;
  commandLine?: string;
  commandLineState?: string;
  ownership: string;
  runId?: string;
  runStatus?: string;
  commandId?: string;
  commandTitle?: string;
  projectId?: string;
  projectName?: string;
  workspaceId?: string;
  workspaceName?: string;
  directory?: string;
  ports?: number[];
  peakValue: number;
  baselineValue?: number;
  deltaValue?: number;
  cumulativeBytes?: number;
  memoryBytes?: number;
  cpuPercent?: number | null;
  gpuPercent?: number | null;
  gpuMemoryBytes?: number;
  networkBytesPerSec?: number | null;
  networkConnections?: number;
  firstObservedAt: string;
  peakAt: string;
  lastObservedAt: string;
  durationSec: number;
  processExited?: boolean;
  acknowledged?: boolean;
};

export type BountyWatch = {
  id: string;
  title: string;
  matchKind: "process" | "command" | string;
  targetName: string;
  executable?: string;
  commandId?: string;
  projectId?: string;
  projectName?: string;
  category: "network" | "cpu" | "memory" | "gpu";
  conditionKind: "threshold" | "rapid_rise" | "cumulative";
  threshold: number;
  notifyToast: boolean;
  enabled: boolean;
  note?: string;
  createdAt: string;
  hitCount: number;
  lastTriggeredAt?: string;
  lastPeakValue?: number;
  lastPid?: number;
  unreadHit?: boolean;
};

export type BountySnapshot = {
  groups: Record<string, BountyGroupSummary>;
  events: BountyEvent[];
  watches: BountyWatch[];
  unreadAlertCount: number;
};

export type ProcessItem = {
  processIdentity?: string;
  controlRunId?: string;
  pid: number;
  ppid: number;
  name: string;
  displayName?: string;
  icon?: string;
  executable?: string;
  commandLine?: string;
  commandLineState: "ok" | "denied" | "unavailable" | string;
  user?: string;
  status: string;
  startedAt?: string;
  cpuPercent: number | null;
  cpuSampling?: boolean;
  memoryBytes: number;
  gpuPercent?: number | null;
  gpuSampling?: boolean;
  gpuMemoryBytes?: number;
  networkBytesPerSec?: number | null;
  networkSampling?: boolean;
  networkConnections?: number;
  ports?: number[];
  ownership: "managed" | "external" | "unconfirmed" | string;
  runId?: string;
  runStatus?: string;
  commandId?: string;
  commandTitle?: string;
  projectId?: string;
  projectName?: string;
  workspaceId?: string;
  workspaceName?: string;
  directory?: string;
  launchSource?: SourceSnapshot;
};

export type PortItem = {
  processIdentity?: string;
  controlRunId?: string;
  id: string;
  protocol: string;
  ipVersion: string;
  protocolLabel: string;
  localAddress: string;
  ip: string;
  port: number;
  remoteAddress?: string;
  state: string;
  pid: number;
  processName: string;
  processDisplayName?: string;
  processIcon?: string;
  executable?: string;
  commandLine?: string;
  commandLineState: string;
  user?: string;
  processExited?: boolean;
  ownership: "managed" | "external" | "unconfirmed" | string;
  runId?: string;
  runStatus?: string;
  commandId?: string;
  commandTitle?: string;
  projectId?: string;
  projectName?: string;
  workspaceId?: string;
  workspaceName?: string;
  directory?: string;
  launchSource?: SourceSnapshot;
};

export type FileHandleItem = {
  processIdentity?: string;
  controlRunId?: string;
  id: string;
  pid: number;
  processName: string;
  processDisplayName?: string;
  processIcon?: string;
  user?: string;
  executable?: string;
  commandLine?: string;
  commandLineState: string;
  referenceType: string;
  blockingDelete: string;
  targetFile: string;
  processExited?: boolean;
  ownership: "managed" | "external" | "unconfirmed" | string;
  runId?: string;
  runStatus?: string;
  commandId?: string;
  commandTitle?: string;
  projectId?: string;
  projectName?: string;
  workspaceId?: string;
  workspaceName?: string;
  directory?: string;
  launchSource?: SourceSnapshot;
};

export type ResourceOriginContext = {
  subview: ProSubview;
  label: string;
  runId: string;
  runTitle: string;
  commandId?: string;
  projectId?: string;
  projectName?: string;
  workspaceId?: string;
  workspaceName?: string;
  portQuery?: string;
  pid?: number;
  targetPath?: string;
  reviewStatus: "active" | "stale" | "released";
};

export type ProcessSortBy = "default" | "cpu" | "memory" | "gpu" | "network";

export type ProModeState = {
  subview: ProSubview;
  processes: {
    query: string;
    project: string;
    workspace: string;
    ownership: string;
    sortBy: ProcessSortBy;
    sortDesc: boolean;
    selectedPid?: number;
  };
  ports: {
    query: string;
    protocol: string;
    state: string;
    selectedId?: string;
  };
  files: {
    targetPath: string;
    isDirectory: boolean;
    recursive: boolean;
    selectedId?: string;
  };
  origin?: ResourceOriginContext;
};

export function createInitialProModeState(): ProModeState {
  return {
    subview: "home",
    processes: {
      query: "",
      project: "all",
      workspace: "all",
      ownership: "all",
      sortBy: "default",
      sortDesc: true,
    },
    ports: {
      query: "",
      protocol: "all",
      state: "all",
    },
    files: {
      targetPath: "",
      isDirectory: false,
      recursive: false,
    },
  };
}

function formatMemory(bytes?: number): string {
  if (!bytes || bytes <= 0) return "--";
  const mb = bytes / (1024 * 1024);
  if (mb >= 1024) {
    return `${(mb / 1024).toFixed(1)} GB`;
  }
  if (mb < 1) {
    return `${Math.max(0.1, mb).toFixed(1)} MB`;
  }
  return `${Math.round(mb).toLocaleString()} MB`;
}

function formatNetworkRate(bytesPerSec: number | null | undefined, sampling?: boolean, m?: Messages): string {
  if (sampling && m) return m.cpuSampling;
  if (bytesPerSec == null || Number.isNaN(bytesPerSec)) return "--";
  if (bytesPerSec <= 0) return "0 B/s";
  if (bytesPerSec < 1024) {
    return `${Math.round(bytesPerSec)} B/s`;
  }
  const kb = bytesPerSec / 1024;
  if (kb < 1024) {
    return `${kb >= 100 ? Math.round(kb) : kb.toFixed(1)} KB/s`;
  }
  const mb = kb / 1024;
  if (mb < 1024) {
    return `${mb >= 100 ? Math.round(mb) : mb.toFixed(1)} MB/s`;
  }
  return `${(mb / 1024).toFixed(1)} GB/s`;
}

function BountyRabbitBadge({
  rank,
  m,
  selected = false,
  iconOnly = false,
}: {
  rank: 1 | 2 | 3;
  m: Messages;
  selected?: boolean;
  iconOnly?: boolean;
}) {
  const meta =
    rank === 1
      ? {
          label: m.goldRabbit,
          fill: "#d49a15",
          ink: "#593b04",
          cls: selected
            ? "border-selected-foreground/35 bg-selected-foreground/15 text-selected-foreground"
            : "border-[#d8b04c]/75 bg-[#fbf3dc] text-[#7a5308] dark:border-[#b8891e]/60 dark:bg-[#2b210a] dark:text-[#f0c451]",
        }
      : rank === 2
        ? {
            label: m.silverRabbit,
            fill: "#7d8790",
            ink: "#333a40",
            cls: selected
              ? "border-selected-foreground/35 bg-selected-foreground/15 text-selected-foreground"
              : "border-[#b4bcc4]/80 bg-[#eef0f2] text-[#464e56] dark:border-[#5b636b]/60 dark:bg-[#1e2226] dark:text-[#c8cfd6]",
          }
        : {
            label: m.bronzeRabbit,
            fill: "#b86838",
            ink: "#54290e",
            cls: selected
              ? "border-selected-foreground/35 bg-selected-foreground/15 text-selected-foreground"
              : "border-[#cf9672]/75 bg-[#f7ebe2] text-[#7c4220] dark:border-[#94532c]/60 dark:bg-[#29180e] dark:text-[#e49d73]",
          };

  return (
    <span
      title={iconOnly ? meta.label : undefined}
      aria-label={iconOnly ? meta.label : undefined}
      className={`inline-flex h-5 shrink-0 items-center border text-[11px] font-medium leading-none ${
        iconOnly ? "w-5 justify-center" : "gap-1 px-1.5"
      } ${meta.cls}`}
      style={
        {
          "--yourun-logo-fill": meta.fill,
          "--yourun-logo-ink": meta.ink,
        } as React.CSSProperties
      }
    >
      <YouRunLogo className="size-3.5 shrink-0" />
      {!iconOnly && <span>{meta.label}</span>}
    </span>
  );
}

function formatObservedTime(iso: string | undefined, locale: string): string {
  if (!iso) return "--";
  try {
    return new Intl.DateTimeFormat(locale, {
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    }).format(new Date(iso));
  } catch {
    return "--";
  }
}

function formatWorkspaceName(name: string | undefined, m: Messages): string {
  if (!name) return "";
  return name === "主工作区" ? m.mainWorkspace : name;
}

function friendlyProcessLabel(name?: string, displayName?: string): string {
  const trimmedDisplay = displayName?.trim();
  if (trimmedDisplay) return trimmedDisplay;
  return name?.trim() || "--";
}

function ProcessIcon({
  name,
  displayName,
  icon,
  size = "sm",
}: {
  name?: string;
  displayName?: string;
  icon?: string;
  size?: "sm" | "md";
}) {
  const [imgFailed, setImgFailed] = useState(false);
  const dimClass = size === "md" ? "size-5" : "size-4";
  if (icon && !imgFailed) {
    return (
      <img
        src={icon}
        alt=""
        className={`${dimClass} shrink-0 select-none object-contain`}
        onError={() => setImgFailed(true)}
      />
    );
  }
  const lowerName = (name ?? "").toLowerCase();
  const lowerDisplay = (displayName ?? "").toLowerCase();
  if (lowerName.startsWith("yourun") || lowerDisplay.includes("yourun")) {
    return <YouRunLogo className={`${dimClass} shrink-0`} />;
  }
  return (
    <span
      className={`inline-flex ${dimClass} shrink-0 items-center justify-center rounded-[2px] border border-border/80 bg-surface-sunken text-muted-foreground`}
      aria-hidden="true"
    >
      <AppWindow className={size === "md" ? "size-3.5" : "size-3"} />
    </span>
  );
}

function ownershipBadge(ownership: string, m: Messages) {
  if (ownership === "managed") {
    return (
      <span className="inline-flex items-center border border-success/30 bg-success-surface px-1.5 py-0.5 text-[11px] font-medium text-success">
        {m.ownershipManaged}
      </span>
    );
  }
  if (ownership === "unconfirmed") {
    return (
      <span className="inline-flex items-center border border-warning/30 bg-warning-surface px-1.5 py-0.5 text-[11px] font-medium text-warning-foreground">
        {m.ownershipUnconfirmed}
      </span>
    );
  }
  return (
    <span className="inline-flex items-center border border-border bg-surface-sunken px-1.5 py-0.5 text-[11px] text-muted-foreground">
      {m.ownershipExternal}
    </span>
  );
}

function portStateLabel(state: string, m: Messages): string {
  const upper = state.toUpperCase();
  if (upper === "LISTEN" || upper === "LISTENING") return m.stateListening;
  if (upper === "BOUND") return m.stateBound;
  if (upper === "ESTABLISHED") return m.stateEstablished;
  return state;
}

type ProcessGroup = {
  key: string;
  title: string;
  icon?: string;
  primary: ProcessItem;
  items: ProcessItem[];
  totalCpu: number | null;
  cpuSampling: boolean;
  totalMemory: number;
  totalGpu: number | null;
  gpuSampling: boolean;
  totalGpuMemory: number;
  totalNetwork: number | null;
  networkSampling: boolean;
  totalNetworkConnections: number;
  windowCpu: number | null;
  windowMemory: number;
  windowGpu: number | null;
  windowGpuMemory: number;
  windowNetwork: number | null;
  cumulativeNetBytes: number;
  cpuDelta: number;
  memoryDeltaBytes: number;
  gpuDelta: number;
  networkDeltaBps: number;
  risingCategories: string[];
  watched: boolean;
  watchTriggered: boolean;
  ownership: string;
  projectName?: string;
  workspaceName?: string;
};

const BOUNTY_WINDOW_MS = 30_000;
const BOUNTY_POLL_INTERVAL_MS = 2_000;

function getProcessGroupKey(item: ProcessItem): string {
  const label = friendlyProcessLabel(item.name, item.displayName);
  return item.ownership === "managed"
    ? `managed:${item.runId || item.pid}:${label.toLowerCase()}`
    : `ext:${label.toLowerCase()}`;
}

function defaultThresholdFor(
  category: "network" | "cpu" | "memory" | "gpu",
  conditionKind: "threshold" | "rapid_rise" | "cumulative",
): number {
  if (category === "network") {
    if (conditionKind === "cumulative") return 100 * 1024 * 1024; // 100 MB
    if (conditionKind === "rapid_rise") return 2 * 1024 * 1024; // +2 MB/s
    return 5 * 1024 * 1024; // 5 MB/s
  }
  if (category === "memory") {
    if (conditionKind === "rapid_rise") return 256 * 1024 * 1024; // +256 MB
    return 1024 * 1024 * 1024; // 1 GB
  }
  if (conditionKind === "rapid_rise") return 30; // +30%
  return 50; // 50%
}

function thresholdPresetsFor(
  category: "network" | "cpu" | "memory" | "gpu",
  conditionKind: "threshold" | "rapid_rise" | "cumulative",
  m: Messages,
): Array<{ value: number; label: string }> {
  if (category === "network") {
    if (conditionKind === "cumulative") {
      return [
        { value: 50 * 1024 * 1024, label: "50 MB" },
        { value: 100 * 1024 * 1024, label: "100 MB" },
        { value: 500 * 1024 * 1024, label: "500 MB" },
        { value: 2 * 1024 * 1024 * 1024, label: "2 GB" },
      ];
    }
    if (conditionKind === "rapid_rise") {
      return [
        { value: 1 * 1024 * 1024, label: `+1 ${m.networkUnitMBps}` },
        { value: 2 * 1024 * 1024, label: `+2 ${m.networkUnitMBps}` },
        { value: 5 * 1024 * 1024, label: `+5 ${m.networkUnitMBps}` },
        { value: 10 * 1024 * 1024, label: `+10 ${m.networkUnitMBps}` },
      ];
    }
    return [
      { value: 1 * 1024 * 1024, label: `1 ${m.networkUnitMBps}` },
      { value: 5 * 1024 * 1024, label: `5 ${m.networkUnitMBps}` },
      { value: 10 * 1024 * 1024, label: `10 ${m.networkUnitMBps}` },
      { value: 25 * 1024 * 1024, label: `25 ${m.networkUnitMBps}` },
    ];
  }
  if (category === "memory") {
    if (conditionKind === "rapid_rise") {
      return [
        { value: 128 * 1024 * 1024, label: "+128 MB" },
        { value: 256 * 1024 * 1024, label: "+256 MB" },
        { value: 512 * 1024 * 1024, label: "+512 MB" },
        { value: 1024 * 1024 * 1024, label: "+1.0 GB" },
      ];
    }
    return [
      { value: 512 * 1024 * 1024, label: "512 MB" },
      { value: 1024 * 1024 * 1024, label: "1.0 GB" },
      { value: 2 * 1024 * 1024 * 1024, label: "2.0 GB" },
      { value: 4 * 1024 * 1024 * 1024, label: "4.0 GB" },
    ];
  }
  if (conditionKind === "rapid_rise") {
    return [
      { value: 20, label: "+20%" },
      { value: 30, label: "+30%" },
      { value: 50, label: "+50%" },
    ];
  }
  return [
    { value: 30, label: "30%" },
    { value: 50, label: "50%" },
    { value: 80, label: "80%" },
    { value: 90, label: "90%" },
  ];
}

function formatWatchRuleSummary(w: BountyWatch, m: Messages): string {
  const catLabel =
    w.category === "cpu"
      ? m.colCpu
      : w.category === "memory"
        ? m.colMemory
        : w.category === "gpu"
          ? m.colGpu
          : m.colNetwork;
  const valStr =
    w.category === "network"
      ? w.conditionKind === "cumulative"
        ? formatMemory(w.threshold)
        : formatNetworkRate(w.threshold, false, m)
      : w.category === "memory"
        ? formatMemory(w.threshold)
        : `${w.threshold.toFixed(0)}%`;
  if (w.conditionKind === "rapid_rise") {
    return `${catLabel} · ${m.bountyConditionRapidRise} +${valStr}`;
  }
  if (w.conditionKind === "cumulative") {
    return `${catLabel} · ${m.bountyConditionCumulative} ${valStr}`;
  }
  return `${catLabel} · > ${valStr}`;
}

export function ProModeWorkspace({
  state,
  onStateChange,
  projects,
  projectDetails,
  runs,
  commands,
  runOperations,
  launchErrors,
  locale,
  m,
  onOpenRun,
  onOpenCommand,
  onReplay,
  onStop,
  onOpenWorkspace,
  onCopy,
  onOpenDirectoryPath,
  recheckTrigger,
}: {
  state: ProModeState;
  onStateChange: (updater: (prev: ProModeState) => ProModeState) => void;
  projects: Project[];
  projectDetails: Record<string, ProjectDetails | undefined>;
  runs: Run[];
  commands: Command[];
  runOperations: RunOperations;
  launchErrors: Record<string, string>;
  locale: string;
  m: Messages;
  onOpenRun: (runId: string, origin?: ResourceOriginContext) => void;
  onOpenCommand: (commandId: string, origin: ResourceOriginContext) => void;
  onReplay: (id: string) => Promise<void>;
  onStop: (id: string) => Promise<void>;
  onOpenWorkspace: (projectId: string, workspaceId?: string) => void;
  onCopy: (text: string) => void;
  onOpenDirectoryPath: (dirPath: string) => void;
  recheckTrigger?: number;
}) {
  const [procData, setProcData] = useState<{
    items: ProcessItem[];
    observedAt?: string;
    coverage?: string;
    unreadableProcessCount?: number;
  }>({ items: [] });
  const [bountySamples, setBountySamples] = useState<
    Array<{ at: number; items: ProcessItem[] }>
  >([]);
  const [bountyMode, setBountyMode] = useState<BountyBoardMode>("live");
  const [bountyData, setBountyData] = useState<BountySnapshot>({
    groups: {},
    events: [],
    watches: [],
    unreadAlertCount: 0,
  });
  const [selectedPeakEventId, setSelectedPeakEventId] = useState<string | undefined>(undefined);
  const [watchDialogOpen, setWatchDialogOpen] = useState(false);
  const [watchDraft, setWatchDraft] = useState<{
    title: string;
    matchKind: "process" | "command";
    targetName: string;
    executable?: string;
    commandId?: string;
    projectId?: string;
    projectName?: string;
    category: "network" | "cpu" | "memory" | "gpu";
    conditionKind: "threshold" | "rapid_rise" | "cumulative";
    threshold: number;
    notifyToast: boolean;
  }>({
    title: "",
    matchKind: "process",
    targetName: "",
    category: "network",
    conditionKind: "threshold",
    threshold: 5 * 1024 * 1024,
    notifyToast: false,
  });
  const notifiedWatchRef = useRef<Record<string, string>>({});

  const [portData, setPortData] = useState<{
    items: PortItem[];
    observedAt?: string;
    coverage?: string;
    unreadableProcessCount?: number;
  }>({ items: [] });
  const [fileData, setFileData] = useState<{
    items: FileHandleItem[];
    observedAt?: string;
    coverage?: string;
    unreadableProcessCount?: number;
    queriedPath?: string;
  }>({ items: [] });

  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [narrowDetailOpen, setNarrowDetailOpen] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState<Record<string, boolean>>({});
  const pickingRef = useRef(false);
  const procFetchingRef = useRef(false);

  const toggleGroup = useCallback((groupKey: string) => {
    setExpandedGroups((prev) => ({ ...prev, [groupKey]: !prev[groupKey] }));
  }, []);

  const applyBountySnapshot = useCallback(
    (snap?: BountySnapshot) => {
      if (!snap) return;
      setBountyData({
        groups: snap.groups ?? {},
        events: snap.events ?? [],
        watches: snap.watches ?? [],
        unreadAlertCount: snap.unreadAlertCount ?? 0,
      });
      if (typeof Notification !== "undefined" && Notification.permission === "granted") {
        for (const w of snap.watches ?? []) {
          if (
            w.enabled &&
            w.notifyToast &&
            w.unreadHit &&
            w.lastTriggeredAt &&
            notifiedWatchRef.current[w.id] !== w.lastTriggeredAt
          ) {
            notifiedWatchRef.current[w.id] = w.lastTriggeredAt;
            try {
              new Notification(
                m.bountyAlertBanner.replace("%s", w.title || w.targetName),
                {
                  body: formatWatchRuleSummary(w, m),
                },
              );
            } catch {
              // ignore notification error in restricted webview
            }
          }
        }
      }
    },
    [m],
  );

  const loadProcesses = useCallback(
    async (options?: { silent?: boolean }) => {
      if (procFetchingRef.current) return;
      procFetchingRef.current = true;
      const silent = options?.silent === true;
      if (!silent) {
        setLoading(true);
        setError("");
      }
      try {
        const res = await api<{
          items: ProcessItem[];
          observedAt: string;
          coverage?: string;
          unreadableProcessCount?: number;
          bounty?: BountySnapshot;
        }>("resources/processes");
        const items = res.items ?? [];
        const now = Date.now();
        setProcData({
          items,
          observedAt: res.observedAt,
          coverage: res.coverage,
          unreadableProcessCount: res.unreadableProcessCount,
        });
        if (res.bounty) {
          applyBountySnapshot(res.bounty);
        }
        setBountySamples((prev) => {
          const cutoff = now - BOUNTY_WINDOW_MS;
          const kept = prev.filter((s) => s.at >= cutoff);
          return [...kept, { at: now, items }];
        });
      } catch {
        if (!silent) {
          setError(m.requestFailed);
        }
      } finally {
        procFetchingRef.current = false;
        if (!silent) {
          setLoading(false);
        }
      }
    },
    [m.requestFailed, applyBountySnapshot],
  );

  const openWatchDialogFor = useCallback(
    (target?: {
      title?: string;
      name?: string;
      executable?: string;
      commandId?: string;
      projectId?: string;
      projectName?: string;
      defaultCategory?: "network" | "cpu" | "memory" | "gpu";
    }) => {
      const cat = target?.defaultCategory ?? "network";
      const label = target?.title || (target?.name ? friendlyProcessLabel(target.name, target.title) : "");
      setWatchDraft({
        title: label,
        matchKind: target?.commandId ? "command" : "process",
        targetName: label || target?.name || "",
        executable: target?.executable,
        commandId: target?.commandId,
        projectId: target?.projectId,
        projectName: target?.projectName,
        category: cat,
        conditionKind: "threshold",
        threshold: defaultThresholdFor(cat, "threshold"),
        notifyToast: false,
      });
      setWatchDialogOpen(true);
    },
    [],
  );

  const submitWatchDraft = useCallback(async () => {
    const targetName = watchDraft.targetName.trim();
    if (!targetName) return;
    try {
      const snap = await api<BountySnapshot>("resources/bounties", "POST", {
        action: "create_watch",
        title: watchDraft.title.trim() || targetName,
        matchKind: watchDraft.matchKind,
        targetName,
        executable: watchDraft.executable,
        commandId: watchDraft.commandId,
        projectId: watchDraft.projectId,
        projectName: watchDraft.projectName,
        category: watchDraft.category,
        conditionKind: watchDraft.conditionKind,
        threshold: watchDraft.threshold,
        notifyToast: watchDraft.notifyToast,
      });
      applyBountySnapshot(snap);
      setWatchDialogOpen(false);
    } catch {
      setError(m.requestFailed);
    }
  }, [watchDraft, applyBountySnapshot, m.requestFailed]);

  const handleToggleWatch = useCallback(
    async (id: string, enabled: boolean) => {
      try {
        const snap = await api<BountySnapshot>("resources/bounties", "POST", {
          action: "toggle_watch",
          id,
          enabled,
        });
        applyBountySnapshot(snap);
      } catch {
        setError(m.requestFailed);
      }
    },
    [applyBountySnapshot, m.requestFailed],
  );

  const handleDeleteWatch = useCallback(
    async (id: string) => {
      try {
        const snap = await api<BountySnapshot>("resources/bounties", "POST", {
          action: "delete_watch",
          id,
        });
        applyBountySnapshot(snap);
      } catch {
        setError(m.requestFailed);
      }
    },
    [applyBountySnapshot, m.requestFailed],
  );

  const handleAckAlerts = useCallback(
    async (id?: string) => {
      try {
        const snap = await api<BountySnapshot>("resources/bounties", "POST", {
          action: "ack_alerts",
          id,
        });
        applyBountySnapshot(snap);
      } catch {
        setError(m.requestFailed);
      }
    },
    [applyBountySnapshot, m.requestFailed],
  );

  const handleClearPeaks = useCallback(
    async (category?: string) => {
      try {
        const snap = await api<BountySnapshot>("resources/bounties", "POST", {
          action: "clear_events",
          category,
        });
        applyBountySnapshot(snap);
        setSelectedPeakEventId(undefined);
      } catch {
        setError(m.requestFailed);
      }
    },
    [applyBountySnapshot, m.requestFailed],
  );

  const loadPorts = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const res = await api<{
        items: PortItem[];
        observedAt: string;
        coverage?: string;
        unreadableProcessCount?: number;
      }>("resources/ports");
      setPortData({
        items: res.items ?? [],
        observedAt: res.observedAt,
        coverage: res.coverage,
        unreadableProcessCount: res.unreadableProcessCount,
      });
    } catch {
      setError(m.requestFailed);
    } finally {
      setLoading(false);
    }
  }, [m.requestFailed]);

  const loadFiles = useCallback(
    async (targetPath: string, recursive: boolean) => {
      const trimmed = targetPath.trim();
      if (!trimmed) return;
      setLoading(true);
      setError("");
      try {
        const res = await api<{
          targetPath: string;
          isDirectory: boolean;
          recursive: boolean;
          observedAt: string;
          coverage: string;
          unreadableProcessCount: number;
          items: FileHandleItem[];
        }>("resources/files", "POST", { targetPath: trimmed, recursive });
        setFileData({
          items: res.items ?? [],
          observedAt: res.observedAt,
          coverage: res.coverage,
          unreadableProcessCount: res.unreadableProcessCount,
          queriedPath: res.targetPath,
        });
        onStateChange((prev) => ({
          ...prev,
          files: {
            ...prev.files,
            targetPath: res.targetPath || trimmed,
            isDirectory: res.isDirectory,
            selectedId: res.items?.[0]?.id,
          },
        }));
      } catch {
        setError(m.requestFailed);
      } finally {
        setLoading(false);
      }
    },
    [m.requestFailed, onStateChange],
  );

  const lastRecheckRef = useRef(recheckTrigger);
  useEffect(() => {
    const isRecheck = recheckTrigger !== undefined && recheckTrigger !== lastRecheckRef.current;
    lastRecheckRef.current = recheckTrigger;

    if (!isRecheck && state.origin?.reviewStatus === "stale" && state.origin.subview === state.subview) {
      return;
    }

    if ((state.subview === "home" || state.subview === "processes") && (!procData.observedAt || isRecheck)) {
      void loadProcesses();
    } else if (state.subview === "ports" && (!portData.observedAt || isRecheck)) {
      void loadPorts();
    } else if (state.subview === "files" && isRecheck && state.files.targetPath.trim()) {
      void loadFiles(state.files.targetPath, state.files.recursive);
    }
  }, [
    state.subview,
    recheckTrigger,
    state.origin?.reviewStatus,
    state.origin?.subview,
    procData.observedAt,
    portData.observedAt,
    state.files.targetPath,
    state.files.recursive,
    loadProcesses,
    loadPorts,
    loadFiles,
  ]);

  useEffect(() => {
    if (state.subview !== "home") return;
    if (state.origin?.subview === "home" && state.origin?.reviewStatus === "stale") return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const poll = async () => {
      await loadProcesses({ silent: true });
      if (!cancelled) {
        timer = setTimeout(() => void poll(), BOUNTY_POLL_INTERVAL_MS);
      }
    };
    timer = setTimeout(() => void poll(), BOUNTY_POLL_INTERVAL_MS);
    return () => {
      cancelled = true;
      if (timer !== undefined) clearTimeout(timer);
    };
  }, [state.subview, state.origin?.subview, state.origin?.reviewStatus, loadProcesses]);

  const handleRefresh = useCallback(() => {
    if (state.origin && state.origin.subview === state.subview && state.origin.reviewStatus === "stale") {
      onStateChange((prev) => ({
        ...prev,
        origin: prev.origin ? { ...prev.origin, reviewStatus: "released" } : undefined,
      }));
    }
    if (state.subview === "home" || state.subview === "processes") {
      return loadProcesses();
    } else if (state.subview === "ports") {
      return loadPorts();
    } else if (state.subview === "files" && state.files.targetPath.trim()) {
      return loadFiles(state.files.targetPath, state.files.recursive);
    }
  }, [state.subview, state.origin, state.files.targetPath, state.files.recursive, loadProcesses, loadPorts, loadFiles, onStateChange]);

  const workspaceOptions = useMemo(() => {
    const opts: [string, string][] = [["all", m.allWorkspaces]];
    const seen = new Set<string>();
    const targetProjects =
      state.processes.project === "all"
        ? projects
        : projects.filter((p) => p.id === state.processes.project);
    for (const proj of targetProjects) {
      const details = projectDetails[proj.id];
      const wsList = details?.workspaces ?? [];
      if (state.processes.project === "all" && wsList.length <= 1) {
        continue;
      }
      for (const ws of wsList) {
        if (!seen.has(ws.id)) {
          seen.add(ws.id);
          const wsName = formatWorkspaceName(ws.name, m);
          opts.push([
            ws.id,
            state.processes.project === "all" ? `${proj.name} · ${wsName}` : wsName,
          ]);
        }
      }
    }
    return opts;
  }, [projects, projectDetails, state.processes.project, m]);

  const filteredProcesses = useMemo(() => {
    const q = state.subview === "processes" ? state.processes.query.trim().toLowerCase() : "";
    const isNumeric = /^\d+$/.test(q);
    const list = procData.items.filter((item) => {
      if (state.processes.project !== "all" && item.projectId !== state.processes.project) {
        return false;
      }
      if (state.processes.workspace !== "all" && item.workspaceId !== state.processes.workspace) {
        return false;
      }
      if (state.processes.ownership !== "all" && item.ownership !== state.processes.ownership) {
        return false;
      }
      if (!q) return true;
      const pidStr = String(item.pid);
      if (
        pidStr.includes(q) ||
        item.name.toLowerCase().includes(q) ||
        (item.displayName ?? "").toLowerCase().includes(q) ||
        (item.commandLine ?? "").toLowerCase().includes(q) ||
        (item.projectName ?? "").toLowerCase().includes(q) ||
        (item.workspaceName ?? "").toLowerCase().includes(q)
      ) {
        return true;
      }
      return false;
    });

    return [...list].sort((a, b) => {
      if (isNumeric) {
        const aExact = String(a.pid) === q;
        const bExact = String(b.pid) === q;
        if (aExact !== bExact) return aExact ? -1 : 1;
      }
      if (state.processes.sortBy === "cpu") {
        const ca = a.cpuPercent ?? -1;
        const cb = b.cpuPercent ?? -1;
        if (ca !== cb) return state.processes.sortDesc ? cb - ca : ca - cb;
      } else if (state.processes.sortBy === "memory") {
        if (a.memoryBytes !== b.memoryBytes) {
          return state.processes.sortDesc
            ? b.memoryBytes - a.memoryBytes
            : a.memoryBytes - b.memoryBytes;
        }
      } else if (state.processes.sortBy === "gpu") {
        const ga = a.gpuPercent ?? -1;
        const gb = b.gpuPercent ?? -1;
        if (ga !== gb) return state.processes.sortDesc ? gb - ga : ga - gb;
        const ma = a.gpuMemoryBytes ?? 0;
        const mb = b.gpuMemoryBytes ?? 0;
        if (ma !== mb) return state.processes.sortDesc ? mb - ma : ma - mb;
      } else if (state.processes.sortBy === "network") {
        const na = a.networkBytesPerSec ?? -1;
        const nb = b.networkBytesPerSec ?? -1;
        if (na !== nb) return state.processes.sortDesc ? nb - na : na - nb;
        const ca = a.networkConnections ?? 0;
        const cb = b.networkConnections ?? 0;
        if (ca !== cb) return state.processes.sortDesc ? cb - ca : ca - cb;
      } else {
        if (a.ownership !== b.ownership) {
          if (a.ownership === "managed") return -1;
          if (b.ownership === "managed") return 1;
        }
        if (a.memoryBytes !== b.memoryBytes) return b.memoryBytes - a.memoryBytes;
      }
      return a.pid - b.pid;
    });
  }, [procData.items, state.processes, state.subview]);

  const windowGroupStats = useMemo(() => {
    const acc = new Map<
      string,
      {
        cpuSum: number;
        cpuCount: number;
        memSum: number;
        memCount: number;
        gpuSum: number;
        gpuCount: number;
        gpuMemSum: number;
        gpuMemCount: number;
        netSum: number;
        netCount: number;
      }
    >();

    for (const sample of bountySamples) {
      const perSample = new Map<
        string,
        {
          cpu: number;
          hasCpu: boolean;
          mem: number;
          gpu: number;
          hasGpu: boolean;
          gpuMem: number;
          net: number;
          hasNet: boolean;
        }
      >();

      for (const item of sample.items) {
        if (state.processes.project !== "all" && item.projectId !== state.processes.project) {
          continue;
        }
        if (state.processes.workspace !== "all" && item.workspaceId !== state.processes.workspace) {
          continue;
        }
        if (state.processes.ownership !== "all" && item.ownership !== state.processes.ownership) {
          continue;
        }
        const key = getProcessGroupKey(item);
        let entry = perSample.get(key);
        if (!entry) {
          entry = {
            cpu: 0,
            hasCpu: false,
            mem: 0,
            gpu: 0,
            hasGpu: false,
            gpuMem: 0,
            net: 0,
            hasNet: false,
          };
          perSample.set(key, entry);
        }
        entry.mem += item.memoryBytes || 0;
        if (item.cpuPercent != null) {
          entry.cpu += item.cpuPercent;
          entry.hasCpu = true;
        }
        if (item.gpuPercent != null) {
          entry.gpu += item.gpuPercent;
          entry.hasGpu = true;
        }
        entry.gpuMem += item.gpuMemoryBytes || 0;
        if (item.networkBytesPerSec != null) {
          entry.net += item.networkBytesPerSec;
          entry.hasNet = true;
        }
      }

      for (const [key, entry] of perSample.entries()) {
        let total = acc.get(key);
        if (!total) {
          total = {
            cpuSum: 0,
            cpuCount: 0,
            memSum: 0,
            memCount: 0,
            gpuSum: 0,
            gpuCount: 0,
            gpuMemSum: 0,
            gpuMemCount: 0,
            netSum: 0,
            netCount: 0,
          };
          acc.set(key, total);
        }
        if (entry.hasCpu) {
          total.cpuSum += Math.min(100, entry.cpu);
          total.cpuCount += 1;
        }
        total.memSum += entry.mem;
        total.memCount += 1;
        if (entry.hasGpu) {
          total.gpuSum += Math.min(100, entry.gpu);
          total.gpuCount += 1;
        }
        total.gpuMemSum += entry.gpuMem;
        total.gpuMemCount += 1;
        if (entry.hasNet) {
          total.netSum += entry.net;
          total.netCount += 1;
        }
      }
    }

    return acc;
  }, [bountySamples, state.processes.project, state.processes.workspace, state.processes.ownership]);

  const processGroups = useMemo<ProcessGroup[]>(() => {
    const q = state.subview === "processes" ? state.processes.query.trim().toLowerCase() : "";
    const isNumeric = /^\d+$/.test(q);
    const map = new Map<string, ProcessItem[]>();

    for (const item of filteredProcesses) {
      const key = getProcessGroupKey(item);
      const existing = map.get(key);
      if (existing) {
        existing.push(item);
      } else {
        map.set(key, [item]);
      }
    }

    const groups: ProcessGroup[] = [];
    for (const [key, items] of map.entries()) {
      const primary = items[0];
      const title = friendlyProcessLabel(primary.name, primary.displayName);
      const icon = items.find((i) => i.icon)?.icon;
      let totalMemory = 0;
      let totalCpuSum = 0;
      let hasCpu = false;
      let cpuSampling = false;
      let totalGpuSum = 0;
      let hasGpu = false;
      let gpuSampling = false;
      let totalGpuMemory = 0;
      let totalNetworkSum = 0;
      let hasNetwork = false;
      let networkSampling = false;
      let totalNetworkConnections = 0;
      for (const item of items) {
        totalMemory += item.memoryBytes || 0;
        if (item.cpuPercent != null) {
          totalCpuSum += item.cpuPercent;
          hasCpu = true;
        }
        if (item.cpuSampling) {
          cpuSampling = true;
        }
        if (item.gpuPercent != null) {
          totalGpuSum += item.gpuPercent;
          hasGpu = true;
        }
        if (item.gpuSampling) {
          gpuSampling = true;
        }
        totalGpuMemory += item.gpuMemoryBytes || 0;
        if (item.networkBytesPerSec != null) {
          totalNetworkSum += item.networkBytesPerSec;
          hasNetwork = true;
        }
        if (item.networkSampling) {
          networkSampling = true;
        }
        totalNetworkConnections += item.networkConnections || 0;
      }
      const totalCpu = hasCpu ? Math.min(100, totalCpuSum) : null;
      const totalGpu = hasGpu ? Math.min(100, totalGpuSum) : null;
      const totalNetwork = hasNetwork ? totalNetworkSum : null;
      const win = windowGroupStats.get(key);
      const backendSummary = bountyData.groups[key];
      const fallbackWinCpu = win && win.cpuCount > 0 ? win.cpuSum / win.cpuCount : totalCpu;
      const fallbackWinMem =
        win && win.memCount > 0 ? Math.round(win.memSum / win.memCount) : totalMemory;
      const fallbackWinGpu = win && win.gpuCount > 0 ? win.gpuSum / win.gpuCount : totalGpu;
      const fallbackWinGpuMem =
        win && win.gpuMemCount > 0 ? Math.round(win.gpuMemSum / win.gpuMemCount) : totalGpuMemory;
      const fallbackWinNet = win && win.netCount > 0 ? win.netSum / win.netCount : totalNetwork;

      groups.push({
        key,
        title,
        icon,
        primary,
        items,
        totalCpu,
        cpuSampling: !hasCpu && cpuSampling,
        totalMemory,
        totalGpu,
        gpuSampling: !hasGpu && gpuSampling,
        totalGpuMemory,
        totalNetwork,
        networkSampling: !hasNetwork && networkSampling,
        totalNetworkConnections,
        windowCpu: backendSummary?.windowCpu ?? fallbackWinCpu,
        windowMemory: backendSummary?.windowMemory ?? fallbackWinMem,
        windowGpu: backendSummary?.windowGpu ?? fallbackWinGpu,
        windowGpuMemory: backendSummary?.windowGpuMemory ?? fallbackWinGpuMem,
        windowNetwork: backendSummary?.windowNetwork ?? fallbackWinNet,
        cumulativeNetBytes: backendSummary?.cumulativeNetBytes ?? 0,
        cpuDelta: backendSummary?.cpuDelta ?? 0,
        memoryDeltaBytes: backendSummary?.memoryDeltaBytes ?? 0,
        gpuDelta: backendSummary?.gpuDelta ?? 0,
        networkDeltaBps: backendSummary?.networkDeltaBps ?? 0,
        risingCategories: backendSummary?.risingCategories ?? [],
        watched: Boolean(backendSummary?.watched),
        watchTriggered: Boolean(backendSummary?.watchTriggered),
        ownership: primary.ownership,
        projectName: primary.projectName,
        workspaceName: primary.workspaceName,
      });
    }

    groups.sort((a, b) => {
      if (isNumeric) {
        const aExact = a.items.some((i) => String(i.pid) === q);
        const bExact = b.items.some((i) => String(i.pid) === q);
        if (aExact !== bExact) return aExact ? -1 : 1;
      }
      if (state.processes.sortBy === "cpu") {
        const ca = a.totalCpu ?? -1;
        const cb = b.totalCpu ?? -1;
        if (ca !== cb) return state.processes.sortDesc ? cb - ca : ca - cb;
      } else if (state.processes.sortBy === "memory") {
        if (a.totalMemory !== b.totalMemory) {
          return state.processes.sortDesc
            ? b.totalMemory - a.totalMemory
            : a.totalMemory - b.totalMemory;
        }
      } else if (state.processes.sortBy === "gpu") {
        const ga = a.totalGpu ?? -1;
        const gb = b.totalGpu ?? -1;
        if (ga !== gb) return state.processes.sortDesc ? gb - ga : ga - gb;
        if (a.totalGpuMemory !== b.totalGpuMemory) {
          return state.processes.sortDesc
            ? b.totalGpuMemory - a.totalGpuMemory
            : a.totalGpuMemory - b.totalGpuMemory;
        }
      } else if (state.processes.sortBy === "network") {
        const na = a.totalNetwork ?? -1;
        const nb = b.totalNetwork ?? -1;
        if (na !== nb) return state.processes.sortDesc ? nb - na : na - nb;
        if (a.totalNetworkConnections !== b.totalNetworkConnections) {
          return state.processes.sortDesc
            ? b.totalNetworkConnections - a.totalNetworkConnections
            : a.totalNetworkConnections - b.totalNetworkConnections;
        }
      } else {
        if (a.ownership !== b.ownership) {
          if (a.ownership === "managed") return -1;
          if (b.ownership === "managed") return 1;
        }
        if (a.totalMemory !== b.totalMemory) return b.totalMemory - a.totalMemory;
      }
      return a.title.localeCompare(b.title);
    });

    return groups;
  }, [
    filteredProcesses,
    windowGroupStats,
    bountyData.groups,
    state.processes.query,
    state.processes.sortBy,
    state.processes.sortDesc,
    state.subview,
  ]);

  const bountyRankings = useMemo(() => {
    const byCpu = [...processGroups]
      .sort((a, b) => {
        const ca = a.windowCpu ?? -1;
        const cb = b.windowCpu ?? -1;
        if (cb !== ca) return cb - ca;
        return b.windowMemory - a.windowMemory;
      })
      .slice(0, 3);

    const byMemory = [...processGroups]
      .sort((a, b) => {
        if (b.windowMemory !== a.windowMemory) return b.windowMemory - a.windowMemory;
        return (b.windowCpu ?? -1) - (a.windowCpu ?? -1);
      })
      .slice(0, 3);

    const byGpu = [...processGroups]
      .sort((a, b) => {
        const ga = a.windowGpu ?? -1;
        const gb = b.windowGpu ?? -1;
        if (gb !== ga) return gb - ga;
        if (b.windowGpuMemory !== a.windowGpuMemory) return b.windowGpuMemory - a.windowGpuMemory;
        return b.windowMemory - a.windowMemory;
      })
      .slice(0, 3);

    const byNetwork = [...processGroups]
      .sort((a, b) => {
        const na = a.windowNetwork ?? -1;
        const nb = b.windowNetwork ?? -1;
        if (nb !== na) return nb - na;
        if (b.cumulativeNetBytes !== a.cumulativeNetBytes) {
          return b.cumulativeNetBytes - a.cumulativeNetBytes;
        }
        if (b.totalNetworkConnections !== a.totalNetworkConnections) {
          return b.totalNetworkConnections - a.totalNetworkConnections;
        }
        return b.windowMemory - a.windowMemory;
      })
      .slice(0, 3);

    return {
      cpu: byCpu,
      memory: byMemory,
      gpu: byGpu,
      network: byNetwork,
    };
  }, [processGroups]);

  const bountyPeakRankings = useMemo(() => {
    const filteredEvents = bountyData.events.filter((ev) => {
      if (state.processes.project !== "all" && ev.projectId !== state.processes.project) {
        return false;
      }
      if (state.processes.workspace !== "all" && ev.workspaceId !== state.processes.workspace) {
        return false;
      }
      if (state.processes.ownership !== "all" && ev.ownership !== state.processes.ownership) {
        return false;
      }
      return true;
    });

    const rankCategory = (cat: "cpu" | "memory" | "gpu" | "network") =>
      filteredEvents
        .filter((e) => e.category === cat)
        .sort((a, b) => {
          if (a.reasonKind !== b.reasonKind) {
            if (a.reasonKind === "watch_triggered") return -1;
            if (b.reasonKind === "watch_triggered") return 1;
            if (a.reasonKind === "rapid_rise") return -1;
            if (b.reasonKind === "rapid_rise") return 1;
          }
          if (b.peakValue !== a.peakValue) return b.peakValue - a.peakValue;
          return (b.cumulativeBytes ?? 0) - (a.cumulativeBytes ?? 0);
        })
        .slice(0, 3);

    return {
      cpu: rankCategory("cpu"),
      memory: rankCategory("memory"),
      gpu: rankCategory("gpu"),
      network: rankCategory("network"),
    };
  }, [bountyData.events, state.processes.project, state.processes.workspace, state.processes.ownership]);

  const selectedPeakEvent = useMemo(() => {
    if (!selectedPeakEventId) return undefined;
    return bountyData.events.find((e) => e.id === selectedPeakEventId);
  }, [bountyData.events, selectedPeakEventId]);

  const selectedProcess = useMemo(() => {
    if (!filteredProcesses.length) return undefined;
    return (
      filteredProcesses.find((p) => p.pid === state.processes.selectedPid) ??
      bountyRankings.cpu[0]?.primary ??
      processGroups[0]?.primary ??
      filteredProcesses[0]
    );
  }, [filteredProcesses, bountyRankings.cpu, processGroups, state.processes.selectedPid]);

  const selectedGroup = useMemo(() => {
    if (!selectedProcess) return undefined;
    return processGroups.find((g) => g.items.some((i) => i.pid === selectedProcess.pid));
  }, [processGroups, selectedProcess]);

  const filteredPorts = useMemo(() => {
    const rawQ = state.ports.query.trim().toLowerCase();
    const portNumeric = rawQ.startsWith(":") ? rawQ.slice(1) : rawQ;
    const isPortNum = /^\d+$/.test(portNumeric);
    return portData.items.filter((item) => {
      if (state.ports.protocol !== "all" && item.protocol.toUpperCase() !== state.ports.protocol) {
        return false;
      }
      if (state.ports.state !== "all" && item.state.toUpperCase() !== state.ports.state) {
        return false;
      }
      if (!rawQ) return true;
      if (isPortNum && String(item.port) === portNumeric) return true;
      return (
        item.localAddress.toLowerCase().includes(rawQ) ||
        String(item.port).includes(portNumeric) ||
        item.processName.toLowerCase().includes(rawQ) ||
        (item.processDisplayName ?? "").toLowerCase().includes(rawQ) ||
        String(item.pid).includes(rawQ) ||
        (item.projectName ?? "").toLowerCase().includes(rawQ) ||
        (item.workspaceName ?? "").toLowerCase().includes(rawQ)
      );
    });
  }, [portData.items, state.ports]);

  const selectedPort = useMemo(() => {
    if (!filteredPorts.length) return undefined;
    return (
      filteredPorts.find((p) => p.id === state.ports.selectedId) ?? filteredPorts[0]
    );
  }, [filteredPorts, state.ports.selectedId]);

  const selectedFileHandle = useMemo(() => {
    if (!fileData.items.length) return undefined;
    return (
      fileData.items.find((f) => f.id === state.files.selectedId) ?? fileData.items[0]
    );
  }, [fileData.items, state.files.selectedId]);

  const originRun = useMemo(() => {
    if (!state.origin || state.origin.subview !== state.subview) return undefined;
    return runs.find((r) => r.id === state.origin?.runId);
  }, [state.origin, state.subview, runs]);

  const isStaleOrigin =
    state.origin?.subview === state.subview && state.origin?.reviewStatus === "stale";
  const isReleasedOrigin =
    state.origin?.subview === state.subview && state.origin?.reviewStatus === "released";

  const activeStats = useMemo(() => {
    if (state.subview === "home" || state.subview === "processes") {
      const managedCount = filteredProcesses.filter((i) => i.ownership === "managed").length;
      return {
        total: filteredProcesses.length,
        managed: managedCount,
        coverage: procData.coverage ?? "complete",
        unreadable: procData.unreadableProcessCount ?? 0,
        observedAt: procData.observedAt,
      };
    }
    if (state.subview === "ports") {
      const managedCount = filteredPorts.filter((i) => i.ownership === "managed").length;
      return {
        total: filteredPorts.length,
        managed: managedCount,
        coverage: portData.coverage ?? "complete",
        unreadable: portData.unreadableProcessCount ?? 0,
        observedAt: portData.observedAt,
      };
    }
    const managedCount = fileData.items.filter((i) => i.ownership === "managed").length;
    return {
      total: fileData.items.length,
      managed: managedCount,
      coverage: fileData.coverage ?? "complete",
      unreadable: fileData.unreadableProcessCount ?? 0,
      observedAt: fileData.observedAt,
    };
  }, [state.subview, filteredProcesses, filteredPorts, fileData, procData, portData]);

  async function chooseTargetPath(kind: "file" | "directory") {
    if (pickingRef.current) return;
    pickingRef.current = true;
    try {
      if (kind === "directory") {
        const res = await api<{ directory: string }>("choose-directory", "POST");
        if (res.directory) {
          onStateChange((prev) => ({
            ...prev,
            files: { ...prev.files, targetPath: res.directory, isDirectory: true },
          }));
          void loadFiles(res.directory, state.files.recursive);
        }
      } else {
        const res = await api<{ path: string }>("choose-file", "POST");
        if (res.path) {
          onStateChange((prev) => ({
            ...prev,
            files: { ...prev.files, targetPath: res.path, isDirectory: false, recursive: false },
          }));
          void loadFiles(res.path, false);
        }
      }
    } catch {
      setError(m.directoryPickerFailed);
    } finally {
      pickingRef.current = false;
    }
  }

  function renderCommandLineText(cmdLine?: string, cmdState?: string) {
    if (cmdLine) return cmdLine;
    if (cmdState === "denied") return m.cmdLineDenied;
    return m.cmdLineUnavailable;
  }

  function openExecutableDirectory(executable?: string, fallbackDir?: string) {
    const target = executable || fallbackDir;
    if (!target) return;
    const normalized = target.replace(/\\/g, "/");
    const lastSlash = normalized.lastIndexOf("/");
    const dir = executable && lastSlash > 0 ? target.slice(0, lastSlash) : target;
    onOpenDirectoryPath(dir);
  }

  function renderProjectCell(
    ownership: string,
    projectName: string | undefined,
    workspaceName: string | undefined,
    rowSelected: boolean,
  ) {
    if (ownership === "managed") {
      return (
        <div className="flex min-w-0 items-center gap-1.5">
          <span
            className={`shrink-0 border px-1.5 py-0.5 text-[10px] font-medium leading-none ${
              rowSelected
                ? "border-selected-foreground/30 bg-selected-foreground/15 text-selected-foreground"
                : "border-success/30 bg-success-surface text-success"
            }`}
          >
            YouRun
          </span>
          <span className="truncate font-medium">
            {[projectName || m.independent, formatWorkspaceName(workspaceName, m)]
              .filter(Boolean)
              .join(" · ")}
          </span>
        </div>
      );
    }
    if (ownership === "unconfirmed") {
      return (
        <span
          className={`inline-flex items-center border px-1.5 py-0.5 text-[10px] font-medium leading-none ${
            rowSelected
              ? "border-selected-foreground/30 text-selected-foreground"
              : "border-warning/30 bg-warning-surface text-warning-foreground"
          }`}
        >
          {m.ownershipUnconfirmed}
        </span>
      );
    }
    return null;
  }

  function resourceOrigin(item: ResourceProcess, run: Run): ResourceOriginContext {
    return {
      subview: state.subview,
      label: `${item.displayName || item.processDisplayName || item.name || item.processName || run.title} (PID ${item.pid})`,
      runId: run.id,
      runTitle: run.title,
      commandId: run.commandId,
      projectId: item.projectId,
      projectName: item.projectName,
      workspaceId: item.workspaceId,
      workspaceName: item.workspaceName,
      pid: item.pid,
      portQuery: state.subview === "ports" ? state.ports.query : undefined,
      targetPath: state.subview === "files" ? state.files.targetPath : undefined,
      reviewStatus: "active",
    };
  }

  return (
    <ResourceActionsProvider items={state.subview === "ports" ? filteredPorts : state.subview === "files" ? fileData.items : processGroups.flatMap(group => group.items)} m={m} onRefresh={handleRefresh}
      runActions={{ runs, commands, operations: runOperations, errors: launchErrors, onReplay, onStop,
        onOpenRun: (item, run) => onOpenRun(run.id, resourceOrigin(item, run)),
        onOpenCommand: (item, run, command) => onOpenCommand(command.id, resourceOrigin(item, run)),
      }}>
    <div className="flex min-h-0 min-w-0 flex-1 flex-col bg-surface">
      {/* Sub-navigation bar */}
      <header className="flex h-10 shrink-0 items-center justify-between gap-2 border-b border-border px-2 @min-[36rem]/desktop:px-4">
        <div className="flex min-w-0 items-center gap-1 overflow-x-auto" role="tablist" aria-label={m.proMode}>
          {(
            [
              ["home", m.proHome],
              ["processes", m.proProcesses],
              ["ports", m.proPorts],
              ["files", m.proFiles],
            ] as [ProSubview, string][]
          ).map(([id, label]) => {
            const active = state.subview === id;
            return (
              <button
                key={id}
                type="button"
                role="tab"
                aria-selected={active}
                className={`inline-flex h-7 shrink-0 items-center whitespace-nowrap rounded-sm px-3 text-xs transition-colors ${
                  active
                    ? "bg-selected font-medium text-selected-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-foreground"
                }`}
                onClick={() => {
                  setNarrowDetailOpen(false);
                  onStateChange((prev) => ({ ...prev, subview: id }));
                }}
              >
                {label}
              </button>
            );
          })}
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {isStaleOrigin && (
            <span className="border border-warning/30 bg-warning-surface px-2 py-0.5 text-xs text-warning-foreground">
              {m.staleResultSuffix}
            </span>
          )}
          <Button
            type="button"
            variant="ghost"
            size="icon-sm"
            className="size-7 text-muted-foreground hover:text-foreground"
            disabled={loading}
            onClick={handleRefresh}
            title={m.refresh}
            aria-label={m.refresh}
          >
            <RefreshCw
              className={`size-3.5 ${loading ? "motion-safe:animate-spin" : ""}`}
              aria-hidden="true"
            />
          </Button>
        </div>
      </header>
      <ResourceBatchActions />

      {/* Filter bar per subview */}
      {(state.subview === "home" || state.subview === "processes") && (
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-surface-sunken/30 px-4 py-2">
          {state.subview === "home" ? (
            <div className="flex min-h-8 flex-wrap items-center gap-2.5">
              <h2 className="text-xs font-semibold tracking-tight text-foreground">
                {m.bountyBoard}
              </h2>
              <div className="flex items-center gap-1.5" aria-hidden="true">
                <BountyRabbitBadge rank={1} m={m} />
                <BountyRabbitBadge rank={2} m={m} />
                <BountyRabbitBadge rank={3} m={m} />
              </div>
              <div
                className="inline-flex items-center border border-border bg-surface p-0.5 text-xs"
                role="tablist"
                aria-label={m.bountyBoard}
              >
                <button
                  type="button"
                  role="tab"
                  aria-selected={bountyMode === "live"}
                  onClick={() => setBountyMode("live")}
                  className={`inline-flex h-6 items-center gap-1.5 px-2 text-[11px] transition-colors ${
                    bountyMode === "live"
                      ? "bg-selected font-medium text-selected-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  <span
                    className={`size-1.5 rounded-full ${
                      bountyMode === "live"
                        ? "bg-selected-foreground motion-safe:animate-pulse"
                        : "bg-success"
                    }`}
                    aria-hidden="true"
                  />
                  <span>{m.bountyWindowSeconds.replace("%d", "30")}</span>
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={bountyMode === "peaks"}
                  onClick={() => setBountyMode("peaks")}
                  className={`inline-flex h-6 items-center gap-1 px-2 text-[11px] transition-colors ${
                    bountyMode === "peaks"
                      ? "bg-selected font-medium text-selected-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  <span>{m.bountyModePeaks}</span>
                  {bountyData.events.length > 0 && (
                    <span className="font-mono text-[10px] tabular-nums opacity-80">
                      {bountyData.events.length}
                    </span>
                  )}
                </button>
                <button
                  type="button"
                  role="tab"
                  aria-selected={bountyMode === "watch"}
                  onClick={() => setBountyMode("watch")}
                  className={`inline-flex h-6 items-center gap-1 px-2 text-[11px] transition-colors ${
                    bountyMode === "watch"
                      ? "bg-selected font-medium text-selected-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                >
                  <Bell className="size-3" aria-hidden="true" />
                  <span>{m.bountyModeWatch}</span>
                  {bountyData.unreadAlertCount > 0 ? (
                    <span className="inline-flex h-4 min-w-4 items-center justify-center bg-warning px-1 font-mono text-[10px] font-semibold leading-none text-warning-foreground">
                      {bountyData.unreadAlertCount}
                    </span>
                  ) : (
                    bountyData.watches.length > 0 && (
                      <span className="font-mono text-[10px] tabular-nums opacity-80">
                        {bountyData.watches.length}
                      </span>
                    )
                  )}
                </button>
              </div>
            </div>
          ) : (
            <div className="relative min-w-52 flex-1">
              <Search
                className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
                aria-hidden="true"
              />
              <Input
                type="search"
                value={state.processes.query}
                onChange={(e) =>
                  onStateChange((prev) => ({
                    ...prev,
                    processes: { ...prev.processes, query: e.target.value },
                  }))
                }
                placeholder={m.searchProcessesPlaceholder}
                aria-label={m.searchProcessesPlaceholder}
                className="h-8 pl-8 pr-7 text-xs [&::-webkit-search-cancel-button]:appearance-none"
              />
              {state.processes.query && (
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="absolute right-1 top-1/2 size-6 -translate-y-1/2"
                  onClick={() =>
                    onStateChange((prev) => ({
                      ...prev,
                      processes: { ...prev.processes, query: "" },
                    }))
                  }
                  aria-label={m.clear}
                >
                  <X className="size-3.5" />
                </Button>
              )}
            </div>
          )}
          <div className="flex items-center gap-1.5">
            {state.subview === "home" && bountyMode === "peaks" && bountyData.events.length > 0 && (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="size-7 text-muted-foreground hover:text-foreground"
                onClick={() => void handleClearPeaks()}
                title={m.bountyClearPeaks}
                aria-label={m.bountyClearPeaks}
              >
                <Trash2 className="size-3.5" aria-hidden="true" />
              </Button>
            )}
            {state.subview === "home" && (
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className="size-7 text-muted-foreground hover:text-foreground"
                onClick={() => openWatchDialogFor()}
                title={m.bountyAddWatch}
                aria-label={m.bountyAddWatch}
              >
                <Plus className="size-3.5" aria-hidden="true" />
              </Button>
            )}
            <DropdownMenu modal={false}>
              <DropdownMenuTrigger asChild>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className={`size-7 ${
                    state.processes.ownership !== "all" ||
                    state.processes.project !== "all" ||
                    state.processes.workspace !== "all"
                      ? "bg-selected text-selected-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                  title={m.filter}
                  aria-label={m.filter}
                >
                  <ListFilter className="size-3.5" aria-hidden="true" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="max-h-80 min-w-44 overflow-y-auto">
                <DropdownMenuLabel>{m.colOwnership}</DropdownMenuLabel>
                {(
                  [
                    ["all", m.allOwnership],
                    ["managed", m.ownershipManaged],
                    ["external", m.ownershipExternal],
                    ["unconfirmed", m.ownershipUnconfirmed],
                  ] as [string, string][]
                ).map(([val, label]) => {
                  const checked = state.processes.ownership === val;
                  return (
                    <DropdownMenuItem
                      key={`ownership-${val}`}
                      role="menuitemradio"
                      aria-checked={checked}
                      onSelect={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          processes: { ...prev.processes, ownership: val },
                        }))
                      }
                    >
                      <Check className={checked ? "" : "invisible"} aria-hidden="true" />
                      <span className="truncate">{label}</span>
                    </DropdownMenuItem>
                  );
                })}
                <DropdownMenuSeparator />
                <DropdownMenuLabel>{m.project}</DropdownMenuLabel>
                {(
                  [
                    ["all", m.allProjects],
                    ...projects.map((p): [string, string] => [p.id, p.name]),
                  ] as [string, string][]
                ).map(([val, label]) => {
                  const checked = state.processes.project === val;
                  return (
                    <DropdownMenuItem
                      key={`project-${val}`}
                      role="menuitemradio"
                      aria-checked={checked}
                      onSelect={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          processes: { ...prev.processes, project: val, workspace: "all" },
                        }))
                      }
                    >
                      <Check className={checked ? "" : "invisible"} aria-hidden="true" />
                      <span className="truncate">{label}</span>
                    </DropdownMenuItem>
                  );
                })}
                {workspaceOptions.length > 1 && (
                  <>
                    <DropdownMenuSeparator />
                    <DropdownMenuLabel>{m.workspace}</DropdownMenuLabel>
                    {workspaceOptions.map(([val, label]) => {
                      const checked = state.processes.workspace === val;
                      return (
                        <DropdownMenuItem
                          key={`workspace-${val}`}
                          role="menuitemradio"
                          aria-checked={checked}
                          onSelect={() =>
                            onStateChange((prev) => ({
                              ...prev,
                              processes: { ...prev.processes, workspace: val },
                            }))
                          }
                        >
                          <Check className={checked ? "" : "invisible"} aria-hidden="true" />
                          <span className="truncate">{label}</span>
                        </DropdownMenuItem>
                      );
                    })}
                  </>
                )}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      )}

      {/* Unread Bounty Watch Alert Banner */}
      {bountyData.watches.some((w) => w.unreadHit) && (
        <div
          role="status"
          className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-warning/30 bg-warning-surface px-4 py-1.5 text-xs"
        >
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Bell className="size-3.5 shrink-0 text-warning-foreground" aria-hidden="true" />
            {bountyData.watches
              .filter((w) => w.unreadHit)
              .slice(0, 3)
              .map((w) => (
                <button
                  key={w.id}
                  type="button"
                  onClick={() => {
                    onStateChange((prev) => ({ ...prev, subview: "home" }));
                    setBountyMode("peaks");
                    const ev = bountyData.events.find(
                      (e) => e.watchId === w.id || e.title.toLowerCase() === w.targetName.toLowerCase(),
                    );
                    if (ev) {
                      setSelectedPeakEventId(ev.id);
                    }
                  }}
                  className="inline-flex items-center gap-1.5 border border-warning/40 bg-background px-2 py-0.5 text-[11px] font-medium text-warning-foreground hover:bg-accent"
                >
                  <span>{m.bountyAlertBanner.replace("%s", w.title || w.targetName)}</span>
                  <span className="font-mono opacity-80">({formatWatchRuleSummary(w, m)})</span>
                </button>
              ))}
          </div>
          <Button variant="ghost" size="xs" onClick={() => void handleAckAlerts()}>
            {m.bountyAcknowledge}
          </Button>
        </div>
      )}

      {state.subview === "ports" && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border bg-surface-sunken/30 px-4 py-2">
          <div className="relative min-w-52 flex-1">
            <Search
              className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              type="search"
              value={state.ports.query}
              onChange={(e) =>
                onStateChange((prev) => ({
                  ...prev,
                  ports: { ...prev.ports, query: e.target.value },
                }))
              }
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  handleRefresh();
                }
              }}
              placeholder={m.searchPortsPlaceholder}
              aria-label={m.searchPortsPlaceholder}
              className="h-8 pl-8 pr-7 font-mono text-xs [&::-webkit-search-cancel-button]:appearance-none"
            />
            {state.ports.query && (
              <Button
                variant="ghost"
                size="icon-sm"
                className="absolute right-1 top-1/2 size-6 -translate-y-1/2"
                onClick={() =>
                  onStateChange((prev) => ({
                    ...prev,
                    ports: { ...prev.ports, query: "" },
                  }))
                }
                aria-label={m.clear}
              >
                <X className="size-3.5" />
              </Button>
            )}
          </div>
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                className={`size-7 ${
                  state.ports.protocol !== "all" || state.ports.state !== "all"
                    ? "bg-selected text-selected-foreground"
                    : "text-muted-foreground hover:text-foreground"
                }`}
                title={m.filter}
                aria-label={m.filter}
              >
                <ListFilter className="size-3.5" aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="min-w-40">
              <DropdownMenuLabel>{m.colProtocol}</DropdownMenuLabel>
              {(
                [
                  ["all", m.protocolAll],
                  ["TCP", "TCP"],
                  ["UDP", "UDP"],
                ] as [string, string][]
              ).map(([val, label]) => {
                const checked = state.ports.protocol === val;
                return (
                  <DropdownMenuItem
                    key={`protocol-${val}`}
                    role="menuitemradio"
                    aria-checked={checked}
                    onSelect={() =>
                      onStateChange((prev) => ({
                        ...prev,
                        ports: { ...prev.ports, protocol: val },
                      }))
                    }
                  >
                    <Check className={checked ? "" : "invisible"} aria-hidden="true" />
                    <span className="truncate">{label}</span>
                  </DropdownMenuItem>
                );
              })}
              <DropdownMenuSeparator />
              <DropdownMenuLabel>{m.colState}</DropdownMenuLabel>
              {(
                [
                  ["all", m.stateAll],
                  ["LISTEN", m.stateListening],
                  ["BOUND", m.stateBound],
                  ["ESTABLISHED", m.stateEstablished],
                ] as [string, string][]
              ).map(([val, label]) => {
                const checked = state.ports.state === val;
                return (
                  <DropdownMenuItem
                    key={`state-${val}`}
                    role="menuitemradio"
                    aria-checked={checked}
                    onSelect={() =>
                      onStateChange((prev) => ({
                        ...prev,
                        ports: { ...prev.ports, state: val },
                      }))
                    }
                  >
                    <Check className={checked ? "" : "invisible"} aria-hidden="true" />
                    <span className="truncate">{label}</span>
                  </DropdownMenuItem>
                );
              })}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}

      {state.subview === "files" && (
        <div className="flex shrink-0 flex-col gap-2 border-b border-border bg-surface-sunken/30 px-2 py-2 @min-[36rem]/desktop:px-4">
          <div className="flex min-w-0 items-center gap-2">
            <div className="min-w-0 flex-1">
              <Input
                value={state.files.targetPath}
                onChange={(e) =>
                  onStateChange((prev) => ({
                    ...prev,
                    files: { ...prev.files, targetPath: e.target.value },
                  }))
                }
                onKeyDown={(e) => {
                  if (e.key === "Enter" && state.files.targetPath.trim()) {
                    e.preventDefault();
                    void loadFiles(state.files.targetPath, state.files.recursive);
                  }
                }}
                placeholder={m.searchFilesPlaceholder}
                aria-label={m.searchFilesPlaceholder}
                className="h-8 font-mono text-xs"
              />
            </div>
            <div className="flex shrink-0 items-center gap-1.5">
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                disabled={loading}
                onClick={() => void chooseTargetPath("file")}
                title={m.chooseFile}
                aria-label={m.chooseFile}
              >
                <FileSearch className="size-3.5" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                disabled={loading}
                onClick={() => void chooseTargetPath("directory")}
                title={m.chooseFolder}
                aria-label={m.chooseFolder}
              >
                <FolderOpen className="size-3.5" aria-hidden="true" />
              </Button>
              <Button
                type="button"
                size="icon-sm"
                disabled={loading || !state.files.targetPath.trim()}
                onClick={() => void loadFiles(state.files.targetPath, state.files.recursive)}
                title={m.queryAction}
                aria-label={m.queryAction}
              >
                <Search className="size-3.5" aria-hidden="true" />
              </Button>
            </div>
          </div>
          {state.files.isDirectory && (
            <label className="flex min-h-6 items-center gap-1.5 self-start px-1 text-xs text-foreground-secondary">
              <Checkbox
                checked={state.files.recursive}
                onCheckedChange={(checked) =>
                  onStateChange((prev) => ({
                    ...prev,
                    files: { ...prev.files, recursive: checked === true },
                  }))
                }
              />
              {m.includeSubdirectories}
            </label>
          )}
        </div>
      )}

      {/* Stale snapshot warning banner */}
      {isStaleOrigin && (
        <div
          role="status"
          className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-warning/30 bg-warning-surface px-4 py-2 text-xs"
        >
          <div className="flex items-center gap-2">
            <span className="border border-warning/30 bg-background px-1.5 py-0.5 font-medium text-warning-foreground">
              {m.staleResultSuffix}
            </span>
            <span>{m.staleResultBanner}</span>
          </div>
          <Button size="icon-sm" onClick={handleRefresh} aria-label={m.recheckNow} title={m.recheckNow}>
            <RefreshCw className="size-4" aria-hidden="true" />
          </Button>
        </div>
      )}

      {/* Partial coverage notice for File Handles */}
      {state.subview === "files" &&
        fileData.coverage === "partial" &&
        (fileData.unreadableProcessCount ?? 0) > 0 && (
          <div
            role="status"
            className="flex shrink-0 items-center justify-between border-b border-border bg-surface-sunken px-4 py-1.5 text-xs text-muted-foreground"
          >
            <span>
              {m.partialCoverageBanner} · {fileData.unreadableProcessCount}{" "}
              {m.unreadableProcessesSuffix}
            </span>
          </div>
        )}

      {error && (
        <div role="alert" className="border-b border-border px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      {/* Main Split Content Area */}
      <div className="flex min-h-0 min-w-0 flex-1 overflow-hidden">
        {/* Left Table / Bounty Board Area */}
        <div
          className={`flex min-h-0 min-w-0 flex-1 flex-col overflow-auto ${
            narrowDetailOpen ? "hidden @min-[63rem]/desktop:flex" : "flex"
          }`}
        >
          {state.subview === "home" && bountyMode === "live" && (
            <div className="grid grid-cols-1 border-b border-border @min-[44rem]/desktop:grid-cols-2">
              {(
                [
                  {
                    id: "cpu" as const,
                    label: m.colCpu,
                    items: bountyRankings.cpu,
                    borderCls: "border-b border-border @min-[44rem]/desktop:border-r",
                  },
                  {
                    id: "memory" as const,
                    label: m.colMemory,
                    items: bountyRankings.memory,
                    borderCls: "border-b border-border",
                  },
                  {
                    id: "gpu" as const,
                    label: m.colGpu,
                    items: bountyRankings.gpu,
                    borderCls:
                      "border-b border-border @min-[44rem]/desktop:border-b-0 @min-[44rem]/desktop:border-r",
                  },
                  {
                    id: "network" as const,
                    label: m.colNetwork,
                    items: bountyRankings.network,
                    borderCls: "",
                  },
                ]
              ).map((cat) => (
                <section
                  key={cat.id}
                  aria-label={`${m.bountyBoard} · ${cat.label}`}
                  className={`flex min-w-0 flex-col ${cat.borderCls}`}
                >
                  <div className="flex h-8 shrink-0 items-center justify-between border-b border-border bg-surface-sunken px-3 text-xs">
                    <span className="font-medium text-foreground">{cat.label}</span>
                    <button
                      type="button"
                      onClick={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          subview: "processes",
                          processes: {
                            ...prev.processes,
                            sortBy: cat.id,
                            sortDesc: true,
                          },
                        }))
                      }
                      className="inline-flex items-center gap-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
                    >
                      <span>{m.proProcesses}</span>
                      <span aria-hidden="true">→</span>
                    </button>
                  </div>
                  <div className="divide-y divide-border/60">
                    {([1, 2, 3] as const).map((rank) => {
                      const group = cat.items[rank - 1];
                      if (!group) {
                        return (
                          <div
                            key={`${cat.id}-empty-${rank}`}
                            className="flex h-13 items-center justify-between gap-2.5 px-3 text-xs text-muted-foreground"
                          >
                            <div className="flex items-center gap-2.5 opacity-45">
                              <BountyRabbitBadge rank={rank} m={m} iconOnly />
                              <span>--</span>
                            </div>
                            <span className="font-mono tabular-nums">--</span>
                          </div>
                        );
                      }

                      const targetItem =
                        cat.id === "cpu"
                          ? [...group.items].sort(
                              (a, b) => (b.cpuPercent ?? -1) - (a.cpuPercent ?? -1),
                            )[0]
                          : cat.id === "memory"
                            ? [...group.items].sort((a, b) => b.memoryBytes - a.memoryBytes)[0]
                            : cat.id === "gpu"
                              ? [...group.items].sort(
                                  (a, b) =>
                                    (b.gpuPercent ?? -1) - (a.gpuPercent ?? -1) ||
                                    (b.gpuMemoryBytes ?? 0) - (a.gpuMemoryBytes ?? 0),
                                )[0]
                              : [...group.items].sort(
                                  (a, b) =>
                                    (b.networkBytesPerSec ?? -1) - (a.networkBytesPerSec ?? -1) ||
                                    (b.networkConnections ?? 0) - (a.networkConnections ?? 0),
                                )[0];
                      const rep = targetItem ?? group.primary;
                      const rowSelected = selectedGroup?.key === group.key;

                      const primaryMetric =
                        cat.id === "cpu"
                          ? group.cpuSampling && group.windowCpu == null
                            ? m.cpuSampling
                            : group.windowCpu != null
                              ? `${group.windowCpu.toFixed(1)}%`
                              : "--"
                          : cat.id === "memory"
                            ? formatMemory(group.windowMemory)
                            : cat.id === "gpu"
                              ? group.gpuSampling && group.windowGpu == null
                                ? m.cpuSampling
                                : group.windowGpu != null
                                  ? `${group.windowGpu.toFixed(1)}%`
                                  : group.windowGpuMemory > 0
                                    ? formatMemory(group.windowGpuMemory)
                                    : "--"
                              : formatNetworkRate(
                                  group.windowNetwork,
                                  group.networkSampling && group.windowNetwork == null,
                                  m,
                                );

                      const rapidRiseText =
                        cat.id === "cpu" && group.cpuDelta >= 25
                          ? `↑ +${Math.round(group.cpuDelta)}%`
                          : cat.id === "memory" && group.memoryDeltaBytes >= 150 * 1024 * 1024
                            ? `↑ +${formatMemory(group.memoryDeltaBytes)}`
                            : cat.id === "gpu" && group.gpuDelta >= 25
                              ? `↑ +${Math.round(group.gpuDelta)}%`
                              : cat.id === "network" && group.networkDeltaBps >= 512 * 1024
                                ? `↑ +${formatNetworkRate(group.networkDeltaBps, false, m)}`
                                : "";

                      const secondaryMetric =
                        cat.id === "cpu"
                          ? `${m.colMemory} ${formatMemory(group.windowMemory)}`
                          : cat.id === "memory"
                            ? group.windowCpu != null
                              ? `${group.windowCpu.toFixed(1)}% ${m.colCpu}`
                              : `PID ${rep.pid}`
                            : cat.id === "gpu"
                              ? group.windowGpuMemory > 0
                                ? `${m.gpuMemoryLabel} ${formatMemory(group.windowGpuMemory)}`
                                : `${m.colMemory} ${formatMemory(group.windowMemory)}`
                              : group.cumulativeNetBytes > 0
                                ? [
                                    `${m.bountyCumulativePrefix} ${formatMemory(group.cumulativeNetBytes)}`,
                                    group.totalNetworkConnections > 0
                                      ? `${group.totalNetworkConnections} ${m.networkConnectionsUnit}`
                                      : "",
                                  ]
                                    .filter(Boolean)
                                    .join(" · ")
                                : group.totalNetworkConnections > 0
                                  ? `${group.totalNetworkConnections} ${m.networkConnectionsUnit}`
                                  : `${m.colMemory} ${formatMemory(group.windowMemory)}`;

                      return (
                        <ResourceMenu key={`${cat.id}-${group.key}`} items={group.items}>
                        <div tabIndex={0}
                          onClick={() => {
                            setSelectedPeakEventId(undefined);
                            onStateChange((prev) => ({
                              ...prev,
                              processes: { ...prev.processes, selectedPid: rep.pid },
                            }));
                            setNarrowDetailOpen(true);
                          }}
                          className={`flex h-13 cursor-pointer select-none items-center justify-between gap-2.5 px-3 text-xs transition-colors ${
                            rowSelected
                              ? "bg-selected text-selected-foreground"
                              : "hover:bg-surface-sunken/70"
                          }`}
                        >
                          <div className="flex min-w-0 flex-1 items-center gap-2.5">
                            <BountyRabbitBadge rank={rank} m={m} selected={rowSelected} iconOnly />
                            <ProcessIcon
                              name={group.primary.name}
                              displayName={group.title}
                              icon={group.icon}
                            />
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5">
                                <span className="truncate font-medium">{group.title}</span>
                                {group.items.length > 1 && (
                                  <span
                                    className={`shrink-0 font-mono text-[11px] tabular-nums ${
                                      rowSelected
                                        ? "text-selected-foreground/75"
                                        : "text-muted-foreground"
                                    }`}
                                  >
                                    ({group.items.length})
                                  </span>
                                )}
                                {group.watched && (
                                  <Bell
                                    className={`size-3 shrink-0 ${
                                      rowSelected
                                        ? "text-selected-foreground"
                                        : group.watchTriggered
                                          ? "text-warning-foreground"
                                          : "text-muted-foreground"
                                    }`}
                                    aria-label={m.bountyModeWatch}
                                  />
                                )}
                                {rapidRiseText && (
                                  <span
                                    className={`shrink-0 border px-1 py-0.5 font-mono text-[10px] font-medium leading-none tabular-nums ${
                                      rowSelected
                                        ? "border-selected-foreground/30 bg-selected-foreground/15 text-selected-foreground"
                                        : "border-warning/35 bg-warning-surface text-warning-foreground"
                                    }`}
                                    title={m.bountyRapidRise}
                                  >
                                    {rapidRiseText}
                                  </span>
                                )}
                                {group.ownership === "managed" && (
                                  <span
                                    className={`shrink-0 border px-1 py-0.5 text-[10px] font-medium leading-none ${
                                      rowSelected
                                        ? "border-selected-foreground/30 bg-selected-foreground/15 text-selected-foreground"
                                        : "border-success/30 bg-success-surface text-success"
                                    }`}
                                  >
                                    YouRun
                                  </span>
                                )}
                              </div>
                              <div
                                className={`truncate font-mono text-[10px] tabular-nums ${
                                  rowSelected
                                    ? "text-selected-foreground/75"
                                    : "text-muted-foreground"
                                }`}
                              >
                                {group.ownership === "managed" && group.projectName
                                  ? [
                                      group.projectName,
                                      formatWorkspaceName(group.workspaceName, m),
                                    ]
                                      .filter(Boolean)
                                      .join(" · ")
                                  : `PID ${rep.pid}`}
                              </div>
                            </div>
                          </div>
                          <div className="shrink-0 text-right font-mono tabular-nums">
                            <div
                              className={`text-xs font-semibold ${
                                rowSelected ? "text-selected-foreground" : "text-foreground"
                              }`}
                            >
                              {primaryMetric}
                            </div>
                            <div
                              className={`text-[10px] ${
                                rowSelected
                                  ? "text-selected-foreground/75"
                                  : "text-muted-foreground"
                              }`}
                            >
                              {secondaryMetric}
                            </div>
                          </div>
                        </div>
                        </ResourceMenu>
                      );
                    })}
                  </div>
                </section>
              ))}
            </div>
          )}

          {state.subview === "home" && bountyMode === "peaks" && (
            <div className="grid grid-cols-1 border-b border-border @min-[44rem]/desktop:grid-cols-2">
              {(
                [
                  {
                    id: "cpu" as const,
                    label: m.colCpu,
                    items: bountyPeakRankings.cpu,
                    borderCls: "border-b border-border @min-[44rem]/desktop:border-r",
                  },
                  {
                    id: "memory" as const,
                    label: m.colMemory,
                    items: bountyPeakRankings.memory,
                    borderCls: "border-b border-border",
                  },
                  {
                    id: "gpu" as const,
                    label: m.colGpu,
                    items: bountyPeakRankings.gpu,
                    borderCls:
                      "border-b border-border @min-[44rem]/desktop:border-b-0 @min-[44rem]/desktop:border-r",
                  },
                  {
                    id: "network" as const,
                    label: m.colNetwork,
                    items: bountyPeakRankings.network,
                    borderCls: "",
                  },
                ]
              ).map((cat) => (
                <section
                  key={`peaks-${cat.id}`}
                  aria-label={`${m.bountyModePeaks} · ${cat.label}`}
                  className={`flex min-w-0 flex-col ${cat.borderCls}`}
                >
                  <div className="flex h-8 shrink-0 items-center justify-between border-b border-border bg-surface-sunken px-3 text-xs">
                    <span className="font-medium text-foreground">{cat.label}</span>
                    <button
                      type="button"
                      onClick={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          subview: "processes",
                          processes: {
                            ...prev.processes,
                            sortBy: cat.id,
                            sortDesc: true,
                          },
                        }))
                      }
                      className="inline-flex items-center gap-1 text-[11px] text-muted-foreground transition-colors hover:text-foreground"
                    >
                      <span>{m.proProcesses}</span>
                      <span aria-hidden="true">→</span>
                    </button>
                  </div>
                  <div className="divide-y divide-border/60">
                    {([1, 2, 3] as const).map((rank) => {
                      const ev = cat.items[rank - 1];
                      if (!ev) {
                        return (
                          <div
                            key={`peaks-${cat.id}-empty-${rank}`}
                            className="flex h-13 items-center justify-between gap-2.5 px-3 text-xs text-muted-foreground"
                          >
                            <div className="flex items-center gap-2.5 opacity-45">
                              <BountyRabbitBadge rank={rank} m={m} iconOnly />
                        <span>{m.bountyNoPeaks}</span>
                            </div>
                            <span className="font-mono tabular-nums">--</span>
                          </div>
                        );
                      }

                      const rowSelected =
                        selectedPeakEvent?.id === ev.id ||
                        (!selectedPeakEvent && selectedGroup?.key === ev.groupKey);

                      const primaryMetric =
                        cat.id === "cpu" || cat.id === "gpu"
                          ? `${m.bountyPeakPrefix} ${ev.peakValue.toFixed(1)}%`
                          : cat.id === "memory"
                            ? `${m.bountyPeakPrefix} ${formatMemory(ev.peakValue)}`
                            : `${m.bountyPeakPrefix} ${formatNetworkRate(ev.peakValue, false, m)}`;

                      const deltaText =
                        ev.deltaValue && ev.deltaValue > 0
                          ? cat.id === "cpu" || cat.id === "gpu"
                            ? `↑ +${Math.round(ev.deltaValue)}%`
                            : cat.id === "memory"
                              ? `↑ +${formatMemory(ev.deltaValue)}`
                              : `↑ +${formatNetworkRate(ev.deltaValue, false, m)}`
                          : "";

                      const secondaryMetric =
                        cat.id === "network" && (ev.cumulativeBytes ?? 0) > 0
                          ? [
                              deltaText,
                              `${m.bountyCumulativePrefix} ${formatMemory(ev.cumulativeBytes!)}`,
                            ]
                              .filter(Boolean)
                              .join(" · ")
                          : deltaText
                            ? `${deltaText} · ${m.colMemory} ${formatMemory(ev.memoryBytes ?? 0)}`
                            : `${m.colMemory} ${formatMemory(ev.memoryBytes ?? 0)}`;

                      return (
                        <div
                          key={ev.id}
                          tabIndex={0}
                          onClick={() => {
                            setSelectedPeakEventId(ev.id);
                            const liveMatch =
                              filteredProcesses.find((p) => p.pid === ev.pid) ??
                              processGroups.find((g) => g.key === ev.groupKey)?.primary;
                            if (liveMatch) {
                              onStateChange((prev) => ({
                                ...prev,
                                processes: { ...prev.processes, selectedPid: liveMatch.pid },
                              }));
                            }
                            setNarrowDetailOpen(true);
                          }}
                          className={`flex h-13 cursor-pointer select-none items-center justify-between gap-2.5 px-3 text-xs transition-colors ${
                            rowSelected
                              ? "bg-selected text-selected-foreground"
                              : "hover:bg-surface-sunken/70"
                          }`}
                        >
                          <div className="flex min-w-0 flex-1 items-center gap-2.5">
                            <BountyRabbitBadge rank={rank} m={m} selected={rowSelected} iconOnly />
                            <ProcessIcon
                              name={ev.processName}
                              displayName={ev.title}
                              icon={ev.icon}
                            />
                            <div className="min-w-0 flex-1">
                              <div className="flex items-center gap-1.5">
                                <span className="truncate font-medium">{ev.title}</span>
                                {ev.reasonKind === "rapid_rise" && (
                                  <span
                                    className={`shrink-0 border px-1 py-0.5 text-[10px] font-medium leading-none ${
                                      rowSelected
                                        ? "border-selected-foreground/30 bg-selected-foreground/15 text-selected-foreground"
                                        : "border-warning/35 bg-warning-surface text-warning-foreground"
                                    }`}
                                  >
                                    {m.bountyRapidRise}
                                  </span>
                                )}
                                {ev.reasonKind === "watch_triggered" && (
                                  <Bell
                                    className={`size-3 shrink-0 ${
                                      rowSelected
                                        ? "text-selected-foreground"
                                        : "text-warning-foreground"
                                    }`}
                                    aria-label={m.bountyModeWatch}
                                  />
                                )}
                                {ev.processExited && (
                                  <span
                                    className={`shrink-0 border px-1 py-0.5 text-[10px] leading-none ${
                                      rowSelected
                                        ? "border-selected-foreground/30 text-selected-foreground/85"
                                        : "border-border bg-surface-sunken text-muted-foreground"
                                    }`}
                                  >
                                    {m.processExited}
                                  </span>
                                )}
                                {ev.ownership === "managed" && (
                                  <span
                                    className={`shrink-0 border px-1 py-0.5 text-[10px] font-medium leading-none ${
                                      rowSelected
                                        ? "border-selected-foreground/30 bg-selected-foreground/15 text-selected-foreground"
                                        : "border-success/30 bg-success-surface text-success"
                                    }`}
                                  >
                                    YouRun
                                  </span>
                                )}
                              </div>
                              <div
                                className={`truncate font-mono text-[10px] tabular-nums ${
                                  rowSelected
                                    ? "text-selected-foreground/75"
                                    : "text-muted-foreground"
                                }`}
                              >
                                {[
                                  formatObservedTime(ev.peakAt, locale),
                                  ev.ownership === "managed" && ev.projectName
                                    ? [ev.projectName, formatWorkspaceName(ev.workspaceName, m)]
                                        .filter(Boolean)
                                        .join(" · ")
                                    : `PID ${ev.pid}`,
                                ]
                                  .filter(Boolean)
                                  .join(" · ")}
                              </div>
                            </div>
                          </div>
                          <div className="shrink-0 text-right font-mono tabular-nums">
                            <div
                              className={`text-xs font-semibold ${
                                rowSelected ? "text-selected-foreground" : "text-foreground"
                              }`}
                            >
                              {primaryMetric}
                            </div>
                            <div
                              className={`text-[10px] ${
                                rowSelected
                                  ? "text-selected-foreground/75"
                                  : "text-muted-foreground"
                              }`}
                            >
                              {secondaryMetric}
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </section>
              ))}
            </div>
          )}

          {state.subview === "home" && bountyMode === "watch" && (
            <div className="flex flex-col border-b border-border">
              <div className="flex h-8 shrink-0 items-center justify-between border-b border-border bg-surface-sunken px-3 text-xs">
                <span className="font-medium text-foreground">
                  {m.bountyModeWatch} ({bountyData.watches.length})
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  onClick={() => openWatchDialogFor()}
                  aria-label={m.bountyAddWatch}
                  title={m.bountyAddWatch}
                >
                  <Plus className="size-4" aria-hidden="true" />
                </Button>
              </div>
              {bountyData.watches.length === 0 ? (
                <div className="flex flex-col items-center justify-center gap-2.5 px-4 py-12 text-center text-xs text-muted-foreground">
                  <span>{m.bountyNoWatches}</span>
                  <Button
                    type="button"
                    variant="outline"
                    size="icon-sm"
                    onClick={() => openWatchDialogFor()}
                    aria-label={m.bountyAddWatch}
                    title={m.bountyAddWatch}
                  >
                    <Plus className="size-4" aria-hidden="true" />
                  </Button>
                </div>
              ) : (
                <div className="divide-y divide-border/60">
                  {bountyData.watches.map((w) => {
                    const lastPeakLabel =
                      w.lastPeakValue != null && w.lastPeakValue > 0
                        ? w.category === "network"
                          ? w.conditionKind === "cumulative"
                            ? formatMemory(w.lastPeakValue)
                            : formatNetworkRate(w.lastPeakValue, false, m)
                          : w.category === "memory"
                            ? formatMemory(w.lastPeakValue)
                            : `${w.lastPeakValue.toFixed(1)}%`
                        : "";
                    return (
                      <div
                        key={w.id}
                        className="flex min-h-13 items-center justify-between gap-3 px-3 py-2 text-xs hover:bg-surface-sunken/50"
                      >
                        <div className="flex min-w-0 flex-1 items-center gap-2.5">
                          <Checkbox
                            checked={w.enabled}
                            onCheckedChange={(checked) =>
                              void handleToggleWatch(w.id, checked === true)
                            }
                            aria-label={w.title || w.targetName}
                          />
                          <ProcessIcon name={w.targetName} displayName={w.title || w.targetName} />
                          <div className="min-w-0 flex-1">
                            <div className="flex flex-wrap items-center gap-1.5">
                              <span
                                className={`truncate font-medium ${
                                  w.enabled ? "text-foreground" : "text-muted-foreground line-through"
                                }`}
                              >
                                {w.title || w.targetName}
                              </span>
                              <span className="border border-border bg-surface-sunken px-1.5 py-0.5 font-mono text-[10px] text-foreground-secondary">
                                {formatWatchRuleSummary(w, m)}
                              </span>
                              {w.unreadHit && (
                                <span className="border border-warning/40 bg-warning-surface px-1.5 py-0.5 text-[10px] font-medium text-warning-foreground">
                                  {m.bountyAlertBanner.replace("%s", "").trim() || "!"}
                                </span>
                              )}
                            </div>
                            <div className="mt-0.5 truncate font-mono text-[10px] tabular-nums text-muted-foreground">
                              {w.hitCount > 0
                                ? [
                                    m.bountyHitCount.replace("%d", String(w.hitCount)),
                                    lastPeakLabel ? `${m.bountyPeakPrefix} ${lastPeakLabel}` : "",
                                    w.lastTriggeredAt
                                      ? formatObservedTime(w.lastTriggeredAt, locale)
                                      : "",
                                  ]
                                    .filter(Boolean)
                                    .join(" · ")
                                : w.enabled
                                  ? m.bountyWatchActive
                                  : m.bountyWatchPaused}
                            </div>
                          </div>
                        </div>
                        <div className="flex shrink-0 items-center gap-1">
                          {w.unreadHit && (
                            <Button
                              type="button"
                              variant="outline"
                              size="xs"
                              onClick={() => void handleAckAlerts(w.id)}
                            >
                              {m.bountyAcknowledge}
                            </Button>
                          )}
                          <Button
                            type="button"
                            variant="ghost"
                            size="icon-sm"
                            className="size-7 text-muted-foreground hover:text-destructive"
                            onClick={() => void handleDeleteWatch(w.id)}
                            title={m.remove}
                            aria-label={m.remove}
                          >
                            <Trash2 className="size-3.5" aria-hidden="true" />
                          </Button>
                        </div>
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          )}

          {state.subview === "processes" && (
            <table className="w-max min-w-full shrink-0 table-auto border-collapse whitespace-nowrap text-left text-xs">
              <thead className="sticky top-0 z-10 border-b border-border bg-surface-sunken text-muted-foreground">
                <tr className="h-8">
                  <ResourceSelectAll items={processGroups.flatMap(group => group.items)} />
                  <th className="px-3 py-1.5 font-medium">{m.colProcess}</th>
                  <th className="w-18 px-2 py-1.5 text-right font-medium">{m.colPid}</th>
                  <th className="w-20 px-2 py-1.5 text-right font-medium">
                    <button
                      type="button"
                      className="ml-auto inline-flex items-center justify-end gap-1 hover:text-foreground"
                      onClick={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          processes: {
                            ...prev.processes,
                            sortBy: "cpu",
                            sortDesc:
                              prev.processes.sortBy === "cpu" ? !prev.processes.sortDesc : true,
                          },
                        }))
                      }
                    >
                      {m.colCpu}
                      {state.processes.sortBy === "cpu" &&
                        (state.processes.sortDesc ? (
                          <ArrowDown className="size-3" />
                        ) : (
                          <ArrowUp className="size-3" />
                        ))}
                    </button>
                  </th>
                  <th className="w-24 px-2 py-1.5 text-right font-medium">
                    <button
                      type="button"
                      className="ml-auto inline-flex items-center justify-end gap-1 hover:text-foreground"
                      onClick={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          processes: {
                            ...prev.processes,
                            sortBy: "memory",
                            sortDesc:
                              prev.processes.sortBy === "memory"
                                ? !prev.processes.sortDesc
                                : true,
                          },
                        }))
                      }
                    >
                      {m.colMemory}
                      {state.processes.sortBy === "memory" &&
                        (state.processes.sortDesc ? (
                          <ArrowDown className="size-3" />
                        ) : (
                          <ArrowUp className="size-3" />
                        ))}
                    </button>
                  </th>
                  <th className="w-20 px-2 py-1.5 text-right font-medium">
                    <button
                      type="button"
                      className="ml-auto inline-flex items-center justify-end gap-1 hover:text-foreground"
                      onClick={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          processes: {
                            ...prev.processes,
                            sortBy: "gpu",
                            sortDesc:
                              prev.processes.sortBy === "gpu" ? !prev.processes.sortDesc : true,
                          },
                        }))
                      }
                    >
                      {m.colGpu}
                      {state.processes.sortBy === "gpu" &&
                        (state.processes.sortDesc ? (
                          <ArrowDown className="size-3" />
                        ) : (
                          <ArrowUp className="size-3" />
                        ))}
                    </button>
                  </th>
                  <th className="w-24 px-2 py-1.5 text-right font-medium">
                    <button
                      type="button"
                      className="ml-auto inline-flex items-center justify-end gap-1 hover:text-foreground"
                      onClick={() =>
                        onStateChange((prev) => ({
                          ...prev,
                          processes: {
                            ...prev.processes,
                            sortBy: "network",
                            sortDesc:
                              prev.processes.sortBy === "network"
                                ? !prev.processes.sortDesc
                                : true,
                          },
                        }))
                      }
                    >
                      {m.colNetwork}
                      {state.processes.sortBy === "network" &&
                        (state.processes.sortDesc ? (
                          <ArrowDown className="size-3" />
                        ) : (
                          <ArrowUp className="size-3" />
                        ))}
                    </button>
                  </th>
                  <th className="w-44 px-3 py-1.5 font-medium">{m.colProjectWorkspace}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/60">
                {processGroups.flatMap((group) => {
                  if (group.items.length === 1) {
                    const item = group.primary;
                    const selected = selectedProcess?.pid === item.pid;
                    return [
                      <ResourceRow items={[item]}
                        key={`proc-${item.pid}`}
                        onClick={() => {
                          onStateChange((prev) => ({
                            ...prev,
                            processes: { ...prev.processes, selectedPid: item.pid },
                          }));
                          setNarrowDetailOpen(true);
                        }}
                        className={`h-9 cursor-pointer select-none transition-colors ${
                          selected
                            ? "bg-selected text-selected-foreground"
                            : "hover:bg-surface-sunken/70"
                        }`}
                      >
                        <td className="px-3 py-1.5">
                          <div className="flex min-w-0 items-center gap-2">
                            <span className="size-4 shrink-0" aria-hidden="true" />
                            <ProcessIcon
                              name={item.name}
                              displayName={item.displayName}
                              icon={item.icon}
                            />
                            <span className="shrink-0 font-medium">
                              {friendlyProcessLabel(item.name, item.displayName)}
                            </span>
                            {item.ports && item.ports.length > 0 && (
                              <span
                                className={`shrink-0 font-mono text-[11px] ${
                                  selected
                                    ? "text-selected-foreground/75"
                                    : "text-muted-foreground"
                                }`}
                              >
                                :{item.ports[0]}
                              </span>
                            )}
                          </div>
                        </td>
                        <td
                          className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                            selected ? "text-selected-foreground/90" : "text-muted-foreground"
                          }`}
                        >
                          {item.pid}
                        </td>
                        <td
                          className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                            selected ? "text-selected-foreground/90" : "text-muted-foreground"
                          }`}
                        >
                          {item.cpuSampling
                            ? m.cpuSampling
                            : item.cpuPercent != null
                              ? `${item.cpuPercent.toFixed(1)}%`
                              : "--"}
                        </td>
                        <td
                          className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                            selected ? "text-selected-foreground/90" : "text-muted-foreground"
                          }`}
                        >
                          {formatMemory(item.memoryBytes)}
                        </td>
                        <td
                          className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                            selected ? "text-selected-foreground/90" : "text-muted-foreground"
                          }`}
                        >
                          {item.gpuSampling
                            ? m.cpuSampling
                            : item.gpuPercent != null
                              ? `${item.gpuPercent.toFixed(1)}%`
                              : "--"}
                        </td>
                        <td
                          className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                            selected ? "text-selected-foreground/90" : "text-muted-foreground"
                          }`}
                        >
                          {formatNetworkRate(item.networkBytesPerSec, item.networkSampling, m)}
                        </td>
                        <td className="px-3 py-1.5">
                          {renderProjectCell(
                            item.ownership,
                            item.projectName,
                            item.workspaceName,
                            selected,
                          )}
                        </td>
                      </ResourceRow>,
                    ];
                  }

                  const hasQuery = Boolean(state.processes.query.trim());
                  const isExpanded = Boolean(expandedGroups[group.key]) || hasQuery;
                  const containsSelected = group.items.some(
                    (i) => i.pid === selectedProcess?.pid,
                  );
                  const groupRowSelected = !isExpanded && containsSelected;

                  const groupHeaderRow = (
                    <ResourceRow items={group.items}
                      key={`group-${group.key}`}
                      onClick={() => {
                        if (containsSelected) {
                          toggleGroup(group.key);
                        } else {
                          onStateChange((prev) => ({
                            ...prev,
                            processes: { ...prev.processes, selectedPid: group.primary.pid },
                          }));
                        }
                        setNarrowDetailOpen(true);
                      }}
                      onDoubleClick={() => toggleGroup(group.key)}
                      className={`h-9 cursor-pointer select-none transition-colors ${
                        groupRowSelected
                          ? "bg-selected text-selected-foreground"
                          : "hover:bg-surface-sunken/70"
                      }`}
                    >
                      <td className="px-3 py-1.5">
                        <div className="flex min-w-0 items-center gap-2">
                          <button
                            type="button"
                            aria-expanded={isExpanded}
                            aria-label={group.title}
                            onClick={(e) => {
                              e.stopPropagation();
                              toggleGroup(group.key);
                            }}
                            className={`inline-flex size-4 shrink-0 items-center justify-center rounded-[2px] ${
                              groupRowSelected
                                ? "text-selected-foreground hover:bg-selected-foreground/15"
                                : "text-muted-foreground hover:bg-accent hover:text-foreground"
                            }`}
                          >
                            {isExpanded ? (
                              <ChevronDown className="size-3.5" aria-hidden="true" />
                            ) : (
                              <ChevronRight className="size-3.5" aria-hidden="true" />
                            )}
                          </button>
                          <ProcessIcon
                            name={group.primary.name}
                            displayName={group.title}
                            icon={group.icon}
                          />
                          <span className="shrink-0 font-medium">{group.title}</span>
                          <span
                            className={`shrink-0 tabular-nums ${
                              groupRowSelected
                                ? "text-selected-foreground/75"
                                : "text-muted-foreground"
                            }`}
                          >
                            ({group.items.length})
                          </span>
                        </div>
                      </td>
                      <td
                        className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                          groupRowSelected
                            ? "text-selected-foreground/75"
                            : "text-muted-foreground"
                        }`}
                      >
                        --
                      </td>
                      <td
                        className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                          groupRowSelected
                            ? "text-selected-foreground/90"
                            : "text-muted-foreground"
                        }`}
                      >
                        {group.cpuSampling
                          ? m.cpuSampling
                          : group.totalCpu != null
                            ? `${group.totalCpu.toFixed(1)}%`
                            : "--"}
                      </td>
                      <td
                        className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                          groupRowSelected
                            ? "text-selected-foreground/90"
                            : "text-muted-foreground"
                        }`}
                      >
                        {formatMemory(group.totalMemory)}
                      </td>
                      <td
                        className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                          groupRowSelected
                            ? "text-selected-foreground/90"
                            : "text-muted-foreground"
                        }`}
                      >
                        {group.gpuSampling
                          ? m.cpuSampling
                          : group.totalGpu != null
                            ? `${group.totalGpu.toFixed(1)}%`
                            : "--"}
                      </td>
                      <td
                        className={`px-2 py-1.5 text-right font-mono tabular-nums ${
                          groupRowSelected
                            ? "text-selected-foreground/90"
                            : "text-muted-foreground"
                        }`}
                      >
                        {formatNetworkRate(group.totalNetwork, group.networkSampling, m)}
                      </td>
                      <td className="px-3 py-1.5">
                        {renderProjectCell(
                          group.ownership,
                          group.projectName,
                          group.workspaceName,
                          groupRowSelected,
                        )}
                      </td>
                    </ResourceRow>
                  );

                  if (!isExpanded) {
                    return [groupHeaderRow];
                  }

                  const childRows = group.items.map((item) => {
                    const childSelected = selectedProcess?.pid === item.pid;
                    return (
                      <ResourceRow items={[item]}
                        key={`proc-${item.pid}`}
                        onClick={() => {
                          onStateChange((prev) => ({
                            ...prev,
                            processes: { ...prev.processes, selectedPid: item.pid },
                          }));
                          setNarrowDetailOpen(true);
                        }}
                        className={`h-8 cursor-pointer select-none transition-colors ${
                          childSelected
                            ? "bg-selected text-selected-foreground"
                            : "bg-surface-sunken/20 hover:bg-surface-sunken/70"
                        }`}
                      >
                        <td className="px-3 py-1">
                          <div className="flex min-w-0 items-center gap-2 pl-6">
                            <ProcessIcon
                              name={item.name}
                              displayName={item.displayName}
                              icon={item.icon || group.icon}
                            />
                            <span className="shrink-0">{item.name}</span>
                            {item.ports && item.ports.length > 0 && (
                              <span
                                className={`shrink-0 font-mono text-[11px] ${
                                  childSelected
                                    ? "text-selected-foreground/75"
                                    : "text-muted-foreground"
                                }`}
                              >
                                :{item.ports[0]}
                              </span>
                            )}
                          </div>
                        </td>
                        <td
                          className={`px-2 py-1 text-right font-mono tabular-nums ${
                            childSelected
                              ? "text-selected-foreground/90"
                              : "text-muted-foreground"
                          }`}
                        >
                          {item.pid}
                        </td>
                        <td
                          className={`px-2 py-1 text-right font-mono tabular-nums ${
                            childSelected
                              ? "text-selected-foreground/90"
                              : "text-muted-foreground"
                          }`}
                        >
                          {item.cpuSampling
                            ? m.cpuSampling
                            : item.cpuPercent != null
                              ? `${item.cpuPercent.toFixed(1)}%`
                              : "--"}
                        </td>
                        <td
                          className={`px-2 py-1 text-right font-mono tabular-nums ${
                            childSelected
                              ? "text-selected-foreground/90"
                              : "text-muted-foreground"
                          }`}
                        >
                          {formatMemory(item.memoryBytes)}
                        </td>
                        <td
                          className={`px-2 py-1 text-right font-mono tabular-nums ${
                            childSelected
                              ? "text-selected-foreground/90"
                              : "text-muted-foreground"
                          }`}
                        >
                          {item.gpuSampling
                            ? m.cpuSampling
                            : item.gpuPercent != null
                              ? `${item.gpuPercent.toFixed(1)}%`
                              : "--"}
                        </td>
                        <td
                          className={`px-2 py-1 text-right font-mono tabular-nums ${
                            childSelected
                              ? "text-selected-foreground/90"
                              : "text-muted-foreground"
                          }`}
                        >
                          {formatNetworkRate(item.networkBytesPerSec, item.networkSampling, m)}
                        </td>
                        <td className="px-3 py-1">
                          {renderProjectCell(
                            item.ownership,
                            item.projectName,
                            item.workspaceName,
                            childSelected,
                          )}
                        </td>
                      </ResourceRow>
                    );
                  });

                  return [groupHeaderRow, ...childRows];
                })}
                {filteredProcesses.length === 0 && !loading && (
                  <tr>
                    <td colSpan={8} className="px-4 py-12 text-center text-muted-foreground">
                      {m.noProcessesMatch}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}

          {state.subview === "ports" && (
            <>
              {filteredPorts.length === 0 && !loading && isReleasedOrigin ? (
                <div className="flex flex-1 flex-col items-center justify-center gap-2 p-8 text-center">
                  <span className="border border-success/30 bg-success-surface px-2 py-0.5 text-xs font-medium text-success">
                    {m.noPortOccupancy}
                  </span>
                  <p className="text-sm font-medium">
                    {state.ports.query.trim()
                      ? `${state.ports.query.trim().startsWith(":") ? state.ports.query.trim() : `:${state.ports.query.trim()}`} · ${m.noPortOccupancy}`
                      : m.noPortOccupancy}
                  </p>
                  <Button variant="outline" size="sm" onClick={handleRefresh}>
                    {m.refresh}
                  </Button>
                </div>
              ) : (
                <table className="w-max min-w-full shrink-0 table-auto border-collapse whitespace-nowrap text-left text-xs">
                  <thead className="sticky top-0 z-10 border-b border-border bg-surface-sunken text-muted-foreground">
                    <tr className="h-8">
                      <ResourceSelectAll items={filteredPorts} />
                      <th className="w-40 px-3 py-1.5 font-medium">{m.colLocalAddress}</th>
                      <th className="w-24 px-2.5 py-1.5 font-medium">{m.colProtocol}</th>
                      <th className="w-24 px-2.5 py-1.5 font-medium">{m.colState}</th>
                      <th className="min-w-48 px-3 py-1.5 font-medium">{m.colProcessPid}</th>
                      <th className="w-52 px-3 py-1.5 font-medium">{m.colProjectWorkspace}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border/60">
                    {filteredPorts.map((item) => {
                      const selected = selectedPort?.id === item.id;
                      return (
                        <ResourceRow items={[item]}
                          key={item.id}
                          onClick={() => {
                            onStateChange((prev) => ({
                              ...prev,
                              ports: { ...prev.ports, selectedId: item.id },
                            }));
                            setNarrowDetailOpen(true);
                          }}
                          className={`h-9 cursor-pointer select-none transition-colors ${
                            selected
                              ? "bg-selected text-selected-foreground"
                              : "hover:bg-surface-sunken/70"
                          }`}
                        >
                          <td className="px-3 py-1.5 font-mono font-medium">
                            {item.localAddress}
                          </td>
                          <td
                            className={`px-2.5 py-1.5 font-mono ${
                              selected ? "text-selected-foreground/85" : "text-muted-foreground"
                            }`}
                          >
                            {item.protocolLabel}
                          </td>
                          <td className="px-2.5 py-1.5">
                            <span
                              className={`inline-flex items-center border px-1.5 py-0.5 text-[11px] leading-none ${
                                selected
                                  ? "border-selected-foreground/30 text-selected-foreground"
                                  : "border-border bg-surface-sunken text-foreground-secondary"
                              }`}
                            >
                              {portStateLabel(item.state, m)}
                            </span>
                          </td>
                          <td className="px-3 py-1.5">
                            {item.processExited ? (
                              <span
                                className={
                                  selected
                                    ? "text-selected-foreground/75"
                                    : "text-muted-foreground"
                                }
                              >
                                {m.processExited} · {item.pid}
                              </span>
                            ) : (
                              <div className="flex min-w-0 items-center gap-2">
                                <ProcessIcon
                                  name={item.processName}
                                  displayName={item.processDisplayName}
                                  icon={item.processIcon}
                                />
                                <span className="truncate font-medium">
                                  {friendlyProcessLabel(item.processName, item.processDisplayName)}
                                </span>
                                <span
                                  className={`shrink-0 font-mono tabular-nums ${
                                    selected
                                      ? "text-selected-foreground/75"
                                      : "text-muted-foreground"
                                  }`}
                                >
                                  {item.pid}
                                </span>
                              </div>
                            )}
                          </td>
                          <td className="px-3 py-1.5">
                            {renderProjectCell(
                              item.ownership,
                              item.projectName,
                              item.workspaceName,
                              selected,
                            )}
                          </td>
                        </ResourceRow>
                      );
                    })}
                    {filteredPorts.length === 0 && !loading && (
                      <tr>
                        <td colSpan={6} className="px-4 py-12 text-center text-muted-foreground">
                          {m.noPortOccupancy}
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              )}
            </>
          )}

          {state.subview === "files" && (
            <>
              <table className="w-max min-w-full shrink-0 table-auto border-collapse whitespace-nowrap text-left text-xs">
                <thead className="sticky top-0 z-10 border-b border-border bg-surface-sunken text-muted-foreground">
                  <tr className="h-8">
                    <ResourceSelectAll items={fileData.items} />
                    <th className="min-w-48 px-3 py-1.5 font-medium">{m.colHolderProcess}</th>
                    <th className="w-20 min-w-20 px-2.5 py-1.5 text-right font-medium">{m.colPid}</th>
                    <th className="w-36 min-w-36 px-2.5 py-1.5 font-medium">{m.colUser}</th>
                    <th className="w-28 min-w-28 px-2.5 py-1.5 font-medium">{m.colFileRef}</th>
                    <th className="w-52 min-w-52 px-3 py-1.5 font-medium">{m.colOwnership}</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/60">
                  {fileData.items.map((item) => {
                    const selected = selectedFileHandle?.id === item.id;
                    return (
                      <ResourceRow items={[item]}
                        key={item.id}
                        onClick={() => {
                          onStateChange((prev) => ({
                            ...prev,
                            files: { ...prev.files, selectedId: item.id },
                          }));
                          setNarrowDetailOpen(true);
                        }}
                        className={`h-9 cursor-pointer select-none transition-colors ${
                          selected
                            ? "bg-selected text-selected-foreground"
                            : "hover:bg-surface-sunken/70"
                        }`}
                      >
                        <td className="px-3 py-1.5">
                          <div className="flex min-w-0 items-center gap-2">
                            <ProcessIcon
                              name={item.processName}
                              displayName={item.processDisplayName}
                              icon={item.processIcon}
                            />
                            <span className="truncate font-medium">
                              {item.processName
                                ? friendlyProcessLabel(item.processName, item.processDisplayName)
                                : m.processExited}
                            </span>
                          </div>
                        </td>
                        <td
                          className={`px-2.5 py-1.5 text-right font-mono tabular-nums ${
                            selected ? "text-selected-foreground/90" : "text-muted-foreground"
                          }`}
                        >
                          {item.pid}
                        </td>
                        <td
                          className={`truncate px-2.5 py-1.5 font-mono ${
                            selected ? "text-selected-foreground/85" : "text-muted-foreground"
                          }`}
                        >
                          {item.user || "--"}
                        </td>
                        <td className="px-2.5 py-1.5">
                          <span
                            className={`inline-flex items-center border px-1.5 py-0.5 text-[11px] leading-none ${
                              selected
                                ? "border-selected-foreground/30 text-selected-foreground"
                                : "border-border bg-surface-sunken text-foreground-secondary"
                            }`}
                          >
                            {m.refOpenFile}
                          </span>
                        </td>
                        <td className="px-3 py-1.5">
                          {renderProjectCell(
                            item.ownership,
                            item.projectName,
                            item.workspaceName,
                            selected,
                          )}
                        </td>
                      </ResourceRow>
                    );
                  })}
                </tbody>
              </table>
              {fileData.items.length === 0 && !loading && (
                <div className="sticky left-0 w-full shrink-0 px-4 py-12 text-center text-xs text-muted-foreground">
                  {fileData.queriedPath ? m.noFileOccupancy : m.enterFileOrFolderPrompt}
                </div>
              )}
            </>
          )}
        </div>

        {/* Right Detail Inspector Pane */}
        <aside
          className={`w-full shrink-0 flex-col overflow-auto border-l border-border bg-surface p-4 text-xs @min-[63rem]/desktop:flex @min-[63rem]/desktop:w-84 ${
            narrowDetailOpen ? "flex" : "hidden"
          }`}
        >
          {narrowDetailOpen && (
            <div className="mb-3 @min-[63rem]/desktop:hidden">
              <Button variant="ghost" size="icon-sm" onClick={() => setNarrowDetailOpen(false)} aria-label={m.back} title={m.back}>
                <ArrowLeft className="size-4" aria-hidden="true" />
              </Button>
            </div>
          )}

          <div className="mb-3"><ResourceActions items={state.subview === "ports" ? selectedPort ? [selectedPort] : [] : state.subview === "files" ? selectedFileHandle ? [selectedFileHandle] : [] : selectedProcess ? [selectedProcess] : []} /></div>
          {state.subview === "home" && bountyMode === "peaks" && selectedPeakEvent && (
            <div className="space-y-4">
              <div className="space-y-2 border-b border-border pb-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <ProcessIcon
                      name={selectedPeakEvent.processName}
                      displayName={selectedPeakEvent.title}
                      icon={selectedPeakEvent.icon}
                      size="md"
                    />
                    <div className="min-w-0">
                      <h3 className="truncate text-sm font-semibold">
                        {selectedPeakEvent.title}
                      </h3>
                      {selectedPeakEvent.processName &&
                        selectedPeakEvent.processName.toLowerCase() !==
                          selectedPeakEvent.title.toLowerCase() && (
                          <div className="truncate font-mono text-[11px] text-muted-foreground">
                            {selectedPeakEvent.processName}
                          </div>
                        )}
                    </div>
                  </div>
                  {ownershipBadge(selectedPeakEvent.ownership, m)}
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-muted-foreground">
                  <span className="font-mono tabular-nums">PID {selectedPeakEvent.pid}</span>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="size-5 text-muted-foreground hover:text-foreground"
                    onClick={() => onCopy(String(selectedPeakEvent.pid))}
                    aria-label={m.copyPid}
                    title={m.copyPid}
                  >
                    <Copy className="size-3" aria-hidden="true" />
                  </Button>
                  <span>·</span>
                  <span>
                    {selectedPeakEvent.processExited ? m.processExited : m.running}
                  </span>
                  <span>·</span>
                  <span className="font-mono tabular-nums">
                    {formatObservedTime(selectedPeakEvent.peakAt, locale)}
                  </span>
                </div>
              </div>

              <div className="grid grid-cols-2 gap-px border border-border bg-border text-xs">
                <div className="bg-surface-sunken/50 px-2.5 py-2">
                  <div className="flex items-center justify-between gap-1 text-[11px] text-muted-foreground">
                    <span>{m.colCpu}</span>
                    {selectedPeakEvent.category === "cpu" &&
                      (selectedPeakEvent.deltaValue ?? 0) > 0 && (
                        <span className="font-mono text-[10px] text-warning-foreground">
                          ↑ +{Math.round(selectedPeakEvent.deltaValue!)}%
                        </span>
                      )}
                  </div>
                  <div className="mt-0.5 font-mono font-semibold tabular-nums">
                    {selectedPeakEvent.cpuPercent != null
                      ? `${selectedPeakEvent.cpuPercent.toFixed(1)}%`
                      : "--"}
                  </div>
                </div>
                <div className="bg-surface-sunken/50 px-2.5 py-2">
                  <div className="flex items-center justify-between gap-1 text-[11px] text-muted-foreground">
                    <span>{m.colMemory}</span>
                    {selectedPeakEvent.category === "memory" &&
                      (selectedPeakEvent.deltaValue ?? 0) > 0 && (
                        <span className="font-mono text-[10px] text-warning-foreground">
                          ↑ +{formatMemory(selectedPeakEvent.deltaValue!)}
                        </span>
                      )}
                  </div>
                  <div className="mt-0.5 font-mono font-semibold tabular-nums">
                    {formatMemory(selectedPeakEvent.memoryBytes ?? 0)}
                  </div>
                </div>
                <div className="bg-surface-sunken/50 px-2.5 py-2">
                  <div className="flex items-center justify-between gap-1 text-[11px] text-muted-foreground">
                    <span>{m.colGpu}</span>
                    {(selectedPeakEvent.gpuMemoryBytes ?? 0) > 0 && (
                      <span className="font-mono text-[10px] tabular-nums">
                        {m.gpuMemoryLabel} {formatMemory(selectedPeakEvent.gpuMemoryBytes!)}
                      </span>
                    )}
                  </div>
                  <div className="mt-0.5 font-mono font-semibold tabular-nums">
                    {selectedPeakEvent.gpuPercent != null
                      ? `${selectedPeakEvent.gpuPercent.toFixed(1)}%`
                      : "--"}
                  </div>
                </div>
                <div className="bg-surface-sunken/50 px-2.5 py-2">
                  <div className="flex items-center justify-between gap-1 text-[11px] text-muted-foreground">
                    <span>{m.colNetwork}</span>
                    {(selectedPeakEvent.cumulativeBytes ?? 0) > 0 && (
                      <span className="font-mono text-[10px] tabular-nums">
                        {m.bountyCumulativePrefix} {formatMemory(selectedPeakEvent.cumulativeBytes!)}
                      </span>
                    )}
                  </div>
                  <div className="mt-0.5 font-mono font-semibold tabular-nums">
                    {formatNetworkRate(selectedPeakEvent.networkBytesPerSec, false, m)}
                  </div>
                </div>
              </div>

              <div className="space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{m.commandText}</span>
                  {selectedPeakEvent.commandLine && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="size-5 text-muted-foreground hover:text-foreground"
                      onClick={() => onCopy(selectedPeakEvent.commandLine!)}
                      aria-label={m.copyCommand}
                      title={m.copyCommand}
                    >
                      <Copy className="size-3" aria-hidden="true" />
                    </Button>
                  )}
                </div>
                <pre className="max-h-36 overflow-auto whitespace-pre-wrap break-all border border-border/60 bg-surface-sunken p-2.5 font-mono text-[11px] leading-5">
                  {renderCommandLineText(
                    selectedPeakEvent.commandLine,
                    selectedPeakEvent.commandLineState,
                  )}
                </pre>
              </div>

              <div className="flex flex-wrap items-center gap-2 pt-1">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 text-xs"
                  onClick={() =>
                    openWatchDialogFor({
                      title: selectedPeakEvent.title,
                      name: selectedPeakEvent.processName,
                      executable: selectedPeakEvent.executable,
                      commandId: selectedPeakEvent.commandId,
                      projectId: selectedPeakEvent.projectId,
                      projectName: selectedPeakEvent.projectName,
                      defaultCategory: selectedPeakEvent.category,
                    })
                  }
                >
                  <Bell className="size-3.5" aria-hidden="true" />
                  {m.bountyAddWatch}
                </Button>
                {selectedPeakEvent.projectId && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      onOpenWorkspace(selectedPeakEvent.projectId!, selectedPeakEvent.workspaceId)
                    }
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.viewWorkspace}
                  </Button>
                )}
                {selectedPeakEvent.executable && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() => openExecutableDirectory(selectedPeakEvent.executable)}
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.openProgramDir}
                  </Button>
                )}
              </div>
            </div>
          )}

          {(state.subview === "home" || state.subview === "processes") &&
            selectedProcess &&
            !(state.subview === "home" && bountyMode === "peaks" && selectedPeakEvent) && (
            <div className="space-y-4">
              <div className="space-y-2 border-b border-border pb-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <ProcessIcon
                      name={selectedProcess.name}
                      displayName={selectedProcess.displayName}
                      icon={selectedProcess.icon || selectedGroup?.icon}
                      size="md"
                    />
                    <div className="min-w-0">
                      <h3 className="truncate text-sm font-semibold">
                        {friendlyProcessLabel(selectedProcess.name, selectedProcess.displayName)}
                      </h3>
                      {selectedProcess.displayName &&
                        selectedProcess.displayName.toLowerCase() !==
                          selectedProcess.name.toLowerCase() && (
                          <div className="truncate font-mono text-[11px] text-muted-foreground">
                            {selectedProcess.name}
                          </div>
                        )}
                    </div>
                  </div>
                  {ownershipBadge(selectedProcess.ownership, m)}
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-muted-foreground">
                  <span className="font-mono tabular-nums">PID {selectedProcess.pid}</span>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="size-5 text-muted-foreground hover:text-foreground"
                    onClick={() => onCopy(String(selectedProcess.pid))}
                    aria-label={m.copyPid}
                    title={m.copyPid}
                  >
                    <Copy className="size-3" aria-hidden="true" />
                  </Button>
                  <span>·</span>
                  <span>{m.running}</span>
                </div>
              </div>

              {(() => {
                const useGroupTotals = Boolean(
                  selectedGroup &&
                    selectedGroup.items.length > 1 &&
                    (state.subview === "home" || !expandedGroups[selectedGroup.key]),
                );
                const cpuVal = useGroupTotals
                  ? selectedGroup!.cpuSampling
                    ? m.cpuSampling
                    : selectedGroup!.totalCpu != null
                      ? `${selectedGroup!.totalCpu.toFixed(1)}%`
                      : "--"
                  : selectedProcess.cpuSampling
                    ? m.cpuSampling
                    : selectedProcess.cpuPercent != null
                      ? `${selectedProcess.cpuPercent.toFixed(1)}%`
                      : "--";
                const memVal = formatMemory(
                  useGroupTotals ? selectedGroup!.totalMemory : selectedProcess.memoryBytes,
                );
                const gpuSampling = useGroupTotals
                  ? selectedGroup!.gpuSampling
                  : selectedProcess.gpuSampling;
                const gpuPct = useGroupTotals
                  ? selectedGroup!.totalGpu
                  : selectedProcess.gpuPercent;
                const gpuMem = useGroupTotals
                  ? selectedGroup!.totalGpuMemory
                  : selectedProcess.gpuMemoryBytes ?? 0;
                const gpuVal = gpuSampling
                  ? m.cpuSampling
                  : gpuPct != null
                    ? `${gpuPct.toFixed(1)}%`
                    : "--";
                const netRate = useGroupTotals
                  ? selectedGroup!.totalNetwork
                  : selectedProcess.networkBytesPerSec;
                const netSampling = useGroupTotals
                  ? selectedGroup!.networkSampling
                  : selectedProcess.networkSampling;
                const netConns = useGroupTotals
                  ? selectedGroup!.totalNetworkConnections
                  : selectedProcess.networkConnections ?? 0;
                const cumNet = selectedGroup?.cumulativeNetBytes ?? 0;
                const netVal = formatNetworkRate(netRate, netSampling, m);

                return (
                  <div className="grid grid-cols-2 gap-px border border-border bg-border text-xs">
                    <div className="bg-surface-sunken/50 px-2.5 py-2">
                      <div className="text-[11px] text-muted-foreground">{m.colCpu}</div>
                      <div className="mt-0.5 font-mono font-semibold tabular-nums">{cpuVal}</div>
                    </div>
                    <div className="bg-surface-sunken/50 px-2.5 py-2">
                      <div className="text-[11px] text-muted-foreground">{m.colMemory}</div>
                      <div className="mt-0.5 font-mono font-semibold tabular-nums">{memVal}</div>
                    </div>
                    <div className="bg-surface-sunken/50 px-2.5 py-2">
                      <div className="flex items-center justify-between gap-1 text-[11px] text-muted-foreground">
                        <span>{m.colGpu}</span>
                        {gpuMem > 0 && (
                          <span className="font-mono text-[10px] tabular-nums">
                            {m.gpuMemoryLabel} {formatMemory(gpuMem)}
                          </span>
                        )}
                      </div>
                      <div className="mt-0.5 font-mono font-semibold tabular-nums">{gpuVal}</div>
                    </div>
                    <div className="bg-surface-sunken/50 px-2.5 py-2">
                      <div className="flex items-center justify-between gap-1 text-[11px] text-muted-foreground">
                        <span>{m.colNetwork}</span>
                        {cumNet > 0 ? (
                          <span className="font-mono text-[10px] tabular-nums">
                            {m.bountyCumulativePrefix} {formatMemory(cumNet)}
                          </span>
                        ) : (
                          netConns > 0 && (
                            <span className="font-mono text-[10px] tabular-nums">
                              {netConns} {m.networkConnectionsUnit}
                            </span>
                          )
                        )}
                      </div>
                      <div className="mt-0.5 font-mono font-semibold tabular-nums">{netVal}</div>
                    </div>
                  </div>
                );
              })()}

              <dl className="space-y-2">
                {selectedProcess.runId && (
                  <div className="flex justify-between gap-2">
                    <dt className="shrink-0 text-muted-foreground">{m.associatedRun}</dt>
                    <dd className="truncate text-right font-mono">
                      #{selectedProcess.runId} · {selectedProcess.commandTitle || ""}
                    </dd>
                  </div>
                )}
                {selectedProcess.ownership === "managed" && (
                  <>
                    <div className="flex justify-between gap-2">
                      <dt className="shrink-0 text-muted-foreground">{m.colProjectWorkspace}</dt>
                      <dd className="truncate text-right font-medium">
                        {[
                          selectedProcess.projectName || m.independent,
                          formatWorkspaceName(selectedProcess.workspaceName, m),
                        ]
                          .filter(Boolean)
                          .join(" / ")}
                      </dd>
                    </div>
                    <div className="flex justify-between gap-2">
                      <dt className="shrink-0 text-muted-foreground">{m.launchTimeSource}</dt>
                      <dd className="truncate text-right font-mono">
                        {formatSourceSummary(selectedProcess.launchSource, m)}
                      </dd>
                    </div>
                    {selectedProcess.directory && (
                      <div className="space-y-1">
                        <dt className="text-muted-foreground">{m.workspaceLabel}</dt>
                        <dd className="break-all font-mono text-[11px] text-foreground-secondary">
                          {selectedProcess.directory}
                        </dd>
                      </div>
                    )}
                  </>
                )}
                {selectedProcess.ports && selectedProcess.ports.length > 0 && (
                  <div className="flex items-center justify-between gap-2">
                    <dt className="shrink-0 text-muted-foreground">{m.proPorts}</dt>
                    <dd className="flex flex-wrap justify-end gap-1 font-mono">
                      {selectedProcess.ports.map((p) => (
                        <button
                          key={p}
                          type="button"
                          onClick={() =>
                            onStateChange((prev) => ({
                              ...prev,
                              subview: "ports",
                              ports: { ...prev.ports, query: `:${p}` },
                            }))
                          }
                          className="inline-flex items-center border border-border bg-surface-sunken px-1.5 py-0.5 text-[11px] text-foreground transition-colors hover:bg-accent"
                        >
                          :{p}
                        </button>
                      ))}
                    </dd>
                  </div>
                )}
              </dl>

              <div className="space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{m.commandText}</span>
                  {selectedProcess.commandLine && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="size-5 text-muted-foreground hover:text-foreground"
                      onClick={() => onCopy(selectedProcess.commandLine!)}
                      aria-label={m.copyCommand}
                      title={m.copyCommand}
                    >
                      <Copy className="size-3" aria-hidden="true" />
                    </Button>
                  )}
                </div>
                <pre className="max-h-36 overflow-auto whitespace-pre-wrap break-all border border-border/60 bg-surface-sunken p-2.5 font-mono text-[11px] leading-5">
                  {renderCommandLineText(
                    selectedProcess.commandLine,
                    selectedProcess.commandLineState,
                  )}
                </pre>
              </div>

              <div className="flex flex-wrap items-center gap-2 pt-1">
                <Button
                  variant="outline"
                  size="sm"
                  className="h-8 text-xs"
                  onClick={() =>
                    openWatchDialogFor({
                      title:
                        selectedGroup?.title ||
                        friendlyProcessLabel(selectedProcess.name, selectedProcess.displayName),
                      name: selectedProcess.name,
                      executable: selectedProcess.executable,
                      commandId: selectedProcess.commandId,
                      projectId: selectedProcess.projectId,
                      projectName: selectedProcess.projectName,
                      defaultCategory:
                        (selectedProcess.networkBytesPerSec ?? 0) > 0 ? "network" : "memory",
                    })
                  }
                >
                  <Bell className="size-3.5" aria-hidden="true" />
                  {m.bountyAddWatch}
                </Button>
                {selectedProcess.runId && (
                  <Button
                    size="icon-sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      onOpenRun(selectedProcess.runId!, {
                        subview: state.subview,
                        label: `${selectedProcess.name} (PID ${selectedProcess.pid})`,
                        runId: selectedProcess.runId!,
                        runTitle: selectedProcess.commandTitle || selectedProcess.name,
                        projectId: selectedProcess.projectId,
                        projectName: selectedProcess.projectName,
                        workspaceId: selectedProcess.workspaceId,
                        workspaceName: selectedProcess.workspaceName,
                        pid: selectedProcess.pid,
                        reviewStatus: "active",
                      })
                    }
                    aria-label={m.viewRun}
                    title={m.viewRun}
                  >
                    <FileText className="size-4" aria-hidden="true" />
                  </Button>
                )}
                {selectedProcess.projectId && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      onOpenWorkspace(selectedProcess.projectId!, selectedProcess.workspaceId)
                    }
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.viewWorkspace}
                  </Button>
                )}
                {!selectedProcess.runId && selectedProcess.executable && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() => openExecutableDirectory(selectedProcess.executable)}
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.openProgramDir}
                  </Button>
                )}
              </div>
            </div>
          )}

          {state.subview === "ports" && selectedPort && (
            <div className="space-y-4">
              <div className="space-y-2 border-b border-border pb-3">
                <div className="flex items-start justify-between gap-2">
                  <h3 className="font-mono text-sm font-semibold">
                    {selectedPort.localAddress} · {selectedPort.protocolLabel}
                  </h3>
                  {ownershipBadge(selectedPort.ownership, m)}
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-muted-foreground">
                  <span>{portStateLabel(selectedPort.state, m)}</span>
                  <span>·</span>
                  <ProcessIcon
                    name={selectedPort.processName}
                    displayName={selectedPort.processDisplayName}
                    icon={selectedPort.processIcon}
                  />
                  <span className="font-medium text-foreground-secondary">
                    {friendlyProcessLabel(
                      selectedPort.processName,
                      selectedPort.processDisplayName,
                    ) || m.processExited}
                  </span>
                  <span className="font-mono tabular-nums">(PID {selectedPort.pid})</span>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="size-5 text-muted-foreground hover:text-foreground"
                    onClick={() => onCopy(String(selectedPort.pid))}
                    aria-label={m.copyPid}
                    title={m.copyPid}
                  >
                    <Copy className="size-3" aria-hidden="true" />
                  </Button>
                </div>
              </div>

              <dl className="space-y-2">
                {selectedPort.runId && (
                  <div className="flex justify-between gap-2">
                    <dt className="shrink-0 text-muted-foreground">{m.associatedRun}</dt>
                    <dd className="truncate text-right font-mono">
                      #{selectedPort.runId} · {selectedPort.commandTitle || ""}
                    </dd>
                  </div>
                )}
                {selectedPort.ownership === "managed" && (
                  <>
                    <div className="flex justify-between gap-2">
                      <dt className="shrink-0 text-muted-foreground">{m.colProjectWorkspace}</dt>
                      <dd className="truncate text-right font-medium">
                        {[
                          selectedPort.projectName || m.independent,
                          formatWorkspaceName(selectedPort.workspaceName, m),
                        ]
                          .filter(Boolean)
                          .join(" / ")}
                      </dd>
                    </div>
                    <div className="flex justify-between gap-2">
                      <dt className="shrink-0 text-muted-foreground">{m.launchTimeSource}</dt>
                      <dd className="truncate text-right font-mono">
                        {formatSourceSummary(selectedPort.launchSource, m)}
                      </dd>
                    </div>
                    {selectedPort.directory && (
                      <div className="space-y-1">
                        <dt className="text-muted-foreground">{m.workspaceLabel}</dt>
                        <dd className="break-all font-mono text-[11px] text-foreground-secondary">
                          {selectedPort.directory}
                        </dd>
                      </div>
                    )}
                  </>
                )}
              </dl>

              <div className="space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{m.commandText}</span>
                  {selectedPort.commandLine && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="size-5 text-muted-foreground hover:text-foreground"
                      onClick={() => onCopy(selectedPort.commandLine!)}
                      aria-label={m.copyCommand}
                      title={m.copyCommand}
                    >
                      <Copy className="size-3" aria-hidden="true" />
                    </Button>
                  )}
                </div>
                <pre className="max-h-36 overflow-auto whitespace-pre-wrap break-all border border-border/60 bg-surface-sunken p-2.5 font-mono text-[11px] leading-5">
                  {renderCommandLineText(
                    selectedPort.commandLine,
                    selectedPort.commandLineState,
                  )}
                </pre>
              </div>

              <div className="flex flex-wrap items-center gap-2 pt-1">
                {selectedPort.runId && (
                  <Button
                    size="icon-sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      onOpenRun(selectedPort.runId!, {
                        subview: "ports",
                        label: `:${selectedPort.port}`,
                        runId: selectedPort.runId!,
                        runTitle: selectedPort.commandTitle || selectedPort.processName,
                        projectId: selectedPort.projectId,
                        projectName: selectedPort.projectName,
                        workspaceId: selectedPort.workspaceId,
                        workspaceName: selectedPort.workspaceName,
                        portQuery: `:${selectedPort.port}`,
                        pid: selectedPort.pid,
                        reviewStatus: "active",
                      })
                    }
                    aria-label={m.viewRun}
                    title={m.viewRun}
                  >
                    <FileText className="size-4" aria-hidden="true" />
                  </Button>
                )}
                {selectedPort.projectId && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      onOpenWorkspace(selectedPort.projectId!, selectedPort.workspaceId)
                    }
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.viewWorkspace}
                  </Button>
                )}
                {!selectedPort.runId && selectedPort.executable && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() => openExecutableDirectory(selectedPort.executable)}
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.openProgramDir}
                  </Button>
                )}
              </div>
            </div>
          )}

          {state.subview === "files" && selectedFileHandle && (
            <div className="space-y-4">
              <div className="space-y-2 border-b border-border pb-3">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <ProcessIcon
                      name={selectedFileHandle.processName}
                      displayName={selectedFileHandle.processDisplayName}
                      icon={selectedFileHandle.processIcon}
                      size="md"
                    />
                    <h3 className="truncate text-sm font-semibold">
                      {selectedFileHandle.processName
                        ? friendlyProcessLabel(
                            selectedFileHandle.processName,
                            selectedFileHandle.processDisplayName,
                          )
                        : m.processExited}
                    </h3>
                  </div>
                  {ownershipBadge(selectedFileHandle.ownership, m)}
                </div>
                <div className="flex flex-wrap items-center gap-1.5 text-muted-foreground">
                  <span className="font-mono tabular-nums">PID {selectedFileHandle.pid}</span>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    className="size-5 text-muted-foreground hover:text-foreground"
                    onClick={() => onCopy(String(selectedFileHandle.pid))}
                    aria-label={m.copyPid}
                    title={m.copyPid}
                  >
                    <Copy className="size-3" aria-hidden="true" />
                  </Button>
                  {selectedFileHandle.user && (
                    <>
                      <span>·</span>
                      <span className="font-mono">{selectedFileHandle.user}</span>
                    </>
                  )}
                </div>
              </div>

              <dl className="space-y-2">
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">{m.colFileRef}</dt>
                  <dd>{m.refOpenFile}</dd>
                </div>
                <div className="flex justify-between gap-2">
                  <dt className="text-muted-foreground">{m.blockingDeleteLabel}</dt>
                  <dd className="text-muted-foreground">{m.unconfirmedLabel}</dd>
                </div>
                {selectedFileHandle.runId && (
                  <div className="flex justify-between gap-2">
                    <dt className="shrink-0 text-muted-foreground">{m.yourunRunLabel}</dt>
                    <dd className="truncate text-right font-mono">
                      #{selectedFileHandle.runId} · {selectedFileHandle.commandTitle || ""}
                    </dd>
                  </div>
                )}
                {selectedFileHandle.executable && (
                  <div className="space-y-1">
                    <dt className="text-muted-foreground">{m.programLocation}</dt>
                    <dd className="break-all font-mono text-[11px] text-foreground-secondary">
                      {selectedFileHandle.executable}
                    </dd>
                  </div>
                )}
                <div className="space-y-1">
                  <dt className="text-muted-foreground">{m.targetFileLabel}</dt>
                  <dd className="break-all font-mono text-[11px] text-foreground-secondary">
                    {selectedFileHandle.targetFile}
                  </dd>
                </div>
              </dl>

              <div className="space-y-1">
                <div className="flex items-center justify-between gap-2">
                  <span className="text-muted-foreground">{m.commandText}</span>
                  {selectedFileHandle.commandLine && (
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      className="size-5 text-muted-foreground hover:text-foreground"
                      onClick={() => onCopy(selectedFileHandle.commandLine!)}
                      aria-label={m.copyCommand}
                      title={m.copyCommand}
                    >
                      <Copy className="size-3" aria-hidden="true" />
                    </Button>
                  )}
                </div>
                <pre className="max-h-36 overflow-auto whitespace-pre-wrap break-all border border-border/60 bg-surface-sunken p-2.5 font-mono text-[11px] leading-5">
                  {renderCommandLineText(
                    selectedFileHandle.commandLine,
                    selectedFileHandle.commandLineState,
                  )}
                </pre>
              </div>

              <div className="flex flex-wrap items-center gap-2 pt-1">
                {selectedFileHandle.runId && (
                  <Button
                    size="icon-sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      onOpenRun(selectedFileHandle.runId!, {
                        subview: "files",
                        label: selectedFileHandle.targetFile,
                        runId: selectedFileHandle.runId!,
                        runTitle:
                          selectedFileHandle.commandTitle || selectedFileHandle.processName,
                        projectId: selectedFileHandle.projectId,
                        projectName: selectedFileHandle.projectName,
                        workspaceId: selectedFileHandle.workspaceId,
                        workspaceName: selectedFileHandle.workspaceName,
                        targetPath: state.files.targetPath,
                        pid: selectedFileHandle.pid,
                        reviewStatus: "active",
                      })
                    }
                    aria-label={m.viewRun}
                    title={m.viewRun}
                  >
                    <FileText className="size-4" aria-hidden="true" />
                  </Button>
                )}
                {(selectedFileHandle.executable || selectedFileHandle.targetFile) && (
                  <Button
                    variant="outline"
                    size="sm"
                    className="h-8 text-xs"
                    onClick={() =>
                      openExecutableDirectory(
                        selectedFileHandle.executable,
                        selectedFileHandle.targetFile,
                      )
                    }
                  >
                    <FolderOpen className="size-3.5" aria-hidden="true" />
                    {m.openProgramDir}
                  </Button>
                )}
              </div>
            </div>
          )}

          {(((state.subview === "home" || state.subview === "processes") &&
            !selectedProcess &&
            !(state.subview === "home" && bountyMode === "peaks" && selectedPeakEvent)) ||
            (state.subview === "ports" && !selectedPort) ||
            (state.subview === "files" && !selectedFileHandle)) && (
            <div className="flex flex-1 items-center justify-center text-muted-foreground">
              {m.emptyResults}
            </div>
          )}
        </aside>
      </div>

      {/* Related stopped run bar (for Stale & Released states) */}
      {(isStaleOrigin || isReleasedOrigin) && state.origin && (
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border bg-surface-sunken/70 px-4 py-2 text-xs">
          <div className="flex min-w-0 items-center gap-2">
            <span className="font-medium text-muted-foreground">{m.relatedRun}</span>
            <span className="truncate font-mono">
              #{state.origin.runId}
              {state.origin.projectName
                ? ` · ${state.origin.projectName}${
                    state.origin.workspaceName
                      ? ` / ${formatWorkspaceName(state.origin.workspaceName, m)}`
                      : ""
                  }`
                : ""}{" "}
              · {state.origin.runTitle}
            </span>
            <span className="border border-border bg-background px-1.5 py-0.5 text-[11px] text-muted-foreground">
              {originRun ? m[originRun.status] : m.stopped}
            </span>
          </div>
          <Button
            variant="outline"
            size="xs"
            onClick={() => onOpenRun(state.origin!.runId, state.origin)}
          >
            {m.viewRun}
          </Button>
        </div>
      )}

      {/* Status Footer */}
      <footer className="flex min-h-8 shrink-0 flex-wrap items-center justify-between gap-x-4 gap-y-1 border-t border-border bg-surface-sunken px-2 py-1 text-[11px] text-muted-foreground @min-[36rem]/desktop:px-4">
        <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
          <span className="tabular-nums">
            {activeStats.total}{" "}
            {state.subview === "ports" ? m.portCountUnit : m.processCountUnit}
          </span>
          <span>·</span>
          <span className="tabular-nums">
            {activeStats.managed} {m.managedCountUnit}
          </span>
          <span>·</span>
          <span>
            {activeStats.coverage === "partial" && activeStats.unreadable > 0
              ? `${m.partialCoverageBanner} · ${activeStats.unreadable} ${m.unreadableProcessesSuffix}`
              : m.coverageComplete}
          </span>
        </div>
        <div className="shrink-0 whitespace-nowrap tabular-nums">
          {m.observedAtLabel} {formatObservedTime(activeStats.observedAt, locale)}
        </div>
      </footer>

      <Dialog open={watchDialogOpen} onOpenChange={setWatchDialogOpen}>
        <DialogContent className="max-w-md gap-4 p-4 text-xs">
          <div className="space-y-1">
            <DialogTitle className="text-sm font-semibold">{m.bountyCreateWatch}</DialogTitle>
            <DialogDescription className="sr-only">{m.bountyCreateWatch}</DialogDescription>
          </div>

          <div className="space-y-3">
            <div className="space-y-1.5">
              <label className="block font-medium text-foreground-secondary">
                {m.bountyWatchTarget}
              </label>
              <Input
                value={watchDraft.targetName}
                onChange={(e) =>
                  setWatchDraft((prev) => ({
                    ...prev,
                    targetName: e.target.value,
                    title: e.target.value,
                  }))
                }
                placeholder={m.bountyWatchTarget}
                className="h-8 text-xs"
              />
            </div>

            <div className="space-y-1.5">
              <span className="block font-medium text-foreground-secondary">
                {m.bountyWatchCondition}
              </span>
              <div className="grid grid-cols-4 gap-1.5">
                {(
                  [
                    ["network", m.colNetwork],
                    ["memory", m.colMemory],
                    ["cpu", m.colCpu],
                    ["gpu", m.colGpu],
                  ] as const
                ).map(([cat, label]) => {
                  const active = watchDraft.category === cat;
                  return (
                    <button
                      key={cat}
                      type="button"
                      onClick={() => {
                        const nextCond =
                          cat !== "network" && watchDraft.conditionKind === "cumulative"
                            ? "threshold"
                            : watchDraft.conditionKind;
                        setWatchDraft((prev) => ({
                          ...prev,
                          category: cat,
                          conditionKind: nextCond,
                          threshold: defaultThresholdFor(cat, nextCond),
                        }));
                      }}
                      className={`h-7 border px-2 font-medium transition-colors ${
                        active
                          ? "border-selected bg-selected text-selected-foreground"
                          : "border-border bg-surface text-foreground-secondary hover:bg-accent"
                      }`}
                    >
                      {label}
                    </button>
                  );
                })}
              </div>
              <div className="flex flex-wrap gap-1.5 pt-1">
                {(
                  [
                    ["threshold", m.bountyConditionThreshold],
                    ["rapid_rise", m.bountyConditionRapidRise],
                    ...(watchDraft.category === "network"
                      ? ([["cumulative", m.bountyConditionCumulative]] as const)
                      : []),
                  ] as Array<["threshold" | "rapid_rise" | "cumulative", string]>
                ).map(([cond, label]) => {
                  const active = watchDraft.conditionKind === cond;
                  return (
                    <button
                      key={cond}
                      type="button"
                      onClick={() =>
                        setWatchDraft((prev) => ({
                          ...prev,
                          conditionKind: cond,
                          threshold: defaultThresholdFor(prev.category, cond),
                        }))
                      }
                      className={`h-7 border px-2.5 font-medium transition-colors ${
                        active
                          ? "border-selected bg-selected text-selected-foreground"
                          : "border-border bg-surface text-foreground-secondary hover:bg-accent"
                      }`}
                    >
                      {label}
                    </button>
                  );
                })}
              </div>
            </div>

            <div className="space-y-1.5">
              <span className="block font-medium text-foreground-secondary">
                {m.bountyThresholdLabel}
              </span>
              <div className="flex flex-wrap gap-1.5">
                {thresholdPresetsFor(watchDraft.category, watchDraft.conditionKind, m).map(
                  (preset) => {
                    const active = Math.abs(watchDraft.threshold - preset.value) < 1;
                    return (
                      <button
                        key={preset.label}
                        type="button"
                        onClick={() =>
                          setWatchDraft((prev) => ({ ...prev, threshold: preset.value }))
                        }
                        className={`h-7 border px-2.5 font-mono text-[11px] tabular-nums transition-colors ${
                          active
                            ? "border-selected bg-selected font-semibold text-selected-foreground"
                            : "border-border bg-surface text-foreground-secondary hover:bg-accent"
                        }`}
                      >
                        {preset.label}
                      </button>
                    );
                  },
                )}
              </div>
            </div>

            <label className="flex items-center gap-2 pt-1 text-xs text-foreground-secondary">
              <Checkbox
                checked={watchDraft.notifyToast}
                onCheckedChange={(checked) => {
                  const next = checked === true;
                  if (
                    next &&
                    typeof Notification !== "undefined" &&
                    Notification.permission === "default"
                  ) {
                    void Notification.requestPermission();
                  }
                  setWatchDraft((prev) => ({ ...prev, notifyToast: next }));
                }}
              />
              <span>{m.bountyNotifySystem}</span>
            </label>
          </div>

          <div className="flex justify-end gap-2 border-t border-border pt-3">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setWatchDialogOpen(false)}
            >
              {m.cancel}
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={!watchDraft.targetName.trim()}
              onClick={() => void submitWatchDraft()}
            >
              {m.save}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
    </ResourceActionsProvider>
  );
}
