import type { Messages } from "./messages";
import { Disclosure } from "@/renderer/components/ui/disclosure";
import { ExpandableList } from "./expandable-list";

export function ScanIssues({ issues = [], m }: {
  issues?: { path: string; code: string }[];
  m: Messages;
}) {
  if (!issues.length) return null;
  const groups = new Map<string, Set<string>>();
  for (const issue of issues) {
    const paths = groups.get(issue.code) ?? new Set<string>();
    paths.add(issue.path);
    groups.set(issue.code, paths);
  }
  const count = [...groups.values()].reduce((total, paths) => total + paths.size, 0);
  return (
    <Disclosure className="border-t border-border pt-3 text-xs"
      label={<span className="text-warning">{m.scanIssues} · {new Intl.NumberFormat().format(count)}</span>}>
      <ul className="space-y-3">
        {[...groups].map(([code, paths]) => (
          <li key={code}>
            <span>{m[code as keyof Messages] ?? m.scanPartial}{paths.size > 1 && ` · ${new Intl.NumberFormat().format(paths.size)}`}</span>
            <ExpandableList items={[...paths]} m={m} className="mt-1 space-y-1"
              renderItem={(path) => <li key={path}><code className="break-all text-muted-foreground">{path}</code></li>} />
          </li>
        ))}
      </ul>
    </Disclosure>
  );
}
