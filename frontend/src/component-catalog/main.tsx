import { useEffect, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { ChevronRight, RotateCcw, Search } from "lucide-react";
import { Button } from "@/renderer/components/ui/button";
import { Input } from "@/renderer/components/ui/input";
import { AppearancePreviewProvider, skinOptions, useAppearance } from "../appearance";
import { Picker } from "../controls";
import { catalogs } from "../messages";
import { YouRunLogo } from "../yourun-logo";
import { audit, inventory, sections, type SectionId, type ComponentEntry } from "./inventory";
import { previews, FoundationsPreview } from "./preview-registry";
import { catalogMessages } from "./messages";
import "../style.css";

const total = inventory.reduce((count, item) => count + item.names.length, 0);
const auditStatusColors = { fixed: "text-success", followUp: "text-warning", unverified: "text-muted-foreground" };
const entryId = (entry: ComponentEntry) => `component/${entry.names[0]}`;
function readLocation() {
  const hash = location.hash.slice(1);
  return sections.includes(hash as SectionId) || inventory.some((entry) => entryId(entry) === hash) ? hash : "buttons";
}

function Catalog() {
  const [locale, setLocale] = useState(navigator.language.startsWith("zh") ? "zh" : "en");
  const [query, setQuery] = useState("");
  const [selection, setSelection] = useState(readLocation);
  const [revision, setRevision] = useState(0);
  const main = useRef<HTMLElement>(null);
  const { skin, change } = useAppearance();
  const m = catalogs[locale];
  const c = catalogMessages[locale];
  const selected = inventory.find((entry) => entryId(entry) === selection);
  const activeSection = selected?.section ?? selection as SectionId;
  const term = query.trim().toLocaleLowerCase();
  const matches = inventory.filter((item) => `${item.names.join(" ")} ${item.source} ${c[item.section]}`.toLocaleLowerCase().includes(term));
  const entries = selected ? [selected] : matches.filter((item) => item.section === activeSection);
  const common = { m, c, locale, onLocale: setLocale };
  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    document.title = `YouRun · ${c.title}`;
  }, [locale, c.title]);
  useEffect(() => {
    const update = () => setSelection(readLocation());
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, []);
  useEffect(() => { main.current?.scrollTo(0, 0); }, [selection]);
  function navigate(id: string) {
    setSelection(id);
    location.hash = id;
  }
  function navigationLink(id: string, label: string, nested = false, count?: number) {
    return <Button asChild key={id} variant="ghost" className={`h-auto min-h-9 w-full min-w-0 justify-start rounded-sm px-2 py-2 text-left ${nested ? "pl-5 text-xs font-normal" : "text-sm"} ${selection === id ? "bg-selected text-foreground" : "text-muted-foreground"}`}>
      <a href={`#${id}`} aria-current={selection === id ? "page" : undefined} onClick={(event) => { event.preventDefault(); navigate(id); }}>
        <span className="min-w-0 flex-1 break-words whitespace-normal">{label}</span>
        {count !== undefined && <span className="shrink-0 text-xs tabular-nums text-muted-foreground">{count}</span>}
      </a>
    </Button>;
  }
  function componentPreview(item: ComponentEntry) {
    const name = item.names[0];
    const definition = item.live ? previews[name] : undefined;
    const Preview = definition?.render;
    return <section key={`${entryId(item)}-${revision}`} aria-label={item.names.join(" / ")}
      className={`min-w-0 space-y-5 border-b border-border pb-8 ${definition?.wide ? "col-span-full" : ""}`}>
      <div className="space-y-1">
        {!selected && <h3 className="flex min-w-0 items-center gap-2 text-sm font-semibold">
          <a href={`#${entryId(item)}`} className="min-w-0 break-words hover:underline focus-visible:outline-2 focus-visible:outline-ring" onClick={(event) => { event.preventDefault(); navigate(entryId(item)); }}>{name}</a>
          <ChevronRight className="size-3 shrink-0 text-muted-foreground" aria-hidden="true" />
        </h3>}
        {item.names.length > 1 && <div className="break-words text-xs text-muted-foreground">{item.names.slice(1).join(" · ")}</div>}
        <code className="block break-all text-xs text-muted-foreground">{item.source}</code>
      </div>
      {Preview ? <Preview {...common} only={name} /> : <span className="inline-block text-xs text-muted-foreground">{c.sourceOnly}</span>}
    </section>;
  }
  return <div className="flex h-dvh flex-col overflow-hidden bg-background">
    <header className="z-30 shrink-0 border-b border-border bg-background">
      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3 lg:px-6">
        <YouRunLogo size={24} /><h1 className="text-base font-semibold">{c.title}</h1>
        <span className="text-xs tabular-nums text-muted-foreground">{total} {c.total}</span>
        <div className="ml-auto flex items-center gap-2">
          <div className="w-28"><Picker label={m.skin} value={skin} onChange={change} options={skinOptions.map(({ id, label }) => [id, m[label]])} /></div>
          <div className="w-24"><Picker label={m.language} value={locale} onChange={setLocale} options={[["zh", "中文"], ["en", "English"]]} /></div>
          <Button variant="ghost" size="icon-sm" aria-label={c.reset} title={c.reset} onClick={() => setRevision((value) => value + 1)}><RotateCcw className="size-4" aria-hidden="true" /></Button>
        </div>
      </div>
    </header>
    <div className="grid min-h-0 flex-1 grid-rows-[auto_minmax(0,1fr)] min-[480px]:grid-cols-[10rem_minmax(0,1fr)] min-[480px]:grid-rows-1 lg:grid-cols-[14rem_minmax(0,1fr)]">
      <aside className="flex min-h-0 min-w-0 flex-col border-b border-border min-[480px]:border-r min-[480px]:border-b-0">
        <div className="shrink-0 space-y-3 p-3">
          <div className="relative">
            <Search className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground" aria-hidden="true" />
            <Input type="search" aria-label={c.search} placeholder={c.search} value={query} onChange={(event) => {
              setQuery(event.target.value);
              if (event.target.value.trim() && selection !== "inventory") navigate("inventory");
            }} className="pl-9" />
          </div>
          <div className="min-[480px]:hidden"><Picker label={c.component} value={selection} onChange={navigate}
            options={[...sections.map((id): [string, string] => [id, c[id]]), ...inventory.map((item): [string, string] => [entryId(item), item.names[0]])]} /></div>
        </div>
        <nav aria-label={c.categories} className="hidden min-h-0 flex-1 overflow-y-auto px-2 pb-4 min-[480px]:block">
          {sections.map((section) => {
            const children = matches.filter((item) => item.section === section);
            if (term && !children.length && section !== "inventory" && section !== "audit") return null;
            return <div key={section} className="mb-1">
              {navigationLink(section, c[section], false, inventory.filter((item) => item.section === section).length || undefined)}
              {(term || activeSection === section) && children.map((item) => navigationLink(entryId(item), item.names[0], true))}
            </div>;
          })}
        </nav>
      </aside>
      <main ref={main} className="min-h-0 min-w-0 overflow-auto px-4 py-6 lg:px-8" aria-labelledby="preview-heading">
        <div className="@container/gallery mx-auto max-w-[90rem]">
          <div className="mb-8 flex flex-wrap items-center gap-3">
            {selected && <Button variant="ghost" size="sm" onClick={() => navigate(activeSection)}>{c[activeSection]}</Button>}
            {selected && <ChevronRight className="size-3 text-muted-foreground" aria-hidden="true" />}
            <h2 id="preview-heading" className="min-w-0 break-words text-md font-semibold">{selected?.names[0] ?? c[activeSection]}</h2>
            {selected && <span className={`text-xs ${selected.live ? "text-success" : "text-muted-foreground"}`}>{selected.live ? c.interactive : c.sourceOnly}</span>}
          </div>
          {activeSection === "inventory" ? <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-border text-muted-foreground"><tr><th className="pb-3 pr-6 font-medium">{c.component}</th><th className="pb-3 pr-6 font-medium">{c.source}</th><th className="pb-3 font-medium">{c.coverage}</th></tr></thead>
              <tbody>{matches.map((item) => <tr key={entryId(item)} className="border-b border-border">
                <td className="min-w-48 py-3 pr-6"><a href={`#${entryId(item)}`} className="hover:underline" onClick={(event) => { event.preventDefault(); navigate(entryId(item)); }}>{item.names.join(" · ")}</a></td>
                <td className="py-3 pr-6 font-mono text-muted-foreground">{item.source}</td><td className={`whitespace-nowrap py-3 ${item.live ? "text-success" : "text-muted-foreground"}`}>{item.live ? c.interactive : c.sourceOnly}</td>
              </tr>)}</tbody>
            </table>
            {!matches.length && <p role="status" className="py-8 text-sm text-muted-foreground">{c.empty}</p>}
          </div> : activeSection === "audit" ? <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-border text-muted-foreground"><tr><th className="pb-3 pr-6 font-medium">{c.audit}</th><th className="pb-3 pr-6 font-medium">{c.source}</th><th className="pb-3 font-medium">{c.result}</th></tr></thead>
              <tbody>{audit.map((item) => <tr key={item.label} className="border-b border-border">
                <td className="min-w-48 py-3 pr-6">{c[item.label]}</td><td className="py-3 pr-6 font-mono text-muted-foreground">{item.source}</td>
                <td className={`whitespace-nowrap py-3 ${auditStatusColors[item.status]}`}>{c[item.status]}</td>
              </tr>)}</tbody>
            </table>
          </div> : <div key={selection} className={selected ? "space-y-8" : "grid grid-cols-1 items-start gap-x-8 gap-y-8 @min-[48rem]/gallery:grid-cols-2 @min-[76rem]/gallery:grid-cols-3"}>
            {activeSection === "foundations" && !selected && <div className="col-span-full"><FoundationsPreview {...common} only="foundations" /></div>}
            {entries.map(componentPreview)}
            {!entries.length && <p role="status" className="text-sm text-muted-foreground">{c.empty}</p>}
          </div>}
        </div>
      </main>
    </div>
  </div>;
}

createRoot(document.getElementById("root")!).render(<AppearancePreviewProvider><Catalog /></AppearancePreviewProvider>);
