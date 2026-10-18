import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AppWindow,
  ArrowDown,
  ArrowLeft,
  ArrowUp,
  ChevronDown,
  ChevronRight,
  Copy,
  FileSearch,
  FileText,
  FolderOpen,
  RefreshCw,
  Search,
  X,
} from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Input } from "@/renderer/components/ui/input";
import { Checkbox } from "@/renderer/components/ui/checkbox";
import { Picker } from "./controls";
import { api } from "./api";
import type { Command, Project, Run, SourceSnapshot } from "./model";
import type { ProjectDetails } from "./project-details";
import type { Messages } from "./messages";
import { formatSourceSummary } from "./project-workspaces";
import { YouRunLogo } from "./yourun-logo";

export type ProSubview = "home" | "processes" | "ports" | "files";

export type ProcessItem = {
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

function formatMemory(bytes: number): string {
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
}: {
  rank: 1 | 2 | 3;
  m: Messages;
  selected?: boolean;
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
      className={`inline-flex h-5 shrink-0 items-center gap-1 border px-1.5 text-[11px] font-medium leading-none ${meta.cls}`}
      style={
        {
          "--yourun-logo-fill": meta.fill,
          "--yourun-logo-ink": meta.ink,
        } as React.CSSProperties
      }
    >
      <YouRunLogo className="size-3.5 shrink-0" />
      <span>{meta.label}</span>
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
  ownership: string;
  projectName?: string;
  workspaceName?: string;
};

export function ProModeWorkspace({
  state,
  onStateChange,
  projects,
  projectDetails,
  runs,
  locale,
  m,
  onOpenRun,
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
  locale: string;
  m: Messages;
  onOpenRun: (runId: string, origin?: ResourceOriginContext) => void;
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

  const toggleGroup = useCallback((groupKey: string) => {
    setExpandedGroups((prev) => ({ ...prev, [groupKey]: !prev[groupKey] }));
  }, []);

  const loadProcesses = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const res = await api<{
        items: ProcessItem[];
        observedAt: string;
        coverage?: string;
        unreadableProcessCount?: number;
      }>("resources/processes");
      setProcData({
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

  const handleRefresh = useCallback(() => {
    if (state.origin && state.origin.subview === state.subview && state.origin.reviewStatus === "stale") {
      onStateChange((prev) => ({
        ...prev,
        origin: prev.origin ? { ...prev.origin, reviewStatus: "released" } : undefined,
      }));
    }
    if (state.subview === "home" || state.subview === "processes") {
      void loadProcesses();
    } else if (state.subview === "ports") {
      void loadPorts();
    } else if (state.subview === "files" && state.files.targetPath.trim()) {
      void loadFiles(state.files.targetPath, state.files.recursive);
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

  const processGroups = useMemo<ProcessGroup[]>(() => {
    const q = state.subview === "processes" ? state.processes.query.trim().toLowerCase() : "";
    const isNumeric = /^\d+$/.test(q);
    const map = new Map<string, ProcessItem[]>();

    for (const item of filteredProcesses) {
      const label = friendlyProcessLabel(item.name, item.displayName);
      const key =
        item.ownership === "managed"
          ? `managed:${item.runId || item.pid}:${label.toLowerCase()}`
          : `ext:${label.toLowerCase()}`;
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
      groups.push({
        key,
        title,
        icon,
        primary,
        items,
        totalCpu: hasCpu ? Math.min(100, totalCpuSum) : null,
        cpuSampling: !hasCpu && cpuSampling,
        totalMemory,
        totalGpu: hasGpu ? Math.min(100, totalGpuSum) : null,
        gpuSampling: !hasGpu && gpuSampling,
        totalGpuMemory,
        totalNetwork: hasNetwork ? totalNetworkSum : null,
        networkSampling: !hasNetwork && networkSampling,
        totalNetworkConnections,
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
  }, [filteredProcesses, state.processes.query, state.processes.sortBy, state.processes.sortDesc, state.subview]);

  const bountyRankings = useMemo(() => {
    const byCpu = [...processGroups]
      .sort((a, b) => {
        const ca = a.totalCpu ?? -1;
        const cb = b.totalCpu ?? -1;
        if (cb !== ca) return cb - ca;
        return b.totalMemory - a.totalMemory;
      })
      .slice(0, 3);

    const byMemory = [...processGroups]
      .sort((a, b) => {
        if (b.totalMemory !== a.totalMemory) return b.totalMemory - a.totalMemory;
        return (b.totalCpu ?? -1) - (a.totalCpu ?? -1);
      })
      .slice(0, 3);

    const byGpu = [...processGroups]
      .sort((a, b) => {
        const ga = a.totalGpu ?? -1;
        const gb = b.totalGpu ?? -1;
        if (gb !== ga) return gb - ga;
        if (b.totalGpuMemory !== a.totalGpuMemory) return b.totalGpuMemory - a.totalGpuMemory;
        return b.totalMemory - a.totalMemory;
      })
      .slice(0, 3);

    const byNetwork = [...processGroups]
      .sort((a, b) => {
        const na = a.totalNetwork ?? -1;
        const nb = b.totalNetwork ?? -1;
        if (nb !== na) return nb - na;
        if (b.totalNetworkConnections !== a.totalNetworkConnections) {
          return b.totalNetworkConnections - a.totalNetworkConnections;
        }
        return b.totalMemory - a.totalMemory;
      })
      .slice(0, 3);

    return {
      cpu: byCpu,
      memory: byMemory,
      gpu: byGpu,
      network: byNetwork,
    };
  }, [processGroups]);

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

  return (
    <div className="flex min-h-0 flex-1 flex-col bg-surface">
      {/* Sub-navigation bar */}
      <header className="flex h-10 shrink-0 items-center justify-between gap-2 border-b border-border px-4">
        <div className="flex items-center gap-1" role="tablist" aria-label={m.proMode}>
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
                className={`inline-flex h-7 items-center rounded-sm px-3 text-xs transition-colors ${
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
        <div className="flex items-center gap-2">
          {isStaleOrigin && (
            <span className="border border-warning/30 bg-warning-surface px-2 py-0.5 text-xs text-warning-foreground">
              {m.staleResultSuffix}
            </span>
          )}
          <Button
            variant="outline"
            size="sm"
            className="h-7 px-2.5 text-xs"
            disabled={loading}
            onClick={handleRefresh}
          >
            <RefreshCw
              className={`size-3.5 ${loading ? "motion-safe:animate-spin" : ""}`}
              aria-hidden="true"
            />
            {m.refresh}
          </Button>
        </div>
      </header>

      {/* Filter bar per subview */}
      {(state.subview === "home" || state.subview === "processes") && (
        <div className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-border bg-surface-sunken/30 px-4 py-2">
          {state.subview === "home" ? (
            <div className="flex min-h-8 flex-wrap items-center gap-2">
              <h2 className="text-xs font-semibold tracking-tight text-foreground">
                {m.bountyBoard}
              </h2>
              <div className="flex items-center gap-1.5" aria-hidden="true">
                <BountyRabbitBadge rank={1} m={m} />
                <BountyRabbitBadge rank={2} m={m} />
                <BountyRabbitBadge rank={3} m={m} />
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
          <div className="flex flex-wrap items-center gap-2">
            <div className="w-36">
              <Picker
                label={m.colOwnership}
                value={state.processes.ownership}
                options={[
                  ["all", m.allOwnership],
                  ["managed", m.ownershipManaged],
                  ["external", m.ownershipExternal],
                  ["unconfirmed", m.ownershipUnconfirmed],
                ]}
                onChange={(ownership) =>
                  onStateChange((prev) => ({
                    ...prev,
                    processes: { ...prev.processes, ownership },
                  }))
                }
                className="h-8 text-xs"
              />
            </div>
            <div className="w-40">
              <Picker
                label={m.project}
                value={state.processes.project}
                options={[
                  ["all", m.allProjects],
                  ...projects.map((p): [string, string] => [p.id, p.name]),
                ]}
                onChange={(project) =>
                  onStateChange((prev) => ({
                    ...prev,
                    processes: { ...prev.processes, project, workspace: "all" },
                  }))
                }
                className="h-8 text-xs"
              />
            </div>
            {workspaceOptions.length > 1 && (
              <div className="w-44">
                <Picker
                  label={m.workspace}
                  value={state.processes.workspace}
                  options={workspaceOptions}
                  onChange={(workspace) =>
                    onStateChange((prev) => ({
                      ...prev,
                      processes: { ...prev.processes, workspace },
                    }))
                  }
                  className="h-8 text-xs"
                />
              </div>
            )}
          </div>
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
          <div className="w-36">
            <Picker
              label={m.colProtocol}
              value={state.ports.protocol}
              options={[
                ["all", m.protocolAll],
                ["TCP", "TCP"],
                ["UDP", "UDP"],
              ]}
              onChange={(protocol) =>
                onStateChange((prev) => ({
                  ...prev,
                  ports: { ...prev.ports, protocol },
                }))
              }
              className="h-8 text-xs"
            />
          </div>
          <div className="w-36">
            <Picker
              label={m.colState}
              value={state.ports.state}
              options={[
                ["all", m.stateAll],
                ["LISTEN", m.stateListening],
                ["BOUND", m.stateBound],
                ["ESTABLISHED", m.stateEstablished],
              ]}
              onChange={(portState) =>
                onStateChange((prev) => ({
                  ...prev,
                  ports: { ...prev.ports, state: portState },
                }))
              }
              className="h-8 text-xs"
            />
          </div>
        </div>
      )}

      {state.subview === "files" && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 border-b border-border bg-surface-sunken/30 px-4 py-2">
          <div className="min-w-60 flex-1">
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
          <div className="flex items-center gap-1.5">
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 text-xs"
              disabled={loading}
              onClick={() => void chooseTargetPath("file")}
            >
              <FileSearch className="size-3.5" aria-hidden="true" />
              {m.chooseFile}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8 text-xs"
              disabled={loading}
              onClick={() => void chooseTargetPath("directory")}
            >
              <FolderOpen className="size-3.5" aria-hidden="true" />
              {m.chooseFolder}
            </Button>
          </div>
          {state.files.isDirectory && (
            <label className="flex h-8 items-center gap-1.5 px-1 text-xs text-foreground-secondary">
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
          <Button
            type="button"
            size="sm"
            className="h-8 text-xs"
            disabled={loading || !state.files.targetPath.trim()}
            onClick={() => void loadFiles(state.files.targetPath, state.files.recursive)}
          >
            {m.queryAction}
          </Button>
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
          <Button size="xs" onClick={handleRefresh}>
            <RefreshCw className="size-3" aria-hidden="true" />
            {m.recheckNow}
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
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* Left Table / Bounty Board Area */}
        <div
          className={`flex min-h-0 min-w-0 flex-1 flex-col overflow-auto ${
            narrowDetailOpen ? "hidden @min-[63rem]/desktop:flex" : "flex"
          }`}
        >
          {state.subview === "home" && (
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
                              <BountyRabbitBadge rank={rank} m={m} />
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
                          ? group.cpuSampling
                            ? m.cpuSampling
                            : group.totalCpu != null
                              ? `${group.totalCpu.toFixed(1)}%`
                              : "--"
                          : cat.id === "memory"
                            ? formatMemory(group.totalMemory)
                            : cat.id === "gpu"
                              ? group.gpuSampling
                                ? m.cpuSampling
                                : group.totalGpu != null
                                  ? `${group.totalGpu.toFixed(1)}%`
                                  : group.totalGpuMemory > 0
                                    ? formatMemory(group.totalGpuMemory)
                                    : "--"
                              : formatNetworkRate(group.totalNetwork, group.networkSampling, m);

                      const secondaryMetric =
                        cat.id === "cpu"
                          ? formatMemory(group.totalMemory)
                          : cat.id === "memory"
                            ? group.totalCpu != null
                              ? `${group.totalCpu.toFixed(1)}% CPU`
                              : `PID ${rep.pid}`
                            : cat.id === "gpu"
                              ? group.totalGpuMemory > 0
                                ? `${m.gpuMemoryLabel} ${formatMemory(group.totalGpuMemory)}`
                                : formatMemory(group.totalMemory)
                              : group.totalNetworkConnections > 0
                                ? `${group.totalNetworkConnections} ${m.networkConnectionsUnit}`
                                : formatMemory(group.totalMemory);

                      return (
                        <div
                          key={`${cat.id}-${group.key}`}
                          onClick={() => {
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
                            <BountyRabbitBadge rank={rank} m={m} selected={rowSelected} />
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
                      );
                    })}
                  </div>
                </section>
              ))}
            </div>
          )}

          {state.subview === "processes" && (
            <table className="w-full table-fixed border-collapse text-left text-xs">
              <thead className="sticky top-0 z-10 border-b border-border bg-surface-sunken text-muted-foreground">
                <tr className="h-8">
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
                      <tr
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
                            <span className="truncate font-medium">
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
                      </tr>,
                    ];
                  }

                  const hasQuery = Boolean(state.processes.query.trim());
                  const isExpanded = Boolean(expandedGroups[group.key]) || hasQuery;
                  const containsSelected = group.items.some(
                    (i) => i.pid === selectedProcess?.pid,
                  );
                  const groupRowSelected = !isExpanded && containsSelected;

                  const groupHeaderRow = (
                    <tr
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
                          <span className="truncate font-medium">{group.title}</span>
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
                    </tr>
                  );

                  if (!isExpanded) {
                    return [groupHeaderRow];
                  }

                  const childRows = group.items.map((item) => {
                    const childSelected = selectedProcess?.pid === item.pid;
                    return (
                      <tr
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
                            <span className="truncate">{item.name}</span>
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
                      </tr>
                    );
                  });

                  return [groupHeaderRow, ...childRows];
                })}
                {filteredProcesses.length === 0 && !loading && (
                  <tr>
                    <td colSpan={7} className="px-4 py-12 text-center text-muted-foreground">
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
                <table className="w-full table-fixed border-collapse text-left text-xs">
                  <thead className="sticky top-0 z-10 border-b border-border bg-surface-sunken text-muted-foreground">
                    <tr className="h-8">
                      <th className="w-40 px-3 py-1.5 font-medium">{m.colLocalAddress}</th>
                      <th className="w-24 px-2.5 py-1.5 font-medium">{m.colProtocol}</th>
                      <th className="w-24 px-2.5 py-1.5 font-medium">{m.colState}</th>
                      <th className="px-3 py-1.5 font-medium">{m.colProcessPid}</th>
                      <th className="w-52 px-3 py-1.5 font-medium">{m.colProjectWorkspace}</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border/60">
                    {filteredPorts.map((item) => {
                      const selected = selectedPort?.id === item.id;
                      return (
                        <tr
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
                          <td className="truncate px-3 py-1.5 font-mono font-medium">
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
                        </tr>
                      );
                    })}
                    {filteredPorts.length === 0 && !loading && (
                      <tr>
                        <td colSpan={5} className="px-4 py-12 text-center text-muted-foreground">
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
            <table className="w-full table-fixed border-collapse text-left text-xs">
              <thead className="sticky top-0 z-10 border-b border-border bg-surface-sunken text-muted-foreground">
                <tr className="h-8">
                  <th className="px-3 py-1.5 font-medium">{m.colHolderProcess}</th>
                  <th className="w-20 px-2.5 py-1.5 text-right font-medium">{m.colPid}</th>
                  <th className="w-36 px-2.5 py-1.5 font-medium">{m.colUser}</th>
                  <th className="w-28 px-2.5 py-1.5 font-medium">{m.colFileRef}</th>
                  <th className="w-52 px-3 py-1.5 font-medium">{m.colOwnership}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/60">
                {fileData.items.map((item) => {
                  const selected = selectedFileHandle?.id === item.id;
                  return (
                    <tr
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
                    </tr>
                  );
                })}
                {fileData.items.length === 0 && !loading && (
                  <tr>
                    <td colSpan={5} className="px-4 py-12 text-center text-muted-foreground">
                      {fileData.queriedPath ? m.noFileOccupancy : m.enterFileOrFolderPrompt}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
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
              <Button variant="ghost" size="xs" onClick={() => setNarrowDetailOpen(false)}>
                <ArrowLeft className="size-3.5" aria-hidden="true" />
                {m.back}
              </Button>
            </div>
          )}

          {(state.subview === "home" || state.subview === "processes") && selectedProcess && (
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
                        {netConns > 0 && (
                          <span className="font-mono text-[10px] tabular-nums">
                            {netConns} {m.networkConnectionsUnit}
                          </span>
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
                {selectedProcess.runId && (
                  <Button
                    size="sm"
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
                  >
                    <FileText className="size-3.5" aria-hidden="true" />
                    {m.viewRun}
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
                    size="sm"
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
                  >
                    <FileText className="size-3.5" aria-hidden="true" />
                    {m.viewRun}
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
                    size="sm"
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
                  >
                    <FileText className="size-3.5" aria-hidden="true" />
                    {m.viewRun}
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

          {(((state.subview === "home" || state.subview === "processes") && !selectedProcess) ||
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
      <footer className="flex h-8 shrink-0 items-center justify-between gap-2 border-t border-border bg-surface-sunken px-4 text-[11px] text-muted-foreground">
        <div className="flex items-center gap-2 truncate">
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
        <div className="shrink-0 tabular-nums">
          {m.observedAtLabel} {formatObservedTime(activeStats.observedAt, locale)}
        </div>
      </footer>
    </div>
  );
}
