import { access, copyFile, mkdir, readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";

function fourPartVersion(value) {
  return typeof value === "string" && /^\d+\.\d+\.\d+\.\d+$/.test(value)
    && value.split(".").every((part) => Number(part) <= 65535 && String(Number(part)) === part);
}

function xml(value) {
  return String(value).replace(/[&<>"']/g, (character) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&apos;",
  })[character]);
}

export async function prepareMsix({ root, options, version, arch }) {
  const configFile = path.resolve(root, options["msix-config"] ?? "packaging/windows/msix.json");
  const config = JSON.parse(await readFile(configFile, "utf8"));
  const identityName = options["msix-identity"] ?? config.identityName;
  const publisher = options["msix-publisher"] ?? config.publisher;
  const publisherDisplayName = options["msix-publisher-name"] ?? config.publisherDisplayName;
  const packageVersion = `${version}.0`;
  if (typeof identityName !== "string" || !/^[A-Za-z0-9.-]{3,50}$/.test(identityName))
    throw new Error("MSIX identityName must contain 3–50 letters, digits, dots or hyphens");
  for (const [name, value] of Object.entries({ publisher, publisherDisplayName, displayName: config.displayName })) {
    if (typeof value !== "string" || !value.trim() || /[\x00-\x1f]/.test(value))
      throw new Error(`Invalid MSIX ${name}`);
  }
  if (!publisher.startsWith("CN=")) throw new Error("MSIX publisher must be the certificate/Partner Center distinguished name (CN=…)");
  if (!fourPartVersion(packageVersion) || !fourPartVersion(config.minVersion))
    throw new Error("MSIX versions require four integers between 0 and 65535");

  let makeappx = options.makeappx;
  if (!makeappx) {
    const kits = path.join(process.env["ProgramFiles(x86)"] ?? "C:/Program Files (x86)", "Windows Kits/10/bin");
    const versions = (await readdir(kits, { withFileTypes: true }))
      .filter((entry) => entry.isDirectory() && fourPartVersion(entry.name))
      .map((entry) => entry.name)
      .sort((a, b) => b.localeCompare(a, "en", { numeric: true }));
    for (const sdkVersion of versions) {
      const candidate = path.join(kits, sdkVersion, process.arch === "arm64" ? "arm64" : "x64", "makeappx.exe");
      try { await access(candidate); makeappx = candidate; break; }
      catch (error) { if (error.code !== "ENOENT") throw error; }
    }
  }
  if (!makeappx) throw new Error("Windows SDK MakeAppx.exe is required; install the SDK or pass --makeappx=PATH");
  makeappx = path.resolve(root, makeappx);
  await access(makeappx);
  return {
    root, makeappx, configFile, identityName, publisher, publisherDisplayName,
    displayName: config.displayName, minVersion: config.minVersion,
    version: packageVersion, architecture: arch === "amd64" ? "x64" : "arm64",
    executable: "YouRun.exe",
  };
}

export async function packageMsix({ settings, output, framework, execute }) {
  const directory = path.join(output, `msix-${framework}`);
  // A fresh directory prevents stale files or an earlier candidate being packaged.
  await mkdir(directory);
  const stage = path.join(directory, "contents");
  await mkdir(stage);
  // Only the application, CLI script and licenses are payloads; never include update archives,
  // build metadata, logs, signing keys or arbitrary files from the output folder.
  for (const name of [settings.executable, "yourun.cmd", "LICENSE", "THIRD-PARTY-NOTICES.txt", "THIRD-PARTY-LICENSES.txt"])
    await copyFile(path.join(output, name), path.join(stage, name));

  const template = await readFile(path.join(settings.root, "packaging/windows/AppxManifest.xml"), "utf8");
  const manifest = template.replace(/\{\{(\w+)\}\}/g, (_, key) => {
    if (!(key in settings)) throw new Error(`Unknown manifest field: ${key}`);
    return xml(settings[key]);
  });
  await writeFile(path.join(stage, "AppxManifest.xml"), manifest, { flag: "wx" });
  await execute("powershell.exe", [
    "-NoLogo", "-NoProfile", "-NonInteractive", "-File",
    path.join(settings.root, "packaging/windows/assets.ps1"),
    "-Destination", path.join(stage, "Assets"),
  ]);
  const filename = `${settings.identityName}_${settings.version}_${settings.architecture}.msix`;
  const artifact = path.join(directory, filename);
  // Keep MakeAppx's schema/semantic validation enabled (no /nv).
  await execute(settings.makeappx, ["pack", "/d", stage, "/p", artifact, "/h", "SHA256"]);
  await writeFile(path.join(directory, "package.json"), JSON.stringify({
    identityName: settings.identityName, publisher: settings.publisher,
    version: settings.version, architecture: settings.architecture, framework,
    signed: false, artifact: filename, minimumWindowsVersion: settings.minVersion,
    webView2Required: framework === "mygo",
  }, null, 2), { flag: "wx" });
  console.log(`Unsigned MSIX: ${artifact}`);
}
