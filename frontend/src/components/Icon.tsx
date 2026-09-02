/*
 * Icons are drawn as thin geometric strokes, matching the hairline borders
 * elsewhere. No icon library: a handful of glyphs is cheaper to draw than to
 * depend on, and it keeps every stroke consistent with the rest of the system.
 */

interface IconProps {
  name: string;
  size?: number;
  className?: string;
}

const paths: Record<string, string> = {
  compress: "M4 8h6M4 8l2-2M4 8l2 2M20 16h-6M20 16l-2-2M20 16l-2 2M3 12h18",
  merge: "M4 5h7v14H4zM13 5h7v14h-7zM11 12h2",
  split: "M12 3v18M6 8H3v8h3M18 8h3v8h-3",
  rotate: "M4 12a8 8 0 1 1 3 6M4 12V7M4 12h5",
  delete: "M5 7h14M9 7V4h6v3M7 7l1 13h8l1-13",
  reorder: "M4 6h16M4 12h16M4 18h16M8 4l-2 2 2 2M16 20l2-2-2-2",
  watermark: "M4 4h16v16H4zM8 15l8-8M9 9h.01M15 15h.01",
  lock: "M6 11h12v9H6zM9 11V7a3 3 0 0 1 6 0v4",
  unlock: "M6 11h12v9H6zM9 11V7a3 3 0 0 1 5.7-1.3",
  images: "M3 5h13v11H3zM8 21h13V10M6 12l3-3 4 4 2-2 1 1",
  pdf: "M6 3h8l4 4v14H6zM14 3v4h4M9 13h6M9 17h4",
  convert: "M4 8h13l-3-3M20 16H7l3 3",
  resize: "M4 4h9v9H4zM13 13h7v7h-7M13 13l7 7",
  crop: "M6 2v16h16M2 6h16v16",
  text: "M5 6h14M5 6V4h14v2M12 6v14M9 20h6",
  search: "M11 4a7 7 0 1 1 0 14 7 7 0 0 1 0-14zM16 16l4 4",
  document: "M6 3h8l4 4v14H6zM14 3v4h4M9 12h6M9 16h6",
  word: "M6 3h8l4 4v14H6zM14 3v4h4M8 12l2 6 2-5 2 5 2-6",
  video: "M3 6h13v12H3zM16 10l5-3v10l-5-3",
  audio: "M9 18V5l10-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM19 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0z",
  gif: "M3 5h18v14H3zM8 12h3v4M8 12a2 2 0 1 1 3-1.7M14 9v6M17 15V9h3M17 12h2",

  // Interface glyphs
  back: "M15 5l-7 7 7 7",
  close: "M6 6l12 12M18 6L6 18",
  minimise: "M5 12h14",
  maximise: "M5 5h14v14H5z",
  check: "M4 12l5 5L20 6",
  chevron: "M8 5l7 7-7 7",
  // Sliders rather than a gear: at 16px a gear collapses into a blob, and
  // sliders read correctly at every size the interface uses.
  settings: "M4 7h9M17 7h3M4 17h3M11 17h9M15 4v6M8 14v6",
  folder: "M3 6h6l2 2h10v11H3z",
  file: "M6 3h8l4 4v14H6zM14 3v4h4",
  download: "M12 4v11M8 11l4 4 4-4M4 20h16",
  warning: "M12 3l9 17H3zM12 9v5M12 17h.01",
  info: "M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18zM12 11v6M12 8h.01",
  plus: "M12 5v14M5 12h14",
};

export function Icon({ name, size = 20, className }: IconProps) {
  const d = paths[name] ?? paths.file;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="square"
      strokeLinejoin="miter"
      className={className}
      aria-hidden="true"
      focusable="false"
    >
      <path d={d} />
    </svg>
  );
}
