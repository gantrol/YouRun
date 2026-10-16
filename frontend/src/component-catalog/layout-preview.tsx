import { useState } from "react";
import { Picker } from "../controls";
import type { Messages } from "../messages";
import type { CatalogMessages } from "./messages";
import { LauncherPreview } from "./launcher-preview";

const windows = [[360, 420], [520, 720], [768, 640], [1008, 640], [1366, 768], [1920, 1080]] as const;

export function LayoutPreview({ m, c, only }: { m: Messages; c: CatalogMessages; only?: string }) {
  const [preset, setPreset] = useState(only === "WorkspaceFrame" ? "1008" : "520");
  const [scene, setScene] = useState(only === "WorkspaceFrame" ? "detail" : "list");
  const [width, height] = windows.find(([width]) => String(width) === preset) ?? windows[1];
  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-3">
        <span className="text-xs text-muted-foreground">{c.windowSize}</span>
        <div className="w-44"><Picker label={c.windowSize} value={preset} onChange={setPreset}
          options={windows.map(([width, height]) => [String(width), `${width} × ${height}`])} /></div>
        <div className="w-40"><Picker label={c.scene} value={scene} onChange={setScene} options={[["list", c.listView], ["detail", c.detailView]]} /></div>
      </div>
      <div className="max-w-full overflow-x-auto pb-3" tabIndex={0} role="region" aria-label={c.windowSize}>
        <LauncherPreview key={scene} m={m} width={width} height={height} initialSelectedId={scene === "detail" ? "preview" : ""} />
      </div>
    </div>
  );
}
