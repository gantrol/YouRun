import { useState } from "react";
import { FileText, RotateCw, Square, Zap } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { ActionMenu, ActionMenuItem, ActionMenuLabel } from "@/renderer/components/ui/action-menu";
import { RabbitActivity } from "../rabbit-activity";
import type { PreviewProps } from "./component-previews";

export function ActionMenuPreview({ m, c }: PreviewProps) {
  const [delay, setDelay] = useState(260);
  const [running, setRunning] = useState(false);
  const [result, setResult] = useState("");
  const activate = () => { setRunning(value => !value); setResult(running ? m.stop : m.start); };
  return <div className="space-y-5">
    <div className="flex flex-wrap gap-2" role="group" aria-label={c.hoverTiming}>
      {([[0, c.hoverImmediate], [260, c.hoverIntent], [500, c.hoverPatient]] as const).map(([value, label]) =>
        <Button key={value} size="sm" variant={delay === value ? "secondary" : "outline"}
          aria-pressed={delay === value} onClick={() => setDelay(value)}>{label}</Button>)}
    </div>
    <div className="flex items-center justify-end gap-2">
      <ActionMenu label={m.activeRuns} hoverDelay={delay}
        trigger={props => <Button {...props} variant="ghost" size="sm" aria-label={m.activeRuns} className="h-8 gap-1.5 px-1.5 text-success">
          <RabbitActivity state="running" /><span className="text-xs tabular-nums">{running ? 1 : 0}</span>
        </Button>}>
        <ActionMenuLabel>YouRun · {m.activeRuns}</ActionMenuLabel>
        <ActionMenuItem disabled={!running} onSelect={() => setResult(m.viewRun)}><FileText />YouRun</ActionMenuItem>
      </ActionMenu>
      <ActionMenu label={m.commandActions} hoverDelay={delay} onAction={activate}
        trigger={props => <Button {...props} variant="outline" size="icon-sm"
          data-command-control={running ? "stop" : "start"} aria-label={running ? m.stop : m.start}>
          {running ? <Square aria-hidden="true" /> : <Zap aria-hidden="true" />}
        </Button>}>
        <ActionMenuLabel>YouRun</ActionMenuLabel>
        <ActionMenuItem onSelect={activate}>{running ? <Square /> : <Zap />}{running ? m.stop : m.start}</ActionMenuItem>
        <ActionMenuItem disabled={!running} onSelect={() => setResult(m.restart)}><RotateCw />{m.restart}</ActionMenuItem>
        <ActionMenuItem onSelect={() => setResult(m.viewCommand)}><FileText />{m.viewCommand}</ActionMenuItem>
      </ActionMenu>
    </div>
    {result && <output className="block text-xs text-muted-foreground">{result}</output>}
  </div>;
}
