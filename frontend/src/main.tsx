import { createRoot } from "react-dom/client";
import { TitleBar } from "./title-bar";
import { WindowControls, useWindowControls } from "./window-controls";
import { App } from "./app";
import { useEffect, useState } from "react";
import { api, type Snapshot } from "./api";
import { catalogs } from "./messages";
import { Button } from "@/renderer/components/ui/button";
import type { HostSession } from "./update-session";
import {
  AppearanceProvider,
  defaultAppearance,
  type Appearance,
} from "./appearance";

function Root() {
  const windowControls = useWindowControls();
  const [windowError, setWindowError] = useState(false);
  const [state, setState] = useState<Snapshot>();
  const [session, setSession] = useState<HostSession>();
  const [appearance, setAppearance] = useState(defaultAppearance);
  const [appearanceFailed, setAppearanceFailed] = useState(false);
  const [failed, setFailed] = useState(false);
  const m = catalogs[navigator.language.startsWith("zh") ? "zh" : "en"];
  function load() {
    setFailed(false);
    Promise.all([
      api<Snapshot>("state"),
      api<HostSession>("updates/session"),
      api<Appearance>("appearance").then(
        (value) => ({ value, failed: false }),
        () => ({ value: defaultAppearance, failed: true }),
      ),
    ])
      .then(([snapshot, restored, preference]) => {
        setState(snapshot);
        setSession(restored);
        setAppearance(preference.value);
        setAppearanceFailed(preference.failed);
      })
      .catch(() => setFailed(true));
  }
  useEffect(load, []);
  if (state && session)
    return (
      <AppearanceProvider initial={appearance} loadFailed={appearanceFailed}>
        <App
          initial={state}
          session={session}
          windowControls={windowControls}
        />
      </AppearanceProvider>
    );
  return (
    <div className="flex h-full flex-col bg-surface">
      <TitleBar
        title={m.productName}
        m={m}
        mac={windowControls.platform === "darwin"}
        fullscreen={windowControls.fullscreen}
        controls={
          windowControls.platform !== "darwin" ? (
            <WindowControls
              m={m}
              maximized={windowControls.maximized}
              onAction={(action) => {
                void windowControls
                  .perform(action)
                  .catch(() => setWindowError(true));
              }}
            />
          ) : undefined
        }
      />
      {windowError && (
        <p role="alert" className="px-4 py-2 text-sm text-destructive">
          {m.windowUnavailable}
        </p>
      )}
      <main
        className="flex flex-1 items-center justify-center gap-3 text-sm"
        role="status"
      >
        {failed ? m.connectionLost : m.loading}
        {failed && <Button onClick={load}>{m.retry}</Button>}
      </main>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(<Root />);
