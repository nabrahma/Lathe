import { useCallback, useRef, useState } from "react";
import { createPortal } from "react-dom";

/*
 * A label has to be short, and short labels are where jargon hides. "Add a
 * bookmark per file" is exact and means nothing to someone who has never
 * opened a PDF outline, so the sentence that explains it lives here rather
 * than cluttering the control with permanent small print.
 */

// The tooltip sits above and to the left of the pointer, clear of the hand
// that is holding the mouse.
const offset = 12;

// Kept off the window edge, so a control near the right or top of the screen
// does not push its own explanation out of view.
const margin = 8;

export function Tooltip({ text, children }: { text: string; children: React.ReactNode }) {
  const [at, setAt] = useState<{ x: number; y: number } | null>(null);
  const box = useRef<HTMLDivElement | null>(null);

  const follow = useCallback((e: React.MouseEvent) => {
    const rect = box.current?.getBoundingClientRect();
    const width = rect?.width ?? 0;
    const height = rect?.height ?? 0;

    // Anchored by its bottom-right corner, which puts it up and to the left,
    // then pulled back inside the window if that would overflow.
    let x = e.clientX - offset - width;
    let y = e.clientY - offset - height;
    if (x < margin) x = Math.min(e.clientX + offset, window.innerWidth - width - margin);
    if (y < margin) y = Math.min(e.clientY + offset, window.innerHeight - height - margin);

    setAt({ x, y });
  }, []);

  if (!text) return <>{children}</>;

  return (
    <span
      className="has-help"
      onMouseEnter={follow}
      onMouseMove={follow}
      onMouseLeave={() => setAt(null)}
    >
      {children}
      {at &&
        createPortal(
          // Rendered at the document root so no ancestor's overflow or
          // stacking context can clip it.
          <div ref={box} className="tip" style={{ left: at.x, top: at.y }} role="tooltip">
            {text}
          </div>,
          document.body,
        )}
    </span>
  );
}
