import fs from "node:fs";
import path from "node:path";
import os from "node:os";
import chokidar from "chokidar";
import { diffLines } from "diff";

const VAULT = process.env.SAURON_VAULT_DIR || path.join(os.homedir(), "Savar");
const STATE_DIR = path.join(os.homedir(), ".sauron-ui");
const EDITS_LOG = path.join(STATE_DIR, "edits.jsonl");
const DEBOUNCE_MS = Number(process.env.SAURON_EDIT_DEBOUNCE_MS ?? 12000);

export type EditEvent = {
  ts: number;
  path: string;
  rel: string;
  note: string;
  added: number;
  removed: number;
  preview: string;
};

type Pending = { timer: ReturnType<typeof setTimeout>; latest: string; firstSeen: number };

const baseline = new Map<string, string>();
const pending = new Map<string, Pending>();
let started = false;

function readSafe(p: string): string | null {
  try {
    return fs.readFileSync(p, "utf8");
  } catch {
    return null;
  }
}

function flush(abs: string) {
  const p = pending.get(abs);
  if (!p) return;
  pending.delete(abs);

  const base = baseline.get(abs) ?? "";
  const latest = p.latest;
  if (base === latest) return;

  const parts = diffLines(base, latest);
  let added = 0;
  let removed = 0;
  const addedSnippets: string[] = [];
  for (const part of parts) {
    const lineCount = part.count ?? Math.max(0, part.value.split("\n").length - 1);
    if (part.added) {
      added += lineCount;
      if (addedSnippets.length < 5) {
        for (const l of part.value.split("\n")) {
          const t = l.trim();
          if (t && addedSnippets.length < 5) addedSnippets.push(t);
        }
      }
    } else if (part.removed) {
      removed += lineCount;
    }
  }

  baseline.set(abs, latest);
  if (added === 0 && removed === 0) return;

  const rel = path.relative(VAULT, abs);
  const event: EditEvent = {
    ts: Math.floor(Date.now() / 1000),
    path: abs,
    rel,
    note: path.basename(rel, ".md"),
    added,
    removed,
    preview: addedSnippets.join(" · ").slice(0, 240),
  };
  fs.appendFile(EDITS_LOG, JSON.stringify(event) + "\n", (err) => {
    if (err) console.error("[vault-watcher] append failed:", err);
  });
}

export function startVaultWatcher() {
  if (started) return;
  started = true;

  if (!fs.existsSync(VAULT)) {
    console.warn(`[vault-watcher] vault not found: ${VAULT}`);
    return;
  }
  fs.mkdirSync(STATE_DIR, { recursive: true });

  console.log(`[vault-watcher] watching ${VAULT}`);

  const watcher = chokidar.watch(VAULT, {
    persistent: true,
    ignoreInitial: false,
    awaitWriteFinish: { stabilityThreshold: 400, pollInterval: 100 },
    ignored: (p: string, stats?: fs.Stats) => {
      const base = path.basename(p);
      if (base.startsWith(".")) return true;
      if (stats && stats.isFile() && !p.endsWith(".md")) return true;
      return false;
    },
  });

  watcher.on("add", (abs) => {
    const content = readSafe(abs);
    if (content !== null) baseline.set(abs, content);
  });

  watcher.on("change", (abs) => {
    const now = readSafe(abs);
    if (now === null) return;
    const base = baseline.get(abs) ?? "";
    if (base === now) return;

    const existing = pending.get(abs);
    if (existing) {
      clearTimeout(existing.timer);
      existing.latest = now;
      existing.timer = setTimeout(() => flush(abs), DEBOUNCE_MS);
    } else {
      pending.set(abs, {
        latest: now,
        firstSeen: Date.now(),
        timer: setTimeout(() => flush(abs), DEBOUNCE_MS),
      });
    }
  });

  watcher.on("unlink", (abs) => {
    baseline.delete(abs);
    const p = pending.get(abs);
    if (p) clearTimeout(p.timer);
    pending.delete(abs);
  });

  watcher.on("error", (err) => {
    console.error("[vault-watcher] error:", err);
  });
}

export function readRecentEdits(limit = 20): EditEvent[] {
  try {
    const data = fs.readFileSync(EDITS_LOG, "utf8").trim();
    if (!data) return [];
    const lines = data.split("\n");
    const tail = lines.slice(-limit).reverse();
    const out: EditEvent[] = [];
    for (const line of tail) {
      try {
        out.push(JSON.parse(line) as EditEvent);
      } catch {}
    }
    return out;
  } catch (e) {
    if ((e as NodeJS.ErrnoException).code === "ENOENT") return [];
    throw e;
  }
}
