import { useEffect, useMemo, useRef, useState } from "react";
import { projectAPI } from "./project-api";
import { api } from "./api";
import type { Command, Project } from "./model";

export type ProjectDetails = {
  directory: string;
  logo: { source: string; data: string } | null;
  logos?: { id: string; name: string; source: string; data: string }[];
  names?: { name: string; source: string }[];
  selectedLogo?: string;
  customAvatar?: boolean;
  revision?: number;
  scannedAt?: string;
  issues?: { path: string; code: string }[];
};

export function useProjectDetails(
  projects: Project[],
  commands: Command[],
  saved: boolean,
  paused = false,
  metadataRevision = 0,
  metadataKeys?: Record<string, string>,
) {
  const [details, setDetails] = useState<Record<string, ProjectDetails>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const retryActions = useRef(new Map<string, () => void>());
  const logoErrors = useRef(new Set<string>());
  function clearError(id: string) {
    retryActions.current.delete(id);
    logoErrors.current.delete(id);
    setErrors((previous) => {
      const next = { ...previous };
      delete next[id];
      return next;
    });
  }
  const [refresh, setRefresh] = useState(0);
  const [loading, setLoading] = useState<string[]>([]);
  const [changingLogo, setChangingLogo] = useState("");
  const [visible, setVisible] = useState(() => !document.hidden);
  const pending = useRef(new Set<string>());
  const forced = useRef(new Set<string>());
  const lastRequest = useRef(0);
  const savingLogo = useRef(false);
  const detailsRef = useRef(details);
  detailsRef.current = details;
  const signatures = useMemo(() => {
    const byProject = new Map<string, unknown[]>();
    for (const command of commands) if (command.project) {
      const entries = byProject.get(command.project) ?? [];
      entries.push([command.directory, command.source]); byProject.set(command.project, entries);
    }
    return Object.fromEntries(projects.map(project => [project.id, JSON.stringify([project.name, byProject.get(project.id), metadataKeys?.[project.id]])]));
  }, [projects, commands, metadataKeys]);
  const key = JSON.stringify(signatures);
  const previousSignatures = useRef<Record<string, string>>({});
  const idsRef = useRef<string[]>([]);
  idsRef.current = projects.map((p) => p.id);
  useEffect(() => {
    const next = JSON.parse(key) as Record<string, string>;
    const ids = new Set(Object.keys(next));
    for (const id of ids) if (previousSignatures.current[id] !== next[id]) pending.current.add(id);
    previousSignatures.current = next;
    pending.current = new Set([...pending.current].filter(id => ids.has(id)));
    setDetails(previous => Object.keys(previous).some(id => !ids.has(id)) ? Object.fromEntries(Object.entries(previous).filter(([id]) => ids.has(id))) : previous);
  }, [key]);
  useEffect(() => {
    if (!saved || paused || changingLogo || !visible) return;
    const controller = new AbortController();
    const ids = [...pending.current].filter((id) => idsRef.current.includes(id));
    setLoading(ids);
    if (ids.length) lastRequest.current = Date.now();
    let cursor = 0;
    async function worker() {
      // Match the host's two scan slots; never launch an unbounded file batch.
      while (cursor < ids.length && !controller.signal.aborted) {
        const id = ids[cursor++];
        try {
          const next = await projectAPI<Record<string, ProjectDetails>>(
            "project-details", "POST",
            { ids: [id], refreshId: forced.current.has(id) ? id : undefined },
            controller.signal,
          );
          if (controller.signal.aborted) return;
          pending.current.delete(id);
          forced.current.delete(id);
          if (!logoErrors.current.has(id)) clearError(id);
          setDetails((previous) => ({ ...previous, ...next }));
        } catch (cause) {
          if (controller.signal.aborted) return;
          if (!logoErrors.current.has(id)) {
            setErrors((previous) => ({ ...previous, [id]: cause instanceof Error ? cause.message : "logoScanFailed" }));
            retryActions.current.set(id, () => rescan(id));
          }
        }
        setLoading((previous) => previous.filter((item) => item !== id));
      }
    }
    void worker();
    void worker();
    return () => {
      controller.abort();
      setLoading([]);
    };
  }, [key, refresh, saved, paused, changingLogo, visible]);
  useEffect(() => {
    const onForeground = () => {
      setVisible(!document.hidden);
      if (document.hidden || Date.now() - lastRequest.current < 30_000 || savingLogo.current) return;
      lastRequest.current = Date.now();
      for (const id of idsRef.current) pending.current.add(id);
      setRefresh((value) => value + 1);
    };
    window.addEventListener("focus", onForeground);
    document.addEventListener("visibilitychange", onForeground);
    return () => {
      window.removeEventListener("focus", onForeground);
      document.removeEventListener("visibilitychange", onForeground);
    };
  }, []);
  function rescan(id?: string) {
    if (id) {
      // Put an explicit action ahead of optional background metadata.
      pending.current = new Set([id, ...pending.current]);
      forced.current.add(id);
    } else {
      for (const projectId of idsRef.current) pending.current.add(projectId);
    }
    if (id) clearError(id);
    setRefresh((value) => value + 1);
  }
  async function changeLogo(id: string, logoId?: string) {
    if (savingLogo.current) return;
    savingLogo.current = true;
    setChangingLogo(id);
    clearError(id);
    try {
      let detail = detailsRef.current[id];
      if (!detail) {
        const next = await projectAPI<Record<string, ProjectDetails>>(
          "project-details", "POST", { ids: [id] },
        );
        detail = next[id];
        setDetails((previous) => ({ ...previous, ...next }));
      }
      const request = { id, revision: detail.revision ?? 0 };
      const next = logoId === undefined
        ? await api<{ revision: number; logo: ProjectDetails["logo"]; cancelled?: false } | { cancelled: true }>(
            "project-avatar", "POST", request,
          )
        : await projectAPI<{ revision: number; cancelled?: false; logo?: ProjectDetails["logo"] }>(
            "project-metadata", "PUT", { ...request, logoId },
          );
      if (next.cancelled) return;
      setDetails((previous) => ({
        ...previous,
        [id]: {
          ...detail,
          ...previous[id],
          logo: next.logo ?? detail.logos?.find((logo) => logo.id === logoId) ?? null,
          selectedLogo: logoId ?? "",
          customAvatar: logoId === undefined,
          revision: next.revision,
        },
      }));
      rescan(id);
    } catch (cause) {
      if (cause instanceof Error && cause.message === "conflict") {
        rescan(id);
        logoErrors.current.add(id);
        setErrors((previous) => ({ ...previous, [id]: "logoConflict" }));
        retryActions.current.set(id, () => rescan(id));
      } else {
        logoErrors.current.add(id);
        setErrors((previous) => ({ ...previous, [id]: cause instanceof Error ? cause.message : "logoSaveFailed" }));
        retryActions.current.set(id, () => { void changeLogo(id, logoId); });
      }
    } finally {
      savingLogo.current = false;
      setChangingLogo("");
    }
  }
  return {
    details,
    errors,
    retry: (id: string) => (retryActions.current.get(id) ?? (() => rescan(id)))(),
    busy: (id: string) => loading.includes(id) || changingLogo === id,
    savingLogo: !!changingLogo,
    rescan,
    selectLogo: (id: string, logoId: string) => changeLogo(id, logoId),
    chooseAvatar: (id: string) => changeLogo(id),
  };
}

