export type { Definitions, Snapshot } from "./models/workspace";
export { onThisDevice, persistedCommand } from "./models/workspace";

let token: string | undefined;

function hostToken() {
  if (token === undefined) {
    token = location.hash.slice(1) || sessionStorage.getItem("hostToken") || "";
    sessionStorage.setItem("hostToken", token);
    history.replaceState(null, "", location.pathname);
  }
  return token;
}

export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch(`/api/${path}`, {
      method,
      headers: {
        Authorization: `Bearer ${hostToken()}`,
        "Content-Type": "application/json",
      },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(15000)])
        : path === "choose-directory" || path === "project-avatar" || path === "updates/source"
          ? undefined
          : AbortSignal.timeout(15000),
    });
  } catch {
    throw new Error("connectionLost");
  }
  const result = await response
    .json()
    .catch(() => ({ error: "requestFailed" }));
  if (!response.ok) throw new Error(result.error ?? "requestFailed");
  return result as T;
}

// Authenticated SSE via fetch; bearer tokens never enter URLs or browser history.
// Timers detect a dead connection and back off reconnects, not state changes.
export async function streamAPI<T>(path: string | (() => string), receive: (event: T) => void,
  failed: () => void, signal: AbortSignal): Promise<void> {
  let delay = 1000;
  while (!signal.aborted) {
    const connection = new AbortController();
    let deadline = setTimeout(() => connection.abort(), 15000);
    let reader: ReadableStreamDefaultReader<Uint8Array> | undefined;
    try {
      const response = await fetch(`/api/${typeof path === "function" ? path() : path}`, {
        headers: { Authorization: `Bearer ${hostToken()}`, Accept: "text/event-stream" },
        signal: AbortSignal.any([signal, connection.signal]),
      });
      if (!response.ok || !response.body || !response.headers.get("Content-Type")?.includes("text/event-stream")) throw new Error("connectionLost");
      reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";
      for (;;) {
        const { value, done } = await reader.read();
        if (done) throw new Error("connectionLost");
        clearTimeout(deadline); deadline = setTimeout(() => connection.abort(), 45000);
        buffer += decoder.decode(value, { stream: true });
        if (buffer.length > 16 << 20) throw new Error("requestFailed");
        let end: number;
        while ((end = buffer.indexOf("\n\n")) >= 0) {
          const frame = buffer.slice(0, end); buffer = buffer.slice(end + 2);
          const data = frame.split("\n").filter(line => line.startsWith("data:")).map(line => line.slice(5).trimStart()).join("\n");
          if (data) { receive(JSON.parse(data) as T); delay = 1000; }
        }
      }
    } catch { if (!signal.aborted) failed(); }
    finally { clearTimeout(deadline); connection.abort(); await reader?.cancel().catch(() => {}); }
    if (signal.aborted) return;
    await new Promise<void>(resolve => {
      const finish = () => { clearTimeout(timer); signal.removeEventListener("abort", finish); resolve(); };
      const timer = setTimeout(finish, delay);
      signal.addEventListener("abort", finish, { once: true });
      if (signal.aborted) finish();
    });
    delay = Math.min(delay * 2, 15000);
  }
}
