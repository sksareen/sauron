"use client";

import { useEffect, useRef, useState } from "react";

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
  experience: { total: number; success: number; failure: number; partial: number };
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

export default function Page() {
  const [snap, setSnap] = useState<Snapshot | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const timer = useRef<ReturnType<typeof setInterval> | null>(null);

  const toggle = (id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
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
        }
      } catch (e) {
        if (alive) setErr(e instanceof Error ? e.message : "offline");
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
  const context = snap?.context;
  const activity = snap?.activity;
  const timeline = snap?.timeline ?? [];
  const clipboard = snap?.clipboard ?? [];
  const traces = snap?.traces ?? [];
  const experience = snap?.experience;
  const edits = snap?.edits ?? [];

  const apps = activity
    ? Object.entries(activity.app_breakdown).sort(([, a], [, b]) => b - a)
    : [];
  const topHours = apps[0]?.[1] ?? 1;
  const totalHours = apps.reduce((s, [, h]) => s + h, 0);

  const stream = mergeStream(timeline, clipboard).slice(0, 40);

  return (
    <main className="mx-auto max-w-3xl px-8 py-16">
      {/* Header */}
      <header className="flex items-baseline justify-between">
        <h1 className="text-[15px] font-medium tracking-tight text-[var(--color-ink)]">
          sauron
        </h1>
        <div className="flex items-center gap-6 text-[11px] text-[var(--color-ink-faint)]">
          {err ? (
            <span className="text-[var(--color-signal)]">offline</span>
          ) : status?.running ? (
            <span className="pulse text-[var(--color-ink-soft)]">
              pid {status.pid}
            </span>
          ) : (
            <span className="text-[var(--color-signal)]">daemon stopped</span>
          )}
        </div>
      </header>

      {/* NOW */}
      <section className="mt-14">
        <div className="eyebrow">now</div>
        <div className="rule mt-3 pt-6">
          {context ? (
            <div className="grid grid-cols-[1fr_auto] gap-8">
              <div>
                <div className="text-[32px] font-light leading-none tracking-tight">
                  {context.session_type.replace(/_/g, " ")}
                </div>
                <div className="mt-2 text-[13px] text-[var(--color-ink-soft)]">
                  {context.dominant_app || "—"}
                </div>
              </div>
              <div className="text-right tnum">
                <div className="text-[32px] font-light leading-none tracking-tight">
                  {Math.round(context.focus_score * 100)}
                </div>
                <div className="mt-2 text-[11px] uppercase tracking-[0.16em] text-[var(--color-ink-faint)]">
                  focus · {fmtAge(context.session_age_min)}
                </div>
              </div>
            </div>
          ) : (
            <Skeleton rows={2} />
          )}
        </div>
      </section>

      {/* TODAY */}
      <section className="mt-14">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">last 24h</div>
          {activity && (
            <div className="text-[11px] tnum text-[var(--color-ink-faint)]">
              {activity.switches} switches · {activity.total_apps} apps
            </div>
          )}
        </div>
        <div className="rule mt-3 pt-6">
          {activity ? (
            <>
              <div className="mb-6 flex items-baseline gap-4">
                <div className="text-[28px] font-light tnum leading-none">
                  {fmtHrs(totalHours)}
                </div>
                <div className="text-[11px] uppercase tracking-[0.16em] text-[var(--color-ink-faint)]">
                  tracked
                </div>
              </div>
              <div className="space-y-2.5">
                {apps.map(([name, h]) => (
                  <div
                    key={name}
                    className="grid grid-cols-[140px_1fr_60px] items-center gap-4"
                  >
                    <div className="truncate text-[13px] text-[var(--color-ink)]">
                      {name}
                    </div>
                    <div className="h-[2px] bg-[var(--color-rule)]">
                      <div
                        className="h-full bg-[var(--color-accent)]"
                        style={{ width: `${(h / topHours) * 100}%` }}
                      />
                    </div>
                    <div className="text-right text-[12px] tnum text-[var(--color-ink-soft)]">
                      {fmtHrs(h)}
                    </div>
                  </div>
                ))}
              </div>
            </>
          ) : (
            <Skeleton rows={4} />
          )}
        </div>
      </section>

      {/* EDITS */}
      <section className="mt-14">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">edits</div>
          {edits.length > 0 && (
            <div className="text-[11px] tnum text-[var(--color-ink-faint)]">
              {edits.length} recent · vault ~/Savar
            </div>
          )}
        </div>
        <div className="rule mt-3 pt-4">
          {edits.length === 0 ? (
            <div className="py-6 text-[12px] text-[var(--color-ink-faint)]">
              no edits recorded yet — save a note in Obsidian to start the stream
            </div>
          ) : (
            <ul className="divide-y divide-[var(--color-rule)]">
              {edits.map((e, i) => (
                <li key={`${e.ts}-${i}`} className="py-3">
                  <div className="grid grid-cols-[56px_1fr_88px] items-baseline gap-4">
                    <span className="text-[11px] tnum text-[var(--color-ink-faint)]">
                      {fmtRel(e.ts, now)}
                    </span>
                    <span className="truncate text-[13px] text-[var(--color-ink)]">
                      {e.note}
                    </span>
                    <span className="text-right text-[11px] tnum">
                      <span className="text-[var(--color-accent)]">+{e.added}</span>
                      <span className="mx-1 text-[var(--color-ink-faint)]">·</span>
                      <span className="text-[var(--color-signal)]">−{e.removed}</span>
                    </span>
                  </div>
                  {e.preview && (
                    <div className="mt-1.5 ml-[60px] text-[12px] text-[var(--color-ink-soft)] truncate">
                      {e.preview}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      {/* STREAM */}
      <section className="mt-14">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">stream</div>
          <div className="pulse text-[11px] text-[var(--color-ink-faint)]">live</div>
        </div>
        <div className="rule mt-3 pt-4">
          {stream.length === 0 ? (
            <Skeleton rows={4} />
          ) : (
            <ul className="divide-y divide-[var(--color-rule)]">
              {stream.map((it) => {
                const hasMore = it.full && it.full.length > it.text.length;
                const isOpen = expanded.has(it.id);
                return (
                  <li key={it.id} className="py-2.5">
                    <div
                      className={`grid grid-cols-[56px_80px_1fr] items-baseline gap-4 ${
                        hasMore ? "cursor-pointer" : ""
                      }`}
                      onClick={() => hasMore && toggle(it.id)}
                      title={hasMore ? it.full : undefined}
                    >
                      <span className="text-[11px] tnum text-[var(--color-ink-faint)]">
                        {fmtRel(it.timestamp, now)}
                      </span>
                      <span className="text-[10px] uppercase tracking-[0.16em] text-[var(--color-ink-faint)]">
                        {it.kind}
                      </span>
                      <span
                        className={`truncate text-[13px] text-[var(--color-ink)] ${
                          hasMore ? "hover:text-[var(--color-accent)]" : ""
                        }`}
                      >
                        {it.text}
                      </span>
                    </div>
                    {isOpen && it.full && (
                      <div className="mt-2 ml-[136px] border-l border-[var(--color-rule)] pl-3">
                        <div className="mb-1 flex items-center justify-between text-[10px] uppercase tracking-[0.16em] text-[var(--color-ink-faint)]">
                          <span>{it.app ?? "clipboard"} · {it.full.length} chars</span>
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              navigator.clipboard?.writeText(it.full ?? "");
                            }}
                            className="hover:text-[var(--color-accent)]"
                          >
                            copy
                          </button>
                        </div>
                        <pre className="whitespace-pre-wrap break-words font-mono text-[12px] leading-[1.55] text-[var(--color-ink)]">
                          {it.full}
                        </pre>
                      </div>
                    )}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </section>

      {/* TRACES */}
      <section className="mt-14">
        <div className="flex items-baseline justify-between">
          <div className="eyebrow">traces</div>
          {experience && experience.total > 0 && (
            <div className="text-[11px] tnum text-[var(--color-ink-faint)]">
              {experience.total} experiences ·{" "}
              {Math.round((experience.success / experience.total) * 100)}% success
            </div>
          )}
        </div>
        <div className="rule mt-3 pt-4">
          {traces.length === 0 ? (
            <Skeleton rows={3} />
          ) : (
            <ul className="divide-y divide-[var(--color-rule)]">
              {traces.map((t) => (
                <li
                  key={t.id}
                  className="grid grid-cols-[80px_1fr_56px] items-baseline gap-4 py-3"
                >
                  <span className="text-[10px] uppercase tracking-[0.16em] text-[var(--color-ink-faint)]">
                    {t.outcome_type.replace(/_/g, " ")}
                  </span>
                  <span className="truncate text-[13px] text-[var(--color-ink)]">
                    {t.outcome_detail}
                  </span>
                  <span className="text-right text-[11px] tnum text-[var(--color-ink-faint)]">
                    {fmtRel(t.completed_at, now)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      {/* Footer */}
      <footer className="mt-20 flex items-center justify-between rule pt-6 text-[11px] text-[var(--color-ink-faint)] tnum">
        <span>
          {status?.clipboard_captures ?? 0} clips ·{" "}
          {status?.activity_entries ?? 0} activity ·{" "}
          {status?.sessions ?? 0} sessions
        </span>
        <span>{snap ? fmtClock(snap.ts) : "—"}</span>
      </footer>
    </main>
  );
}

type StreamItem = {
  id: string;
  timestamp: number;
  kind: string;
  text: string;
  full?: string;
  app?: string;
};

function mergeStream(
  timeline: { timestamp: number; type: string; summary: string }[],
  clipboard: { id: number; content: string; source_app: string; captured_at: number }[],
): StreamItem[] {
  const items: StreamItem[] = [];
  for (const t of timeline) {
    items.push({
      id: `tl-${t.timestamp}-${t.summary.slice(0, 16)}`,
      timestamp: t.timestamp,
      kind: t.type,
      text: t.summary,
    });
  }
  for (const c of clipboard) {
    const full = c.content;
    const flat = full.replace(/\s+/g, " ").trim();
    items.push({
      id: `cb-${c.id}`,
      timestamp: c.captured_at,
      kind: "clipboard",
      text: truncate(flat, 120),
      full,
      app: c.source_app,
    });
  }
  return items
    .filter((i) => i.text && i.text.length > 0)
    .sort((a, b) => b.timestamp - a.timestamp);
}

function truncate(s: string, n: number) {
  return s.length > n ? s.slice(0, n - 1) + "…" : s;
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
