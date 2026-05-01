import { NextResponse } from "next/server";
import {
  getActivity,
  getClipboard,
  getContext,
  getExperienceStats,
  getHints,
  getReentryContext,
  getStatus,
  getTimeline,
  getTraces,
} from "@/lib/sauron";
import { readRecentEdits } from "@/lib/vault-watcher";

export const dynamic = "force-dynamic";
export const revalidate = 0;

export async function GET() {
  const [status, context, activity, timeline, clipboard, traces, experience, reentry, hints] =
    await Promise.allSettled([
      getStatus(),
      getContext(),
      getActivity(24),
      getTimeline(2),
      getClipboard(20),
      getTraces(8),
      getExperienceStats(),
      getReentryContext(),
      getHints(3),
    ]);

  const unwrap = <T,>(r: PromiseSettledResult<T>, fallback: T): T =>
    r.status === "fulfilled" ? r.value : fallback;

  const edits = readRecentEdits(20);

  return NextResponse.json({
    ts: Math.floor(Date.now() / 1000),
    status: unwrap(status, {
      running: false,
      pid: null,
      clipboard_captures: 0,
      activity_entries: 0,
      sessions: 0,
    }),
    context: unwrap(context, null),
    activity: unwrap(activity, null),
    timeline: unwrap(timeline, []),
    clipboard: unwrap(clipboard, []),
    traces: unwrap(traces, []),
    experience: unwrap(experience, { total: 0, success: 0, failure: 0, partial: 0 }),
    reentry: unwrap(reentry, null),
    hints: unwrap(hints, []),
    edits,
  });
}
