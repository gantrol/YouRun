import { api } from "./api";

// The host rejects scanBusy before changing metadata; only that response is
// safe to retry while a previous scan is releasing its slot.
export async function projectAPI<T>(
  path: "project-details" | "project-metadata" | "discover",
  method: string,
  body: unknown,
  signal?: AbortSignal,
): Promise<T> {
  for (let attempt = 0; ; attempt++) {
    signal?.throwIfAborted();
    try {
      return await api<T>(path, method, body, signal);
    } catch (error) {
      if (
        signal?.aborted ||
        !(error instanceof Error) ||
        error.message !== "scanBusy" ||
        attempt >= 16
      ) throw error;
      await new Promise<void>((resolve) => {
        const finish = () => {
          clearTimeout(timer);
          signal?.removeEventListener("abort", finish);
          resolve();
        };
        const timer = setTimeout(finish, Math.min(250 * (attempt + 1), 750));
        signal?.addEventListener("abort", finish, { once: true });
      });
    }
  }
}
