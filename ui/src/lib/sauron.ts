import { exec } from "node:child_process";
import { promisify } from "node:util";

const execAsync = promisify(exec);

const BIN = process.env.SAURON_BIN || "sauron";
const TIMEOUT_MS = 4000;

async function run(args: string): Promise<string> {
  const { stdout } = await execAsync(`${BIN} ${args}`, {
    timeout: TIMEOUT_MS,
    maxBuffer: 8 * 1024 * 1024,
    env: { ...process.env, PATH: `${process.env.PATH}:/usr/local/bin:/opt/homebrew/bin` },
  });
  return stdout;
}

async function runJSON<T>(args: string): Promise<T> {
  const out = await run(`${args} --json`);
  return JSON.parse(out) as T;
}

export type Context = {
  session_type: string;
  focus_score: number;
  session_age_min: number;
  dominant_app: string;
  recent_clipboard: string[];
  local_servers?: { port: string; process: string; pid: string }[];
};

export type Activity = {
  hours: number;
  focus_score: number;
  app_breakdown: Record<string, number>;
  total_apps: number;
  switches: number;
};

export type TimelineItem = {
  timestamp: number;
  type: "session" | "activity" | "clipboard" | string;
  summary: string;
};

export type Clipboard = {
  id: number;
  content: string;
  content_type: string;
  source_app: string;
  bundle_id: string;
  window_title: string;
  captured_at: number;
};

export type Trace = {
  id: number;
  outcome_type: string;
  outcome_detail: string;
  trace_summary: string;
  activity_window_minutes: number;
  started_at: number;
  completed_at: number;
};

export type Status = {
  running: boolean;
  pid: number | null;
  clipboard_captures: number;
  activity_entries: number;
  sessions: number;
};

export async function getStatus(): Promise<Status> {
  try {
    const out = await run("status");
    const pidMatch = out.match(/running \(pid (\d+)\)/);
    const clip = out.match(/clipboard captures:\s+(\d+)/);
    const act = out.match(/activity entries:\s+(\d+)/);
    const ses = out.match(/sessions:\s+(\d+)/);
    return {
      running: Boolean(pidMatch),
      pid: pidMatch ? Number(pidMatch[1]) : null,
      clipboard_captures: clip ? Number(clip[1]) : 0,
      activity_entries: act ? Number(act[1]) : 0,
      sessions: ses ? Number(ses[1]) : 0,
    };
  } catch {
    return {
      running: false,
      pid: null,
      clipboard_captures: 0,
      activity_entries: 0,
      sessions: 0,
    };
  }
}

export function getContext() {
  return runJSON<Context>("context");
}

export function getActivity(hours = 24) {
  return runJSON<Activity>(`activity ${hours}`);
}

export function getTimeline(hours = 2) {
  return runJSON<TimelineItem[]>(`timeline --hours ${hours}`);
}

export function getClipboard(n = 15) {
  return runJSON<Clipboard[]>(`clipboard ${n}`);
}

export function getTraces(limit = 10) {
  return runJSON<Trace[]>(`traces --limit ${limit}`);
}

export type ExperienceStats = {
  total: number;
  success: number;
  failure: number;
  partial: number;
};

export async function getExperienceStats(): Promise<ExperienceStats> {
  try {
    const out = await run("experience stats");
    const total = Number(out.match(/(\d+)\s+total/)?.[1] ?? 0);
    const success = Number(out.match(/success:\s+(\d+)/)?.[1] ?? 0);
    const failure = Number(out.match(/failure:\s+(\d+)/)?.[1] ?? 0);
    const partial = Number(out.match(/partial:\s+(\d+)/)?.[1] ?? 0);
    return { total, success, failure, partial };
  } catch {
    return { total: 0, success: 0, failure: 0, partial: 0 };
  }
}
