import { useEffect, useMemo, useRef, useState } from "react";

import { DropZone } from "../components/DropZone";
import { Icon } from "../components/Icon";
import { Tooltip } from "../components/Tooltip";
import type { FileInfo, Task, TaskOption } from "../lib/api";
import { api, humanBytes } from "../lib/api";

/*
 * The task screen: a drop zone, the file list, at most three options with
 * their defaults already chosen, and one primary button carrying a verb.
 *
 * Anything beyond three options lives behind "More options", because every
 * extra control on this screen makes the app harder for the people it is for.
 */

interface TaskScreenProps {
  task: Task;
  initialFiles: FileInfo[];
  onBack: () => void;
  onRun: (files: FileInfo[], options: Record<string, unknown>, outputDir: string) => void;
}

export function TaskScreen({ task, initialFiles, onBack, onRun }: TaskScreenProps) {
  const [files, setFiles] = useState<FileInfo[]>(initialFiles);
  const [options, setOptions] = useState<Record<string, unknown>>(() => defaultsOf(task));
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [outputDir, setOutputDir] = useState("");
  // Which row is being carried, and which one the pointer is currently over.
  const [dragging, setDragging] = useState<number | null>(null);
  const [dragOver, setDragOver] = useState<number | null>(null);
  // The state above draws the drag; these carry it, so the drop can read where
  // it started and ended without going through a state updater.
  const dragFrom = useRef<number | null>(null);
  const dragTo = useRef<number | null>(null);

  const beginDrag = (i: number) => {
    dragFrom.current = i;
    dragTo.current = i;
    setDragging(i);
  };

  // Reordering is driven by pointer events rather than the HTML drag API on
  // purpose. The window already listens for files dragged in from the desktop,
  // and a native drag starting inside it muddies that; pointer events keep the
  // two entirely separate, and they behave the same under touch.
  const moveFile = (from: number, to: number) => {
    setFiles((current) => {
      if (to < 0 || to >= current.length || from === to) return current;
      const next = current.slice();
      const [moved] = next.splice(from, 1);
      next.splice(to, 0, moved);
      return next;
    });
  };

  useEffect(() => {
    if (dragging === null) return;

    const over = (e: PointerEvent) => {
      const row = document
        .elementsFromPoint(e.clientX, e.clientY)
        .find((el) => el instanceof HTMLElement && el.dataset.row !== undefined);
      if (row instanceof HTMLElement) {
        const i = Number(row.dataset.row);
        dragTo.current = i;
        setDragOver(i);
      }
    };

    // The move is made here, from refs, rather than inside a state updater.
    // React calls updaters twice under StrictMode to catch impure ones, and a
    // reorder performed in there would run twice and shift the row two places.
    const drop = () => {
      const from = dragFrom.current;
      const to = dragTo.current;
      dragFrom.current = null;
      dragTo.current = null;
      setDragging(null);
      setDragOver(null);
      if (from !== null && to !== null) moveFile(from, to);
    };

    window.addEventListener("pointermove", over);
    window.addEventListener("pointerup", drop);
    window.addEventListener("pointercancel", drop);
    return () => {
      window.removeEventListener("pointermove", over);
      window.removeEventListener("pointerup", drop);
      window.removeEventListener("pointercancel", drop);
    };
    // Rebound only when a drag starts or ends: moveFile is recreated every
    // render but closes over nothing that changes within a drag.
  }, [dragging]);

  const primary = task.options.filter((o) => !o.advanced);
  const advanced = task.options.filter((o) => o.advanced);

  const accepted = useMemo(
    () => files.filter((f) => task.accepts.includes(f.category)),
    [files, task],
  );
  const rejected = useMemo(
    () => files.filter((f) => !task.accepts.includes(f.category)),
    [files, task],
  );

  const needsPassword = accepted.some((f) => f.encrypted) && !options.password;
  const tooFew = accepted.length < task.minInputs;
  const tooMany = task.maxInputs > 0 && accepted.length > task.maxInputs;
  const ready = !tooFew && !tooMany && !needsPassword;

  const addFiles = async (paths: string[]) => {
    const info = await api.inspect(paths);
    setFiles((current) => {
      const seen = new Set(current.map((f) => f.path));
      return [...current, ...info.filter((f) => !seen.has(f.path))];
    });
  };

  const browse = async () => {
    const paths = await api.chooseFiles(`Choose files for ${task.name}`);
    if (paths?.length) await addFiles(paths);
  };

  const pickFolder = async () => {
    const dir = await api.chooseFolder("Where should the results go?");
    if (dir) setOutputDir(dir);
  };

  return (
    <div className="scroll fade-in">
      <div className="screen">
        <div className="row">
          <button type="button" className="btn btn-ghost" onClick={onBack}>
            <Icon name="back" size={16} />
            Back
          </button>
          <h1 className="section-label">{task.name}</h1>
        </div>

        {files.length === 0 ? (
          <DropZone
            onFiles={(paths) => void addFiles(paths)}
            onBrowse={() => void browse()}
            hint={hintFor(task)}
          />
        ) : (
          <div className="panel">
            {files.map((f, i) => (
              <FileRow
                key={f.path}
                file={f}
                index={i}
                total={files.length}
                accepted={task.accepts.includes(f.category)}
                // Handles appear only where rearranging changes the result.
                // On a task that treats its inputs as a set, they would be a
                // control that does nothing.
                reorderable={task.orderMatters && files.length > 1}
                dragging={dragging === i}
                dropTarget={dragOver === i && dragging !== null && dragging !== i}
                onGrab={() => beginDrag(i)}
                onMove={(to) => moveFile(i, to)}
                onRemove={() => setFiles((c) => c.filter((x) => x.path !== f.path))}
              />
            ))}
            <div className="file-row">
              <button type="button" className="btn btn-ghost" onClick={() => void browse()}>
                <Icon name="plus" size={16} />
                Add more
              </button>
            </div>
          </div>
        )}

        {rejected.length > 0 && (
          <p className="note">
            {rejected.length === 1
              ? `${rejected[0].name} can't be used with ${task.name}, so it will be skipped.`
              : `${rejected.length} files can't be used with ${task.name}, so they will be skipped.`}
          </p>
        )}

        {primary.length > 0 && (
          <div className="col" style={{ gap: "var(--gap-5)" }}>
            {primary.map((o) => (
              <OptionControl
                key={o.id}
                option={o}
                value={options[o.id]}
                onChange={(v) => setOptions((c) => ({ ...c, [o.id]: v }))}
              />
            ))}
          </div>
        )}

        {advanced.length > 0 && (
          <>
            <button
              type="button"
              className="more-options"
              onClick={() => setShowAdvanced((v) => !v)}
            >
              {showAdvanced ? "Fewer options" : "More options"}
            </button>
            {showAdvanced && (
              <div className="col" style={{ gap: "var(--gap-5)" }}>
                {advanced.map((o) => (
                  <OptionControl
                    key={o.id}
                    option={o}
                    value={options[o.id]}
                    onChange={(v) => setOptions((c) => ({ ...c, [o.id]: v }))}
                  />
                ))}
              </div>
            )}
          </>
        )}

        {needsPassword && (
          <div className="option">
            <label className="option-label" htmlFor="pw">
              Password
            </label>
            <input
              id="pw"
              type="password"
              className="field"
              placeholder="This file is protected"
              value={String(options.password ?? "")}
              onChange={(e) => setOptions((c) => ({ ...c, password: e.target.value }))}
            />
          </div>
        )}

        <div className="divider" />

        <div className="row">
          <button type="button" className="btn btn-secondary" onClick={() => void pickFolder()}>
            <Icon name="folder" size={16} />
            {outputDir ? "Change folder" : "Save beside the original"}
          </button>
          {outputDir && <span className="filemeta selectable">{outputDir}</span>}
          <span className="spacer" />
          <button
            type="button"
            className="btn btn-primary btn-lg"
            disabled={!ready}
            onClick={() => onRun(accepted, options, outputDir)}
          >
            {task.verb}
          </button>
        </div>

        {tooFew && files.length > 0 && (
          <p className="note">
            {task.name} needs at least {task.minInputs}{" "}
            {task.minInputs === 1 ? "file" : "files"}.
          </p>
        )}
        {tooMany && (
          <p className="note">
            {task.name} takes at most {task.maxInputs}{" "}
            {task.maxInputs === 1 ? "file" : "files"} at a time.
          </p>
        )}
      </div>
    </div>
  );
}

function FileRow({
  file,
  index,
  total,
  accepted,
  reorderable,
  dragging,
  dropTarget,
  onGrab,
  onMove,
  onRemove,
}: {
  file: FileInfo;
  index: number;
  total: number;
  accepted: boolean;
  reorderable: boolean;
  dragging: boolean;
  dropTarget: boolean;
  onGrab: () => void;
  onMove: (to: number) => void;
  onRemove: () => void;
}) {
  const state = `${dragging ? " dragging" : ""}${dropTarget ? " drop-target" : ""}`;

  return (
    <div className={`file-row${state}`} data-row={index}>
      {reorderable && (
        <button
          type="button"
          className="grip"
          // The handle needs no explanation of its own: a grip beside a
          // numbered row, in a list about to be combined, says what it is by
          // being there. The label below is for screen readers, which cannot
          // see either the grip or the number.
          aria-label={`Move ${file.name}, currently ${index + 1} of ${total}`}
          onPointerDown={(e) => {
            // Stops the press turning into a text selection or a native drag
            // while the row is being carried.
            e.preventDefault();
            onGrab();
          }}
          // The same reordering without a pointer, for anyone using the
          // keyboard. Dragging is otherwise the only way, and dragging cannot
          // be done from a keyboard at all.
          onKeyDown={(e) => {
            if (e.key === "ArrowUp") {
              e.preventDefault();
              onMove(index - 1);
            }
            if (e.key === "ArrowDown") {
              e.preventDefault();
              onMove(index + 1);
            }
          }}
        >
          <Icon name="grip" size={16} />
        </button>
      )}
      {reorderable && <span className="filemeta">{index + 1}</span>}
      <Icon name={iconFor(file.category)} size={18} />
      <span className="filename" style={{ flex: 1 }}>
        {file.name}
      </span>
      {file.mismatch && (
        <span className="file-warn">Actually {file.mismatch.toUpperCase()}</span>
      )}
      {file.encrypted && <span className="file-warn">Protected</span>}
      {!accepted && <span className="file-warn">Not supported here</span>}
      <span className="filemeta">{humanBytes(file.sizeBytes)}</span>
      <button
        type="button"
        className="btn btn-ghost"
        onClick={onRemove}
        aria-label={`Remove ${file.name}`}
      >
        <Icon name="close" size={14} />
      </button>
    </div>
  );
}

// Wrapped once here rather than in each branch below, so every kind of
// control explains itself the same way and a new one cannot forget to.
function OptionControl(props: {
  option: TaskOption;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  if (!props.option.help) return <OptionBody {...props} />;
  return (
    <Tooltip text={props.option.help}>
      <OptionBody {...props} />
    </Tooltip>
  );
}

function OptionBody({
  option,
  value,
  onChange,
}: {
  option: TaskOption;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  if (option.type === "choice") {
    return (
      <div className="option">
        <span className="option-label">{option.label}</span>
        <div className="choices" role="radiogroup" aria-label={option.label}>
          {option.choices?.map((c) => (
            <button
              key={c.value}
              type="button"
              role="radio"
              aria-checked={value === c.value}
              className={`choice${value === c.value ? " on" : ""}`}
              onClick={() => onChange(c.value)}
            >
              <span className="choice-label">{c.label}</span>
              {c.hint && <span className="choice-hint">{c.hint}</span>}
            </button>
          ))}
        </div>
      </div>
    );
  }

  if (option.type === "toggle") {
    const on = Boolean(value);
    return (
      <button
        type="button"
        className="toggle"
        role="switch"
        aria-checked={on}
        onClick={() => onChange(!on)}
      >
        <span className={`toggle-box${on ? " on" : ""}`}>
          {on && <Icon name="check" size={12} />}
        </span>
        <span className="option-label">{option.label}</span>
      </button>
    );
  }

  if (option.type === "range") {
    const current = Number(value ?? option.default ?? 0);
    return (
      <div className="option">
        <span className="option-label">{option.label}</span>
        <div className="slider">
          <input
            type="range"
            min={option.min ?? 0}
            max={option.max ?? 100}
            step={option.step ?? 1}
            value={current}
            aria-label={option.label}
            onChange={(e) => onChange(Number(e.target.value))}
          />
          <span className="slider-value">{current}</span>
        </div>
      </div>
    );
  }

  return (
    <div className="option">
      <label className="option-label" htmlFor={option.id}>
        {option.label}
      </label>
      <input
        id={option.id}
        type={option.type === "password" ? "password" : "text"}
        className="field"
        placeholder={option.placeholder}
        value={String(value ?? "")}
        onChange={(e) => onChange(e.target.value)}
      />
    </div>
  );
}

function defaultsOf(task: Task): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const o of task.options) {
    if (o.default !== null && o.default !== undefined) out[o.id] = o.default;
  }
  return out;
}

function hintFor(task: Task): string {
  if (task.maxInputs === 1) return "One file at a time for this one";
  if (task.minInputs > 1) return `At least ${task.minInputs} files`;
  return "One or several";
}

function iconFor(category: string): string {
  switch (category) {
    case "pdf":
      return "pdf";
    case "image":
      return "images";
    case "video":
      return "video";
    case "audio":
      return "audio";
    case "document":
      return "document";
    default:
      return "file";
  }
}
