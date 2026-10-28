import { access, unlink, writeFile } from "node:fs/promises";
import path from "node:path";

// Pin the build tool without adding it to the application's runtime modules.
const winres = "github.com/tc-hib/go-winres@v0.3.3";

export async function withWindowsResources({ root, framework, arch, version, executable, execute }, build) {
  const icon = path.join(root, "resources/branding/yourun-rabbit.png");
  await access(icon);
  const resource = path.join(root, framework, `yourun_windows_${arch}.syso`);
  // Claim the file exclusively; never overwrite another build's resource file.
  await writeFile(resource, "", { flag: "wx" });
  try {
    await execute("go", [
      "run", winres, "simply",
      "--arch", arch,
      "--no-suffix", "--out", resource,
      "--manifest", "none",
      "--icon", icon,
      "--product-name", "YouRun",
      "--file-description", "YouRun",
      "--original-filename", executable,
      "--file-version", version,
      "--product-version", version,
    ], root, {
      // The resource generator runs on the host even when cross-compiling.
      GOOS: process.platform === "win32" ? "windows" : process.platform,
      GOARCH: process.arch === "x64" ? "amd64" : process.arch,
      GOTOOLCHAIN: "go1.27.2+auto",
    });
    await build();
  } finally {
    await unlink(resource);
  }
}
