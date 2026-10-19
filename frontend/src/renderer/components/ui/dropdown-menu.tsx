import type { ComponentProps } from "react";
import * as Menu from "@radix-ui/react-dropdown-menu";
import { cn } from "@/renderer/lib/utils";
import { useOverlayPortalContainer } from "./overlay-layer";
import { menuContentClassName, menuItemClassName } from "./menu-styles";
import "./context-menu.css";

export const DropdownMenu = Menu.Root;
export const DropdownMenuTrigger = Menu.Trigger;

export function DropdownMenuContent({ className, sideOffset = 4, collisionPadding = 8, ...props }: ComponentProps<typeof Menu.Content>) {
  const container = useOverlayPortalContainer();
  return <Menu.Portal container={container ?? undefined}>
    <Menu.Content
      data-slot="dropdown-menu-content"
      data-overlay-layer="popup"
      data-overlay-surface=""
      sideOffset={sideOffset}
      collisionPadding={collisionPadding}
      className={cn(menuContentClassName, className)}
      {...props}
    />
  </Menu.Portal>;
}

export function DropdownMenuItem({ className, ...props }: ComponentProps<typeof Menu.Item>) {
  return <Menu.Item className={cn(menuItemClassName, className)} {...props} />;
}

export function DropdownMenuLabel({ className, ...props }: ComponentProps<typeof Menu.Label>) {
  return (
    <Menu.Label
      className={cn(
        "px-2 py-1.5 text-xs font-medium text-muted-foreground [overflow-wrap:anywhere]",
        className,
      )}
      {...props}
    />
  );
}

export function DropdownMenuSeparator() {
  return <Menu.Separator className="my-1 h-px bg-border" />;
}

