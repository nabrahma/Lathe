import { Icon } from "../components/Icon";
import type { Job } from "../lib/api";

/*
 * Progress and result share a screen, because a job that finishes in two
 * seconds should not flash a progress bar and then move the user somewhere
 * else. The card simply changes state in place.
 */

interface ProgressProps {
  job: Job;
  onCancel: () => void;
  onOpen: (path: string) => void;
  onReveal: (path: string) => void;
  onAgain: () => void;
  onRetry: () => void;
}

export function Progress({ job, onCancel, onOpen, onReveal, onAgain, onRetry }: ProgressProps) {
  const running = !isTerminal(job.state);
  const determinate = job.progress >= 0 && job.progress <= 1;

  return (
    <div className="scroll fade-in">
      <div className="screen">
        <h1 className="section-label">{job.name}</h1>

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
          <Completed job={job} onOpen={onOpen} onReveal={onReveal} onAgain={onAgain} />
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
  onOpen,
  onReveal,
  onAgain,
}: {
  job: Job;
  onOpen: (p: string) => void;
  onReveal: (p: string) => void;
  onAgain: () => void;
}) {
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
              <span className="result-delta">{delta(job.inputBytes, job.outputBytes)}</span>
            )}
            <div className="row" style={{ marginTop: "var(--gap-2)" }}>
              <button type="button" className="btn btn-secondary" onClick={() => onOpen(path)}>
                Open
              </button>
              <button type="button" className="btn btn-secondary" onClick={() => onReveal(path)}>
                <Icon name="folder" size={14} />
                Show in folder
              </button>
            </div>
          </div>
        </div>
      ))}

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
    </div>
  );
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

function delta(before?: number, after?: number): string {
  if (!before || !after) return "";
  const percent = Math.round(((before - after) / before) * 100);
  const change =
    percent > 0 ? `${percent}% smaller` : percent < 0 ? `${-percent}% larger` : "same size";
  return `${bytes(before)} → ${bytes(after)} · ${change}`;
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
