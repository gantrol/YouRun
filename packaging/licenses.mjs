import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { readFile, readdir, writeFile } from "node:fs/promises";
import path from "node:path";

const execute = promisify(execFile);
const legalName = /^(licen[cs]e|copying|copyright|notice|authors)([._-]|$)/i;

async function legalFiles(directory, prefix = "") {
  const entries = await readdir(directory, { withFileTypes: true });
  const result = [];
  for (const entry of entries.sort((a, b) => a.name.localeCompare(b.name, "en"))) {
    if (entry.name.toLowerCase().includes("trash")) continue;
    const relative = path.posix.join(prefix, entry.name);
    if (entry.isFile() && legalName.test(entry.name)) {
      result.push({ name: relative, text: await readFile(path.join(directory, entry.name), "utf8") });
    } else if (entry.isDirectory() && /^(licenses?|legal)$/i.test(entry.name)) {
      result.push(...await legalFiles(path.join(directory, entry.name), relative));
    }
  }
  return result;
}

// Read only installed production dependencies and modules actually linked for
// this target. License text is preserved; local cache paths are never emitted.
export async function writeDependencyLicenses({ root, framework, target, arch, output }) {
  const dependencies = new Map();
  const environment = { ...process.env, GOOS: target, GOARCH: arch, CGO_ENABLED: target === "darwin" && framework === "wails" ? "1" : "0", GOTOOLCHAIN: "go1.27.2+auto" };
  const { stdout } = await execute("go", ["list", "-deps", "-f", "{{if .Module}}{{.Module.Path}}|{{.Module.Version}}|{{.Module.Dir}}{{end}}", "."], {
    cwd: path.join(root, framework), env: environment, windowsHide: true, maxBuffer: 8 * 1024 * 1024,
  });
  for (const line of stdout.split(/\r?\n/).filter(Boolean)) {
    const [name, version, directory] = line.split("|");
    if (name.startsWith("commandmanager.local/")) continue;
    dependencies.set(`Go: ${name}@${version}`, directory);
  }
  const { stdout: goroot } = await execute("go", ["env", "GOROOT"], { env: environment, windowsHide: true });
  dependencies.set("Go standard library and runtime", goroot.trim());

  const frontend = path.join(root, "frontend");
  const lock = JSON.parse(await readFile(path.join(frontend, "package-lock.json"), "utf8"));
  for (const [location, record] of Object.entries(lock.packages)) {
    if (!location || record.dev || !location.startsWith("node_modules/")) continue;
    const directory = path.join(frontend, location);
    const manifest = JSON.parse(await readFile(path.join(directory, "package.json"), "utf8"));
    dependencies.set(`npm: ${manifest.name}@${manifest.version}`, directory);
  }
  const sections = ["YouRun bundled dependency licenses", "Generated from installed dependencies for this build. See THIRD-PARTY-NOTICES.txt for additional attributions."];
  for (const [name, directory] of [...dependencies].sort(([a], [b]) => a.localeCompare(b, "en"))) {
    const files = await legalFiles(directory);
    // The npm tarball omits its license. Preserve the upstream text locally so
    // builds do not fetch unpinned network content or invent copyright notices.
    if (name === "npm: react-remove-scroll-bar@2.3.8" && files.length === 0) {
      files.push({
        name: "LICENSE (https://github.com/theKashey/react-remove-scroll-bar/blob/8ca9ba5ea52de03308fe8ced94f7b159a44d28ff/LICENSE)",
        text: await readFile(path.join(root, "packaging/licenses/react-remove-scroll-bar-2.3.8.LICENSE"), "utf8"),
      });
    }
    if (!files.some((file) => /^(licen[cs]e|copying)/i.test(path.basename(file.name))))
      throw new Error(`Missing license text for ${name}`);
    sections.push(`\n${"=".repeat(72)}\n${name}\n${"=".repeat(72)}`);
    for (const file of files) sections.push(`--- ${file.name} ---\n${file.text.trimEnd()}`);
  }
  await writeFile(path.join(output, "THIRD-PARTY-LICENSES.txt"), sections.join("\n\n") + "\n");
  console.log(`Collected licenses for ${dependencies.size} dependencies`);
}
