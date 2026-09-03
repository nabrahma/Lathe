/*
 * The Lathe mark: a workpiece seen end on, its face broken where the tool has
 * cut, and the tool sitting in the cut.
 *
 * The same geometry is in docs/brand/logo.svg and scripts/appicon, in the same
 * 64-unit space. Changing one means changing all three.
 */
export function Logo({ size = 20 }: { size?: number }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 64 64"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      {/* The workpiece takes the surrounding text colour, so the mark inverts
          wherever it is placed. */}
      <circle
        cx="29"
        cy="32"
        r="22"
        stroke="currentColor"
        strokeWidth="4"
        strokeDasharray="128.247 9.983"
        transform="rotate(13 29 32)"
      />
      <circle cx="29" cy="32" r="5.5" fill="currentColor" />
      <path d="M47 32 56 27.5H64v9h-8Z" fill="var(--hi)" />
    </svg>
  );
}
