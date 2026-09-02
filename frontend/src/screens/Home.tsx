import { useEffect, useMemo, useRef, useState } from "react";

import { Icon } from "../components/Icon";
import type { Category, FileInfo, Task } from "../lib/api";
import { api } from "../lib/api";

/*
 * Home.
 *
 * Dragging a file anywhere on this screen filters the grid to the tasks that
 * accept it. Drop a HEIC and see Convert, Compress, Resize, Extract text,
 * Images to PDF — which removes the need to understand the categories at all,
 * and is the best interaction in the product.
 */

interface HomeProps {
  tasks: Task[];
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

export function Home({ tasks, onPick, onSettings }: HomeProps) {
  const [query, setQuery] = useState("");
  const [dragging, setDragging] = useState<Category | null>(null);
  const [pending, setPending] = useState<FileInfo[]>([]);
  const searchRef = useRef<HTMLInputElement>(null);
  const depth = useRef(0);

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

  // Files dropped on the window, from the OS or from a second launch.
  useEffect(() => {
    const runtime = window.runtime;
    if (!runtime) return;

    const off = runtime.EventsOn("wails:file-drop", (...data: unknown[]) => {
      const paths = data.find(Array.isArray) as string[] | undefined;
      if (!paths?.length) return;

      depth.current = 0;
      void api.inspect(paths).then((files) => {
        setPending(files);
        setDragging(files[0]?.category ?? null);
      });
    });
    return off;
  }, []);

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase();
    return tasks.filter((t) => {
      if (dragging && !t.accepts.includes(dragging)) return false;
      if (!q) return true;
      return (
        t.name.toLowerCase().includes(q) ||
        t.description.toLowerCase().includes(q) ||
        t.id.includes(q)
      );
    });
  }, [tasks, query, dragging]);

  const grouped = useMemo(() => {
    const out = new Map<string, Task[]>();
    for (const t of visible) {
      const list = out.get(t.category) ?? [];
      list.push(t);
      out.set(t.category, list);
    }
    return out;
  }, [visible]);

  const clearFilter = () => {
    setDragging(null);
    setPending([]);
  };

  return (
    <div
      className="scroll fade-in"
      onDragOver={(e) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = "copy";
      }}
      onDragEnter={(e) => {
        e.preventDefault();
        depth.current += 1;
      }}
      onDragLeave={() => {
        depth.current -= 1;
      }}
    >
      <div className="screen">
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

        {dragging && (
          <div className="row panel" style={{ padding: "12px 16px" }}>
            <Icon name="file" size={16} />
            <span className="t-data">
              {pending.length === 1
                ? pending[0].name
                : `${pending.length} files`}
            </span>
            {pending[0]?.mismatch && (
              <span className="file-warn">
                This is actually a {pending[0].mismatch.toUpperCase()} file.
              </span>
            )}
            <span className="spacer" />
            <span className="t-data" style={{ color: "var(--fg-mid)" }}>
              Showing what you can do with it
            </span>
            <button type="button" className="btn btn-ghost" onClick={clearFilter}>
              Clear
            </button>
          </div>
        )}

        {visible.length === 0 && (
          <p className="empty">Nothing here matches that. Try a different word.</p>
        )}

        {[...grouped.entries()].map(([category, list]) => (
          <section key={category} className="col" style={{ gap: "var(--gap-3)" }}>
            <h2 className="section-label">{groupTitles[category] ?? category}</h2>
            <div className="task-grid">
              {list.map((t) => (
                <TaskCard key={t.id} task={t} onPick={() => onPick(t, pending)} />
              ))}
            </div>
          </section>
        ))}
      </div>
    </div>
  );
}

function TaskCard({ task, onPick }: { task: Task; onPick: () => void }) {
  return (
    <button type="button" className="task-card" onClick={onPick}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <span className="task-icon">
          <Icon name={task.icon} size={22} />
        </span>
        {/* Honest before the click, not after. */}
        {!task.available && task.downloadMB > 0 && (
          <span className="badge badge-warn">+{task.downloadMB} MB</span>
        )}
        {!task.available && task.downloadMB === 0 && (
          <span className="badge badge-warn">Needs setup</span>
        )}
      </div>
      <span className="task-name">{task.name}</span>
      <span className="task-desc">{task.description}</span>
    </button>
  );
}
