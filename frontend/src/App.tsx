import { useCallback, useEffect, useState } from "react";

import { Icon } from "./components/Icon";
import { Logo } from "./components/Logo";
import { Home } from "./screens/Home";
import { Progress } from "./screens/Progress";
import { SettingsScreen } from "./screens/Settings";
import { TaskScreen } from "./screens/TaskScreen";
import type { FileInfo, Job, Task } from "./lib/api";
import { api, onEvent } from "./lib/api";

/*
 * Four screens is the whole application: home, task, progress and result,
 * with progress and result sharing one. Settings is reachable but off the path.
 */

type Screen =
  | { name: "home" }
  | { name: "task"; task: Task; files: FileInfo[] }
  | { name: "job"; jobId: string; task: Task }
  | { name: "settings" };

export function App() {
  const [screen, setScreen] = useState<Screen>({ name: "home" });
  const [tasks, setTasks] = useState<Task[]>([]);
  const [jobs, setJobs] = useState<Record<string, Job>>({});
  const [platform, setPlatform] = useState("");
  const [quitting, setQuitting] = useState(0);
  const [startupError, setStartupError] = useState("");
  // Files the user has offered, however they arrived: a drag onto the window,
  // or launching the app with a file from the context menu.
  const [incoming, setIncoming] = useState<FileInfo[]>([]);

  useEffect(() => {
    void api
      .tasks()
      .then(setTasks)
      .catch((err: unknown) => setStartupError(String(err)));
    void api.platform().then((p) => setPlatform(p.os));

    // A cold launch from "Convert with Lathe" arrives as a command-line
    // argument, and lands on the same filtered home screen as a drag.
    void api.pendingFiles().then((files) => {
      if (files.length > 0) setIncoming(files);
    });
  }, []);

  // Files dropped onto the window from the operating system.
  useEffect(() => {
    const runtime = window.runtime;
    if (!runtime) return;

    return runtime.EventsOn("wails:file-drop", (...data: unknown[]) => {
      const paths = data.find(Array.isArray) as string[] | undefined;
      if (!paths?.length) return;
      void api.inspect(paths).then(setIncoming);
    });
  }, []);

  // Job updates arrive as events rather than being polled, so the interface
  // never asks the backend "are you done yet".
  useEffect(
    () =>
      onEvent<Job>("job:update", (job) => {
        setJobs((current) => ({ ...current, [job.id]: job }));
      }),
    [],
  );

  useEffect(() => onEvent<number>("quit:confirm", setQuitting), []);

  useEffect(
    () =>
      onEvent<string>("menu:screen", (name) => {
        if (name === "settings") setScreen({ name: "settings" });
      }),
    [],
  );

  // A second launch, which happens when someone uses the context menu while a
  // window is already open.
  useEffect(
    () =>
      onEvent<FileInfo[]>("files:opened", (files) => {
        if (files.length === 0) return;
        setIncoming(files);
        setScreen({ name: "home" });
      }),
    [],
  );

  // Remember where the window was, without writing on every pixel of a drag.
  useEffect(() => {
    const save = () => void api.saveWindowState();
    let timer: number | undefined;
    const onResize = () => {
      window.clearTimeout(timer);
      timer = window.setTimeout(save, 400);
    };
    window.addEventListener("resize", onResize);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("resize", onResize);
    };
  }, []);

  const run = useCallback(
    async (task: Task, files: FileInfo[], options: Record<string, unknown>, outputDir: string) => {
      const job = await api.submit(
        task.id,
        files.map((f) => f.path),
        options,
        outputDir,
      );
      setJobs((current) => ({ ...current, [job.id]: job }));
      setScreen({ name: "job", jobId: job.id, task });
    },
    [],
  );

  if (startupError) {
    return (
      <div className="app">
        <TitleBar platform={platform} />
        <div className="screen">
          <div className="error">
            <p className="error-message">
              Lathe couldn&apos;t start up properly. Restarting usually fixes it.
            </p>
            <pre className="error-detail">{startupError}</pre>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app">
      <TitleBar platform={platform} onHome={() => setScreen({ name: "home" })} />

      {screen.name === "home" && (
        <Home
          tasks={tasks}
          incoming={incoming}
          onClearIncoming={() => setIncoming([])}
          onPick={(task, files) => setScreen({ name: "task", task, files })}
          onSettings={() => setScreen({ name: "settings" })}
        />
      )}

      {screen.name === "task" && (
        <TaskScreen
          // Keyed by task, so switching tasks remounts with that task's own
          // defaults rather than syncing state in an effect.
          key={screen.task.id}
          task={screen.task}
          initialFiles={screen.files}
          onBack={() => setScreen({ name: "home" })}
          onRun={(files, options, outputDir) =>
            void run(screen.task, files, options, outputDir)
          }
        />
      )}

      {screen.name === "job" && jobs[screen.jobId] && (
        <Progress
          job={jobs[screen.jobId]}
          task={screen.task}
          tasks={tasks}
          onCancel={() => void api.cancel(screen.jobId)}
          onOpen={(p) => api.open(p)}
          onReveal={(p) => api.reveal(p)}
          onAgain={() => setScreen({ name: "home" })}
          onHome={() => setScreen({ name: "home" })}
          onRetry={() => setScreen({ name: "task", task: screen.task, files: [] })}
          onPickTask={(task, files) => setScreen({ name: "task", task, files })}
        />
      )}

      {screen.name === "settings" && (
        <SettingsScreen onBack={() => setScreen({ name: "home" })} />
      )}

      {quitting > 0 && (
        <QuitPrompt
          active={quitting}
          onStay={() => setQuitting(0)}
          onQuit={() => void api.quit()}
        />
      )}
    </div>
  );
}

function TitleBar({ platform, onHome }: { platform: string; onHome?: () => void }) {
  // Linux keeps the window manager's own title bar, so the app draws none.
  const framed = platform === "linux";

  return (
    <header className={`titlebar${platform === "darwin" ? " mac" : ""}`}>
      <button type="button" className="wordmark" onClick={onHome}>
        <Logo size={18} />
        LATHE
      </button>
      <span className="spacer" />

      {!framed && platform !== "darwin" && (
        <div className="window-controls">
          <button
            type="button"
            className="window-control"
            onClick={() => void api.minimise()}
            aria-label="Minimise"
          >
            <Icon name="minimise" size={14} />
          </button>
          <button
            type="button"
            className="window-control"
            onClick={() => void api.toggleMaximise()}
            aria-label="Maximise"
          >
            <Icon name="maximise" size={12} />
          </button>
          <button
            type="button"
            className="window-control close"
            onClick={() => void api.close()}
            aria-label="Close"
          >
            <Icon name="close" size={14} />
          </button>
        </div>
      )}
    </header>
  );
}

function QuitPrompt({
  active,
  onStay,
  onQuit,
}: {
  active: number;
  onStay: () => void;
  onQuit: () => void;
}) {
  return (
    <div className="scrim">
      <div className="dialog">
        <h2 className="dialog-title">Still working</h2>
        <p className="dialog-body">
          {active === 1
            ? "One job is still running. Quitting now cancels it; your original files are untouched either way."
            : `${active} jobs are still running. Quitting now cancels them; your original files are untouched either way.`}
        </p>
        <div className="row">
          <span className="spacer" />
          <button type="button" className="btn btn-secondary" onClick={onStay}>
            Keep working
          </button>
          <button type="button" className="btn btn-destructive" onClick={onQuit}>
            Quit anyway
          </button>
        </div>
      </div>
    </div>
  );
}
