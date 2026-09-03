import { useEffect, useMemo, useRef, useState } from "react";

import { Icon } from "../components/Icon";
import { PixelIcon } from "../components/PixelIcon";
import type { FileInfo, Task } from "../lib/api";
import { humanBytes } from "../lib/api";

/*
 * Home.
 *
 * Dragging a file anywhere on this screen filters the grid to the tasks that
 * accept it. Drop a HEIC and see Convert, Compress, Resize, Extract text,
 * Images to PDF, which removes the need to understand the categories at all
 * and is the best interaction in the product.
 *
 * The dropped files live in App rather than here, because the same filtered
 * state is reached three ways: a drag, the file dialog, and launching the app
 * with a file from the context menu.
 */

interface HomeProps {
  tasks: Task[];
  /** Files the user has offered, from a drag or from the command line. */
  incoming: FileInfo[];
  onClearIncoming: () => void;
  onPick: (task: Task, files: FileInfo[]) => void;
  onSettings: () => void;
}

const groupTitles: Record<string, string> = {
  pdf: "PDF",
  image: "Images",
  text: "Text and reading",
  document: "Documents",
  media: "Video and audio",
};

export function Home({ tasks, incoming, onClearIncoming, onPick, onSettings }: HomeProps) {
  const [query, setQuery] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);

  // Cmd+K on macOS, Ctrl+K elsewhere, resolved once rather than hardcoded.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const modifier = navigator.userAgent.includes("Mac") ? e.metaKey : e.ctrlKey;
      if (modifier && e.key.toLowerCase() === "k") {
        e.preventDefault();
        searchRef.current?.focus();
        searchRef.current?.select();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const filterCategory = incoming[0]?.category ?? null;

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return tasks.filter((t) => {
      if (filterCategory && !t.accepts.includes(filterCategory)) return false;
      if (!q) return true;
      return (
        t.name.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q) ||
        t.id.includes(q)
      );
    });
  }, [tasks, query, filterCategory]);

  const grouped = useMemo(() => {
    const out = new Map<string, Task[]>();
    for (const t of visible) {
      const list = out.get(t.category) ?? [];
      list.push(t);
      out.set(t.category, list);
    }
    return out;
  }, [visible]);

  return (
    <div className="scroll fade-in">
      <div className="screen">
        <header className="masthead">
          <div className="section-head">
            <span className="t-micro" style={{ color: "var(--fg-low)" }}>
              {tasks.length} tasks
            </span>
            <span className="section-rule" />
            <span className="t-micro" style={{ color: "var(--fg-low)" }}>
              Offline
            </span>
          </div>
          <h1>
            Convert, compress and read files.{" "}
            <span>Nothing leaves this machine.</span>
          </h1>
        </header>

        <div className="row">
          <div className="search" style={{ flex: 1 }}>
            <Icon name="search" size={16} />
            <input
              ref={searchRef}
              type="text"
              value={query}
              placeholder="What do you want to do?"
              onChange={(e) => setQuery(e.target.value)}
              aria-label="Search tasks"
            />
            <span className="kbd">
              {navigator.userAgent.includes("Mac") ? "⌘K" : "Ctrl K"}
            </span>
          </div>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={onSettings}
            aria-label="Settings"
          >
            <Icon name="settings" size={16} />
          </button>
        </div>

        {incoming.length > 0 && (
          <div className="row panel" style={{ padding: "12px 16px" }}>
            <Icon name="file" size={16} />
            <span className="filename">
              {incoming.length === 1 ? incoming[0].name : `${incoming.length} files`}
            </span>
            <span className="filemeta">
              {humanBytes(incoming.reduce((total, f) => total + f.sizeBytes, 0))}
            </span>
            {incoming[0]?.mismatch && (
              <span className="file-warn">
                This is actually a {incoming[0].mismatch.toUpperCase()} file.
              </span>
            )}
            <span className="spacer" />
            <span className="t-data" style={{ color: "var(--fg-mid)" }}>
              Showing what you can do with it
            </span>
            <button type="button" className="btn btn-ghost" onClick={onClearIncoming}>
              Clear
            </button>
          </div>
        )}

        {visible.length === 0 && (
          <p className="empty">Nothing here matches that. Try a different word.</p>
        )}

        {[...grouped.entries()].map(([category, list]) => (
          <section key={category} className="col" style={{ gap: "var(--gap-4)" }}>
            <div className="section-head">
              <h2 className="section-label">{groupTitles[category] ?? category}</h2>
              <span className="section-rule" />
              <span className="section-count">
                {String(list.length).padStart(2, "0")}
              </span>
            </div>
            <div className="task-grid">
              {list.map((t, i) => (
                <TaskCard
                  key={t.id}
                  task={t}
                  index={i + 1}
                  onPick={() => onPick(t, incoming)}
                />
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

function TaskCard({
  task,
  index,
  onPick,
}: {
  task: Task;
  index: number;
  onPick: () => void;
}) {
  return (
    <button type="button" className="task-card" onClick={onPick}>
      <span className="task-plate">
        <span className="task-icon">
          <PixelIcon name={task.icon} size={40} />
        </span>
        {/* Honest before the click, not after. */}
        {!task.available && task.downloadMB > 0 ? (
          <span className="badge badge-warn">+{task.downloadMB} MB</span>
        ) : !task.available ? (
          <span className="badge badge-warn">Needs setup</span>
        ) : (
          <span className="task-index">{String(index).padStart(3, "0")}</span>
        )}
      </span>
      <span className="task-body">
        <span className="task-name">{task.name}</span>
        <span className="task-desc">{task.description}</span>
      </span>
    </button>
  );
}
