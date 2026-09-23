#!/usr/bin/env node
// third-party-notices writes THIRD_PARTY_NOTICES.md: every piece of other
// people's software that ships inside Lasso, with its licence and copyright
// notice.
//
// Most of these licences (MIT, BSD, ISC) allow redistribution on one
// condition: the notice travels with the copy. The GPL programs also owe
// recipients their source. This file is how Lasso keeps both promises, and it
// ships inside the app as well as sitting in the repository.
//
// It is generated from what is actually built rather than written by hand:
//   - Go modules: `go list -deps` of the app, for the platform it ships on,
//     so only modules compiled into the binary are listed.
//   - JavaScript: pnpm's production dependency tree of the frontend, plus
//     Tailwind, whose base styles are written into the built CSS.
//   - The four helper programs: the notices kept in third_party/.
//
//   node scripts/third-party-notices.mjs          write the file
//   node scripts/third-party-notices.mjs --check  fail if it is out of date
//
// --check runs in `make lint`, so adding a dependency without regenerating
// this file fails the build.

import { execFileSync } from "node:child_process";
import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const out = join(root, "THIRD_PARTY_NOTICES.md");
const check = process.argv.includes("--check");

const LICENSE_FILES = /^(licen[cs]e|copying|unlicense)(\.(md|txt|markdown))?$/i;
const NOTICE_FILES = /^notice(\.(md|txt))?$/i;

// Packages that declare a licence but publish no licence file. The text comes
// from the project's own repository and is kept in third_party/licences/,
// named after the package, so the notice still travels with the copy.
const OVERRIDES = join(root, "third_party/licences");
function override(name) {
  const file = join(OVERRIDES, `${name.replace("/", "__")}.txt`);
  return existsSync(file) ? readFileSync(file, "utf8").trim() : "";
}

/** licenceTexts reads a package's licence file, and its NOTICE if it has one. */
function licenceTexts(dir) {
  if (!dir || !existsSync(dir)) return { licence: "", notice: "" };
  const files = readdirSync(dir);
  const pick = (re) => files.filter((f) => re.test(f)).sort()[0];
  const read = (f) => (f ? readFileSync(join(dir, f), "utf8").trim() : "");
  return { licence: read(pick(LICENSE_FILES)), notice: read(pick(NOTICE_FILES)) };
}

function goModules() {
  const lines = execFileSync(
    "go",
    ["list", "-deps", "-f", "{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}", "."],
    {
      cwd: join(root, "apps/desktop"),
      env: { ...process.env, GOOS: "darwin", GOARCH: "arm64", CGO_ENABLED: "1" },
      encoding: "utf8",
    },
  )
    .split("\n")
    .filter(Boolean);

  const seen = new Map();
  for (const line of lines) {
    const [path, version, dir] = line.split("\t");
    // Lasso's own modules are covered by its own licence.
    if (path.startsWith("github.com/swayyaam/lasso")) continue;
    seen.set(path, { name: path, version, ...licenceTexts(dir) });
  }
  return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function jsPackages() {
  const frontend = join(root, "apps/desktop/frontend");
  const report = JSON.parse(
    execFileSync("pnpm", ["licenses", "list", "--prod", "--json"], { cwd: frontend, encoding: "utf8" }),
  );

  const seen = new Map();
  for (const [licence, pkgs] of Object.entries(report)) {
    for (const p of pkgs) {
      // Type declarations are read by the compiler and never reach the app.
      if (p.name.startsWith("@types/")) continue;
      // The workspace's own UI package is Lasso's code.
      if (p.name.startsWith("@lasso/")) continue;
      const texts = licenceTexts(p.paths?.[0]);
      if (!texts.licence) texts.licence = override(p.name);
      seen.set(p.name, {
        name: p.name,
        version: (p.versions ?? []).join(", "),
        spdx: licence,
        ...texts,
      });
    }
  }

  // Tailwind is a build tool, but its base styles are copied into the CSS the
  // app ships, so its notice travels too.
  const pnpmStore = join(root, "node_modules/.pnpm");
  const tw = readdirSync(pnpmStore).find((d) => /^tailwindcss@\d/.test(d));
  if (tw) {
    const dir = join(pnpmStore, tw, "node_modules/tailwindcss");
    const pkg = JSON.parse(readFileSync(join(dir, "package.json"), "utf8"));
    seen.set("tailwindcss", { name: "tailwindcss", version: pkg.version, spdx: pkg.license, ...licenceTexts(dir) });
  }

  return [...seen.values()].sort((a, b) => a.name.localeCompare(b.name));
}

function helpers() {
  const lock = JSON.parse(readFileSync(join(root, "binaries.lock.json"), "utf8"));
  const version = (name) => lock.binaries?.[name]?.version ?? "";
  const ffmpegNotice = readFileSync(join(root, "third_party/ffmpeg/NOTICE.md"), "utf8").trim();

  // The ffmpeg notice names the version it describes; a bump in the lock file
  // without a fresh notice would publish the wrong source directions.
  if (!ffmpegNotice.includes(`**Version:** ${version("ffmpeg")}`)) {
    throw new Error(
      `third_party/ffmpeg/NOTICE.md does not describe ffmpeg ${version("ffmpeg")} from binaries.lock.json`,
    );
  }

  return [
    `## yt-dlp ${version("yt-dlp")}

<https://github.com/yt-dlp/yt-dlp>. Released into the public domain under the
Unlicense, below. yt-dlp's own build bundles further libraries; their licences
ship beside it, in \`bin/yt-dlp/_internal/THIRD_PARTY_LICENSES.txt\`.

\`\`\`
${readFileSync(join(root, "third_party/yt-dlp/LICENSE"), "utf8").trim()}
\`\`\``,
    // Demoted a level so it sits under this file's own headings.
    ffmpegNotice.replace(/^# /m, "## ").replace(/^## (?!ffmpeg)/gm, "### ").replace("../../LICENSE", "LICENSE"),
    `## deno ${version("deno")}

<https://github.com/denoland/deno>. MIT licence:

\`\`\`
${readFileSync(join(root, "third_party/deno/LICENSE.md"), "utf8").trim()}
\`\`\``,
  ];
}

function section(pkg) {
  const head = `### ${pkg.name}${pkg.version ? ` ${pkg.version}` : ""}${pkg.spdx ? ` (${pkg.spdx})` : ""}`;
  if (!pkg.licence) {
    // A missing notice is a licence condition unmet, not a formatting gap.
    throw new Error(`${pkg.name} has no licence text: add it to third_party/licences/`);
  }
  const body = "```\n" + pkg.licence + "\n```";
  const notice = pkg.notice ? "\n\nNOTICE:\n\n```\n" + pkg.notice + "\n```" : "";
  return `${head}\n\n${body}${notice}`;
}

const doc = [
  `# Third-party notices

Lasso is © 2026 Swayam Mishra and its contributors, licensed under the GPL-3.0
(see LICENSE). It is built from other people's free software, listed here with
the licence and copyright notice each one asks to travel with it.

This file is generated by \`scripts/third-party-notices.mjs\` from what is
actually compiled into the app. Do not edit it by hand.

## The programs Lasso runs`,
  ...helpers(),
  `## Go libraries compiled into Lasso`,
  ...goModules().map(section),
  `## JavaScript libraries in Lasso's interface`,
  ...jsPackages().map(section),
].join("\n\n") + "\n";

if (check) {
  const current = existsSync(out) ? readFileSync(out, "utf8") : "";
  if (current !== doc) {
    console.error("THIRD_PARTY_NOTICES.md is out of date: run `make notices` and commit the result.");
    process.exit(1);
  }
  console.log("notices: THIRD_PARTY_NOTICES.md matches what is built");
} else {
  writeFileSync(out, doc);
  console.log(`wrote ${out}`);
}
