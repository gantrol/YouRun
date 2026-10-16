import { useId, type ComponentProps } from "react";
import { TextField } from "./controls";
import { Input } from "@/renderer/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger } from "@/renderer/components/ui/select";
import type { ImportResult } from "./project-import-data";

export function ProjectNameField({
  names = [],
  label,
  error,
  inputRef,
  onChange,
  ...props
}: Omit<ComponentProps<typeof TextField>, "onChange"> & {
  names?: ImportResult["names"];
  onChange: (value: string) => void;
}) {
  const id = useId();
  const unique = [...new Map((names ?? []).map((item) => [item.name, item])).values()];
  return (
    <div className="grid gap-2 text-sm">
      <label htmlFor={id} className="text-foreground-secondary">{label}</label>
      <div className="flex">
        <Input
          {...props}
          id={id}
          ref={inputRef}
          autoComplete="off"
          maxLength={200}
          aria-invalid={!!error}
          aria-describedby={error ? `${id}-error` : undefined}
          onChange={(event) => onChange(event.target.value)}
          className={`border-muted-foreground/75 bg-surface ${unique.length ? "rounded-r-none" : ""} ${error ? "border-destructive" : ""}`}
        />
        {unique.length > 0 && (
          <Select value={unique.some((item) => item.name === props.value) ? String(props.value) : ""} onValueChange={onChange} disabled={props.disabled}>
            <SelectTrigger aria-label={label} className="w-9 shrink-0 justify-center rounded-l-none border-l-0 border-muted-foreground/75 bg-surface px-2" />
            <SelectContent align="end" className="w-72 max-w-[calc(100vw-3rem)]">
              {unique.map((item) => (
                <SelectItem key={item.name} value={item.name} title={item.source}>{item.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        )}
      </div>
      {error && <span id={`${id}-error`} className="text-xs text-destructive">{error}</span>}
    </div>
  );
}
