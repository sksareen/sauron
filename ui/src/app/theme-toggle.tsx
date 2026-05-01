"use client";

import { useEffect, useState } from "react";

type Theme = "light" | "dark";

function applyTheme(t: Theme) {
  document.documentElement.setAttribute("data-theme", t);
  try {
    localStorage.setItem("theme", t);
  } catch {
    /* ignore */
  }
}

export function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>("light");

  useEffect(() => {
    const current = (document.documentElement.getAttribute("data-theme") as Theme) || "light";
    setTheme(current);
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!(e.metaKey || e.ctrlKey) || e.key.toLowerCase() !== "i") return;
      const el = e.target as HTMLElement | null;
      const tag = el?.tagName;
      if (tag === "INPUT" || tag === "TEXTAREA" || el?.isContentEditable) return;
      e.preventDefault();
      setTheme((prev) => {
        const next: Theme = prev === "dark" ? "light" : "dark";
        applyTheme(next);
        return next;
      });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const flip = () => {
    setTheme((prev) => {
      const next: Theme = prev === "dark" ? "light" : "dark";
      applyTheme(next);
      return next;
    });
  };

  return (
    <button
      onClick={flip}
      title={`${theme === "dark" ? "dark" : "light"} · ⌘I`}
      aria-label="Toggle theme"
      className="text-[12px] text-[var(--color-ink-faint)] hover:text-[var(--color-accent)] transition-colors"
    >
      {theme === "dark" ? "☾" : "☀"}
    </button>
  );
}
