import * as React from "react";
import * as SelectPrimitive from "@radix-ui/react-select";
import { CheckIcon, ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import { cn } from "@/renderer/lib/utils";
import { useOverlayPortalContainer } from "@/renderer/components/ui/overlay-layer";
import "./context-menu.css";

const Select = SelectPrimitive.Root;
const SelectValue = SelectPrimitive.Value;

function SelectTrigger({
  className,
  children,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Trigger>) {
  return (
    <SelectPrimitive.Trigger
      data-slot="select-trigger"
      className={cn(
        "group/select flex h-9 min-w-0 items-center justify-between gap-2 rounded-md border border-input bg-background px-3 text-sm outline-none transition-colors duration-fast hover:bg-hover active:bg-pressed focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:pointer-events-none disabled:border-border disabled:bg-surface-sunken disabled:text-disabled-foreground disabled:opacity-100 [&>span]:truncate",
        className,
      )}
      {...props}
    >
      {children}
      <SelectPrimitive.Icon asChild>
        <ChevronDownIcon className="size-3.5 shrink-0 opacity-60 motion-safe:transition-transform motion-safe:duration-fast group-data-[state=open]/select:rotate-180" />
      </SelectPrimitive.Icon>
    </SelectPrimitive.Trigger>
  );
}

function SelectContent({
  className,
  children,
  position = "popper",
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Content>) {
  const container = useOverlayPortalContainer();
  // Modal scopes and their body-level fallback both require an interactive surface.
  return (
    <SelectPrimitive.Portal container={container ?? undefined}>
      <SelectPrimitive.Content
        data-slot="select-content"
        data-overlay-layer="popup"
        data-overlay-surface=""
        position={position}
        className={cn(
          "runshelf-context-menu pointer-events-auto z-popup max-h-[min(18rem,var(--radix-select-content-available-height))] min-w-[8rem] max-w-[calc(100vw-2rem)] overflow-hidden rounded-md border border-border bg-overlay text-foreground shadow-overlay",
          position === "popper" && "w-[var(--radix-select-trigger-width)]",
          className,
        )}
        {...props}
      >
        <SelectPrimitive.ScrollUpButton className="flex h-6 items-center justify-center">
          <ChevronUpIcon className="size-3.5" />
        </SelectPrimitive.ScrollUpButton>
        <SelectPrimitive.Viewport className="p-1">
          {children}
        </SelectPrimitive.Viewport>
        <SelectPrimitive.ScrollDownButton className="flex h-6 items-center justify-center">
          <ChevronDownIcon className="size-3.5" />
        </SelectPrimitive.ScrollDownButton>
      </SelectPrimitive.Content>
    </SelectPrimitive.Portal>
  );
}

function SelectItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof SelectPrimitive.Item>) {
  return (
    <SelectPrimitive.Item
      className={cn(
        "relative flex min-h-8 cursor-default select-none items-center rounded-sm py-1.5 pr-8 pl-2 text-xs text-foreground outline-none transition-colors duration-fast data-[highlighted]:bg-hover data-[highlighted]:text-foreground active:bg-pressed focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring data-[state=checked]:bg-selected data-[state=checked]:font-semibold data-[state=checked]:text-selected-foreground [&[data-state=checked][data-highlighted]]:bg-selected [&[data-state=checked][data-highlighted]]:text-selected-foreground data-[disabled]:pointer-events-none data-[disabled]:text-disabled-foreground data-[disabled]:opacity-100",
        className,
      )}
      {...props}
    >
      <SelectPrimitive.ItemText className="min-w-0 whitespace-normal break-words [overflow-wrap:anywhere]">{children}</SelectPrimitive.ItemText>
      <span className="absolute right-2 grid size-4 place-items-center">
        <SelectPrimitive.ItemIndicator>
          <CheckIcon className="size-3.5" />
        </SelectPrimitive.ItemIndicator>
      </span>
    </SelectPrimitive.Item>
  );
}

export { Select, SelectContent, SelectItem, SelectTrigger, SelectValue };
