// SPDX-License-Identifier: AGPL-3.0-only
import { execFileSync } from "node:child_process";
import fs from "node:fs";

const version = fs.readFileSync("VERSION", "utf8").trim();
const tag = `v${version}`;
const tags = execFileSync("git", ["tag", "-l"], { encoding: "utf8" }).split("\n").map((line) => line.trim());
if (!tags.includes(tag)) {
  console.error(`tags check failed: annotated tag ${tag} is missing`);
  process.exit(1);
}

let topics = [];
try {
  const output = execFileSync("gh", ["repo", "view", "--json", "repositoryTopics"], { encoding: "utf8" });
  topics = (JSON.parse(output).repositoryTopics || []).map((topic) => topic.name);
} catch (error) {
  console.error("tags check failed: unable to read GitHub topics: " + error.message);
  process.exit(1);
}

const required = ["customer-support", "ai", "go", "self-hosted", "helpdesk", "postgres", "pgvector", "htmx", "templ", "rag"];
const missing = required.filter((topic) => !topics.includes(topic));
if (missing.length > 0) {
  console.error("tags check failed: missing topics " + missing.join(", "));
  process.exit(1);
}

console.log("tags check passed");
