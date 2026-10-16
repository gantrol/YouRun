import { useEffect, useId, useRef, useState, type ComponentProps, type ReactElement, type ReactNode } from "react";
import * as Menu from "@radix-ui/react-menu";
import { cn } from "@/renderer/lib/utils";
import { useOverlayPortalContainer } from "./overlay-layer";
import { menuContentClassName, menuItemClassName } from "./menu-styles";
import "./context-menu.css";
import "./action-menu.css";

const OPEN_DELAY = 260;
const CLOSE_DELAY = 220;
const LONG_PRESS_DELAY = 500;
type OpenReason = "hover" | "pointer" | "keyboard";
type TriggerProps = ComponentProps<"button">;

// A newly opened action menu replaces the previous one without moving focus.
let activeMenu: { id: string; close: () => void } | undefined;

export function ActionMenu({ trigger, children, label, enabled = true, onAction, actionDisabled = false,
  onOpenChange, getReturnFocus, className, hoverDelay = OPEN_DELAY }: {
  trigger: (props: TriggerProps) => ReactElement;
  children: ReactNode;
  label: string;
  enabled?: boolean;
  onAction?: () => void;
  actionDisabled?: boolean;
  onOpenChange?: (open: boolean, trigger: HTMLButtonElement | null) => void;
  getReturnFocus?: () => HTMLElement | null;
  className?: string;
  hoverDelay?: number;
}) {
  const id = useId();
  const container = useOverlayPortalContainer();
  const anchor = useRef<HTMLButtonElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const isOpen = useRef(false);
  const reason = useRef<OpenReason>("hover");
  const previousFocus = useRef<HTMLElement | null>(null);
  const restoreFocus = useRef(true);
  const lastItem = useRef(false);
  const timers = useRef<{ open?: number; close?: number; touch?: number }>({});
  const touch = useRef<{ x: number; y: number } | null>(null);
  const suppressClick = useRef(false);
  const options = useRef({ enabled, onAction, actionDisabled, onOpenChange, getReturnFocus });
  options.current = { enabled, onAction, actionDisabled, onOpenChange, getReturnFocus };

  function clearTimer(key: keyof typeof timers.current) {
    window.clearTimeout(timers.current[key]);
    timers.current[key] = undefined;
  }
  function clearTimers() { clearTimer("open"); clearTimer("close"); clearTimer("touch"); }
  function close(restore = restoreFocus.current) {
    clearTimers();
    restoreFocus.current = restore;
    if (activeMenu?.id === id) activeMenu = undefined;
    if (!isOpen.current) return;
    isOpen.current = false;
    setOpen(false);
    options.current.onOpenChange?.(false, anchor.current);
  }
  function focusItem(last = false) {
    const items = content.current?.querySelectorAll<HTMLElement>('[role="menuitem"]:not([data-disabled])');
    (items?.[last ? items.length - 1 : 0] ?? content.current)?.focus({ preventScroll: true });
  }
  function show(nextReason: OpenReason, last = false) {
    clearTimers();
    if (!options.current.enabled) return;
    if (activeMenu?.id !== id) activeMenu?.close();
    activeMenu = { id, close: () => close(false) };
    reason.current = nextReason;
    lastItem.current = last;
    restoreFocus.current = true;
    if (isOpen.current) {
      if (nextReason !== "hover") focusItem(last);
      return;
    }
    previousFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    isOpen.current = true;
    setOpen(true);
    options.current.onOpenChange?.(true, anchor.current);
  }
  function leave() {
    clearTimer("open");
    clearTimer("close");
    if (reason.current === "hover") timers.current.close = window.setTimeout(() => close(), CLOSE_DELAY);
  }
  function tabOut(backward: boolean) {
    const controls = Array.from(document.querySelectorAll<HTMLElement>(
      'button, a[href], input, select, textarea, [tabindex]',
    )).filter(element => element.tabIndex >= 0 && !element.matches(':disabled') &&
      !element.closest('[inert], [aria-hidden="true"]') && !content.current?.contains(element) && element.getClientRects().length > 0);
    const index = controls.indexOf(anchor.current!);
    close(false);
    (controls[index + (backward ? -1 : 1)] ?? anchor.current)?.focus();
  }
  useEffect(() => () => {
    clearTimers();
    if (activeMenu?.id === id) activeMenu = undefined;
  }, [id]);
  useEffect(() => {
    if (!enabled && isOpen.current) close(false);
    else if (!enabled) clearTimers();
  }, [enabled]);

  const triggerProps: TriggerProps = {
    ref: anchor,
    id: `${id}-trigger`,
    "aria-haspopup": enabled ? "menu" : undefined,
    "aria-expanded": enabled ? open : undefined,
    "aria-controls": open ? `${id}-menu` : undefined,
    "aria-keyshortcuts": enabled ? "ArrowDown ArrowUp Shift+F10" : undefined,
    onPointerEnter: event => {
      clearTimer("close");
      if (event.pointerType !== "mouse" || event.buttons || !options.current.enabled || isOpen.current) return;
      clearTimer("open");
      timers.current.open = window.setTimeout(() => show("hover"), hoverDelay);
    },
    onPointerLeave: () => { clearTimer("touch"); touch.current = null; leave(); },
    onPointerDown: event => {
      clearTimers();
      suppressClick.current = false;
      if (event.pointerType !== "touch" || !options.current.enabled) return;
      // Own this gesture so the enclosing project's long-press menu cannot also open.
      event.stopPropagation();
      touch.current = { x: event.clientX, y: event.clientY };
      timers.current.touch = window.setTimeout(() => {
        suppressClick.current = true;
        show("pointer");
      }, LONG_PRESS_DELAY);
    },
    onPointerMove: event => {
      if (touch.current && Math.hypot(event.clientX - touch.current.x, event.clientY - touch.current.y) > 8) {
        clearTimer("touch"); touch.current = null;
      }
    },
    onPointerUp: () => { clearTimer("touch"); touch.current = null; },
    onPointerCancel: () => { clearTimers(); touch.current = null; suppressClick.current = false; },
    onContextMenu: event => {
      if (!options.current.enabled) return;
      if (touch.current) suppressClick.current = true;
      event.preventDefault(); event.stopPropagation(); show("pointer");
    },
    onClick: event => {
      if (suppressClick.current) { event.preventDefault(); suppressClick.current = false; return; }
      if (options.current.onAction) {
        close(false);
        if (!options.current.actionDisabled) options.current.onAction();
      } else if (isOpen.current && reason.current !== "hover") close();
      else show(event.detail === 0 ? "keyboard" : "pointer");
    },
    onKeyDown: event => {
      if (!options.current.enabled) return;
      if (["ArrowDown", "ArrowUp", "ContextMenu"].includes(event.key) || (event.shiftKey && event.key === "F10")) {
        event.preventDefault(); event.stopPropagation(); show("keyboard", event.key === "ArrowUp");
      } else if (event.key === "Escape" && isOpen.current) {
        event.preventDefault(); event.stopPropagation(); close();
      } else if (event.key === "Tab") close(false);
    },
  };

  // Menu 2.1.24 forwards this FocusScope callback but omits it from its public type.
  // A hover opening must not move focus away from the user's current control.
  const mountFocus = { onOpenAutoFocus: (event: Event) => {
    event.preventDefault();
    if (reason.current !== "hover") focusItem(lastItem.current);
  } };
  return <Menu.Root open={open} onOpenChange={value => value ? show("pointer") : close()} modal={false}>
    <Menu.Anchor asChild>{trigger(triggerProps)}</Menu.Anchor>
    <Menu.Portal container={container ?? undefined}>
      <Menu.Content {...mountFocus} ref={content} id={`${id}-menu`} aria-label={label} data-action-menu-content=""
        data-slot="action-menu-content" data-overlay-layer="popup" data-overlay-surface=""
        inert={!open} aria-hidden={!open}
        align="end" sideOffset={6} collisionPadding={8} hideWhenDetached loop
        className={cn(menuContentClassName, "action-menu-content", className)}
        onPointerEnter={() => clearTimer("close")} onPointerLeave={leave}
        onPointerDown={() => { clearTimer("close"); reason.current = "pointer"; }}
        onCloseAutoFocus={event => {
          event.preventDefault();
          const focused = document.activeElement;
          if (!restoreFocus.current || (focused !== document.body && !content.current?.contains(focused))) return;
          const target = reason.current === "hover" && previousFocus.current !== anchor.current
            ? previousFocus.current : options.current.getReturnFocus?.() ?? anchor.current;
          if (target?.isConnected) target.focus({ preventScroll: true });
        }}
        onInteractOutside={event => {
          if (anchor.current?.contains(event.target as Node)) event.preventDefault();
          else restoreFocus.current = false;
        }}
        onKeyDown={event => {
          if (event.key === "Tab") { event.preventDefault(); tabOut(event.shiftKey); }
          else if (event.key !== "Escape") reason.current = "keyboard";
        }}>
        {children}
      </Menu.Content>
    </Menu.Portal>
  </Menu.Root>;
}

export function ActionMenuItem({ className, ...props }: ComponentProps<typeof Menu.Item>) {
  return <Menu.Item className={cn(menuItemClassName, className)} {...props} />;
}

export function ActionMenuLabel({ children }: { children: ReactNode }) {
  return <Menu.Label className="px-2 py-1.5 text-xs font-medium text-muted-foreground [overflow-wrap:anywhere]">{children}</Menu.Label>;
}

export function ActionMenuSeparator() { return <Menu.Separator className="my-1 h-px bg-border" />; }
