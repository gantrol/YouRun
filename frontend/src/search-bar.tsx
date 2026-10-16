import type { Ref } from "react";
import { Search, X } from "lucide-react";
import { Input } from "@/renderer/components/ui/input";
import { Button } from "@/renderer/components/ui/button";
import type { Messages } from "./messages";
import "./search-bar.css";

export const modifierKey = /Mac|iPhone|iPad/.test(navigator.platform)
  ? "⌘"
  : "Ctrl";

export function SearchBar({
  m,
  value,
  onChange,
  inputRef,
  count,
}: {
  m: Messages;
  value: string;
  onChange: (value: string) => void;
  inputRef?: Ref<HTMLInputElement>;
  count?: number;
}) {
  return (
    <form
      role="search"
      aria-label={m.search}
      onSubmit={(event) => event.preventDefault()}
      className="runshelf-search shrink-0 border-t border-border bg-background px-3 py-3"
    >
      <div className="runshelf-search-field mx-auto flex min-h-10 w-full max-w-[34.5rem] items-center gap-2 rounded-md border border-muted-foreground/75 bg-surface px-3 focus-within:border-ring focus-within:ring-2 focus-within:ring-ring/20">
        <Search
          className="size-4 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          ref={inputRef}
          focusIndicator="container"
          type="search"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={(event) => {
            if (event.nativeEvent.isComposing) return;
            if (event.key === "Escape" && value) {
              event.preventDefault();
              event.stopPropagation();
              onChange("");
              return;
            }
            if (event.key === "ArrowDown" || event.key === "ArrowUp") {
              const scope =
                event.currentTarget.closest("section, .workspace-home, [role='region']") ??
                document;
              const rows = Array.from(
                scope.querySelectorAll<HTMLElement>(
                  "[data-project-heading], [data-collection-row], [data-command-row]",
                ),
              );
              const target =
                event.key === "ArrowDown" ? rows[0] : rows.at(-1);
              if (target) {
                event.preventDefault();
                target.focus();
              }
            }
          }}
          aria-label={m.search}
          placeholder={m.search}
          autoComplete="off"
          spellCheck={false}
          className="h-10 border-0 bg-transparent px-0 text-base shadow-none [&::-webkit-search-cancel-button]:appearance-none"
        />
        {value ? (
          <>
            {count !== undefined && (
              <span
                aria-live="polite"
                className="shrink-0 text-xs tabular-nums text-muted-foreground"
              >
                {new Intl.NumberFormat().format(count)}
              </span>
            )}
            <Button
              variant="ghost"
              size="icon-sm"
              type="button"
              aria-label={m.clear}
              title={m.clear}
              onClick={(event) => {
                onChange("");
                event.currentTarget.form?.querySelector("input")?.focus();
              }}
            >
              <X className="size-4" aria-hidden="true" />
            </Button>
          </>
        ) : (
          <kbd
            aria-label={m.searchShortcut}
            className="shrink-0 rounded-sm border border-border bg-background px-1.5 py-0.5 font-sans text-xs text-muted-foreground"
          >
            {modifierKey} K
          </kbd>
        )}
      </div>
    </form>
  );
}

