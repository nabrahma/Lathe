import { useEffect, useState } from "react";

import { Icon } from "../components/Icon";
import { TaskCard } from "../components/TaskCard";
import type { FileInfo, Job, Task } from "../lib/api";
import { api } from "../lib/api";

/*
 * Progress and result share a screen, because a job that finishes in two
 * seconds should not flash a progress bar and then move the user somewhere
 * else. The card simply changes state in place.
 */

interface ProgressProps {
  job: Job;
  task: Task;
  tasks: Task[];
  onCancel: () => void;
  onOpen: (path: string) => Promise<void>;
  onReveal: (path: string) => Promise<void>;
  onAgain: () => void;
  onHome: () => void;
  onRetry: () => void;
  onPickTask: (task: Task, files: FileInfo[]) => void;
}

export function Progress({
  job,
  task,
  tasks,
  onCancel,
  onOpen,
  onReveal,
  onAgain,
  onHome,
  onRetry,
  onPickTask,
}: ProgressProps) {
  const running = !isTerminal(job.state);
  const determinate = job.progress >= 0 && job.progress <= 1;

  return (
    <div className="scroll fade-in">
      <div className="screen">
        <div className="row">
          <h1 className="section-label">{job.name}</h1>
          <span className="spacer" />
          {/* The wordmark goes home too, but only someone who has already
              found that knows it. A finished job is exactly where people look
              for the way back. */}
          <button type="button" className="btn btn-ghost" onClick={onHome}>
            <Icon name="back" size={16} />
            Back to home
          </button>
        </div>

        {running && (
          <div className="col" style={{ gap: "var(--gap-4)" }}>
            <div className="bar">
              {determinate ? (
                <div
                  className="bar-fill"
                  style={{ width: `${Math.round(job.progress * 100)}%` }}
                />
              ) : (
                // No fake percentage: an indeterminate bar with a stage label
                // is honest, and a bar that sits at 90% is not.
                <div className="bar-indeterminate" />
              )}
            </div>
            <div className="row">
              <span className="stage">{job.stage || "Working"}</span>
              {determinate && (
                <span className="filemeta">{Math.round(job.progress * 100)}%</span>
              )}
              <span className="spacer" />
              {/* Cancel is always available while a job runs. */}
              <button type="button" className="btn btn-secondary" onClick={onCancel}>
                Cancel
              </button>
            </div>
          </div>
        )}

        {job.state === "completed" && (
          <Completed
            job={job}
            task={task}
            tasks={tasks}
            onOpen={onOpen}
            onReveal={onReveal}
            onAgain={onAgain}
            onPickTask={onPickTask}
          />
        )}

        {job.state === "failed" && job.error && (
          <Failed job={job} onRetry={onRetry} onAgain={onAgain} />
        )}

        {job.state === "cancelled" && (
          <div className="col" style={{ gap: "var(--gap-4)" }}>
            <p className="t-prose">
              The job was cancelled. Nothing was written, and your original file is
              exactly as it was.
            </p>
            <div className="row">
              <button type="button" className="btn btn-primary" onClick={onAgain}>
                Start again
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}

function Completed({
  job,
  task,
  tasks,
  onOpen,
  onReveal,
  onAgain,
  onPickTask,
}: {
  job: Job;
  task: Task;
  tasks: Task[];
  onOpen: (p: string) => Promise<void>;
  onReveal: (p: string) => Promise<void>;
  onAgain: () => void;
  onPickTask: (task: Task, files: FileInfo[]) => void;
}) {
  // Opening a file can fail for reasons the user can act on, the file having
  // been moved being the common one, so the failure is shown next to the
  // button that caused it rather than swallowed.
  const [problem, setProblem] = useState("");
  const [produced, setProduced] = useState<FileInfo[]>([]);

  const outputs = job.outputs;
  useEffect(() => {
    if (!outputs?.length) return;
    let live = true;
    // What was produced decides what is worth suggesting next, and only the
    // backend can say what kind of file it is.
    void api
      .inspect(outputs)
      .then((files) => {
        if (live) setProduced(files);
      })
      .catch(() => {
        // Suggestions are a convenience; failing to build them is not worth
        // telling anyone about.
      });
    return () => {
      live = false;
    };
  }, [outputs]);

  const act = (run: (p: string) => Promise<void>, path: string) => {
    setProblem("");
    void run(path).catch((err: unknown) => setProblem(messageOf(err)));
  };

  const suggestions = suggestFor(tasks, task, produced);

  return (
    <div className="col" style={{ gap: "var(--gap-4)" }}>
      {job.outputs?.map((path) => (
        <div key={path} className="result-card ticked">
          <span className="tick-b" />
          <div className="col" style={{ gap: "var(--gap-3)" }}>
            <div className="row">
              <span className="status-dot go" />
              <span className="t-micro" style={{ color: "var(--go)" }}>
                Done
              </span>
            </div>
            <span className="filename">{baseName(path)}</span>
            {job.outputs?.length === 1 && (
              <span className="result-delta">{summary(job, task)}</span>
            )}
            <div className="row" style={{ marginTop: "var(--gap-2)" }}>
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => act(onOpen, path)}
              >
                Open
              </button>
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => act(onReveal, path)}
              >
                <Icon name="folder" size={14} />
                Show in folder
              </button>
            </div>
            {problem && <span className="file-error">{problem}</span>}
          </div>
        </div>
      ))}

      {/* One line for the job as a whole when it produced several files.
          Compressing three images has a saving worth reporting, and it belongs
          to the batch rather than to any one card. The totals are summed
          across every input and output, so this is the real figure. */}
      {(job.outputs?.length ?? 0) > 1 && (
        <span className="result-delta">{summary(job, task)}</span>
      )}

      {job.notes?.map((note) => (
        <p key={note} className="note">
          {note}
        </p>
      ))}

      <div className="row">
        <button type="button" className="btn btn-primary" onClick={onAgain}>
          Convert another
        </button>
      </div>

      {suggestions.length > 0 && (
        <div className="col" style={{ gap: "var(--gap-3)", marginTop: "var(--gap-5)" }}>
          <h2 className="section-label">You can also try</h2>
          <p className="task-desc">
            These work on what you just made, and it is already loaded.
          </p>
          <div className="task-grid">
            {suggestions.map((next, i) => (
              <TaskCard
                key={next.id}
                task={next}
                index={i + 1}
                onPick={() => onPickTask(next, produced)}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// suggestFor picks what to offer next: tasks that accept the files just
// produced, so choosing one carries them straight through rather than asking
// for them again.
//
// The count has to fit at both ends. Splitting a PDF leaves ten files, and
// most tasks take one, so offering Compress next would land the user on a
// screen that immediately refuses what it was handed. A suggestion that cannot
// be acted on is worse than an empty space.
function suggestFor(tasks: Task[], current: Task, produced: FileInfo[]): Task[] {
  if (produced.length === 0) return [];
  const category = produced[0].category;

  return tasks
    .filter(
      (t) =>
        t.id !== current.id &&
        t.available &&
        t.accepts.includes(category) &&
        produced.length >= t.minInputs &&
        // MaxInputs of zero means unlimited, as merge needs.
        (t.maxInputs === 0 || produced.length <= t.maxInputs),
    )
    .slice(0, 4);
}

function Failed({
  job,
  onRetry,
  onAgain,
}: {
  job: Job;
  onRetry: () => void;
  onAgain: () => void;
}) {
  const error = job.error!;
  const actions = error.actions ?? [];

  return (
    <div className="col" style={{ gap: "var(--gap-4)" }}>
      <div className="error">
        <div className="row" style={{ marginBottom: "var(--gap-2)" }}>
          <span className="status-dot stop" />
          <span className="t-micro" style={{ color: "var(--stop)" }}>
            Didn&apos;t work
          </span>
        </div>
        {/* Sentence case, sans, never uppercase. */}
        <p className="error-message">{error.message}</p>
        {actions.includes("copy_details") && error.detail && (
          <details>
            <summary className="more-options" style={{ marginTop: "var(--gap-3)" }}>
              Technical details
            </summary>
            <pre className="error-detail">{error.detail}</pre>
          </details>
        )}
      </div>

      <div className="row">
        {actions.includes("retry") && (
          <button type="button" className="btn btn-primary" onClick={onRetry}>
            Try again
          </button>
        )}
        <button type="button" className="btn btn-secondary" onClick={onAgain}>
          Choose another file
        </button>
        {error.detail && (
          <button
            type="button"
            className="btn btn-ghost"
            onClick={() => void navigator.clipboard.writeText(error.detail!)}
          >
            Copy details
          </button>
        )}
      </div>
    </div>
  );
}

function isTerminal(state: Job["state"]): boolean {
  return state === "completed" || state === "failed" || state === "cancelled";
}

function baseName(path: string): string {
  return path.split(/[\\/]/).pop() ?? path;
}

// summary says what actually happened, which depends on what was asked for.
//
// A before-and-after size belongs only to the tasks whose purpose is to shrink
// a file. Reporting it after a merge, where it is an artefact of two documents
// no longer repeating each other's fonts, reads as a saving nobody asked for
// and invites the reader to wonder what was discarded.
function summary(job: Job, task: Task): string {
  const after = job.outputBytes;
  const made = job.outputs?.length ?? 0;
  const size = after ? ` · ${bytes(after)}` : "";

  // Only where shrinking was the point. The figures are totals, so this reads
  // correctly for one file and for thirty.
  if (task.shrinksFile && job.inputBytes && after) {
    const change = delta(job.inputBytes, after);
    return made > 1 ? `${change} across ${made} files` : change;
  }
  // Several files in, one out: what happened is that they were combined.
  if (made === 1 && job.inputs.length > 1) {
    return `${job.inputs.length} files combined${size}`;
  }
  if (made > 1) {
    return `${made} files created${size}`;
  }
  return after ? bytes(after) : "";
}

function delta(before: number, after: number): string {
  const percent = Math.round(((before - after) / before) * 100);
  const change =
    percent > 0 ? `${percent}% smaller` : percent < 0 ? `${-percent}% larger` : "same size";
  return `${bytes(before)} → ${bytes(after)} · ${change}`;
}

function messageOf(err: unknown): string {
  if (typeof err === "string") return err;
  if (err instanceof Error) return err.message;
  return "That didn't work.";
}

function bytes(n: number): string {
  if (n < 1000) return `${n} B`;
  const units = ["kB", "MB", "GB"];
  let value = n / 1000;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}
