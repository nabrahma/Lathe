import { useCallback, useEffect, useRef, useState } from "react";

/*
 * The drop zone.
 *
 * On drag-over the four corner ticks move inward and thicken: a machine
 * closing on a workpiece. It is the signature gesture of the app and appears
 * nowhere else.
 *
 * Files arrive from the operating system through Wails' drag-and-drop, which
 * gives real paths. A browser drop event gives a File object with no path,
 * which is useless to a local converter, so the desktop path is the only one
 * that matters, and there is deliberately no HTML file input anywhere.
 */

interface DropZoneProps {
  /** Called with absolute paths once files land. */
  onFiles: (paths: string[]) => void;
  /** Opens the operating system's own file dialog. */
  onBrowse: () => void;
  title?: string;
  hint?: string;
  count?: number;
}

export function DropZone({ onFiles, onBrowse, title, hint, count = 0 }: DropZoneProps) {
  const [over, setOver] = useState(false);
  const depth = useRef(0);

  const handleDropped = useCallback(
    (paths: string[]) => {
      depth.current = 0;
      setOver(false);
      if (paths.length > 0) onFiles(paths);
    },
    [onFiles],
  );

  useEffect(() => {
    // Wails reports OS drops on the window; the element only tracks whether
    // the pointer is over it, for the tick animation.
    const runtime = window.runtime as
      | { EventsOn(e: string, h: (...d: unknown[]) => void): () => void }
      | undefined;
    if (!runtime) return;

    return runtime.EventsOn("wails:file-drop", (...data: unknown[]) => {
      const paths = data.find(Array.isArray) as string[] | undefined;
      handleDropped(paths ?? []);
    });
  }, [handleDropped]);

  const onDragOver = (e: React.DragEvent) => {
    e.preventDefault();
    // Without this the browser shows its own "copy" badge on the cursor, which
    // is one of the clearest tells that a window is a web page.
    e.dataTransfer.dropEffect = "copy";
  };

  const onDragEnter = (e: React.DragEvent) => {
    e.preventDefault();
    // Counting enter and leave avoids the flicker caused by moving between
    // child elements inside the zone.
    depth.current += 1;
    setOver(true);
  };

  const onDragLeave = () => {
    depth.current -= 1;
    if (depth.current <= 0) {
      depth.current = 0;
      setOver(false);
    }
  };

  const onDrop = (e: React.DragEvent) => {
    e.preventDefault();
    depth.current = 0;
    setOver(false);
  };

  const heading = over && count > 0 ? `${count} files · ready` : (title ?? "Drop files here");

  return (
    <div
      className={`dropzone worksurface ticked${over ? " over" : ""}`}
      onDragOver={onDragOver}
      onDragEnter={onDragEnter}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
      onDoubleClick={onBrowse}
    >
      <span className="tick-b" />
      <div>
        <div className="dropzone-title">{heading}</div>
        <div className="dropzone-hint">{hint ?? "or"}</div>
        <button type="button" className="btn btn-secondary" onClick={onBrowse} style={{ marginTop: 12 }}>
          Choose files
        </button>
      </div>
    </div>
  );
}
