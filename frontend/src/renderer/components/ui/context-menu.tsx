import type { ComponentProps } from "react";
import * as Menu from "@radix-ui/react-context-menu";
import { ChevronRight } from "lucide-react";
import { cn } from "@/renderer/lib/utils";
import { useOverlayPortalContainer } from "./overlay-layer";
import "./context-menu.css";

export const ContextMenu = Menu.Root;
export const ContextMenuTrigger = Menu.Trigger;
export const ContextMenuSub = Menu.Sub;
export function ContextMenuSubTrigger({ children, className, ...props }: ComponentProps<typeof Menu.SubTrigger>) {
  return <Menu.SubTrigger className={cn(
    "flex min-h-8 cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm whitespace-normal [overflow-wrap:anywhere] outline-none data-[highlighted]:bg-hover data-[state=open]:bg-hover focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring data-[disabled]:pointer-events-none data-[disabled]:text-disabled-foreground [&>span]:min-w-0 [&_svg]:size-4 [&_svg]:shrink-0",
    className,
  )} {...props}>{children}<ChevronRight className="ml-auto" aria-hidden="true" /></Menu.SubTrigger>;
}
export function ContextMenuSubContent({
  className,
  collisionPadding = 8,
  sideOffset = 4,
  hideWhenDetached = true,
  ...props
}: ComponentProps<typeof Menu.SubContent>) {
  const container = useOverlayPortalContainer();
  return <Menu.Portal container={container ?? undefined}>
    <Menu.SubContent
      data-slot="context-menu-sub-content"
      data-overlay-layer="popup"
      data-overlay-surface=""
      collisionPadding={collisionPadding}
      sideOffset={sideOffset}
      hideWhenDetached={hideWhenDetached}
      className={cn(
      "runshelf-context-menu pointer-events-auto z-popup min-w-[min(12rem,var(--radix-context-menu-content-available-width))] max-w-[min(24rem,calc(100vw-1rem),var(--radix-context-menu-content-available-width))] max-h-[var(--radix-context-menu-content-available-height)] overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border bg-overlay p-1 text-foreground shadow-overlay",
      className,
    )} {...props} />
  </Menu.Portal>;
}
export function ContextMenuContent({
  className,
  collisionPadding = 8,
  ...props
}: ComponentProps<typeof Menu.Content>) {
  const container = useOverlayPortalContainer();
  return (
    <Menu.Portal container={container ?? undefined}>
      <Menu.Content
        data-slot="context-menu-content"
        data-overlay-layer="popup"
        data-overlay-surface=""
        collisionPadding={collisionPadding}
        className={cn(
          "runshelf-context-menu pointer-events-auto z-popup min-w-[min(12rem,var(--radix-context-menu-content-available-width))] max-w-[min(24rem,calc(100vw-1rem),var(--radix-context-menu-content-available-width))] max-h-[var(--radix-context-menu-content-available-height)] overflow-x-hidden overflow-y-auto overscroll-contain rounded-md border border-border bg-overlay p-1 text-foreground shadow-overlay",
          className,
        )}
        {...props}
      />
    </Menu.Portal>
  );
}
export function ContextMenuItem({
  className,
  destructive,
  ...props
}: ComponentProps<typeof Menu.Item> & { destructive?: boolean }) {
  return (
    <Menu.Item
      className={cn(
        "flex min-h-8 cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm whitespace-normal [overflow-wrap:anywhere] outline-none data-[highlighted]:bg-hover focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring data-[disabled]:pointer-events-none data-[disabled]:text-disabled-foreground [&>span]:min-w-0 [&_svg]:size-4 [&_svg]:shrink-0",
        destructive &&
          "text-destructive data-[highlighted]:bg-destructive-surface",
        className,
      )}
      {...props}
    />
  );
}
export function ContextMenuSeparator() {
  return <Menu.Separator className="my-1 h-px bg-border" />;
}
