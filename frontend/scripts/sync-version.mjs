// Keeps package.json and package-lock.json in sync with the repo-root
// VERSION file, the single place where the release version is maintained.
import { readFileSync, writeFileSync } from "node:fs";

const root = new URL("../../", import.meta.url);
const version = readFileSync(new URL("VERSION", root), "utf8").trim();

for (const name of ["package.json", "package-lock.json"]) {
  const path = new URL(`../${name}`, import.meta.url);
  const raw = readFileSync(path, "utf8");
  const json = JSON.parse(raw);
  let changed = json.version !== version;
  json.version = version;
  if (name === "package-lock.json" && json.packages?.[""]) {
    changed ||= json.packages[""].version !== version;
    json.packages[""].version = version;
  }
  if (changed) writeFileSync(path, JSON.stringify(json, null, 2) + "\n");
}
console.log("frontend version synced to", version);
