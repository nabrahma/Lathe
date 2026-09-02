/*
 * The single place the interface talks to Go.
 *
 * Wails injects bindings on window.go at runtime. Wrapping them here keeps
 * every component free of that detail, gives the calls real types, and means a
 * Wails v3 migration touches one file on this side as well as one package on
 * the other.
 */

export type Category =
  | "pdf"
  | "image"
  | "document"
  | "video"
  | "audio"
  | "text"
  | "archive"
  | "unknown";

export type OptionType =
  | "choice"
  | "range"
  | "toggle"
  | "text"
  | "pagerange"
  | "password";

export interface Choice {
  value: string;
  label: string;
  hint?: string;
}

export interface TaskOption {
  id: string;
  label: string;
  type: OptionType;
  default: unknown;
  choices?: Choice[];
  min?: number;
  max?: number;
  step?: number;
  advanced: boolean;
  placeholder?: string;
}

export interface Task {
  id: string;
  name: string;
  description: string;
  category: "pdf" | "image" | "text" | "document" | "media";
  icon: string;
  verb: string;
  accepts: Category[];
  minInputs: number;
  maxInputs: number;
  options: TaskOption[];
  requiredTier: number;
  engine: string;
  /** False when the task needs a component that is not installed yet. */
  available: boolean;
  downloadMB: number;
  requires?: string;
}

export interface FileInfo {
  path: string;
  name: string;
  sizeBytes: number;
  category: Category;
  extension: string;
  encrypted: boolean;
  /** Set when the contents disagree with the extension. */
  mismatch?: string;
  error?: string;
}

export type JobState =
  | "queued"
  | "preparing"
  | "running"
  | "verifying"
  | "completed"
  | "failed"
  | "cancelled";

export interface UserError {
  code: string;
  message: string;
  detail?: string;
  actions?: string[];
  file?: string;
}

export interface Job {
  id: string;
  taskId: string;
  name: string;
  inputs: string[];
  options: Record<string, unknown>;
  outputDir: string;
  state: JobState;
  /** 0 to 1, or -1 when the engine cannot report a real figure. */
  progress: number;
  stage: string;
  outputs?: string[];
  notes?: string[];
  error?: UserError;
  inputBytes?: number;
  outputBytes?: number;
  queuedAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface Component {
  id: string;
  tier: number;
  displayName: string;
  explanation: string;
  downloadBytes: number;
  installedBytes: number;
  version: string;
  systemOnly: boolean;
}

export interface ComponentStatus {
  component: Component;
  installed: boolean;
  usable: boolean;
  path?: string;
  diskBytes?: number;
  problem?: string;
}

export interface ComponentProgress {
  componentId: string;
  stage: string;
  fraction: number;
  bytesDone: number;
  bytesTotal: number;
}

export interface Settings {
  concurrency: number;
  outputDir: string;
  checkUpdates: boolean;
  askedAboutUpdates: boolean;
  shellIntegration: boolean;
  enhanceBeforeOcr: boolean;
  language: string;
  window: { width: number; height: number; x: number; y: number; maximised: boolean };
}

/** The Go methods Wails binds, as this side sees them. */
interface Backend {
  Tasks(): Promise<Task[]>;
  TasksFor(category: string): Promise<Task[]>;
  Inspect(paths: string[]): Promise<FileInfo[]>;
  ChooseFiles(title: string): Promise<string[]>;
  ChooseFolder(title: string): Promise<string>;
  Reveal(path: string): Promise<void>;
  Open(path: string): Promise<void>;
  Submit(
    taskId: string,
    inputs: string[],
    options: Record<string, unknown>,
    outputDir: string,
  ): Promise<Job>;
  Cancel(jobId: string): Promise<void>;
  Jobs(): Promise<Job[]>;
  ActiveJobs(): Promise<number>;
  Quit(): Promise<void>;
  Components(): Promise<ComponentStatus[]>;
  InstallComponent(id: string): Promise<void>;
  RemoveComponent(id: string): Promise<void>;
  Settings(): Promise<Settings>;
  SaveSettings(s: Settings): Promise<void>;
  Platform(): Promise<{ os: string; version: string; name: string }>;
  Minimise(): Promise<void>;
  ToggleMaximise(): Promise<void>;
  Close(): Promise<void>;
  SaveWindowState(): Promise<void>;
}

interface WailsRuntime {
  EventsOn(event: string, handler: (...data: unknown[]) => void): () => void;
  EventsEmit(event: string, ...data: unknown[]): void;
}

declare global {
  interface Window {
    go?: { app?: { App?: Backend } };
    runtime?: WailsRuntime;
  }
}

/** True when running inside the desktop shell rather than a plain browser. */
export const inDesktop = (): boolean => Boolean(window.go?.app?.App);

function backend(): Backend {
  const bound = window.go?.app?.App;
  if (!bound) {
    throw new Error("The application backend is not available.");
  }
  return bound;
}

export const api = {
  tasks: () => backend().Tasks(),
  tasksFor: (category: string) => backend().TasksFor(category),
  inspect: (paths: string[]) => backend().Inspect(paths),
  chooseFiles: (title: string) => backend().ChooseFiles(title),
  chooseFolder: (title: string) => backend().ChooseFolder(title),
  reveal: (path: string) => backend().Reveal(path),
  open: (path: string) => backend().Open(path),
  submit: (
    taskId: string,
    inputs: string[],
    options: Record<string, unknown>,
    outputDir: string,
  ) => backend().Submit(taskId, inputs, options, outputDir),
  cancel: (jobId: string) => backend().Cancel(jobId),
  jobs: () => backend().Jobs(),
  activeJobs: () => backend().ActiveJobs(),
  quit: () => backend().Quit(),
  components: () => backend().Components(),
  installComponent: (id: string) => backend().InstallComponent(id),
  removeComponent: (id: string) => backend().RemoveComponent(id),
  settings: () => backend().Settings(),
  saveSettings: (s: Settings) => backend().SaveSettings(s),
  platform: () => backend().Platform(),
  minimise: () => backend().Minimise(),
  toggleMaximise: () => backend().ToggleMaximise(),
  close: () => backend().Close(),
  saveWindowState: () => backend().SaveWindowState(),
};

/** Subscribes to a backend event, returning an unsubscribe function. */
export function onEvent<T>(event: string, handler: (payload: T) => void): () => void {
  const runtime = window.runtime;
  if (!runtime) return () => {};
  return runtime.EventsOn(event, (...data: unknown[]) => handler(data[0] as T));
}

/** Formats a byte count the way the file list and result card show it. */
export function humanBytes(n: number): string {
  if (!Number.isFinite(n) || n < 0) return "";
  if (n < 1000) return `${n} B`;

  const units = ["kB", "MB", "GB", "TB"];
  let value = n / 1000;
  let unit = 0;
  while (value >= 1000 && unit < units.length - 1) {
    value /= 1000;
    unit += 1;
  }
  return `${value < 10 ? value.toFixed(1) : Math.round(value)} ${units[unit]}`;
}

/** The size delta shown on a finished result: "14.2 MB to 1.8 MB, 87% smaller". */
export function sizeDelta(before?: number, after?: number): string {
  if (!before || !after) return "";
  const percent = Math.round(((before - after) / before) * 100);
  const change =
    percent > 0 ? `${percent}% smaller` : percent < 0 ? `${-percent}% larger` : "same size";
  return `${humanBytes(before)} → ${humanBytes(after)} · ${change}`;
}
