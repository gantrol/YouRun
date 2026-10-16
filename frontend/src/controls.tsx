import { useId, type ReactNode, type ComponentProps, type Ref } from "react";
import { Input } from "@/renderer/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/renderer/components/ui/select";

export function TextField({
  label,
  error,
  inputRef,
  ...props
}: ComponentProps<typeof Input> & {
  label: string;
  error?: string;
  inputRef?: Ref<HTMLInputElement>;
}) {
  const id = useId();
  return (
    <div className="grid gap-2 text-sm">
      <label htmlFor={id} className="text-foreground-secondary">
        {label}
      </label>
      <Input
        {...props}
        ref={inputRef}
        id={id}
        aria-invalid={!!error}
        aria-describedby={error ? `${id}-error` : undefined}
        className={`border-muted-foreground/75 bg-surface ${error ? "border-destructive" : ""} ${props.className ?? ""}`}
      />
      {error && (
        <span id={`${id}-error`} className="text-xs text-destructive">
          {error}
        </span>
      )}
    </div>
  );
}

export function Picker({
  value,
  onChange,
  label,
  options,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  options: [string, string][];
  disabled?: boolean;
}) {
  return (
    <Select value={value} onValueChange={onChange} disabled={disabled}>
      <SelectTrigger
        aria-label={label}
        className="w-full border-muted-foreground/75 bg-surface"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map(([id, text]) => (
          <SelectItem key={id} value={id}>
            {text}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

export function FieldRow({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="grid gap-2 text-sm" role="group" aria-label={label}>
      <span className="text-foreground-secondary">{label}</span>
      {children}
    </div>
  );
}
