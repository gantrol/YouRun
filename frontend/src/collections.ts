import { commandTitle, type Collection, type Command, type Project } from "./model";
import type { Messages } from "./messages";
import {
  HighlightText,
  collectionSearchScore,
  commandSearchScore,
  projectSearchScore,
  rankByScore,
  scoreField,
  scoreMultiTerm,
  searchTerms,
} from "./search";

export {
  HighlightText,
  collectionSearchScore,
  commandSearchScore,
  projectSearchScore,
  rankByScore,
  scoreField,
  scoreMultiTerm,
  searchTerms,
};

export function validateCollectionTree(collections: Collection[]) {
  const byId = new Map(collections.map(collection => [collection.id, collection]));
  if (byId.size !== collections.length || collections.length > 500) throw new Error("invalidDefinition");
  for (const collection of collections) {
    if (!collection.name.trim() || collection.name.length > 200) throw new Error("invalidDefinition");
    const seen = new Set([collection.id]);
    for (let parent = collection.parent; parent; parent = byId.get(parent)!.parent) {
      if (seen.has(parent) || !byId.has(parent)) throw new Error("collectionCycle");
      seen.add(parent);
    }
  }
}

export function collectionPath(collection: Collection, collections: Collection[]): string {
  const names = [collection.name], seen = new Set([collection.id]);
  let parent = collection.parent;
  while (parent && !seen.has(parent)) {
    seen.add(parent);
    const item = collections.find(item => item.id === parent);
    if (!item) break;
    names.unshift(item.name); parent = item.parent;
  }
  return names.join(" / ");
}

export function collectionDescendants(id: string, collections: Collection[]): Set<string> {
  const found = new Set([id]);
  let added = true;
  while (added) {
    added = false;
    for (const item of collections) if (item.parent && found.has(item.parent) && !found.has(item.id)) { found.add(item.id); added = true; }
  }
  return found;
}

export function commandMatches(
  command: Command,
  query: string,
  m: Messages,
  projectName = "",
  collectionNames: readonly string[] = [],
) {
  return commandSearchScore(command, query, m, projectName, collectionNames) > 0;
}

export function commandContexts(commands: Command[], projects: Project[], m: Messages, query = "") {
  const terms = searchTerms(query);
  const groups = new Map<string, Command[]>();
  for (const c of commands) {
    const name = commandTitle(c, m), group = groups.get(name) ?? [];
    group.push(c); groups.set(name, group);
  }
  return Object.fromEntries(commands.map(c => {
    const title = commandTitle(c, m);
    const projectName = projects.find(p => p.id === c.project)?.name;
    const duplicates = groups.get(title) ?? [];
    const sameProject = duplicates.filter(other => other.project === c.project);
    const source = c.source || c.directory;
    let matchedAlias: string | undefined;
    let showSource = duplicates.length > 1;
    let showText = sameProject.length > 1 && sameProject.filter(other => (other.source || other.directory) === source).length > 1;
    if (terms.length > 0) {
      matchedAlias = (c.aliases ?? []).find(alias =>
        terms.some(t => scoreField(alias, t, true) > 0 && scoreField(title, t, true) <= 0));
      for (const term of terms) {
        const inPrimary = scoreField(title, term, true) > 0
          || (projectName ? scoreField(projectName, term, true) > 0 : false)
          || (matchedAlias ? scoreField(matchedAlias, term, true) > 0 : false);
        if (!inPrimary) {
          if (scoreField(c.text, term, true) > 0) showText = true;
          else if (source && scoreField(source, term, false) > 0) showSource = true;
        }
      }
    }
    const context = showSource ? source : undefined;
    return [c.id, [projectName, matchedAlias, context, showText ? c.text : undefined].filter(Boolean).join(" · ")];
  }));
}

export function compactContext(context: string) {
  return context.split(" · ").map(part => {
    const segments = part.split(/[\\/]/);
    return segments.length > 3 ? "…/" + segments.slice(-2).join("/") : part;
  }).join(" · ");
}

