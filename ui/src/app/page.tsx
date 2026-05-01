"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { ThemeToggle } from "./theme-toggle";

type Snapshot = {
  ts: number;
  status: {
    running: boolean;
    pid: number | null;
    clipboard_captures: number;
    activity_entries: number;
    sessions: number;
  };
  context: {
    open_thread?: string;
    next_action?: string;
    recent_decisions?: string[];
    session_type: string;
    focus_score: number;
    session_age_min: number;
    dominant_app: string;
    recent_clipboard: string[];
    local_servers?: { port: string; process: string; pid: string }[];
  } | null;
  activity: {
    hours: number;
    focus_score: number;
    app_breakdown: Record<string, number>;
    total_apps: number;
    switches: number;
  } | null;
  timeline: { timestamp: number; type: string; summary: string }[];
  clipboard: {
    id: number;
    content: string;
    source_app: string;
    captured_at: number;
  }[];
  traces: {
    id: number;
    outcome_type: string;
    outcome_detail: string;
    started_at: number;
    completed_at: number;
  }[];
  reentry: {
    project?: {
      name: string;
      kind: string;
    };
    task?: {
      task_id: string;
      status: string;
      goal: string;
      last_useful_state: string;
      next_action: string;
      confidence: number;
      started_at: number;
      updated_at: number;
    };
    trace?: {
      trace_id: string;
      trace_type: string;
      status: string;
      summary: string;
      completed_at: number;
    };
    events?: {
      id: number;
      ts: number;
      event_type: string;
      source_table?: string;
      source_id?: number;
      summary: string;
      app_name?: string;
      window_title?: string;
      artifact_uri?: string;
      severity: string;
    }[];
    reason: string;
    next_action: string;
    confidence: number;
    generated_at: number;
  } | null;
  experience: { total: number; success: number; failure: number; partial: number };
  hints: {
    id: string;
    label: string;
    confidence: number;
    weight: number;
    status: string;
    dominant_app: string;
    started_at: number;
    last_active_at: number;
    evidence_count: number;
    evidence: {
      id: number;
      ts: number;
      summary: string;
      app_name: string;
      severity: string;
    }[];
  }[];
  edits: {
    ts: number;
    rel: string;
    note: string;
    added: number;
    removed: number;
    preview: string;
  }[];
};

const POLL_MS = 2000;
const SESSION_GAP_SEC = 900;

function fmtHrs(h: number): string {
  if (!h) return "0m";
  const totalMin = Math.round(h * 60);
  const hh = Math.floor(totalMin / 60);
  const mm = totalMin % 60;
  if (hh === 0) return `${mm}m`;
  if (mm === 0) return `${hh}h`;
  return `${hh}h ${mm}m`;
}

function fmtAge(min: number): string {
  if (min < 1) return "just now";
  if (min < 60) return `${Math.round(min)}m`;
  const h = Math.floor(min / 60);
  const m = Math.round(min % 60);
  return m ? `${h}h ${m}m` : `${h}h`;
}

function fmtRel(ts: number, now: number): string {
  const s = now - ts;
  if (s < 5) return "now";
  if (s < 60) return `${s}s`;
  if (s < 3600) return `${Math.floor(s / 60)}m`;
  if (s < 86400) return `${Math.floor(s / 3600)}h`;
  return `${Math.floor(s / 86400)}d`;
}

function fmtClock(ts: number): string {
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
}

function fmtClockFull(ts: number): string {
  const d = new Date(ts * 1000);
  return d.toLocaleTimeString([], {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function plural(n: number, one: string, many = `${one}s`): string {
  return `${n} ${n === 1 ? one : many}`;
}

type Severity = "ok" | "warn" | "error";
type Category = "clipboard" | "edit" | "trace" | "session" | "activity" | "other";

type LogItem = {
  id: string;
  timestamp: number;
  category: Category;
  kind: string;
  severity: Severity;
  icon: string;
  text: string;
  full?: string;
  app?: string;
};

type Session = {
  id: string;
  firstTs: number;
  lastTs: number;
  items: LogItem[];
};

function severityForTrace(outcomeType: string): Severity {
  const o = (outcomeType || "").toLowerCase();
  if (o.includes("fail") || o.includes("error")) return "error";
  if (o.includes("partial") || o.includes("warn")) return "warn";
  return "ok";
}

function iconFor(category: Category, kind: string, severity: Severity): string {
  if (category === "trace") {
    if (severity === "error") return "❌";
    if (severity === "warn") return "⚠";
    return "✓";
  }
  if (category === "clipboard") return "📋";
  if (category === "edit") return "✎";
  if (category === "session") return "⚡";
  if (category === "activity") return "◦";
  if (kind === "reconnect") return "↻";
  return "·";
}

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
}

function buildLog(
  timeline: Snapshot["timeline"],
  clipboard: Snapshot["clipboard"],
  traces: Snapshot["traces"],
  edits: Snapshot["edits"],
): LogItem[] {
  const items: LogItem[] = [];

  for (const t of timeline) {
    if (t.type === "session" || t.type === "activity") continue;
    if (t.type === "clipboard" || t.type === "trace") continue;
    const cat: Category = "other";
    items.push({
      id: `tl-${t.timestamp}-${t.type}-${t.summary.slice(0, 20)}`,
      timestamp: t.timestamp,
      category: cat,
      kind: t.type,
      severity: "ok",
      icon: iconFor(cat, t.type, "ok"),
      text: truncate(t.summary, 120),
    });
  }

  for (const c of clipboard) {
    const flat = c.content.replace(/\s+/g, " ").trim();
    const head = c.source_app ? `[${c.source_app}] ` : "";
    items.push({
      id: `cb-${c.id}`,
      timestamp: c.captured_at,
      category: "clipboard",
      kind: "clipboard",
      severity: "ok",
      icon: iconFor("clipboard", "clipboard", "ok"),
      text: truncate(head + flat, 120),
      full: c.content,
      app: c.source_app,
    });
  }

  for (const t of traces) {
    const sev = severityForTrace(t.outcome_type);
    const label = t.outcome_type.replace(/_/g, " ");
    items.push({
      id: `tr-${t.id}`,
      timestamp: t.completed_at || t.started_at,
      category: "trace",
      kind: t.outcome_type,
      severity: sev,
      icon: iconFor("trace", t.outcome_type, sev),
      text: truncate(`${label} · ${t.outcome_detail}`, 120),
    });
  }

  for (const e of edits) {
    const meta = `+${e.added}/−${e.removed}`;
    items.push({
      id: `ed-${e.ts}-${e.rel}`,
      timestamp: e.ts,
      category: "edit",
      kind: "edit",
      severity: "ok",
      icon: iconFor("edit", "edit", "ok"),
      text: truncate(`${e.rel} ${meta} · ${e.note}`, 120),
      full: e.preview || undefined,
    });
  }

  return items
    .filter((i) => i.text && i.text.length > 0)
    .sort((a, b) => b.timestamp - a.timestamp);
}

function clusterSessions(items: LogItem[], gap = SESSION_GAP_SEC): Session[] {
  if (items.length === 0) return [];
  const sessions: Session[] = [];
  let current: Session | null = null;

  for (const it of items) {
    if (!current) {
      current = { id: `s-${it.timestamp}`, firstTs: it.timestamp, lastTs: it.timestamp, items: [it] };
      sessions.push(current);
      continue;
    }
    if (current.lastTs - it.timestamp < gap) {
      current.items.push(it);
      current.lastTs = it.timestamp;
    } else {
      current = { id: `s-${it.timestamp}`, firstTs: it.timestamp, lastTs: it.timestamp, items: [it] };
      sessions.push(current);
    }
  }

  return sessions;
}

function sessionMeta(s: Session): {
  title: string;
  subtitle: string;
  avatarGlyph: string;
  dominantCategory: Category;
  severity: Severity;
} {
  const categoryCount: Record<string, number> = {};
  const appCount: Record<string, number> = {};
  let severity: Severity = "ok";
  for (const it of s.items) {
    categoryCount[it.category] = (categoryCount[it.category] || 0) + 1;
    if (it.app) appCount[it.app] = (appCount[it.app] || 0) + 1;
    if (it.severity === "error") severity = "error";
    else if (it.severity === "warn" && severity !== "error") severity = "warn";
  }
  const dominantCategory = (Object.entries(categoryCount).sort((a, b) => b[1] - a[1])[0]?.[0] ||
    "other") as Category;
  const dominantApp = Object.entries(appCount).sort((a, b) => b[1] - a[1])[0]?.[0];

  const hasEdit = categoryCount["edit"] > 0;
  const hasTrace = categoryCount["trace"] > 0;

  let title: string;
  if (severity === "error") title = "Agent run with failure";
  else if (hasEdit && dominantApp) title = `${dominantApp} · ${categoryCount["edit"]} edit${categoryCount["edit"] > 1 ? "s" : ""}`;
  else if (hasEdit) title = `${categoryCount["edit"]} file edit${categoryCount["edit"] > 1 ? "s" : ""}`;
  else if (dominantApp) title = dominantApp;
  else if (hasTrace) title = "Agent activity";
  else title = `${s.items.length} events`;

  const parts: string[] = [];
  for (const [cat, n] of Object.entries(categoryCount).sort((a, b) => b[1] - a[1])) {
    parts.push(`${n} ${cat}`);
  }
  const subtitle = parts.join(" · ");

  const avatarGlyph = dominantApp
    ? dominantApp[0]?.toUpperCase() || "·"
    : iconFor(dominantCategory, dominantCategory, severity);

  return { title, subtitle, avatarGlyph, dominantCategory, severity };
}

function fmtDur(fromTs: number, toTs: number): string {
  const s = Math.max(0, toTs - fromTs);
  if (s < 60) return `${s}s`;
  const m = Math.round(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const rm = m % 60;
  return rm ? `${h}h ${rm}m` : `${h}h`;
}

const LIVE_WINDOW_SEC = 5 * 60;

function partitionByAge(log: LogItem[], nowSec: number, threshold = LIVE_WINDOW_SEC): [LogItem[], LogItem[]] {
  const live: LogItem[] = [];
  const older: LogItem[] = [];
  for (const it of log) {
    if (nowSec - it.timestamp < threshold) live.push(it);
    else older.push(it);
  }
  return [live, older];
}

function eventRateBuckets(log: LogItem[], nowSec: number, minutesBack = 30, bucketSec = 60): number[] {
  const buckets = new Array(minutesBack).fill(0) as number[];
  const windowStart = nowSec - minutesBack * bucketSec;
  for (const it of log) {
    if (it.timestamp < windowStart) continue;
    const idx = Math.floor((it.timestamp - windowStart) / bucketSec);
    if (idx >= 0 && idx < buckets.length) buckets[idx]++;
  }
  return buckets;
}

function computeDirection(log: LogItem[], nowSec: number, windowMin = 10): { icon: string; label: string } {
  const recent = log.filter((it) => nowSec - it.timestamp < windowMin * 60).length;
  const prior = log.filter((it) => {
    const age = nowSec - it.timestamp;
    return age >= windowMin * 60 && age < windowMin * 120;
  }).length;
  if (prior === 0 && recent === 0) return { icon: "·", label: "idle" };
  if (prior === 0 && recent > 0) return { icon: "↗", label: "starting" };
  const ratio = recent / Math.max(1, prior);
  if (ratio > 1.3) return { icon: "↗", label: "rising" };
  if (ratio < 0.7) return { icon: "↘", label: "slowing" };
  return { icon: "→", label: "steady" };
}

function basename(p: string): string {
  const parts = p.split("/");
  return parts[parts.length - 1] || p;
}

function chapterNarrative(
  s: Session,
  firstIds: Set<string>,
): { title: string; summary: string; severity: Severity; hasFirst: boolean } {
  const cat = { clipboard: 0, edit: 0, trace: 0, session: 0, activity: 0, other: 0 };
  const appCount: Record<string, number> = {};
  const fileCount: Record<string, number> = {};
  let errors = 0;
  let warns = 0;
  let hasFirst = false;

  for (const it of s.items) {
    cat[it.category] = (cat[it.category] || 0) + 1;
    if (it.app) appCount[it.app] = (appCount[it.app] || 0) + 1;
    if (it.severity === "error") errors++;
    else if (it.severity === "warn") warns++;
    if (firstIds.has(it.id)) hasFirst = true;
    if (it.category === "edit") {
      const rel = it.text.split(" ")[0];
      if (rel) fileCount[rel] = (fileCount[rel] || 0) + 1;
    }
  }

  const topApp = Object.entries(appCount).sort((a, b) => b[1] - a[1])[0]?.[0];
  const topFileEntry = Object.entries(fileCount).sort((a, b) => b[1] - a[1])[0];
  const topFile = topFileEntry ? basename(topFileEntry[0]) : undefined;
  const severity: Severity = errors > 0 ? "error" : warns > 0 ? "warn" : "ok";

  let title: string;
  if (errors > 0) title = "Interrupted by failure.";
  else if (cat.edit >= 3) title = topFile ? `Editing ${topFile}.` : "Editing session.";
  else if (cat.edit > 0 && cat.clipboard > 0) title = "Writing and gathering.";
  else if (cat.clipboard >= 5 && topApp) title = `Gathering from ${topApp}.`;
  else if (cat.clipboard > 0 && topApp) title = `${topApp} notes.`;
  else if (cat.trace > 0) title = "Agent activity.";
  else if (topApp) title = `${topApp}.`;
  else title = `A quiet moment.`;

  const parts: string[] = [];
  if (cat.edit > 0) {
    parts.push(
      `${cat.edit} edit${cat.edit > 1 ? "s" : ""}${topFile ? ` to ${topFile}` : ""}`,
    );
  }
  if (cat.clipboard > 0) {
    parts.push(
      `${cat.clipboard} clip${cat.clipboard > 1 ? "s" : ""}${topApp ? ` from ${topApp}` : ""}`,
    );
  }
  if (errors > 0) parts.push(`${errors} failure${errors > 1 ? "s" : ""}`);
  else if (cat.trace > 0) parts.push(`${cat.trace} trace${cat.trace > 1 ? "s" : ""}`);
  if (parts.length === 0 && cat.activity > 0) {
    parts.push(`${cat.activity} activity event${cat.activity > 1 ? "s" : ""}`);
  }

  const summary =
    parts.length > 0
      ? parts.join(", ").replace(/^./, (c) => c.toUpperCase()) + "."
      : `${s.items.length} events in this moment.`;

  return { title, summary, severity, hasFirst };
}

type SessionSample = {
  ts: number;
  app: string;
  focus: number;
  sessionType: string;
};

type InterruptTrigger = {
  ts: number;
  kind: "clipboard" | "edit" | "trace" | "app";
  summary: string;
  isNotify: boolean;
};

type InterruptAttribution = {
  triggers: InterruptTrigger[];
  focusBefore: number;
  focusAfter: number;
  appsAddedAfter: string[];
  sessionTypeBefore: string;
  sessionTypeAfter: string;
};

type InterruptType = "exo" | "endo" | "other";

type FlowWindow = {
  id: string;
  startTs: number;
  endTs: number;
  app: string;
  sessionType: string;
  avgFocus: number;
  sampleCount: number;
  durationSec: number;
  microLapses: number;
  isFlow: boolean;
  isAlmost: boolean;
  endedBy?: { app: string; isNotify: boolean };
  attribution?: InterruptAttribution;
  interruptType?: InterruptType;
  reEntrySec?: number;
};

const MICRO_LAPSE_FOCUS = 0.5;

const FLOW_MIN_SEC = 10 * 60;
const FLOW_ALMOST_SEC = 5 * 60;
const FLOW_FOCUS_AVG = 0.7;
const CONTIGUITY_GAP_SEC = 120;
const NOTIFY_APPS = new Set([
  "Messages",
  "Slack",
  "Mail",
  "Signal",
  "WhatsApp",
  "Discord",
  "Telegram",
  "Microsoft Outlook",
  "iMessage",
]);

const SESSION_SUMMARY_RE = /^(.+?) \(focus: (\d+)%, app: (.+)\)$/;

function parseSessionSamples(timeline: Snapshot["timeline"]): SessionSample[] {
  const out: SessionSample[] = [];
  for (const t of timeline) {
    if (t.type !== "session") continue;
    const m = SESSION_SUMMARY_RE.exec(t.summary);
    if (!m) continue;
    out.push({
      ts: t.timestamp,
      sessionType: m[1],
      focus: Number(m[2]) / 100,
      app: m[3],
    });
  }
  out.sort((a, b) => a.ts - b.ts);
  return out;
}

const INTERRUPT_WINDOW_SEC = 90;

function attributeInterrupt(
  w: FlowWindow,
  samples: SessionSample[],
  clipboard: Snapshot["clipboard"],
  edits: Snapshot["edits"],
  traces: Snapshot["traces"],
): InterruptAttribution {
  const boundary = w.endTs;
  const wStart = boundary - INTERRUPT_WINDOW_SEC;
  const wEnd = boundary + INTERRUPT_WINDOW_SEC;

  const triggers: InterruptTrigger[] = [];

  for (const c of clipboard) {
    if (c.captured_at < wStart || c.captured_at > wEnd) continue;
    const snippet = c.content.replace(/\s+/g, " ").trim().slice(0, 56);
    triggers.push({
      ts: c.captured_at,
      kind: "clipboard",
      summary: `clipboard from ${c.source_app || "?"} — "${snippet}${c.content.length > 56 ? "…" : ""}"`,
      isNotify: NOTIFY_APPS.has(c.source_app),
    });
  }
  for (const e of edits) {
    if (e.ts < wStart || e.ts > wEnd) continue;
    triggers.push({
      ts: e.ts,
      kind: "edit",
      summary: `edit ${basename(e.rel)} (+${e.added}/−${e.removed}) · ${e.note}`,
      isNotify: false,
    });
  }
  for (const t of traces) {
    const tts = t.completed_at || t.started_at;
    if (tts < wStart || tts > wEnd) continue;
    if (severityForTrace(t.outcome_type) !== "error") continue;
    triggers.push({
      ts: tts,
      kind: "trace",
      summary: `trace failed: ${t.outcome_detail}`,
      isNotify: false,
    });
  }
  triggers.sort((a, b) => a.ts - b.ts);

  const inFlow = samples.filter((s) => s.ts >= w.startTs && s.ts <= w.endTs);
  const lastFew = inFlow.slice(-3);
  const nextFew = samples.filter((s) => s.ts > w.endTs).slice(0, 3);
  const focusBefore = lastFew.length
    ? lastFew.reduce((a, s) => a + s.focus, 0) / lastFew.length
    : w.avgFocus;
  const focusAfter = nextFew.length
    ? nextFew.reduce((a, s) => a + s.focus, 0) / nextFew.length
    : 0;

  const appsIn = new Set(inFlow.map((s) => s.app));
  const appsAddedAfter: string[] = [];
  for (const s of samples) {
    if (s.ts <= w.endTs || s.ts > w.endTs + 180) continue;
    if (!appsIn.has(s.app) && !appsAddedAfter.includes(s.app)) {
      appsAddedAfter.push(s.app);
    }
  }

  const sessionTypeBefore = lastFew[lastFew.length - 1]?.sessionType || w.sessionType;
  const sessionTypeAfter = nextFew[0]?.sessionType || "?";

  return {
    triggers,
    focusBefore,
    focusAfter,
    appsAddedAfter,
    sessionTypeBefore,
    sessionTypeAfter,
  };
}

function detectFlowWindows(samples: SessionSample[]): FlowWindow[] {
  const windows: FlowWindow[] = [];
  if (samples.length === 0) return windows;

  type Candidate = {
    startTs: number;
    endTs: number;
    app: string;
    sessionType: string;
    focusSum: number;
    count: number;
    lapses: number;
  };
  let cur: Candidate | null = null;

  const close = (endedBySample: SessionSample | null) => {
    if (!cur) return;
    const durationSec = cur.endTs - cur.startTs;
    const avgFocus = cur.focusSum / cur.count;
    const isFlow = durationSec >= FLOW_MIN_SEC && avgFocus >= FLOW_FOCUS_AVG;
    const isAlmost =
      !isFlow && durationSec >= FLOW_ALMOST_SEC && avgFocus >= FLOW_FOCUS_AVG - 0.1;
    windows.push({
      id: `fw-${cur.startTs}-${cur.app}`,
      startTs: cur.startTs,
      endTs: cur.endTs,
      app: cur.app,
      sessionType: cur.sessionType,
      avgFocus,
      sampleCount: cur.count,
      durationSec,
      microLapses: cur.lapses,
      isFlow,
      isAlmost,
      endedBy: endedBySample
        ? { app: endedBySample.app, isNotify: NOTIFY_APPS.has(endedBySample.app) }
        : undefined,
    });
  };

  const initLapses = (s: SessionSample) => (s.focus < MICRO_LAPSE_FOCUS ? 1 : 0);

  for (const s of samples) {
    if (!cur) {
      cur = {
        startTs: s.ts,
        endTs: s.ts,
        app: s.app,
        sessionType: s.sessionType,
        focusSum: s.focus,
        count: 1,
        lapses: initLapses(s),
      };
      continue;
    }
    const sameApp = s.app === cur.app;
    const gap = s.ts - cur.endTs;
    if (sameApp && gap <= CONTIGUITY_GAP_SEC) {
      cur.endTs = s.ts;
      cur.focusSum += s.focus;
      cur.count += 1;
      cur.sessionType = s.sessionType;
      if (s.focus < MICRO_LAPSE_FOCUS) cur.lapses += 1;
    } else {
      close(s);
      cur = {
        startTs: s.ts,
        endTs: s.ts,
        app: s.app,
        sessionType: s.sessionType,
        focusSum: s.focus,
        count: 1,
        lapses: initLapses(s),
      };
    }
  }
  close(null);

  return windows;
}

function classifyInterrupt(w: FlowWindow): InterruptType {
  if (!w.endedBy || !w.attribution) return "other";
  const triggers = w.attribution.triggers;
  if (triggers.length === 0) return "endo";
  const nonEdit = triggers.filter((t) => t.kind !== "edit");
  if (nonEdit.length === 0) return "other";
  return "exo";
}

function median(nums: number[]): number {
  if (nums.length === 0) return 0;
  const sorted = [...nums].sort((a, b) => a - b);
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 0 ? (sorted[mid - 1] + sorted[mid]) / 2 : sorted[mid];
}

function buildEvidenceLine(
  events: NonNullable<Snapshot["reentry"]>["events"],
  edits: Snapshot["edits"],
): string {
  if (!events || events.length === 0) return "";

  const appMs: Record<string, number> = {};
  let editCount = 0;
  let editFile = "";

  for (const e of events) {
    if (e.app_name && e.app_name !== "" && e.event_type === "activity") {
      appMs[e.app_name] = (appMs[e.app_name] || 0) + 1;
    }
    if (e.event_type === "diff_summary") {
      editCount++;
      if (e.artifact_uri) editFile = basename(e.artifact_uri);
    }
  }

  if (editCount === 0 && edits.length > 0) {
    editCount = Math.min(edits.length, 3);
    editFile = basename(edits[0]?.rel || "");
  }

  const topApps = Object.entries(appMs)
    .sort((a, b) => b[1] - a[1])
    .slice(0, 3)
    .map(([app]) => app);

  const parts = [...topApps];
  if (editCount > 0) {
    parts.push(`${editCount} edit${editCount > 1 ? "s" : ""}${editFile ? ` to ${editFile}` : ""}`);
  }

  return parts.join(" · ");
}

function recentApps(clipboard: Snapshot["clipboard"], n = 3): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  const sorted = [...clipboard].sort((a, b) => b.captured_at - a.captured_at);
  for (const c of sorted) {
    if (!c.source_app) continue;
    if (seen.has(c.source_app)) continue;
    seen.add(c.source_app);
    out.push(c.source_app);
    if (out.length >= n) break;
  }
  return out;
}

export default function Page() {
  const [snap, setSnap] = useState<Snapshot | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [reconnecting, setReconnecting] = useState(false);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [expandedSessions, setExpandedSessions] = useState<Set<string>>(new Set());
  const [copied, setCopied] = useState<string | null>(null);
  const [firstIds, setFirstIds] = useState<Set<string>>(new Set());
  const firstSeenRef = useRef<Set<string> | null>(null);
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  const toggle = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSession = (id: string) => {
    setExpandedSessions((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const copyPermalink = async (id: string) => {
    if (typeof window === "undefined") return;
    const url = `${window.location.origin}${window.location.pathname}#${id}`;
    try {
      await navigator.clipboard.writeText(url);
      setCopied(id);
      setTimeout(() => setCopied((c) => (c === id ? null : c)), 1200);
    } catch {
      /* ignore */
    }
  };

  useEffect(() => {
    let alive = true;
    const tick = async () => {
      try {
        const r = await fetch("/api/snapshot", { cache: "no-store" });
        if (!r.ok) throw new Error(`${r.status}`);
        const j = (await r.json()) as Snapshot;
        if (alive) {
          setSnap(j);
          setErr(null);
          setReconnecting(false);
        }
      } catch (e) {
        if (alive) {
          setErr(e instanceof Error ? e.message : "offline");
          setReconnecting(true);
        }
      }
    };
    tick();
    timer.current = setInterval(tick, POLL_MS);
    return () => {
      alive = false;
      if (timer.current) clearInterval(timer.current);
    };
  }, []);

  const now = Math.floor(Date.now() / 1000);
  const status = snap?.status;
  const daemonPaused = status?.running === false;
  const context = snap?.context ?? null;
  const activity = snap?.activity;
  const reentry = snap?.reentry ?? null;
  const hints = useMemo(() => snap?.hints ?? [], [snap]);
  const timeline = useMemo(() => snap?.timeline ?? [], [snap]);
  const clipboard = useMemo(() => snap?.clipboard ?? [], [snap]);
  const traces = useMemo(() => snap?.traces ?? [], [snap]);
  const edits = useMemo(() => snap?.edits ?? [], [snap]);

  const apps = activity
    ? Object.entries(activity.app_breakdown).sort(([, a], [, b]) => b - a)
    : [];
  const topHours = apps[0]?.[1] ?? 1;
  const totalHours = apps.reduce((s, [, h]) => s + h, 0);

  const log = useMemo(
    () => buildLog(timeline, clipboard, traces, edits),
    [timeline, clipboard, traces, edits],
  );

  const [liveEvents, olderEvents] = useMemo(
    () => partitionByAge(log, now),
    [log, now],
  );

  const sessions = useMemo(() => clusterSessions(olderEvents), [olderEvents]);

  const rateBuckets = useMemo(() => eventRateBuckets(log, now, 30, 60), [log, now]);
  const direction = useMemo(() => computeDirection(log, now, 10), [log, now]);
  const flow = useMemo(() => recentApps(clipboard, 3), [clipboard]);
  const eventsLast30 = rateBuckets.reduce((a, b) => a + b, 0);
  const lastEventAgeSec = log.length > 0 ? now - log[0].timestamp : null;

  const flowWindows = useMemo(() => {
    const samples = parseSessionSamples(timeline);
    const windows = detectFlowWindows(samples);
    const enriched = windows.map((w) =>
      (w.isFlow || w.isAlmost) && w.endedBy
        ? { ...w, attribution: attributeInterrupt(w, samples, clipboard, edits, traces) }
        : w,
    );

    const flowOnly = enriched
      .filter((w) => w.isFlow)
      .sort((a, b) => a.startTs - b.startTs);
    const reEntryById = new Map<string, number>();
    for (let i = 0; i < flowOnly.length - 1; i++) {
      const gap = flowOnly[i + 1].startTs - flowOnly[i].endTs;
      reEntryById.set(flowOnly[i].id, gap);
    }

    const final = enriched.map((w) => ({
      ...w,
      interruptType: (w.isFlow || w.isAlmost) && w.endedBy ? classifyInterrupt(w) : undefined,
      reEntrySec: reEntryById.get(w.id),
    }));

    return final.sort((a, b) => b.startTs - a.startTs);
  }, [timeline, clipboard, edits, traces]);

  const flowStats = useMemo(() => {
    let flows = 0;
    let almosts = 0;
    let flowTime = 0;
    let interruptions = 0;
    let notifyInterruptions = 0;
    let exo = 0;
    let endo = 0;
    let totalLapses = 0;
    const reEntries: number[] = [];
    for (const w of flowWindows) {
      if (w.isFlow) {
        flows++;
        flowTime += w.durationSec;
        totalLapses += w.microLapses;
        if (w.endedBy) {
          interruptions++;
          if (w.endedBy.isNotify) notifyInterruptions++;
        }
        if (w.reEntrySec !== undefined) reEntries.push(w.reEntrySec);
      } else if (w.isAlmost) {
        almosts++;
        if (w.endedBy?.isNotify) notifyInterruptions++;
      }
      if (w.interruptType === "exo") exo++;
      else if (w.interruptType === "endo") endo++;
    }
    return {
      flows,
      almosts,
      flowTime,
      interruptions,
      notifyInterruptions,
      exo,
      endo,
      totalLapses,
      reEntryMedian: median(reEntries),
      reEntryMax: reEntries.length ? Math.max(...reEntries) : 0,
      reEntryCount: reEntries.length,
    };
  }, [flowWindows]);

  useEffect(() => {
    if (log.length === 0) return;
    const seen = firstSeenRef.current;
    if (seen === null) {
      const s = new Set<string>();
      for (const it of log) s.add(`${it.category}:${it.kind}`);
      firstSeenRef.current = s;
      return;
    }
    const newly: string[] = [];
    for (const it of log) {
      const key = `${it.category}:${it.kind}`;
      if (!seen.has(key)) {
        seen.add(key);
        newly.push(it.id);
      }
    }
    if (newly.length > 0) {
      setFirstIds((prev) => {
        const next = new Set(prev);
        newly.forEach((id) => next.add(id));
        return next;
      });
    }
  }, [log]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    const hash = window.location.hash.slice(1);
    if (!hash || !snap) return;
    const t = setTimeout(() => {
      const el = document.getElementById(hash);
      if (!el) return;
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      el.classList.add("flash");
      setTimeout(() => el.classList.remove("flash"), 1500);
    }, 300);
    return () => clearTimeout(t);
  }, [snap]);

  return (
    <main className="mx-auto max-w-6xl px-5 py-8 sm:px-8 lg:py-10">
      <header className="flex items-center justify-between">
        <h1 className="text-[16px] font-bold tracking-tight text-[var(--color-ink)]">
          sauron
        </h1>
        <div className="flex items-center gap-6 text-[12px] text-[var(--color-ink-soft)]">
          {err ? (
            <span className="text-[var(--color-signal)]">offline</span>
          ) : status === undefined ? (
            <span>checking</span>
          ) : status.running ? (
            <span className="pulse">pid {status.pid}</span>
          ) : (
            <span className="text-[var(--color-signal)]">daemon stopped</span>
          )}
          <ThemeToggle />
        </div>
      </header>

      {daemonPaused ? (
        <PausedPanel />
      ) : (
        <>
      <section className="mt-8 grid gap-4 lg:grid-cols-[minmax(0,1.45fr)_minmax(300px,0.75fr)]">
        <ReentryPanel reentry={reentry} context={context} edits={edits} hints={hints} />
        <NowPanel
          context={context}
          rateBuckets={rateBuckets}
          direction={direction}
          eventsLast30={eventsLast30}
          flow={flow}
        />
      </section>

      <details className="mt-4 border border-[var(--color-rule)] bg-[var(--color-paper-soft)] px-5 py-4">
        <summary className="cursor-pointer list-none">
          <div className="flex items-baseline justify-between">
            <div>
              <div className="eyebrow">diagnostics</div>
              <div className="mt-1 text-[12px] text-[var(--color-ink-soft)]">
                flow windows, interruptions, and model diagnostics
              </div>
            </div>
            <div className="text-right text-[12px] tnum text-[var(--color-ink-soft)]">
              {flowStats.flows} flow · {flowStats.interruptions} interrupted
            </div>
          </div>
        </summary>

        <div className="mt-4 flex items-baseline justify-between border-t border-[var(--color-rule)] pt-4">
          <div>
            <div className="eyebrow">attention signals</div>
            <div className="mt-1 text-[12px] text-[var(--color-ink-soft)]">
              last 2h · threshold &gt;=10m · avg focus &gt;=70%
            </div>
          </div>
          <div className="text-right text-[12px] tnum text-[var(--color-ink-soft)]">
            {flowStats.flows} flow · {flowStats.interruptions} interrupted
          </div>
        </div>

        <div className="mt-4 grid gap-3 sm:grid-cols-3 tnum">
          <div>
            <span className="text-[22px] font-bold text-[var(--color-ink)]">
              {flowStats.flows}
            </span>
            <span className="ml-1.5 text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)]">
              flow windows
            </span>
          </div>
          <div>
            <span className="text-[22px] font-bold text-[var(--color-ink)]">
              {fmtDur(0, flowStats.flowTime)}
            </span>
            <span className="ml-1.5 text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)]">
              in flow
            </span>
          </div>
          <div>
            <span className="text-[22px] font-bold text-[var(--color-signal)]">
              {flowStats.interruptions}
            </span>
            <span className="ml-1.5 text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)]">
              interruptions{flowStats.notifyInterruptions > 0 && ` · ${flowStats.notifyInterruptions} from notify apps`}
            </span>
          </div>
          {flowStats.almosts > 0 && (
            <div>
              <span className="text-[24px] font-bold text-[var(--color-accent)]">
                {flowStats.almosts}
              </span>
              <span className="ml-1.5 text-[12px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)]">
                almost-flow (5–10m)
              </span>
            </div>
          )}
        </div>

        <div className="mt-3 flex flex-wrap items-baseline gap-x-5 gap-y-1.5 text-[12px] text-[var(--color-ink-soft)] tnum">
          <div>
            <span className="text-[var(--color-ink-faint)]">re-entry</span>{" "}
            <span className="font-medium text-[var(--color-ink)]">
              {flowStats.reEntryCount > 0
                ? `~${fmtDur(0, Math.round(flowStats.reEntryMedian))} median`
                : "no data yet"}
            </span>
            {flowStats.reEntryMax > 0 && flowStats.reEntryCount > 1 && (
              <span className="text-[var(--color-ink-faint)]">
                {" "}
                · max {fmtDur(0, Math.round(flowStats.reEntryMax))}
              </span>
            )}
          </div>
          <div>
            <span className="text-[var(--color-ink-faint)]">interrupt split</span>{" "}
            <span className="font-medium text-[var(--color-signal)]">{flowStats.exo}</span>{" "}
            <span className="text-[var(--color-ink-faint)]">exo /</span>{" "}
            <span className="font-medium text-[var(--color-accent)]">{flowStats.endo}</span>{" "}
            <span className="text-[var(--color-ink-faint)]">endo</span>
          </div>
          <div>
            <span className="text-[var(--color-ink-faint)]">μ-lapses in flow</span>{" "}
            <span className="font-medium text-[var(--color-ink)]">
              {flowStats.totalLapses}
            </span>
          </div>
        </div>

        <details className="mt-4 border-t border-[var(--color-rule)] pt-3">
          <summary className="cursor-pointer text-[12px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]">
            show flow validation rows
          </summary>
          <div className="mt-3 overflow-x-auto">
            {flowWindows.length === 0 ? (
              <div className="py-3 text-[13px] text-[var(--color-ink-soft)]">
                no session samples in snapshot.
              </div>
            ) : (
              <ul className="min-w-[760px] divide-y divide-[var(--color-rule)]">
                {flowWindows.map((w) => {
                  const badge = w.isFlow ? "FLOW" : w.isAlmost ? "ALMOST" : "scatter";
                const badgeClass = w.isFlow
                  ? "text-[var(--color-accent)]"
                  : w.isAlmost
                    ? "text-[var(--color-accent)]"
                    : "text-[var(--color-ink-faint)]";
                const rowClass = w.isFlow
                  ? ""
                  : w.isAlmost
                    ? "opacity-90"
                    : "opacity-60";
                const trigger = w.endedBy
                  ? `→ ${w.endedBy.app}${w.endedBy.isNotify ? " (notify)" : ""}`
                  : "→ still ongoing";
                const attr = w.attribution;
                const focusDropPct = attr
                  ? Math.round((attr.focusBefore - attr.focusAfter) * 100)
                  : 0;
                  return (
                    <li key={w.id} className={`py-2 ${rowClass}`}>
                    <div className="grid grid-cols-[110px_52px_1fr_44px_84px_1fr] items-baseline gap-3 font-mono text-[12.5px] tnum">
                      <span className="text-[var(--color-ink-soft)]">
                        {fmtClock(w.startTs)}–{fmtClock(w.endTs)}
                      </span>
                      <span className="text-[var(--color-ink)]">
                        {fmtDur(0, w.durationSec)}
                      </span>
                      <span className="truncate text-[var(--color-ink)]">{w.app}</span>
                      <span className="text-right text-[var(--color-ink-soft)]">
                        {Math.round(w.avgFocus * 100)}%
                      </span>
                      <span className={`${badgeClass} tracking-wide`}>{badge}</span>
                      <span
                        className={`truncate ${
                          w.endedBy?.isNotify
                            ? "text-[var(--color-signal)]"
                            : "text-[var(--color-ink-faint)]"
                        }`}
                      >
                        {trigger}
                      </span>
                    </div>

                    {attr && (
                      <div className="ml-[122px] mt-1.5 space-y-1 border-l-2 border-[var(--color-rule)] pl-3 text-[12px] text-[var(--color-ink-soft)]">
                        {attr.triggers.length > 0 ? (
                          attr.triggers.map((t, i) => (
                            <div
                              key={i}
                              className={`flex items-baseline gap-2 ${
                                t.isNotify ? "text-[var(--color-signal)]" : ""
                              }`}
                            >
                              <span className="font-mono text-[11px] tnum text-[var(--color-ink-faint)]">
                                {fmtClockFull(t.ts)}
                              </span>
                              <span className="flex-1">{t.summary}</span>
                            </div>
                          ))
                        ) : (
                          <div className="italic text-[var(--color-ink-faint)]">
                            no explicit trigger event captured in ±90s window
                          </div>
                        )}
                        <div className="text-[11px] text-[var(--color-ink-faint)] pt-0.5">
                          focus {Math.round(attr.focusBefore * 100)}% →{" "}
                          {Math.round(attr.focusAfter * 100)}%
                          {focusDropPct >= 20 && (
                            <span className="ml-1 text-[var(--color-signal)]">
                              (−{focusDropPct}pt)
                            </span>
                          )}
                          {attr.appsAddedAfter.length > 0 &&
                            ` · +apps ${attr.appsAddedAfter.slice(0, 3).join(", ")}`}
                          {attr.sessionTypeBefore !== attr.sessionTypeAfter &&
                            ` · ${attr.sessionTypeBefore} → ${attr.sessionTypeAfter}`}
                        </div>
                        <div className="pt-0.5 text-[11px] text-[var(--color-ink-faint)]">
                          {w.interruptType && (
                            <span
                              className={
                                w.interruptType === "exo"
                                  ? "text-[var(--color-signal)]"
                                  : w.interruptType === "endo"
                                    ? "text-[var(--color-accent)]"
                                    : ""
                              }
                            >
                              {w.interruptType === "exo"
                                ? "exogenous"
                                : w.interruptType === "endo"
                                  ? "endogenous (mind-wandered)"
                                  : "unclassified"}
                            </span>
                          )}
                          {w.isFlow && (
                            <>
                              {w.interruptType && " · "}
                              μ-lapses in window: {w.microLapses}
                              {w.reEntrySec !== undefined && (
                                <>
                                  {" · re-entered after "}
                                  <span
                                    className={
                                      w.reEntrySec > 15 * 60
                                        ? "text-[var(--color-signal)]"
                                        : "text-[var(--color-ink-soft)]"
                                    }
                                  >
                                    {fmtDur(0, w.reEntrySec)}
                                  </span>
                                </>
                              )}
                              {w.reEntrySec === undefined &&
                                w.endedBy &&
                                " · never re-entered flow in this window"}
                            </>
                          )}
                        </div>
                      </div>
                    )}
                    </li>
                  );
                })}
              </ul>
            )}
          </div>
        </details>
      </details>

      <section className="mt-12">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">live · last 5m</div>
          <div className="text-[12px] tnum text-[var(--color-ink-soft)]">
            {liveEvents.length === 0
              ? lastEventAgeSec !== null
                ? `quiet · last event ${fmtRel(now - lastEventAgeSec, now)} ago`
                : "quiet"
              : `${liveEvents.length} ${liveEvents.length === 1 ? "event" : "events"}`}
          </div>
        </div>
        <div className="rule mt-3 pt-3">
          {liveEvents.length === 0 ? (
            <div className="py-4 text-[13px] text-[var(--color-ink-soft)]">
              no activity in the last 5 minutes.
            </div>
          ) : (
            <ul>
              {liveEvents.map((it) => (
                <li key={it.id}>
                  <LogLine
                    item={it}
                    isOpen={expanded.has(it.id)}
                    onToggle={() => it.full && toggle(it.id)}
                    onPermalink={() => copyPermalink(it.id)}
                    copied={copied === it.id}
                  />
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      <section className="mt-14">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">last 24h</div>
          {activity && (
            <div className="text-[12px] tnum text-[var(--color-ink-soft)]">
              {activity.switches} switches · {activity.total_apps} apps
            </div>
          )}
        </div>
        <div className="rule mt-3 pt-6">
          {activity ? (
            <>
              <div className="mb-6 flex items-baseline gap-4">
                <div className="text-[28px] font-bold tnum leading-none">
                  {fmtHrs(totalHours)}
                </div>
                <div className="text-[12px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)]">
                  tracked
                </div>
              </div>
              <div className="space-y-3">
                {apps.map(([name, h]) => (
                  <div
                    key={name}
                    className="grid grid-cols-[160px_1fr_72px] items-center gap-4"
                  >
                    <div className="truncate text-[14px] text-[var(--color-ink)]">
                      {name}
                    </div>
                    <div className="h-[3px] rounded bg-[var(--color-rule)]">
                      <div
                        className="h-full rounded bg-[var(--color-accent)]"
                        style={{ width: `${(h / topHours) * 100}%` }}
                      />
                    </div>
                    <div className="text-right text-[13px] tnum text-[var(--color-ink-soft)]">
                      {fmtHrs(h)}
                    </div>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <div className="py-4 text-[13px] leading-[1.55] text-[var(--color-ink-soft)]">
              no activity summary available. start the daemon to populate this view.
            </div>
          )}
        </div>
      </section>

      <section className="mt-14">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">earlier</div>
          <div className="flex items-center gap-4 text-[12px] text-[var(--color-ink-soft)]">
            {reconnecting && (
              <span className="text-[var(--color-signal)] tnum">↻ reconnecting</span>
            )}
            <span className="tnum">
              {olderEvents.length} events · {sessions.length} moments
            </span>
          </div>
        </div>
        <div className="mt-4">
          {sessions.length === 0 ? (
            <div className="rule pt-4">
              <div className="py-4 text-[13px] leading-[1.55] text-[var(--color-ink-soft)]">
                no earlier moments yet.
              </div>
            </div>
          ) : (
            sessions.map((s, idx) => {
              const narrative = chapterNarrative(s, firstIds);
              const orderedEvents = [...s.items].sort((a, b) => a.timestamp - b.timestamp);
              const age = fmtRel(s.firstTs, now);
              const dur = fmtDur(s.lastTs, s.firstTs);

              const chapterEyebrowClass =
                narrative.severity === "error"
                  ? "text-[var(--color-signal)]"
                  : narrative.hasFirst
                    ? "text-[var(--color-accent)]"
                    : "text-[var(--color-ink-soft)]";
              const chapterLineClass =
                narrative.severity === "error"
                  ? "bg-[var(--color-signal)]/40"
                  : narrative.hasFirst
                    ? "bg-[var(--color-accent)]/40"
                    : "bg-[var(--color-rule)]";

              return (
                <section
                  key={s.id}
                  id={s.id}
                  className={idx > 0 ? "mt-12" : "mt-2"}
                >
                  <div className="mb-5 flex items-center gap-3">
                    <div
                      className={`text-[11px] font-medium uppercase tracking-[0.14em] ${chapterEyebrowClass}`}
                    >
                      {narrative.hasFirst ? "✦ " : ""}
                      {age} ago
                    </div>
                    <div className={`h-px flex-1 ${chapterLineClass}`} />
                    <button
                      onClick={() => copyPermalink(s.id)}
                      className="font-mono text-[11px] tnum text-[var(--color-ink-soft)] hover:text-[var(--color-accent)]"
                      title="copy permalink"
                    >
                      {copied === s.id
                        ? "copied"
                        : `${fmtClock(s.lastTs)}${dur ? ` · ${dur}` : ""}`}
                    </button>
                  </div>

                  <h2 className="mb-2 text-[22px] font-bold leading-[1.2] tracking-tight text-[var(--color-ink)]">
                    {narrative.title}
                  </h2>
                  <p className="mb-6 max-w-[60ch] text-[15px] leading-[1.55] text-[var(--color-ink-soft)]">
                    {narrative.summary}
                  </p>

                  <ul className="space-y-3">
                    {orderedEvents.map((it) => {
                      const isExpanded = expanded.has(it.id);
                      const isFirst = firstIds.has(it.id);

                      const dotColor =
                        it.severity === "error"
                          ? "bg-[var(--color-signal)]"
                          : it.severity === "warn"
                            ? "bg-[var(--color-accent)]"
                            : isFirst
                              ? "bg-[var(--color-accent)]"
                              : isExpanded
                                ? "bg-[var(--color-ink-soft)]"
                                : "bg-[var(--color-ink-faint)]";

                      return (
                        <li
                          key={it.id}
                          className="flex items-start gap-4 opacity-80 transition-opacity hover:opacity-100"
                        >
                          <div className="mt-[5px] w-14 shrink-0 text-right font-mono text-[11px] tnum text-[var(--color-ink-soft)]">
                            {fmtClock(it.timestamp)}
                          </div>
                          <div className="relative mt-[7px] shrink-0">
                            <div
                              className={`h-1.5 w-1.5 rounded-full ring-[3px] ring-[var(--color-paper)] ${dotColor}`}
                            />
                          </div>
                          <div className="min-w-0 flex-1">
                            {isExpanded ? (
                              <FeaturedEvent
                                item={it}
                                isOpen={isExpanded}
                                onToggle={() => toggle(it.id)}
                                onPermalink={() => copyPermalink(it.id)}
                                copied={copied === it.id}
                              />
                            ) : (
                              <LogLine
                                item={it}
                                isOpen={isExpanded}
                                onToggle={() => toggle(it.id)}
                                onPermalink={() => copyPermalink(it.id)}
                                copied={copied === it.id}
                                forceExpandable
                              />
                            )}
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                </section>
              );
            })
          )}
        </div>
      </section>

      <footer className="mt-20 flex items-center justify-between rule pt-6 text-[11px] text-[var(--color-ink-faint)] tnum">
        <span>
          {status?.clipboard_captures ?? 0} clips · {status?.activity_entries ?? 0}{" "}
          activity · {status?.sessions ?? 0} sessions
        </span>
        <span>{snap ? fmtClockFull(snap.ts) : "—"}</span>
      </footer>
        </>
      )}
    </main>
  );
}

type ReentryEvent = NonNullable<NonNullable<Snapshot["reentry"]>["events"]>[number];

function normalizedEvidenceApp(e: ReentryEvent): string {
  const raw =
    e.app_name ||
    e.summary.match(/dominant app ([^.·,]+)/i)?.[1] ||
    e.window_title?.match(/^([^—|-]+)/)?.[1] ||
    "";
  return raw.replace(/\s+/g, " ").trim();
}

function extractLocalActivity(e: ReentryEvent): string | null {
  const haystack = `${e.summary || ""} ${e.window_title || ""} ${e.artifact_uri || ""}`;
  return haystack.match(/\blocalhost:\d+\b/i)?.[0] ?? null;
}

function extractEditedFile(e: ReentryEvent): string | null {
  const haystack = `${e.summary || ""} ${e.artifact_uri || ""}`;
  const match = haystack.match(/[\w.-]+\.(tsx|ts|jsx|js|go|css|md|json|html)\b/i);
  return match?.[0] ?? null;
}

function isEditEvidence(e: ReentryEvent): boolean {
  const haystack = `${e.event_type || ""} ${e.summary || ""} ${e.source_table || ""}`;
  return /\b(edit|file|diff|changed|added|removed)\b/i.test(haystack);
}

function formatEvidenceSummary(
  reentry: NonNullable<Snapshot["reentry"]>,
  context?: Snapshot["context"],
  edits: Snapshot["edits"] = [],
): string {
  const events = reentry.events ?? [];

  const appSpans = new Map<string, { first: number; last: number; count: number }>();
  const localhost = new Set<string>();
  const files = new Map<string, number>();
  let editCount = 0;

  for (const e of events) {
    const app = normalizedEvidenceApp(e);
    if (app) {
      const prev = appSpans.get(app);
      appSpans.set(app, {
        first: prev ? Math.min(prev.first, e.ts) : e.ts,
        last: prev ? Math.max(prev.last, e.ts) : e.ts,
        count: (prev?.count ?? 0) + 1,
      });
    }

    const local = extractLocalActivity(e);
    if (local) localhost.add(local);

    if (isEditEvidence(e)) {
      editCount++;
      const file = extractEditedFile(e);
      if (file) files.set(file, (files.get(file) ?? 0) + 1);
    }
  }

  const parts = [...appSpans.entries()]
    .sort((a, b) => b[1].count - a[1].count)
    .slice(0, 2)
    .map(([app, span]) => {
      const dur = fmtDur(span.first, span.last);
      return dur === "0s" ? `${app} active` : `${app} ${dur}`;
    });

  const local = [...localhost][0];
  if (local) parts.push(`${local} active`);

  if (parts.length === 0 && context?.dominant_app) {
    parts.push(`${context.dominant_app} ${fmtAge(context.session_age_min)}`);
  }

  const activeServer = context?.local_servers?.find((server) => {
    const process = server.process.toLowerCase();
    return (
      ["3000", "4000", "5173", "5174", "8000", "8080"].includes(server.port) ||
      /\b(node|next|vite|webpack|bun|deno)\b/.test(process)
    );
  })?.port;
  if (activeServer && !parts.some((part) => part.includes(`localhost:${activeServer}`))) {
    parts.push(`localhost:${activeServer} active`);
  }

  if (editCount === 0 && edits.length > 0) {
    const cutoff = (reentry.generated_at || Math.floor(Date.now() / 1000)) - 30 * 60;
    const recentEdits = edits.filter((e) => e.ts >= cutoff);
    editCount = recentEdits.length;
    for (const e of recentEdits) {
      const file = e.rel.split("/").pop();
      if (file) files.set(file, (files.get(file) ?? 0) + 1);
    }
  }

  if (editCount > 0) {
    const file = [...files.entries()].sort((a, b) => b[1] - a[1])[0]?.[0];
    parts.push(`${plural(editCount, "edit")}${file ? ` to ${file}` : ""}`);
  }

  if (parts.length > 0) return parts.join(" · ");

  const eventText = events
    .slice(-3)
    .map((e) => cleanContextText(e.summary))
    .filter(Boolean)
    .join(" · ");
  return eventText || "Evidence will appear once Sauron records activity for this thread.";
}

function PausedPanel() {
  return (
    <section className="mt-8 border border-[var(--color-rule)] bg-[var(--color-paper-soft)] px-5 py-6 md:px-6">
      <div className="eyebrow">daemon paused</div>
      <div className="mt-4 text-[28px] font-bold leading-[1.12] tracking-tight text-[var(--color-ink)]">
        Sauron is paused.
      </div>
      <p className="mt-3 max-w-[58ch] text-[15px] font-medium leading-[1.55] text-[var(--color-ink-soft)]">
        Run <span className="font-mono text-[var(--color-ink)]">sauron start</span> in your terminal to recover your current thread.
      </p>
    </section>
  );
}

const HINT_WEIGHT_THRESHOLD = 0.35;

function ReentryPanel({
  reentry,
  context,
  edits,
  hints,
}: {
  reentry: Snapshot["reentry"];
  context: Snapshot["context"];
  edits: Snapshot["edits"];
  hints: Snapshot["hints"];
}) {
  const primaryHint = hints.find((h) => h.weight >= HINT_WEIGHT_THRESHOLD && h.label !== "");
  const secondaryHint = hints.find((h) => h !== primaryHint && h.weight >= HINT_WEIGHT_THRESHOLD && h.label !== "");

  const CONFIDENCE_THRESHOLD = 0.75;
  const hasReentryTask = !!(reentry?.task && reentry.task.confidence >= CONFIDENCE_THRESHOLD);

  if (!primaryHint && !hasReentryTask) {
    return (
      <section className="border border-[var(--color-rule)] bg-[var(--color-paper-soft)] px-5 py-5">
        <div className="eyebrow">current thread</div>
        <div className="mt-4 text-[28px] font-bold leading-[1.12] tracking-tight text-[var(--color-ink)]">
          No active thread.
        </div>
        <p className="mt-3 max-w-[64ch] text-[14px] font-medium leading-[1.55] text-[var(--color-ink-soft)]">
          Keep working. Sauron will detect your current task automatically, or run{" "}
          <span className="font-mono text-[var(--color-ink)]">
            sauron task mark &quot;what you&apos;re doing&quot;
          </span>{" "}
          to set it explicitly.
        </p>
      </section>
    );
  }

  // Prefer HINT data when a labelled, high-weight hint exists.
  const title = primaryHint
    ? primaryHint.label
    : readableTaskTitle(reentry?.task?.goal ?? "", reentry?.project?.name);

  const startedAt = primaryHint
    ? primaryHint.started_at
    : (reentry?.task?.started_at || reentry?.task?.updated_at || 0);
  const generatedAt = reentry?.generated_at ?? Math.floor(Date.now() / 1000);
  const activeFor = startedAt && generatedAt > startedAt
    ? fmtDur(startedAt, generatedAt)
    : "just started";

  const decision = primaryHint
    ? `Return to ${primaryHint.dominant_app}.`
    : readableNextAction(reentry?.next_action || reentry?.task?.next_action || "");

  // Evidence: prefer hint evidence rows, fall back to reentry events.
  let evidenceLine = "";
  if (primaryHint && primaryHint.evidence.length > 0) {
    const appCounts: Record<string, number> = {};
    for (const e of primaryHint.evidence) {
      if (e.app_name) appCounts[e.app_name] = (appCounts[e.app_name] || 0) + 1;
    }
    const parts = Object.entries(appCounts)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 3)
      .map(([app]) => app);
    evidenceLine = parts.join(" · ");
  } else if (reentry) {
    evidenceLine = formatEvidenceSummary(reentry, context, edits);
  }

  const rawLastUseful = reentry?.task?.last_useful_state || "";
  const isNoisyState = !rawLastUseful
    || /^Last useful context:\s*([\w\s]+)$/.test(rawLastUseful)
    || rawLastUseful.length < 30;
  const lastUseful = isNoisyState ? null : cleanContextText(rawLastUseful);

  const hintConfidence = primaryHint ? Math.round(primaryHint.confidence * 100) : null;

  return (
    <section className="border border-[var(--color-rule)] bg-[var(--color-paper-soft)] px-5 py-5 md:px-6 md:py-6">
      <div>
        <div className="eyebrow">current thread</div>
        <div className="mt-3 text-[30px] font-bold leading-[1.08] tracking-tight text-[var(--color-ink)]">
          {title}
        </div>
        <div className="mt-1 flex items-center gap-3 text-[12px] tnum text-[var(--color-ink-soft)]">
          <span>active for {activeFor}</span>
          {hintConfidence !== null && (
            <span className="text-[var(--color-ink-faint)]">{hintConfidence}% confidence</span>
          )}
        </div>
      </div>

      {evidenceLine && (
        <div className="mt-5 border-y border-[var(--color-rule)] py-3">
          <p className="text-[13px] text-[var(--color-ink-soft)]">{evidenceLine}</p>
        </div>
      )}

      <div className="mt-5">
        <p className="text-[18px] font-bold leading-[1.38] text-[var(--color-ink)]">
          {decision}
        </p>
        {lastUseful && (
          <p className="mt-3 text-[14px] font-medium leading-[1.55] text-[var(--color-ink-soft)]">
            {lastUseful}
          </p>
        )}
      </div>

      {secondaryHint && (
        <div className="mt-5 border-t border-[var(--color-rule)] pt-4">
          <div className="text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-faint)]">also active</div>
          <div className="mt-1.5 text-[14px] text-[var(--color-ink-soft)]">
            {secondaryHint.label}
            <span className="ml-2 text-[12px] text-[var(--color-ink-faint)]">
              weight {Math.round(secondaryHint.weight * 100)}%
            </span>
          </div>
        </div>
      )}
    </section>
  );
}

function NowPanel({
  context,
  rateBuckets,
  direction,
  eventsLast30,
  flow,
}: {
  context: Snapshot["context"];
  rateBuckets: number[];
  direction: { icon: string; label: string };
  eventsLast30: number;
  flow: string[];
}) {
  return (
    <section className="border border-[var(--color-rule)] px-5 py-5">
      <div className="eyebrow">activity now</div>
      {context ? (
        <>
          <div className="mt-4 flex items-start justify-between gap-4">
            <div>
              <div className="text-[24px] font-bold leading-none tracking-tight">
                {context.session_type.replace(/_/g, " ")}
              </div>
              <div className="mt-2 text-[14px] font-medium text-[var(--color-ink-soft)]">
                {context.dominant_app || "-"}
              </div>
            </div>
            <div className="text-right tnum">
              <div className="text-[24px] font-bold leading-none tracking-tight">
                {fmtAge(context.session_age_min)}
              </div>
            </div>
          </div>

          <div className="mt-6 text-[var(--color-accent)]">
            <Sparkline values={rateBuckets} width={260} height={34} />
          </div>
          <div className="mt-2 text-[13px] font-medium text-[var(--color-ink-soft)] tnum">
            <span className="text-[var(--color-ink)]">{direction.icon}</span>{" "}
            {direction.label} · {eventsLast30} events in 30m
          </div>

          {flow.length > 0 && (
            <div className="mt-5 border-t border-[var(--color-rule)] pt-4">
              <div className="text-[11px] uppercase tracking-[0.14em] text-[var(--color-ink-soft)]">
                app flow
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-x-1.5 gap-y-1 text-[13px]">
                {flow.map((a, i) => (
                  <span key={a} className="flex items-center gap-1.5">
                    {i > 0 && <span className="text-[var(--color-ink-faint)]">←</span>}
                    <span
                      className={
                        i === 0
                          ? "text-[var(--color-ink)] font-medium"
                          : "text-[var(--color-ink-soft)]"
                      }
                    >
                      {a}
                    </span>
                  </span>
                ))}
              </div>
            </div>
          )}
        </>
      ) : (
        <div className="mt-4 border-t border-[var(--color-rule)] pt-4 text-[13px] leading-[1.55] text-[var(--color-ink-soft)]">
          waiting for current context.
        </div>
      )}
    </section>
  );
}

function readableTaskTitle(goal: string, projectName?: string): string {
  let s = (goal || projectName || "Open task").trim();
  s = s.replace(/^Continue\s+/i, "").trim();
  s = s.replace(/^(Codex|Claude Code|iTerm2|Terminal|Brave Browser)\s*\/\s*/i, "").trim();
  if (s === "Codex") return "Current coding conversation";
  if (s === "iTerm2" || s === "Terminal") return "Current terminal task";
  return s || "Open task";
}

function cleanContextText(text: string): string {
  return text
    .replace(/^Last useful context:\s*/i, "")
    .replace(/\s*;\s*recent clipboard from\s*/i, ". Clipboard context from ")
    .trim();
}

function readableNextAction(action: string): string {
  const s = (action || "").trim();
  if (!s) return "Choose the next concrete step.";
  // "Return to X." → just show the action as-is, it's clean now
  s.replace(/^Continue:\s*/i, "Continue ");
  return s;
}

function Sparkline({
  values,
  width = 150,
  height = 24,
}: {
  values: number[];
  width?: number;
  height?: number;
}) {
  const max = Math.max(1, ...values);
  const barWidth = width / Math.max(1, values.length);
  return (
    <svg width={width} height={height} className="block" aria-hidden>
      {values.map((v, i) => {
        const h = v === 0 ? 1 : Math.max(1.5, (v / max) * height);
        return (
          <rect
            key={i}
            x={i * barWidth}
            y={height - h}
            width={Math.max(1, barWidth - 1)}
            height={h}
            fill="currentColor"
            opacity={v === 0 ? 0.18 : 0.75}
            rx={0.5}
          />
        );
      })}
    </svg>
  );
}

function Skeleton({ rows }: { rows: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: rows }).map((_, i) => (
        <div
          key={i}
          className="h-3 w-full bg-[var(--color-rule)] opacity-60"
          style={{ width: `${100 - i * 10}%` }}
        />
      ))}
    </div>
  );
}

type EventProps = {
  item: LogItem;
  isOpen: boolean;
  onToggle: () => void;
  onPermalink: () => void;
  copied: boolean;
  forceExpandable?: boolean;
};

function FeaturedEvent({ item, isOpen, onToggle, onPermalink, copied }: EventProps) {
  const hasBody = !!item.full && item.full.length > 0;
  const sevClass =
    item.severity === "error"
      ? "text-[var(--color-signal)]"
      : item.severity === "warn"
        ? "text-[var(--color-accent)]"
        : "text-[var(--color-ink)]";

  const categoryLabel =
    item.category === "clipboard" && item.app
      ? `copied from ${item.app}`
      : item.category === "edit"
        ? "edit"
        : item.category === "trace"
          ? item.kind.replace(/_/g, " ")
          : item.category;

  return (
    <div id={item.id} className="group">
      <div className="flex items-baseline gap-2 text-[12px] text-[var(--color-ink-soft)] tnum">
        <span aria-hidden className="text-[13px]">{item.icon}</span>
        <span className="flex-1 truncate">{categoryLabel}</span>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onPermalink();
          }}
          className="font-mono text-[12px] text-[var(--color-ink-soft)] hover:text-[var(--color-accent)]"
          title="copy permalink"
        >
          {copied ? "copied" : fmtClockFull(item.timestamp)}
        </button>
      </div>

      {item.category === "clipboard" && item.full ? (
        <div className="mt-2">
          <blockquote
            className="cursor-pointer border-l-2 border-[var(--color-rule)] pl-3.5 font-mono text-[13px] leading-[1.6] text-[var(--color-ink)]"
            onClick={hasBody ? onToggle : undefined}
          >
            {isOpen ? (
              <pre className="whitespace-pre-wrap break-words">{item.full}</pre>
            ) : (
              <span className="line-clamp-3 block">{item.full}</span>
            )}
          </blockquote>
          {hasBody && item.full && item.full.length > 200 && (
            <button
              onClick={onToggle}
              className="mt-1.5 text-[12px] text-[var(--color-ink-soft)] hover:text-[var(--color-accent)]"
            >
              {isOpen ? "collapse" : `show all · ${item.full.length} chars`}
            </button>
          )}
        </div>
      ) : item.category === "edit" ? (
        <div className="mt-2">
          <div className={`text-[14px] leading-[1.55] ${sevClass}`}>{item.text}</div>
          {item.full && (
            <div
              className="mt-2 cursor-pointer border-l-2 border-[var(--color-rule)] pl-3.5 font-mono text-[12.5px] leading-[1.6] text-[var(--color-ink-soft)]"
              onClick={hasBody ? onToggle : undefined}
            >
              {isOpen ? (
                <pre className="whitespace-pre-wrap break-words">{item.full}</pre>
              ) : (
                <span className="line-clamp-2 block">{item.full}</span>
              )}
            </div>
          )}
        </div>
      ) : item.category === "trace" ? (
        <div className="mt-2 flex flex-wrap items-baseline gap-2">
          <span
            className={`rounded px-2 py-0.5 text-[11px] font-medium uppercase tracking-[0.08em] ${
              item.severity === "error"
                ? "bg-[var(--color-signal)]/15 text-[var(--color-signal)]"
                : item.severity === "warn"
                  ? "bg-[var(--color-accent-soft)] text-[var(--color-accent)]"
                  : "bg-[var(--color-accent-soft)]/70 text-[var(--color-accent)]"
            }`}
          >
            {item.kind.replace(/_/g, " ")}
          </span>
          <span className={`text-[14px] leading-[1.55] ${sevClass}`}>
            {item.text.replace(new RegExp(`^${item.kind.replace(/_/g, " ")} · `), "")}
          </span>
        </div>
      ) : (
        <div className={`mt-2 text-[14px] leading-[1.55] ${sevClass}`}>{item.text}</div>
      )}
    </div>
  );
}

function LogLine({ item, isOpen, onToggle, onPermalink, copied, forceExpandable }: EventProps) {
  const hasMore = forceExpandable || (!!item.full && item.full.length > 0);
  const sevClass =
    item.severity === "error"
      ? "text-[var(--color-signal)]"
      : item.severity === "warn"
        ? "text-[var(--color-accent)]"
        : "text-[var(--color-ink)]";
  return (
    <div id={item.id} className="border-b border-[var(--color-rule)] last:border-0">
      <div
        className={`grid grid-cols-[78px_20px_1fr_20px] items-baseline gap-3 py-2 ${
          hasMore ? "cursor-pointer" : ""
        }`}
        onClick={onToggle}
        title={item.text}
      >
        <button
          onClick={(e) => {
            e.stopPropagation();
            onPermalink();
          }}
          className="text-left font-mono text-[12px] tnum text-[var(--color-ink-soft)] hover:text-[var(--color-accent)]"
        >
          {copied ? "copied" : fmtClockFull(item.timestamp)}
        </button>
        <span className="text-[13px] leading-none" aria-hidden>
          {item.icon}
        </span>
        <span className={`line-clamp-2 text-[13.5px] leading-[1.5] ${sevClass}`}>
          {item.text}
        </span>
        <span className="text-right text-[12px] text-[var(--color-ink-faint)]">
          {hasMore ? (isOpen ? "−" : "+") : ""}
        </span>
      </div>
      {isOpen && !forceExpandable && item.full && (
        <pre className="mb-2 ml-[98px] whitespace-pre-wrap break-words border-l border-[var(--color-rule)] pl-3 font-mono text-[12.5px] leading-[1.6] text-[var(--color-ink)]">
          {item.full}
        </pre>
      )}
    </div>
  );
}
