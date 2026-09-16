#!/usr/bin/env node
/**
 * Fails if any component hardcodes a colour.
 *
 * The definition of done requires that the UI's colours come only from the
 * token config. That is the kind of rule which quietly decays under time
 * pressure, so it is checked rather than trusted: tokens.css is the one file
 * allowed to contain literal colour values.
 */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const root = resolve(import.meta.dirname, "..");

const roots = ["packages/ui/src", "apps/desktop/frontend/src"];
const allowed = new Set(["packages/ui/src/tokens.css"]);
const extensions = /\.(tsx?|css)$/;

// #abc, #aabbcc, #aabbccdd, rgb(...), rgba(...), hsl(...), hsla(...)
const colourPattern =
  /(#[0-9a-fA-F]{3,8}\b)|(\b(?:rgba?|hsla?)\s*\([^)]*\))/g;

const problems = [];

/**
 * Tailwind resolves `max-w-<name>` against the spacing namespace when a key of
 * that name exists. Because tokens.css defines --spacing-sm/lg/xl, the classes
 * max-w-sm/lg/xl silently mean 12px/24px/32px rather than a panel width — which
 * once collapsed the settings sheet to a 32px column. Use the named panel
 * widths (max-w-sheet, max-w-dialog, max-w-note) instead.
 */
const collidingWidth = /\bmax-w-(xs|sm|md|lg|xl|2xl|3xl|4xl|5xl|6xl|7xl)\b/g;

/**
 * Blanks out comments while preserving line numbering, so documentation that
 * quotes a token's value is not mistaken for a component using one.
 */
function stripComments(source) {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, " "))
    .replace(/(^|[^:])\/\/[^\n]*/g, (m, lead) => lead + " ".repeat(m.length - lead.length));
}

function walk(dir) {
  for (const entry of readdirSync(dir)) {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      walk(full);
      continue;
    }
    if (!extensions.test(entry)) continue;

    const rel = relative(root, full);
    if (allowed.has(rel)) continue;

    const lines = stripComments(readFileSync(full, "utf8")).split("\n");
    lines.forEach((line, i) => {
      // color-mix over a token is composition, not a hardcoded colour.
      if (line.includes("color-mix(")) return;
      for (const match of line.matchAll(colourPattern)) {
        problems.push({ file: rel, line: i + 1, text: match[0], source: line.trim(), kind: "colour" });
      }
      for (const match of line.matchAll(collidingWidth)) {
        problems.push({ file: rel, line: i + 1, text: match[0], source: line.trim(), kind: "width" });
      }
    });
  }
}

for (const dir of roots) walk(join(root, dir));

const colours = problems.filter((p) => p.kind === "colour");
const widths = problems.filter((p) => p.kind === "width");

if (colours.length > 0) {
  console.error(`\nHardcoded colours found (${colours.length}). Use a token from packages/ui/src/tokens.css.\n`);
  for (const p of colours) {
    console.error(`  ${p.file}:${p.line}  ${p.text}`);
    console.error(`    ${p.source}`);
  }
}

if (widths.length > 0) {
  console.error(`\nColliding width classes found (${widths.length}).`);
  console.error("max-w-<size> resolves against the spacing scale here, so it is not a panel width.");
  console.error("Use max-w-sheet, max-w-dialog or max-w-note.\n");
  for (const p of widths) {
    console.error(`  ${p.file}:${p.line}  ${p.text}`);
    console.error(`    ${p.source}`);
  }
}

if (problems.length > 0) {
  console.error("");
  process.exit(1);
}

console.log("tokens: colours and panel widths all come from the token config");
