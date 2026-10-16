import { memo, type ReactNode } from "react";
import { commandTitle, type Collection, type Command, type Project } from "./model";
import type { Messages } from "./messages";

export type SearchField = {
  value: string;
  weight: number;
  subsequence?: boolean;
};

const WORD_BOUNDARY = /[\s\-_./\\:·,;|()[\]{}'"]+ /u;

function isBoundaryChar(ch: string | undefined): boolean {
  if (!ch) return true;
  return /[\s\-_./\\:·,;|()[\]{}'"`]/u.test(ch);
}

function isWordStart(text: string, index: number): boolean {
  if (index <= 0) return true;
  const prev = text[index - 1];
  const curr = text[index];
  if (isBoundaryChar(prev) && !isBoundaryChar(curr)) return true;
  const prevAscii = prev.charCodeAt(0) < 128;
  const currAscii = curr.charCodeAt(0) < 128;
  return prevAscii !== currAscii;
}

export function searchTerms(query: string, locale?: string): string[] {
  const trimmed = query.trim();
  if (!trimmed) return [];
  return trimmed
    .toLocaleLowerCase(locale)
    .split(/\s+/)
    .filter(Boolean);
}

function findCompactSubsequence(text: string, term: string): number[] | null {
  if (term.length < 2 || term.length > text.length) return null;
  const words: { start: number; token: string }[] = [];
  let tokenStart = -1;
  for (let i = 0; i <= text.length; i++) {
    const ch = text[i];
    if (i < text.length && !isBoundaryChar(ch)) {
      if (tokenStart < 0) tokenStart = i;
    } else if (tokenStart >= 0) {
      words.push({ start: tokenStart, token: text.slice(tokenStart, i) });
      tokenStart = -1;
    }
  }
  if (words.length >= term.length) {
    for (let offset = 0; offset <= words.length - term.length; offset++) {
      let allMatch = true;
      const indices: number[] = [];
      for (let k = 0; k < term.length; k++) {
        if (words[offset + k].token[0] !== term[k]) {
          allMatch = false;
          break;
        }
        indices.push(words[offset + k].start);
      }
      if (allMatch) return indices;
    }
  }

  const indices: number[] = [];
  let pos = 0;
  let boundaryOrConsecutive = 0;
  for (let i = 0; i < term.length; i++) {
    const ch = term[i];
    let found = -1;
    for (let j = pos; j < text.length; j++) {
      if (text[j] === ch && (j === pos || isWordStart(text, j))) {
        found = j;
        break;
      }
    }
    if (found < 0) {
      found = text.indexOf(ch, pos);
    }
    if (found < 0) return null;
    if (found === pos || isWordStart(text, found)) boundaryOrConsecutive++;
    indices.push(found);
    pos = found + 1;
  }

  const span = indices[indices.length - 1] - indices[0] + 1;
  if (boundaryOrConsecutive === term.length) return indices;
  if (term.length >= 3 && span <= Math.ceil(term.length * 2.2)) return indices;
  return null;
}

export function scoreField(value: string, term: string, allowSubsequence = false, locale?: string): number {
  const trimmed = value.trim();
  if (!trimmed || !term) return 0;
  const text = trimmed.toLocaleLowerCase(locale);
  if (text === term) return 120;
  if (text.startsWith(term)) {
    return 102 - Math.min(18, (text.length - term.length) * 0.35);
  }

  let boundaryExact = false;
  let boundaryPrefix = -1;
  for (let i = 0; i <= text.length - term.length; i++) {
    if (isWordStart(text, i) && text.startsWith(term, i)) {
      const end = i + term.length;
      if (end === text.length || isBoundaryChar(text[end])) {
        boundaryExact = true;
        break;
      }
      if (boundaryPrefix < 0) boundaryPrefix = i;
    }
  }
  if (boundaryExact) return 92;
  if (boundaryPrefix >= 0) return 84 - Math.min(12, boundaryPrefix * 0.15);

  const index = text.indexOf(term);
  if (index >= 0) {
    return 68 - Math.min(18, index * 0.25);
  }

  if (allowSubsequence) {
    const sub = findCompactSubsequence(text, term);
    if (sub) {
      const span = sub[sub.length - 1] - sub[0] + 1;
      return Math.max(36, 54 - Math.min(16, (span - term.length) * 2));
    }
  }
  return 0;
}

export function scoreMultiTerm(query: string, fields: readonly SearchField[], locale?: string): number {
  const terms = searchTerms(query, locale);
  if (!terms.length) return 1;
  let total = 0;
  let primaryMatchedAll = fields.length > 0;
  const maxWeight = fields.reduce((max, f) => Math.max(max, f.weight), 0);

  for (const term of terms) {
    let best = 0;
    let matchedPrimary = false;
    for (const field of fields) {
      if (!field.value) continue;
      const raw = scoreField(field.value, term, field.subsequence, locale);
      if (raw <= 0) continue;
      const weighted = raw * field.weight;
      if (weighted > best) best = weighted;
      if (field.weight >= maxWeight * 0.85) matchedPrimary = true;
    }
    if (best <= 0) return 0;
    if (!matchedPrimary) primaryMatchedAll = false;
    total += best;
  }

  if (primaryMatchedAll && terms.length > 1) total += 25;
  return total;
}

export function commandSearchScore(
  command: Command,
  query: string,
  m: Messages,
  projectName = "",
  collectionNames: readonly string[] = [],
): number {
  const terms = searchTerms(query);
  if (!terms.length) return 1;
  const title = commandTitle(command, m);
  const fields: SearchField[] = [
    { value: title, weight: 4.0, subsequence: true },
    ...(command.aliases ?? []).map((alias) => ({ value: alias, weight: 3.6, subsequence: true })),
    { value: projectName, weight: 2.4, subsequence: true },
    ...collectionNames.map((name) => ({ value: name, weight: 2.2, subsequence: true })),
    { value: command.text, weight: 2.0, subsequence: true },
    { value: command.source ?? "", weight: 1.3, subsequence: false },
    { value: command.directory, weight: 1.0, subsequence: false },
    { value: command.shell, weight: 0.8, subsequence: false },
  ];
  return scoreMultiTerm(query, fields);
}

export function projectSearchScore(
  project: Project,
  query: string,
  directory = "",
  bestCommandScore = 0,
): number {
  const terms = searchTerms(query);
  if (!terms.length) return 1;
  const direct = scoreMultiTerm(query, [
    { value: project.name, weight: 4.2, subsequence: true },
    { value: directory, weight: 1.4, subsequence: false },
  ]);
  const inherited = bestCommandScore * 0.85;
  if (direct <= 0 && inherited <= 0) return 0;
  return Math.max(direct, inherited) + (direct > 0 && inherited > 0 ? 20 : 0);
}

export function collectionSearchScore(
  collection: Collection,
  collections: readonly Collection[],
  query: string,
  pathText: string,
  bestCommandScore = 0,
): number {
  const terms = searchTerms(query);
  if (!terms.length) return 1;
  const direct = scoreMultiTerm(query, [
    { value: collection.name, weight: 4.0, subsequence: true },
    { value: pathText, weight: 2.8, subsequence: true },
  ]);
  const inherited = bestCommandScore * 0.85;
  return Math.max(direct, inherited);
}

export function rankByScore<T>(
  items: readonly T[],
  query: string,
  getScore: (item: T) => number,
): T[] {
  if (!searchTerms(query).length) return [...items];
  const scored: { item: T; score: number; index: number }[] = [];
  items.forEach((item, index) => {
    const score = getScore(item);
    if (score > 0) scored.push({ item, score, index });
  });
  scored.sort((a, b) => b.score - a.score || a.index - b.index);
  return scored.map((entry) => entry.item);
}

function collectHighlightRanges(text: string, terms: readonly string[], locale?: string): [number, number][] {
  if (!text || !terms.length) return [];
  const lower = text.toLocaleLowerCase(locale);
  if (lower.length !== text.length) return [];
  const ranges: [number, number][] = [];

  for (const term of terms) {
    if (!term) continue;
    let pos = 0;
    let foundSubstring = false;
    while (pos <= lower.length - term.length) {
      const idx = lower.indexOf(term, pos);
      if (idx < 0) break;
      ranges.push([idx, idx + term.length]);
      foundSubstring = true;
      pos = idx + Math.max(1, term.length);
    }
    if (!foundSubstring) {
      const sub = findCompactSubsequence(lower, term);
      if (sub) {
        for (const index of sub) {
          ranges.push([index, index + 1]);
        }
      }
    }
  }

  if (!ranges.length) return [];
  ranges.sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  const merged: [number, number][] = [ranges[0]];
  for (let i = 1; i < ranges.length; i++) {
    const prev = merged[merged.length - 1];
    const curr = ranges[i];
    if (curr[0] <= prev[1]) {
      prev[1] = Math.max(prev[1], curr[1]);
    } else {
      merged.push([...curr]);
    }
  }
  return merged;
}

export const HighlightText = memo(function HighlightText({
  text,
  query = "",
  locale,
}: {
  text: string;
  query?: string;
  locale?: string;
}): ReactNode {
  const terms = searchTerms(query, locale);
  if (!terms.length || !text) return text;
  const ranges = collectHighlightRanges(text, terms, locale);
  if (!ranges.length) return text;

  const nodes: ReactNode[] = [];
  let cursor = 0;
  for (const [start, end] of ranges) {
    if (start > cursor) {
      nodes.push(text.slice(cursor, start));
    }
    nodes.push(
      <mark
        key={start}
        className="rounded-none bg-selected px-0.5 font-medium text-foreground underline decoration-foreground/45 underline-offset-2"
      >
        {text.slice(start, end)}
      </mark>,
    );
    cursor = end;
  }
  if (cursor < text.length) {
    nodes.push(text.slice(cursor));
  }
  return <>{nodes}</>;
});

export { WORD_BOUNDARY };
