import { useMemo, useState } from "react";

import { DropZone } from "../components/DropZone";
import { Icon } from "../components/Icon";
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
            {files.map((f) => (
              <FileRow
                key={f.path}
                file={f}
                accepted={task.accepts.includes(f.category)}
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
  accepted,
  onRemove,
}: {
  file: FileInfo;
  accepted: boolean;
  onRemove: () => void;
}) {
  return (
    <div className="file-row">
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

function OptionControl({
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
