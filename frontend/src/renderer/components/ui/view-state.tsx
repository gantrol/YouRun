import { createContext, useContext, useRef, useState, type ReactNode } from "react";

const ViewStateContext = createContext<{ scope: string; values: Map<string, boolean>; positions: Map<string, number> } | null>(null);

// Keep presentation state when returning to a parent page; never persist form drafts.
export function ViewStateProvider({ scope, children }: { scope: string; children: ReactNode }) {
  const positions = useRef(new Map<string, number>());
  const values = useRef(new Map<string, boolean>());
  return <ViewStateContext.Provider value={{ scope, values: values.current, positions: positions.current }}>{children}</ViewStateContext.Provider>;
}

export function useViewToggle(stateKey: string | undefined, initial: boolean) {
  const context = useContext(ViewStateContext);
  const key = stateKey && context ? `${context.scope}:${stateKey}` : undefined;
  const [value, setValue] = useState(() => key ? context!.values.get(key) ?? initial : initial);
  function change(next: boolean) {
    if (key) context!.values.set(key, next);
    setValue(next);
  }
  return [value, change] as const;
}

export function useViewPosition(stateKey: string) {
  const context = useContext(ViewStateContext);
  const key = `${context?.scope ?? ""}:${stateKey}`;
  return { read: () => context?.positions.get(key), write: (value: number) => context?.positions.set(key, value) };
}
