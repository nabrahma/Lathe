import { useEffect, useState } from "react";

import { Icon } from "../components/Icon";
import type {
  ComponentProgress,
  ComponentStatus,
  Settings as Prefs,
  ShellIntegrationStatus,
} from "../lib/api";
import { api, humanBytes, onEvent } from "../lib/api";

/*
 * Settings is off the main path, reachable but never in the way. It is also
 * where the tier system becomes visible: what is installed, what it costs in
 * disk space, and how to get rid of it.
 */

interface SettingsProps {
  onBack: () => void;
}

export function SettingsScreen({ onBack }: SettingsProps) {
  const [prefs, setPrefs] = useState<Prefs | null>(null);
  const [components, setComponents] = useState<ComponentStatus[]>([]);
  const [installing, setInstalling] = useState<Record<string, ComponentProgress>>({});
  const [problem, setProblem] = useState("");
  const [shell, setShell] = useState<ShellIntegrationStatus | null>(null);

  useEffect(() => {
    void api.settings().then(setPrefs);
    void api.components().then(setComponents);
    void api.shellIntegrationStatus().then(setShell);

    return onEvent<ComponentProgress>("component:progress", (p) => {
      setInstalling((c) => ({ ...c, [p.componentId]: p }));
    });
  }, []);

  const update = (patch: Partial<Prefs>) => {
    if (!prefs) return;
    const next = { ...prefs, ...patch };
    setPrefs(next);
    setProblem("");

    // Saving can fail, because the shell entry actually touches the system, so
    // the toggle is reconciled with what the backend reports afterwards rather
    // than left showing a change that did not happen.
    void api
      .saveSettings(next)
      .catch((err: unknown) => setProblem(messageOf(err)))
      .finally(() => {
        void api.settings().then(setPrefs);
        void api.shellIntegrationStatus().then(setShell);
      });
  };

  const install = async (id: string) => {
    setProblem("");
    try {
      await api.installComponent(id);
      setComponents(await api.components());
    } catch (err) {
      setProblem(messageOf(err));
    } finally {
      setInstalling((c) => {
        const next = { ...c };
        delete next[id];
        return next;
      });
    }
  };

  const remove = async (id: string) => {
    await api.removeComponent(id);
    setComponents(await api.components());
  };

  if (!prefs) return null;

  return (
    <div className="scroll fade-in">
      <div className="screen">
        <div className="row">
          <button type="button" className="btn btn-ghost" onClick={onBack}>
            <Icon name="back" size={16} />
            Back
          </button>
          <h1 className="section-label">Settings</h1>
        </div>

        <section className="col" style={{ gap: "var(--gap-4)" }}>
          <h2 className="section-label">Components</h2>
          <p className="t-prose" style={{ color: "var(--fg-mid)" }}>
            Lathe converts PDFs and images on its own. Video and Office documents
            need extra software, which is downloaded or detected once and then
            works offline like everything else.
          </p>

          {problem && (
            <div className="error">
              <p className="error-message">{problem}</p>
            </div>
          )}

          <div className="panel">
            {components.map((c) => (
              <ComponentRow
                key={c.component.id}
                status={c}
                progress={installing[c.component.id]}
                onInstall={() => void install(c.component.id)}
                onRemove={() => void remove(c.component.id)}
              />
            ))}
          </div>
        </section>

        <section className="col" style={{ gap: "var(--gap-4)" }}>
          <h2 className="section-label">Converting</h2>

          <div className="option">
            <span className="option-label">Jobs at the same time</span>
            <div className="slider">
              <input
                type="range"
                min={1}
                max={8}
                step={1}
                value={prefs.concurrency}
                aria-label="Jobs at the same time"
                onChange={(e) => update({ concurrency: Number(e.target.value) })}
              />
              <span className="slider-value">{prefs.concurrency}</span>
            </div>
            <p className="t-prose" style={{ color: "var(--fg-low)" }}>
              Video conversion uses every core it is given. A higher number is
              not faster overall, and makes the rest of the machine sluggish.
            </p>
          </div>

          <Toggle
            label="Enhance images before reading text"
            on={prefs.enhanceBeforeOcr}
            onChange={(v) => update({ enhanceBeforeOcr: v })}
          />
          {shell?.supported !== false && (
            <Toggle
              label={"Add “Convert with Lathe” to the right-click menu"}
              on={shell?.installed ?? prefs.shellIntegration}
              onChange={(v) => update({ shellIntegration: v })}
            />
          )}
          {shell?.supported === false && shell.detail && (
            <p className="t-prose" style={{ color: "var(--fg-low)" }}>
              {shell.detail}
            </p>
          )}
        </section>

        <section className="col" style={{ gap: "var(--gap-4)" }}>
          <h2 className="section-label">Privacy</h2>
          <p className="t-prose" style={{ color: "var(--fg-mid)" }}>
            Lathe never uploads your files. There is no server, no account and no
            tracking of any kind. You can check this yourself: disconnect from the
            internet and everything except a component download works exactly the
            same.
          </p>
          <Toggle
            label="Check for updates"
            on={prefs.checkUpdates}
            onChange={(v) => update({ checkUpdates: v, askedAboutUpdates: true })}
          />
          <p className="t-prose" style={{ color: "var(--fg-low)" }}>
            When this is on, Lathe asks a server whether a newer version exists and
            sends nothing but its own version number. When it is off, Lathe makes no
            network request at all.
          </p>
        </section>
      </div>
    </div>
  );
}

function ComponentRow({
  status,
  progress,
  onInstall,
  onRemove,
}: {
  status: ComponentStatus;
  progress?: ComponentProgress;
  onInstall: () => void;
  onRemove: () => void;
}) {
  const { component } = status;
  const busy = Boolean(progress) && progress!.fraction < 1;

  return (
    <div className="file-row" style={{ alignItems: "flex-start" }}>
      <span className={`status-dot ${status.usable ? "go" : "warn"}`} style={{ marginTop: 6 }} />
      <div className="col" style={{ flex: 1, gap: "var(--gap-2)" }}>
        <span className="t-data">{component.displayName}</span>
        <span className="task-desc">{component.explanation}</span>

        {busy && (
          <>
            <div className="bar" style={{ marginTop: 4 }}>
              <div
                className="bar-fill"
                style={{ width: `${Math.max(0, Math.round(progress!.fraction * 100))}%` }}
              />
            </div>
            <span className="filemeta">
              {progress!.stage} · {humanBytes(progress!.bytesDone)} of{" "}
              {humanBytes(progress!.bytesTotal)}
            </span>
          </>
        )}

        {!status.usable && status.problem && !busy && (
          <span className="file-warn">{status.problem}</span>
        )}
      </div>

      <span className="filemeta">
        {status.usable
          ? status.diskBytes
            ? humanBytes(status.diskBytes)
            : "Installed"
          : component.downloadBytes
            ? humanBytes(component.downloadBytes)
            : ""}
      </span>

      {status.usable && status.diskBytes ? (
        <button type="button" className="btn btn-ghost" onClick={onRemove}>
          Remove
        </button>
      ) : status.usable ? null : component.systemOnly ? null : (
        <button type="button" className="btn btn-secondary" disabled={busy} onClick={onInstall}>
          <Icon name="download" size={14} />
          Download
        </button>
      )}
    </div>
  );
}

function Toggle({
  label,
  on,
  onChange,
}: {
  label: string;
  on: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <button type="button" className="toggle" role="switch" aria-checked={on} onClick={() => onChange(!on)}>
      <span className={`toggle-box${on ? " on" : ""}`}>{on && <Icon name="check" size={12} />}</span>
      <span className="option-label">{label}</span>
    </button>
  );
}

function messageOf(err: unknown): string {
  if (typeof err === "string") return err;
  if (err instanceof Error) return err.message;
  return "Something went wrong.";
}
