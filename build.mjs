import { spawn } from "node:child_process";
import { mkdir, chmod, writeFile, readFile, access, copyFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { prepareMsix, packageMsix } from "./packaging/windows/msix.mjs";
import { withWindowsResources } from "./packaging/windows/resources.mjs";
import { writeDependencyLicenses } from "./packaging/licenses.mjs";

const root = path.dirname(fileURLToPath(import.meta.url));
const options = Object.fromEntries(
  process.argv.slice(2).map((arg) => {
    const value = arg.replace(/^--/, "");
    const separator = value.indexOf("=");
    return separator < 0
      ? [value, "true"]
      : [value.slice(0, separator), value.slice(separator + 1)];
  }),
);
const target =
  options.target ??
  (process.platform === "win32" ? "windows" : process.platform);
const arch = options.arch ?? (process.arch === "x64" ? "amd64" : process.arch);
const frameworks = options.framework
  ? [options.framework]
  : ["mygo"];
const version = JSON.parse(
  await readFile(path.join(root, "frontend/package.json"), "utf8"),
).version;
const build = options.build ?? String(Date.now());
const bundleBuild = options["bundle-build"] ?? "1";
if (!/^\d{1,4}(\.\d{1,2}){0,2}$/.test(bundleBuild))
  throw new Error("Invalid Apple bundle build number");
if (!/^\d+$/.test(build) || !/^\d+\.\d+\.\d+$/.test(version))
  throw new Error("Invalid build metadata");
const distribution = options.package === "msix" ? "msix" : "portable";
const metadataFlags = `-X commandmanager.local/core/internal/updater.Version=${version} -X commandmanager.local/core/internal/updater.Build=${build} -X commandmanager.local/core/internal/updater.Distribution=${distribution}`;
if (options.package && !["true", "false", "msix"].includes(options.package))
  throw new Error("Use --package=true|false|msix");
if (options.manifest && !["true", "false"].includes(options.manifest))
  throw new Error("Use --manifest=true|false");
if (options.release && !["true", "false"].includes(options.release))
  throw new Error("Use --release=true|false");
const releaseDirectory = path.join(root, "dist", "releases", version);
if (options.release === "true") {
  try {
    await access(path.join(releaseDirectory, "PUBLISHED"));
    throw new Error(`Release ${version} is marked published`);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
}
if (distribution === "msix" && (target !== "windows" || process.platform !== "win32" || options.framework !== "mygo"))
  throw new Error("MSIX needs Windows and --framework=mygo");
if (
  !["windows", "darwin"].includes(target) ||
  !["amd64", "arm64"].includes(arch) ||
  frameworks.some((f) => !["mygo", "mygo-native", "wails"].includes(f))
)
  throw new Error(
    "Use --target=windows|darwin --arch=amd64|arm64 --framework=mygo|mygo-native|wails",
  );
if (
  target === "darwin" &&
  process.platform !== "darwin" &&
  frameworks.includes("wails")
)
  throw new Error(
    "Build Wails for macOS on a Mac with Xcode command line tools. MyGo supports cross-compilation: --framework=mygo",
  );

// Resolve the SDK and validate package identity before compiling anything.
const msix = distribution === "msix"
  ? await prepareMsix({ root, options, version, framework: frameworks[0], arch })
  : undefined;

function execute(command, args, cwd = root, env = {}) {
  if (command === "npm.cmd") {
    args = [
      path.join(
        path.dirname(process.execPath),
        "node_modules/npm/bin/npm-cli.js",
      ),
      ...args,
    ];
    command = process.execPath;
  }
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, {
      cwd,
      env: { ...process.env, ...env },
      stdio: "inherit",
      shell: false,
      windowsHide: true,
    });
    child.on("error", reject);
    child.on("exit", (code) =>
      code === 0 ? resolve() : reject(new Error(`${command} exited ${code}`)),
    );
  });
}

await execute(
  process.platform === "win32" ? "npm.cmd" : "npm",
  ["run", "build"],
  path.join(root, "frontend"),
);
const output = options.release === "true"
  ? path.join(releaseDirectory, distribution, `${target}-${arch}`)
  : path.join(root, "dist", "builds", build, `${target}-${arch}`);
await mkdir(output, { recursive: true });
const environment = {
  GOOS: target,
  GOARCH: arch,
  GOTOOLCHAIN: "go1.27.2+auto",
};
for (const framework of frameworks) {
  let folder = output;
  const executable = target === "windows" ? "YouRun.exe" : `command-manager-${framework}`;
  const artifact =
    target === "darwin"
      ? path.join(output, `Command Manager ${framework}.app`)
      : path.join(output, executable);
  try {
    await access(artifact);
    throw new Error(
      `Build artifact already exists; use a new --build value: ${artifact}`,
    );
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  if (target === "darwin") {
    const contents = path.join(
      output,
      `Command Manager ${framework}.app`,
      "Contents",
    );
    folder = path.join(contents, "MacOS");
    const resourcesDir = path.join(contents, "Resources");
    await mkdir(folder, { recursive: true });
    await mkdir(resourcesDir, { recursive: true });
    const iconPng = await readFile(
      path.join(root, "resources/branding/yourun-rabbit-app.png"),
    );
    const icnsHeader = Buffer.alloc(16);
    icnsHeader.write("icns", 0, "ascii");
    icnsHeader.writeUInt32BE(16 + iconPng.length, 4);
    icnsHeader.write("ic09", 8, "ascii");
    icnsHeader.writeUInt32BE(8 + iconPng.length, 12);
    await writeFile(
      path.join(resourcesDir, "icon.icns"),
      Buffer.concat([icnsHeader, iconPng]),
    );
    await writeFile(
      path.join(contents, "Info.plist"),
      `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleName</key><string>YouRun ${framework}</string>
<key>CFBundleDisplayName</key><string>YouRun</string>
<key>CFBundleIdentifier</key><string>local.commandmanager.${framework}</string>
<key>CFBundleExecutable</key><string>${executable}</string>
<key>CFBundleIconFile</key><string>icon</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleShortVersionString</key><string>${version}</string>
<key>CFBundleVersion</key><string>${bundleBuild}</string>
<key>LSMinimumSystemVersion</key><string>13.0</string>
<key>NSHighResolutionCapable</key><true/>
</dict></plist>`,
    );
  }
  const buildApplication = () => execute(
    "go",
    [
      "build",
      "-trimpath",
      "-ldflags",
      `${target === "windows" ? "-s -w -H=windowsgui" : "-s -w"} ${metadataFlags}`,
      "-o",
      path.join(folder, executable),
      ".",
    ],
    path.join(root, framework),
    {
      ...environment,
      CGO_ENABLED: target === "darwin" && framework === "wails" ? "1" : "0",
    },
  );
  if (target === "windows") {
    await withWindowsResources({ root, framework, arch, version, executable, execute }, buildApplication);
  } else {
    await buildApplication();
  }
  // One executable contains both interfaces. A tiny command script preserves
  // terminal waiting/exit-code behavior for Windows GUI-subsystem applications.
  const cli = path.join(folder, target === "windows" ? "yourun.cmd" : "yourun");
  await writeFile(cli, target === "windows"
    ? `@echo off\r\nsetlocal DisableDelayedExpansion\r\n"%~dp0${executable}" cli %*\r\nexit /b %errorlevel%\r\n`
    : `#!/bin/sh\ntarget=$0\nwhile [ -L "$target" ]; do\n  dir=$(CDPATH= cd -- "$(dirname -- "$target")" && pwd)\n  target=$(readlink "$target")\n  case "$target" in\n    /*) ;;\n    *) target="$dir/$target" ;;\n  esac\ndone\ndir=$(CDPATH= cd -- "$(dirname -- "$target")" && pwd)\nexec "$dir/${executable}" cli "$@"\n`, { flag: "wx" });
  if (target === "darwin") {
    await chmod(path.join(folder, executable), 0o755);
    await chmod(cli, 0o755);
  }
  const licenseDirectory = target === "darwin"
    ? path.join(path.dirname(folder), "Resources") : output;
  for (const name of ["LICENSE", "THIRD-PARTY-NOTICES.txt"])
    await copyFile(path.join(root, name), path.join(licenseDirectory, name));
  await writeDependencyLicenses({ root, framework, target, arch, output: licenseDirectory });
  console.log(path.join(folder, executable));
  await writeFile(
    path.join(output, `build-${framework}.json`),
    JSON.stringify(
      { version, build, platform: target, arch, framework, distribution },
      null,
      2,
    ),
  );
  // This machine-local source hint is intentionally excluded from update ZIPs.
  await writeFile(
    path.join(output, `local-source-${framework}.json`),
    JSON.stringify({ directory: root }),
  );
  if (msix) {
    await packageMsix({ settings: msix, output, framework, execute });
  }
  if (options.manifest === "true" || options.package === "true") {
    await execute("go", [
      "run",
      "./cmd/release",
      "--mode",
      options.package === "true" ? "pack" : "manifest",
      "--source",
      output,
      "--output",
      options.package === "true"
        ? (options.release === "true" ? path.join(releaseDirectory, "updates") : path.join(root, "dist/updates"))
        : output,
      "--framework",
      framework,
      "--platform",
      target,
      "--arch",
      arch,
      "--version",
      version,
      "--build",
      build,
      ...(options.key ? ["--key", options.key] : []),
      ...(options["base-url"] ? ["--base-url", options["base-url"]] : []),
    ]);
  }
}
