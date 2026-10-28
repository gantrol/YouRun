import { useMemo, type CSSProperties } from "react";
import { ExternalLink } from "lucide-react";
import type { Messages } from "./messages";

export type AnsiColor =
  | { kind: "index"; index: number }
  | { kind: "rgb"; r: number; g: number; b: number };

export type AnsiStyle = {
  fg?: AnsiColor;
  bg?: AnsiColor;
  bold?: boolean;
  dim?: boolean;
  italic?: boolean;
  underline?: boolean;
  strikethrough?: boolean;
  inverse?: boolean;
  linkUrl?: string;
};

export type StyledSpan = {
  text: string;
  style: AnsiStyle;
};

export type LineToken =
  | { kind: "text"; span: StyledSpan }
  | {
      kind: "url";
      displayUrl: string;
      openUrl: string;
      spans: StyledSpan[];
    };

export type DetectedUrl = {
  displayUrl: string;
  openUrl: string;
};

export type ParsedTerminalOutput = {
  lines: LineToken[][];
  urls: DetectedUrl[];
};

const EMPTY_STYLE: AnsiStyle = {};

function styleEquals(a: AnsiStyle, b: AnsiStyle): boolean {
  if (a === b) return true;
  return (
    colorEquals(a.fg, b.fg) &&
    colorEquals(a.bg, b.bg) &&
    !!a.bold === !!b.bold &&
    !!a.dim === !!b.dim &&
    !!a.italic === !!b.italic &&
    !!a.underline === !!b.underline &&
    !!a.strikethrough === !!b.strikethrough &&
    !!a.inverse === !!b.inverse &&
    (a.linkUrl ?? "") === (b.linkUrl ?? "")
  );
}

function colorEquals(a?: AnsiColor, b?: AnsiColor): boolean {
  if (!a && !b) return true;
  if (!a || !b || a.kind !== b.kind) return false;
  if (a.kind === "index" && b.kind === "index") return a.index === b.index;
  if (a.kind === "rgb" && b.kind === "rgb") {
    return a.r === b.r && a.g === b.g && a.b === b.b;
  }
  return false;
}

// Contrast-tuned terminal colors for YouRun's light semantic surfaces.
const FG_PALETTE: readonly string[] = [
  "#262320", // 0: black
  "#b42318", // 1: red
  "#2f6b4f", // 2: green
  "#9a5b06", // 3: yellow / amber
  "#1d5685", // 4: blue
  "#6e4052", // 5: magenta
  "#15616d", // 6: cyan / teal
  "#4a453e", // 7: white (adjusted for light background)
  "#6c665e", // 8: bright black / gray
  "#c92a2a", // 9: bright red
  "#23704a", // 10: bright green
  "#854d0e", // 11: bright yellow
  "#1864ab", // 12: bright blue
  "#862e65", // 13: bright magenta
  "#0c6978", // 14: bright cyan
  "#262320", // 15: bright white (adjusted for light background)
];

const BG_PALETTE: readonly string[] = [
  "#302b29", // 0: black badge
  "#b42318", // 1: red badge
  "#2f6b4f", // 2: green badge
  "#a15c07", // 3: yellow badge
  "#1d5685", // 4: blue badge
  "#6e4052", // 5: magenta badge
  "#15616d", // 6: cyan badge
  "#e6e1d6", // 7: white / light gray surface
  "#57514a", // 8: bright black
  "#c92a2a", // 9: bright red
  "#23704a", // 10: bright green
  "#854d0e", // 11: bright yellow
  "#1864ab", // 12: bright blue
  "#862e65", // 13: bright magenta
  "#0c6978", // 14: bright cyan
  "#fffefb", // 15: bright white surface
];

function xterm256ToRgb(index: number): [number, number, number] {
  if (index >= 16 && index <= 231) {
    const n = index - 16;
    const r = Math.floor(n / 36);
    const g = Math.floor((n % 36) / 6);
    const b = n % 6;
    const step = (v: number) => (v === 0 ? 0 : 55 + v * 40);
    return [step(r), step(g), step(b)];
  }
  const gray = 8 + (index - 232) * 10;
  return [gray, gray, gray];
}

function readableRgbForLightSurface(r: number, g: number, b: number): string {
  // Relative luminance check: darken overly bright colors meant for dark terminals.
  const lum = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  if (lum > 0.68) {
    const scale = 0.55 / lum;
    return `rgb(${Math.round(r * scale)}, ${Math.round(g * scale)}, ${Math.round(b * scale)})`;
  }
  return `rgb(${r}, ${g}, ${b})`;
}

function isDarkBackgroundColor(color?: AnsiColor): boolean {
  if (!color) return false;
  if (color.kind === "index") {
    if (color.index === 7 || color.index === 15) return false;
    if (color.index >= 0 && color.index <= 14) return true;
    const [r, g, b] = xterm256ToRgb(color.index);
    return (0.299 * r + 0.587 * g + 0.114 * b) / 255 < 0.55;
  }
  return (0.299 * color.r + 0.587 * color.g + 0.114 * color.b) / 255 < 0.55;
}

function resolveFgColor(color: AnsiColor | undefined, hasDarkBg: boolean): string | undefined {
  if (!color) return hasDarkBg ? "#fffefb" : undefined;
  if (color.kind === "index") {
    if (hasDarkBg && (color.index === 0 || color.index === 7 || color.index === 15)) {
      return "#fffefb";
    }
    if (color.index >= 0 && color.index < 16) {
      return FG_PALETTE[color.index];
    }
    const [r, g, b] = xterm256ToRgb(color.index);
    return hasDarkBg ? `rgb(${r}, ${g}, ${b})` : readableRgbForLightSurface(r, g, b);
  }
  return hasDarkBg
    ? `rgb(${color.r}, ${color.g}, ${color.b})`
    : readableRgbForLightSurface(color.r, color.g, color.b);
}

function resolveBgColor(color?: AnsiColor): string | undefined {
  if (!color) return undefined;
  if (color.kind === "index") {
    if (color.index >= 0 && color.index < 16) {
      return BG_PALETTE[color.index];
    }
    const [r, g, b] = xterm256ToRgb(color.index);
    return `rgb(${r}, ${g}, ${b})`;
  }
  return `rgb(${color.r}, ${color.g}, ${color.b})`;
}

function applySgrCodes(current: AnsiStyle, rawParams: string): AnsiStyle {
  if (!rawParams || rawParams === "0") {
    return current.linkUrl ? { linkUrl: current.linkUrl } : EMPTY_STYLE;
  }
  const next: AnsiStyle = { ...current };
  const parts = rawParams.split(";").flatMap((part) => part.split(":"));
  const codes = parts.map((p) => (p === "" ? 0 : Number.parseInt(p, 10)));

  for (let i = 0; i < codes.length; i++) {
    const code = codes[i];
    if (Number.isNaN(code)) continue;
    if (code === 0) {
      const linkUrl = next.linkUrl;
      Object.keys(next).forEach((k) => delete next[k as keyof AnsiStyle]);
      if (linkUrl) next.linkUrl = linkUrl;
    } else if (code === 1) {
      next.bold = true;
    } else if (code === 2) {
      next.dim = true;
    } else if (code === 3) {
      next.italic = true;
    } else if (code === 4) {
      next.underline = true;
    } else if (code === 7) {
      next.inverse = true;
    } else if (code === 9) {
      next.strikethrough = true;
    } else if (code === 21 || code === 22) {
      delete next.bold;
      delete next.dim;
    } else if (code === 23) {
      delete next.italic;
    } else if (code === 24) {
      delete next.underline;
    } else if (code === 27) {
      delete next.inverse;
    } else if (code === 29) {
      delete next.strikethrough;
    } else if (code >= 30 && code <= 37) {
      next.fg = { kind: "index", index: code - 30 };
    } else if (code === 39) {
      delete next.fg;
    } else if (code >= 40 && code <= 47) {
      next.bg = { kind: "index", index: code - 40 };
    } else if (code === 49) {
      delete next.bg;
    } else if (code >= 90 && code <= 97) {
      next.fg = { kind: "index", index: code - 90 + 8 };
    } else if (code >= 100 && code <= 107) {
      next.bg = { kind: "index", index: code - 100 + 8 };
    } else if (code === 38 || code === 48) {
      const isFg = code === 38;
      const mode = codes[i + 1];
      if (mode === 5 && i + 2 < codes.length) {
        const index = Math.max(0, Math.min(255, codes[i + 2] || 0));
        if (isFg) next.fg = { kind: "index", index };
        else next.bg = { kind: "index", index };
        i += 2;
      } else if (mode === 2 && i + 4 < codes.length) {
        const r = Math.max(0, Math.min(255, codes[i + 2] || 0));
        const g = Math.max(0, Math.min(255, codes[i + 3] || 0));
        const b = Math.max(0, Math.min(255, codes[i + 4] || 0));
        if (isFg) next.fg = { kind: "rgb", r, g, b };
        else next.bg = { kind: "rgb", r, g, b };
        i += 4;
      }
    }
  }
  return next;
}

export function normalizeOpenUrl(raw: string): string | null {
  const trimmed = raw.trim();
  if (!trimmed || trimmed.length > 2048) return null;
  try {
    const parsed = new URL(trimmed);
    if (parsed.protocol !== "http:" && parsed.protocol !== "https:") return null;
    if (!parsed.hostname || parsed.username || parsed.password) return null;
    if (parsed.hostname === "0.0.0.0") {
      parsed.hostname = "127.0.0.1";
    } else if (parsed.hostname === "[::]" || parsed.hostname === "::") {
      parsed.hostname = "localhost";
    }
    return parsed.toString();
  } catch {
    return null;
  }
}

const TRAILING_PUNCTUATION = new Set([
  ".",
  ",",
  ";",
  ":",
  "!",
  "?",
  "'",
  '"',
  "`",
  ">",
  "，",
  "。",
  "；",
  "：",
  "、",
  "！",
  "？",
  "）",
  "】",
  "」",
  "』",
  "》",
]);

function countChar(str: string, ch: string): number {
  let count = 0;
  for (let i = 0; i < str.length; i++) {
    if (str[i] === ch) count++;
  }
  return count;
}

function cleanMatchedUrl(candidate: string): string {
  let end = candidate.length;
  while (end > 0) {
    const ch = candidate[end - 1];
    if (TRAILING_PUNCTUATION.has(ch)) {
      end--;
      continue;
    }
    if (ch === ")") {
      const sub = candidate.slice(0, end);
      if (countChar(sub, ")") > countChar(sub, "(")) {
        end--;
        continue;
      }
    }
    if (ch === "]") {
      const sub = candidate.slice(0, end);
      if (countChar(sub, "]") > countChar(sub, "[")) {
        end--;
        continue;
      }
    }
    if (ch === "}") {
      const sub = candidate.slice(0, end);
      if (countChar(sub, "}") > countChar(sub, "{")) {
        end--;
        continue;
      }
    }
    break;
  }
  return candidate.slice(0, end);
}

const URL_PATTERN = /https?:\/\/[^\s<>'"`\x00-\x1f\uFF08\uFF09\u3010\u3011\u300A\u300B]+/gi;

function sliceSpans(spans: StyledSpan[], start: number, end: number): StyledSpan[] {
  if (start >= end) return [];
  const result: StyledSpan[] = [];
  let offset = 0;
  for (const span of spans) {
    const nextOffset = offset + span.text.length;
    if (nextOffset > start && offset < end) {
      const subStart = Math.max(0, start - offset);
      const subEnd = Math.min(span.text.length, end - offset);
      if (subStart < subEnd) {
        const text = span.text.slice(subStart, subEnd);
        const prev = result[result.length - 1];
        if (prev && styleEquals(prev.style, span.style)) {
          prev.text += text;
        } else {
          result.push({ text, style: span.style });
        }
      }
    }
    offset = nextOffset;
    if (offset >= end) break;
  }
  return result;
}

function tokenizeLineUrls(
  spans: StyledSpan[],
  seenUrls: Map<string, DetectedUrl>,
): LineToken[] {
  if (spans.length === 0) return [];
  const lineText = spans.map((s) => s.text).join("");
  if (
    !lineText.includes("http://") &&
    !lineText.includes("https://") &&
    !spans.some((s) => s.style.linkUrl)
  ) {
    return spans.map((span) => ({ kind: "text", span }));
  }

  const tokens: LineToken[] = [];
  let lastIndex = 0;
  URL_PATTERN.lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = URL_PATTERN.exec(lineText)) !== null) {
    const rawMatch = match[0];
    const cleaned = cleanMatchedUrl(rawMatch);
    if (!cleaned) continue;
    const openUrl = normalizeOpenUrl(cleaned);
    if (!openUrl) continue;

    const matchStart = match.index;
    const matchEnd = matchStart + cleaned.length;

    if (matchStart > lastIndex) {
      for (const span of sliceSpans(spans, lastIndex, matchStart)) {
        tokens.push({ kind: "text", span });
      }
    }

    const urlSpans = sliceSpans(spans, matchStart, matchEnd);
    tokens.push({
      kind: "url",
      displayUrl: cleaned,
      openUrl,
      spans: urlSpans,
    });
    if (!seenUrls.has(openUrl)) {
      seenUrls.set(openUrl, { displayUrl: cleaned, openUrl });
    }
    lastIndex = matchEnd;
    URL_PATTERN.lastIndex = matchEnd;
  }

  if (lastIndex < lineText.length) {
    for (const span of sliceSpans(spans, lastIndex, lineText.length)) {
      tokens.push({ kind: "text", span });
    }
  }

  const finalTokens: LineToken[] = [];
  for (const token of tokens) {
    if (token.kind === "text" && token.span.style.linkUrl) {
      const openUrl = token.span.style.linkUrl;
      const prev = finalTokens[finalTokens.length - 1];
      if (prev && prev.kind === "url" && prev.openUrl === openUrl && prev.displayUrl === openUrl) {
        prev.spans.push(token.span);
      } else {
        finalTokens.push({
          kind: "url",
          displayUrl: openUrl,
          openUrl,
          spans: [token.span],
        });
      }
      if (!seenUrls.has(openUrl)) {
        seenUrls.set(openUrl, { displayUrl: openUrl, openUrl });
      }
    } else {
      finalTokens.push(token);
    }
  }

  return finalTokens;
}

function isLocalUrl(url: string): boolean {
  try {
    const host = new URL(url).hostname.toLowerCase();
    return host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]";
  } catch {
    return false;
  }
}

export function stripAnsiAndControl(input: string): string {
  const parsed = parseTerminalOutput(input);
  return parsed.lines
    .map((tokens) =>
      tokens
        .map((t) => (t.kind === "text" ? t.span.text : t.displayUrl))
        .join(""),
    )
    .join("\n");
}

export function parseTerminalOutput(rawInput: string): ParsedTerminalOutput {
  if (!rawInput) return { lines: [], urls: [] };

  // Strip incomplete trailing ESC sequence at stream chunk boundary.
  const input = rawInput.replace(
    /\x1b(?:\[[\x30-\x3f]*[\x20-\x2f]*|\][^\x07\x1b]*)?$/,
    "",
  );

  type Cell = { char: string; style: AnsiStyle };
  const rawLines: StyledSpan[][] = [];
  let cells: Cell[] = [];
  let cursor = 0;
  let style: AnsiStyle = EMPTY_STYLE;

  const flushLine = () => {
    if (cells.length === 0) {
      rawLines.push([]);
      cursor = 0;
      return;
    }
    const spans: StyledSpan[] = [];
    for (let i = 0; i < cells.length; i++) {
      const cell = cells[i];
      const prev = spans[spans.length - 1];
      if (prev && styleEquals(prev.style, cell.style)) {
        prev.text += cell.char;
      } else {
        spans.push({ text: cell.char, style: cell.style });
      }
    }
    rawLines.push(spans);
    cells = [];
    cursor = 0;
  };

  const writeText = (chunk: string) => {
    for (const ch of chunk) {
      const code = ch.charCodeAt(0);
      if (code < 0x20 || code === 0x7f) {
        if (ch === "\t") {
          cells[cursor] = { char: "\t", style };
          cursor++;
        }
        continue;
      }
      cells[cursor] = { char: ch, style };
      cursor++;
    }
  };

  let i = 0;
  const len = input.length;
  while (i < len) {
    const ch = input[i];
    if (ch === "\r") {
      if (input[i + 1] === "\n") {
        flushLine();
        i += 2;
      } else {
        cursor = 0;
        i++;
      }
      continue;
    }
    if (ch === "\n") {
      flushLine();
      i++;
      continue;
    }
    if (ch === "\b") {
      cursor = Math.max(0, cursor - 1);
      i++;
      continue;
    }
    if (ch === "\x1b") {
      const next = input[i + 1];
      if (next === "[") {
        // CSI sequence: ESC [ params intermediates final
        let j = i + 2;
        while (j < len && input.charCodeAt(j) >= 0x30 && input.charCodeAt(j) <= 0x3f) j++;
        const params = input.slice(i + 2, j);
        while (j < len && input.charCodeAt(j) >= 0x20 && input.charCodeAt(j) <= 0x2f) j++;
        if (j < len && input.charCodeAt(j) >= 0x40 && input.charCodeAt(j) <= 0x7e) {
          const finalChar = input[j];
          if (finalChar === "m") {
            style = applySgrCodes(style, params);
          } else if (finalChar === "K") {
            const mode = Number.parseInt(params || "0", 10) || 0;
            if (mode === 0) {
              cells.length = Math.min(cells.length, cursor);
            } else if (mode === 1) {
              for (let k = 0; k < Math.min(cursor, cells.length); k++) {
                cells[k] = { char: " ", style: EMPTY_STYLE };
              }
            } else if (mode === 2) {
              cells = [];
              cursor = 0;
            }
          } else if (finalChar === "G") {
            const target = Math.max(0, (Number.parseInt(params || "1", 10) || 1) - 1);
            while (cells.length < target && cells.length < cursor + 128) {
              cells.push({ char: " ", style: EMPTY_STYLE });
            }
            cursor = Math.min(target, cells.length);
          } else if (finalChar === "C") {
            const step = Math.min(64, Math.max(1, Number.parseInt(params || "1", 10) || 1));
            for (let k = 0; k < step; k++) {
              if (cursor >= cells.length) {
                cells.push({ char: " ", style: EMPTY_STYLE });
              }
              cursor++;
            }
          } else if (finalChar === "D") {
            const step = Math.max(1, Number.parseInt(params || "1", 10) || 1);
            cursor = Math.max(0, cursor - step);
          }
          i = j + 1;
          continue;
        }
        i = j;
        continue;
      }
      if (next === "]") {
        // OSC sequence: ESC ] ... (BEL | ESC \)
        let j = i + 2;
        while (j < len) {
          if (input[j] === "\x07") {
            const oscBody = input.slice(i + 2, j);
            if (oscBody.startsWith("8;")) {
              const semi = oscBody.indexOf(";", 2);
              if (semi >= 0) {
                const rawHref = oscBody.slice(semi + 1);
                const openHref = rawHref ? normalizeOpenUrl(rawHref) : null;
                style = openHref
                  ? { ...style, linkUrl: openHref }
                  : { ...style, linkUrl: undefined };
              }
            }
            j++;
            break;
          }
          if (input[j] === "\x1b" && input[j + 1] === "\\") {
            const oscBody = input.slice(i + 2, j);
            if (oscBody.startsWith("8;")) {
              const semi = oscBody.indexOf(";", 2);
              if (semi >= 0) {
                const rawHref = oscBody.slice(semi + 1);
                const openHref = rawHref ? normalizeOpenUrl(rawHref) : null;
                style = openHref
                  ? { ...style, linkUrl: openHref }
                  : { ...style, linkUrl: undefined };
              }
            }
            j += 2;
            break;
          }
          j++;
        }
        i = j;
        continue;
      }
      if (next === "P" || next === "X" || next === "^" || next === "_") {
        // DCS / SOS / PM / APC string terminated by ST (ESC \) or BEL
        let j = i + 2;
        while (j < len) {
          if (input[j] === "\x07") {
            j++;
            break;
          }
          if (input[j] === "\x1b" && input[j + 1] === "\\") {
            j += 2;
            break;
          }
          j++;
        }
        i = j;
        continue;
      }
      // Two-byte or charset ESC sequence (e.g., ESC ( B, ESC =, ESC >, ESC c)
      let j = i + 1;
      while (j < len && input.charCodeAt(j) >= 0x20 && input.charCodeAt(j) <= 0x2f) j++;
      if (j < len && input.charCodeAt(j) >= 0x30 && input.charCodeAt(j) <= 0x7e) {
        if (input[j] === "c") style = EMPTY_STYLE;
        i = j + 1;
        continue;
      }
      i++;
      continue;
    }

    // Consume contiguous normal text chunk up to next control character
    let nextSpecial = i + 1;
    while (nextSpecial < len) {
      const c = input.charCodeAt(nextSpecial);
      if (c === 0x1b || c === 0x0a || c === 0x0d || c === 0x08) break;
      nextSpecial++;
    }
    writeText(input.slice(i, nextSpecial));
    i = nextSpecial;
  }

  flushLine();

  const seenUrls = new Map<string, DetectedUrl>();
  const lines = rawLines.map((spans) => tokenizeLineUrls(spans, seenUrls));
  const urls = Array.from(seenUrls.values()).sort((a, b) => {
    const aLocal = isLocalUrl(a.openUrl) ? 0 : 1;
    const bLocal = isLocalUrl(b.openUrl) ? 0 : 1;
    return aLocal - bLocal;
  });

  return { lines, urls };
}

function spanStyleProps(style: AnsiStyle): {
  className?: string;
  style?: CSSProperties;
} {
  const fgSource = style.inverse ? style.bg : style.fg;
  const bgSource = style.inverse ? style.fg : style.bg;
  const hasDarkBg = style.inverse
    ? !style.fg || isDarkBackgroundColor(style.fg)
    : isDarkBackgroundColor(bgSource);

  const color = style.inverse && !fgSource
    ? "var(--surface)"
    : resolveFgColor(fgSource, hasDarkBg);
  const backgroundColor = style.inverse && !bgSource
    ? "var(--foreground)"
    : resolveBgColor(bgSource);

  const classes: string[] = [];
  if (style.bold) classes.push("font-semibold");
  if (style.italic) classes.push("italic");
  if (style.underline && style.strikethrough) {
    classes.push("underline line-through");
  } else if (style.underline) {
    classes.push("underline");
  } else if (style.strikethrough) {
    classes.push("line-through");
  }
  if (style.dim && !color) {
    classes.push("text-muted-foreground");
  }
  if (backgroundColor) {
    classes.push("rounded-[2px] px-0.5");
  }

  const inlineStyle: CSSProperties = {};
  if (color) inlineStyle.color = color;
  if (backgroundColor) inlineStyle.backgroundColor = backgroundColor;
  if (style.dim && color) inlineStyle.opacity = 0.75;

  return {
    className: classes.length > 0 ? classes.join(" ") : undefined,
    style: Object.keys(inlineStyle).length > 0 ? inlineStyle : undefined,
  };
}

function renderSpan(span: StyledSpan, key: number) {
  const { className, style } = spanStyleProps(span.style);
  if (!className && !style) return span.text;
  return (
    <span key={key} className={className} style={style}>
      {span.text}
    </span>
  );
}

export function TerminalOutputContent({
  commandText,
  parsed,
  m,
  onOpenUrl,
}: {
  commandText: string;
  parsed: ParsedTerminalOutput;
  m: Pick<Messages, "openUrl">;
  onOpenUrl: (url: string) => void;
}) {
  return (
    <>
      {`$ ${commandText}\n`}
      {parsed.lines.map((tokens, lineIndex) => (
        <span key={lineIndex}>
          {lineIndex > 0 ? "\n" : null}
          {tokens.map((token, tokenIndex) => {
            if (token.kind === "text") {
              return renderSpan(token.span, tokenIndex);
            }
            const label = `${m.openUrl} · ${token.displayUrl}`;
            return (
              <span key={tokenIndex} className="inline">
                <a
                  href={token.openUrl}
                  onClick={(event) => {
                    event.preventDefault();
                    onOpenUrl(token.openUrl);
                  }}
                  title={label}
                  className="cursor-pointer underline decoration-current/45 underline-offset-2 hover:decoration-current"
                >
                  {token.spans.map((span, spanIndex) =>
                    renderSpan(span, spanIndex),
                  )}
                </a>
                <button
                  type="button"
                  onClick={() => onOpenUrl(token.openUrl)}
                  title={label}
                  aria-label={label}
                  className="ml-1.5 inline-flex h-5 select-none items-center gap-1 rounded-[var(--radius-md)] border border-border-strong/45 bg-surface px-1.5 align-middle font-sans text-[11px] leading-none font-medium text-foreground transition-colors hover:bg-hover-strong active:bg-pressed"
                >
                  <ExternalLink className="size-3 shrink-0" aria-hidden="true" />
                  <span>{m.openUrl}</span>
                </button>
              </span>
            );
          })}
        </span>
      ))}
    </>
  );
}

export function useParsedTerminalOutput(output: string): ParsedTerminalOutput {
  return useMemo(() => parseTerminalOutput(output), [output]);
}
