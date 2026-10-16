import {
  createContext,
  useContext,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import skins from "../../resources/skins.json";
import { api } from "./api";
import type { Messages } from "./messages";

export type SkinId = keyof typeof skins;
export type Appearance = { skin: SkinId };
export const defaultAppearance: Appearance = { skin: "comic" };
export const skinOptions = Object.entries(skins).map(([id, skin]) => ({
  id: id as SkinId,
  label: skin.label as keyof Messages,
}));

function useSkinPresentation(skin: SkinId) {
  useLayoutEffect(() => {
    const root = document.documentElement;
    const colors = Object.entries(skins[skin].colors);
    root.dataset.skin = skin;
    root.style.setProperty("--radius-md", `${skins[skin].radius}px`);
    for (const [key, value] of colors)
      root.style.setProperty(`--${key}`, value);
    return () => {
      delete root.dataset.skin;
      root.style.removeProperty("--radius-md");
      for (const [key] of colors) root.style.removeProperty(`--${key}`);
    };
  }, [skin]);
}

function useAppearanceState(initial: Appearance, loadFailed: boolean) {
  const [skin, setSkin] = useState<SkinId>(
    Object.hasOwn(skins, initial.skin) ? initial.skin : "comic",
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState(loadFailed ? "skinLoadFailed" : "");
  const pending = useRef<Promise<void> | null>(null);
  const generation = useRef(0);
  const readSequence = useRef(0);
  useSkinPresentation(skin);

  async function refresh() {
    const current = generation.current;
    const sequence = ++readSequence.current;
    try {
      const saved = await api<Appearance>("appearance");
      if (pending.current || current !== generation.current || sequence !== readSequence.current) return;
      setSkin(Object.hasOwn(skins, saved.skin) ? saved.skin : "comic");
      setError("");
    } catch { if (current === generation.current && sequence === readSequence.current) setError("skinLoadFailed"); }
  }

  function change(value: string) {
    if (!Object.hasOwn(skins, value) || pending.current) return;
    generation.current++;
    setSaving(true);
    setError("");
    pending.current = api<Appearance>("appearance", "PUT", { skin: value })
      .then((saved) => {
        setSkin(saved.skin);
      })
      .catch(() => {
        setError("skinSaveFailed");
      })
      .finally(() => {
        pending.current = null;
        generation.current++;
        void refresh();
        setSaving(false);
      });
  }
  return {
    skin,
    saving,
    error,
    change,
    refresh,
    flush: () => pending.current ?? Promise.resolve(),
  };
}

const AppearanceContext = createContext<ReturnType<
  typeof useAppearanceState
> | null>(null);
export function AppearanceProvider({
  initial,
  loadFailed,
  children,
}: {
  initial: Appearance;
  loadFailed: boolean;
  children: ReactNode;
}) {
  return (
    <AppearanceContext.Provider value={useAppearanceState(initial, loadFailed)}>
      {children}
    </AppearanceContext.Provider>
  );
}
export function useAppearance() {
  const value = useContext(AppearanceContext);
  if (!value) throw new Error("AppearanceProvider is required");
  return value;
}

// Component previews share the real palette without persisting app preferences.
export function AppearancePreviewProvider({ children }: { children: ReactNode }) {
  const [skin, setSkin] = useState<SkinId>(defaultAppearance.skin);
  useSkinPresentation(skin);
  return (
    <AppearanceContext.Provider value={{
      skin,
      saving: false,
      error: "",
      change: (value) => { if (Object.hasOwn(skins, value)) setSkin(value as SkinId); },
      flush: () => Promise.resolve(),
      refresh: () => Promise.resolve(),
    }}>
      {children}
    </AppearanceContext.Provider>
  );
}
