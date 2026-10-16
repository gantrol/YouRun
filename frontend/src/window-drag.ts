// Wails' JS window option is not injected into URL-based pages on Windows.
// Keep the small, version-pinned native drag adapter in the loaded UI bundle.
export function installWailsDrag() {
  type Bridge = { postMessage: (message: string) => void };
  const native = window as Window & {
    chrome?: { webview?: Bridge };
    webkit?: { messageHandlers?: { external?: Bridge } };
  };
  const drag = (event: MouseEvent) => {
    if (event.button !== 0 || !(event.target instanceof Element)) return;
    if (!event.target.closest("[data-window-drag]")) return;
    if (event.target.closest("button, input, a, [data-window-no-drag]")) return;
    const bridge =
      native.chrome?.webview ?? native.webkit?.messageHandlers?.external;
    if (!bridge) return;
    event.preventDefault();
    bridge.postMessage(
      event.detail === 2 ? "wails:drag:doubleclick" : "wails:drag",
    );
  };
  document.addEventListener("mousedown", drag);
  return () => document.removeEventListener("mousedown", drag);
}
