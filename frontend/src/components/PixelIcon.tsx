import { GLYPH_SIZE, glyphs } from "./pixelGlyphs";

/*
 * Task icons, drawn on a pixel grid rather than as hairline strokes.
 *
 * The chrome glyphs in Icon.tsx stay as strokes on purpose: at 16px in a
 * button or a title bar a pixel grid turns to mush. These sit at 40px on the
 * card plate, where the grid reads as deliberate and matches the terminal face
 * around it.
 *
 * Shapes come from pixelarticons and are baked in by scripts/gen-icons.mjs, so
 * nothing is fetched at runtime and the bundle stays self-contained.
 */
export function PixelIcon({ name, size = 40 }: { name: string; size?: number }) {
  const paths = glyphs[name] ?? glyphs.document;
  return (
    <svg
      width={size}
      height={size}
      viewBox={`0 0 ${GLYPH_SIZE} ${GLYPH_SIZE}`}
      fill="currentColor"
      // Without this the grid softens into grey edges at the sizes the cards
      // use, which is the whole point of a pixel icon lost.
      shapeRendering="crispEdges"
      aria-hidden="true"
      focusable="false"
    >
      {paths.map((d) => (
        <path key={d} d={d} />
      ))}
    </svg>
  );
}
