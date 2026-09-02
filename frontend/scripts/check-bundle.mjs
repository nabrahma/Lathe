/*
 * Guards the rules that are cheap to state and easy to lose.
 *
 * Each check below prevents a regression that would otherwise be found late,
 * by a user, on the one platform nobody tested.
 */

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const failures = [];

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) out.push(...walk(p));
    else out.push(p);
  }
  return out;
}

const sourceFiles = walk("src").filter((f) => /\.(ts|tsx|css|html)$/.test(f));
const bundleFiles = walk("dist").filter((f) => /\.(js|css|html)$/.test(f));
const all = [...sourceFiles, ...bundleFiles, "index.html"];

const read = (f) => readFileSync(f, "utf8");

// A native file dialog is the single highest-impact item on the native-feel
// list, and an HTML file input is the shortcut that survives to release.
for (const file of all) {
  if (/<input[^>]*type=["']?file/i.test(read(file))) {
    failures.push(`${file}: contains an HTML file input; use the OS dialog via api.chooseFiles`);
  }
}

// <video> does not reliably fire "ended" on WebKitGTK, so the interface uses
// none at all rather than shipping a workaround.
for (const file of all) {
  if (/<video[\s>]/i.test(read(file))) {
    failures.push(`${file}: contains a <video> element, which is unreliable on WebKitGTK`);
  }
}

// Fonts must be self-hosted: a network request at launch would break the
// offline guarantee, and an app that renders differently offline is a bug.
for (const file of all) {
  const text = read(file);
  for (const host of ["fonts.googleapis.com", "fonts.gstatic.com", "use.typekit", "cdn.jsdelivr", "unpkg.com"]) {
    if (text.includes(host)) {
      failures.push(`${file}: references ${host}; every asset must be bundled`);
    }
  }
}

// The hand cursor is a web convention. Native applications do not use it.
for (const file of all.filter((f) => f.endsWith(".css"))) {
  if (/cursor:\s*pointer/.test(read(file))) {
    failures.push(`${file}: uses cursor: pointer, which is a browser convention`);
  }
}

// CSS features WebKitGTK support for varies by distro. The design system is
// built from flexbox, grid, borders and solid fills precisely so it renders
// identically on all three engines.
const risky = [
  [/:has\(/, ":has()"],
  [/@container/, "container queries"],
  [/backdrop-filter/, "backdrop-filter"],
  [/grid-template-columns:\s*subgrid/, "subgrid"],
];
for (const file of all.filter((f) => f.endsWith(".css"))) {
  const text = read(file);
  for (const [pattern, name] of risky) {
    if (pattern.test(text)) {
      failures.push(`${file}: uses ${name}, which is not reliable across the three webviews`);
    }
  }
}

// No box-shadow anywhere: elevation is a background step plus a hairline.
for (const file of all.filter((f) => f.endsWith(".css"))) {
  if (/box-shadow:\s*(?!none)/.test(read(file))) {
    failures.push(`${file}: uses box-shadow; elevation is a background step and a border`);
  }
}

// Overscroll bounce is a browser behaviour, not an application one.
if (!sourceFiles.some((f) => f.endsWith(".css") && /overscroll-behavior:\s*none/.test(read(f)))) {
  failures.push("no stylesheet sets overscroll-behavior: none on the root");
}

if (failures.length > 0) {
  console.error("bundle checks failed:");
  for (const f of failures) console.error("  " + f);
  process.exit(1);
}
console.log(`bundle checks passed (${all.length} files)`);
