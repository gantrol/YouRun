import type { ComponentProps } from "react";
import { cn } from "@/renderer/lib/utils";
import "./workspace-layout.css";

export function WorkspaceFrame({ children, className, ...props }: ComponentProps<"div">) {
  return (
    <div className={cn("@container/workspace flex min-h-0 min-w-0 flex-1", className)} {...props}>
      <div className="mx-auto flex min-h-0 min-w-0 w-full max-w-6xl">{children}</div>
    </div>
  );
}

export function WorkspaceOverview({ hasDetail, split, className, ...props }: ComponentProps<"div"> & {
  hasDetail: boolean;
  split: boolean;
}) {
  return <div className={cn(
    "min-h-0 min-w-0 flex-1 flex-col",
    hasDetail ? split ? "hidden @min-[1008px]/workspace:flex @min-[1008px]/workspace:w-[26rem] @min-[1008px]/workspace:flex-none" : "hidden" : "flex",
    className,
  )} {...props} />;
}

export function WorkspaceDetail({ split, as: Comp = "main", className, ...props }: ComponentProps<"main"> & { split: boolean; as?: "main" | "section" }) {
  return <Comp className={cn(
    "mx-auto min-h-0 min-w-0 w-full max-w-xl flex-1",
    split && "@min-[1008px]/workspace:max-w-none @min-[1008px]/workspace:border-l @min-[1008px]/workspace:border-border",
    className,
  )} {...props} />;
}
