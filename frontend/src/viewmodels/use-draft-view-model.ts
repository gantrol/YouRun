import { useEffect, useState, type SetStateAction } from "react";
import { fingerprint } from "../models/workspace";

// Saved state follows SQLite; drafts remain separate until explicitly saved or
// reloaded and cannot silently overwrite an externally changed value.
export function useDraftViewModel<T>(saved: T) {
  const [base, setBase] = useState(saved);
  const [draft, setDraft] = useState(saved);
  const dirty = fingerprint(draft) !== fingerprint(base);
  const changed = fingerprint(saved) !== fingerprint(base);
  function reload() { setBase(saved); setDraft(saved); }
  useEffect(() => { if (changed && !dirty) reload(); }, [changed, dirty, fingerprint(saved)]);
  function setField<K extends keyof T>(key: K, value: SetStateAction<T[K]>) {
    setDraft((previous) => ({ ...previous, [key]: typeof value === "function"
      ? (value as (value: T[K]) => T[K])(previous[key]) : value }));
  }
  return { base, draft, setDraft, setField, dirty, conflict: dirty && changed, reload,
    accept: (value: T) => { setBase(value); setDraft(value); } };
}
