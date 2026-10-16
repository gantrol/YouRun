import { useLayoutEffect, useRef, useState } from "react";
import "./workspace-layout.css";

export function WorkspaceResizer({ pane, label }: { pane: "navigation" | "commands"; label: string }) {
  const element = useRef<HTMLDivElement>(null);
  const drag = useRef<{ x: number; width: number; saved: number } | null>(null);
  const initial = pane === "navigation" ? 336 : 352;
  const key = `yourun.pane-width.${pane}`;
  const property = `--${pane}-width`;
  const [width, setWidth] = useState(() => {
    try { const value = Number(localStorage.getItem(key)); return Number.isFinite(value) && value >= 256 ? Math.min(value, 640) : initial; }
    catch { return initial; }
  });
  const [limits, setLimits] = useState({ max: 640, actual: width });
  useLayoutEffect(() => {
    const divider = element.current, container = divider?.parentElement;
    if (!divider || !container) return;
    container.style.setProperty(property, `${width}px`);
    const update = () => {
      if (!divider.offsetWidth) return;
      const triple = container.querySelector<HTMLElement>(".workspace-home")?.offsetWidth && container.querySelector<HTMLElement>(".workspace-project")?.offsetWidth && container.querySelector<HTMLElement>(".workspace-leaf")?.offsetWidth;
      const max = Math.max(256, Math.min(640, container.clientWidth * (triple ? 0.32 : 0.45)));
      setLimits({ max: Math.round(max), actual: Math.round((divider.previousElementSibling as HTMLElement)?.getBoundingClientRect().width ?? width) });
    };
    const observer = new ResizeObserver(update); observer.observe(container);
    if (divider.previousElementSibling) observer.observe(divider.previousElementSibling);
    update(); return () => observer.disconnect();
  }, [property, width]);
  function save(value: number) {
    const next = Math.round(Math.max(256, Math.min(limits.max, value)));
    setWidth(next);
    try { localStorage.setItem(key, String(next)); } catch { /* In-memory sizing remains available. */ }
  }
  return <div ref={element} className={`workspace-resizer workspace-resizer-${pane}`} role="separator" tabIndex={0}
    aria-orientation="vertical" aria-label={label} aria-valuemin={256} aria-valuemax={limits.max} aria-valuenow={Math.min(limits.max, limits.actual)}
    onDoubleClick={() => save(initial)} onKeyDown={event => {
      if (!["ArrowLeft", "ArrowRight", "Home", "End", "Enter"].includes(event.key)) return;
      event.preventDefault(); save(event.key === "Home" ? 256 : event.key === "End" ? limits.max : event.key === "Enter" ? initial : limits.actual + (event.key === "ArrowLeft" ? -1 : 1) * (event.shiftKey ? 32 : 8));
    }} onPointerDown={event => {
      if (event.button !== 0) return;
      event.preventDefault(); event.currentTarget.setPointerCapture(event.pointerId);
      drag.current = { x: event.clientX, width: limits.actual, saved: width };
      event.currentTarget.dataset.dragging = "true";
    }} onPointerMove={event => {
      if (drag.current) setWidth(Math.max(256, Math.min(limits.max, drag.current.width + event.clientX - drag.current.x)));
    }} onPointerUp={event => {
      if (!drag.current) return;
      save(drag.current.width + event.clientX - drag.current.x); drag.current = null;
      delete event.currentTarget.dataset.dragging;
      event.currentTarget.releasePointerCapture(event.pointerId);
    }} onPointerCancel={event => {
      if (drag.current) setWidth(drag.current.saved);
      drag.current = null; delete event.currentTarget.dataset.dragging;
    }} onLostPointerCapture={event => { drag.current = null; delete event.currentTarget.dataset.dragging; }}/>
}
