# Decisions

Architecture decisions, with the reasoning that produced them. Entries are
append-only: superseding a decision means adding a new entry that says so.

## D1 — Wails v2, not v3

**Decision.** Build on Wails v2 (`v2.15.0`).

**Why.** v3 is technically better in every respect — first-class dialogs, a
system tray, GTK4/WebKitGTK 6.0, Windows ARM64 — but is still pre-stable. Lathe
handles people's documents; a pre-stable shell is not an acceptable dependency
for that.

**Consequence.** v3 will eventually be the right target, so migration is kept
cheap: no package under `internal/` may import Wails except `internal/app`,
which is a thin binding layer over the same interfaces the CLI drives. The rule
is enforced in CI by `scripts/boundary`. If the boundary holds, migrating is
rewriting one package.

**Rejected.** Electron (~150 MB before a line of code, against a 60 MB
installer budget); a local web server plus the system browser (loses OS file
dialogs and drag-from-Explorer, and reproduces the "localhost tab" experience
Lathe exists to avoid); three native UIs (not viable for one maintainer).

## D2 — Shell out to external binaries, never link them

**Decision.** Tesseract, FFmpeg and LibreOffice are invoked as subprocesses via
`internal/exec`. Nothing is linked with cgo.

**Why.** Keeps `CGO_ENABLED=0` for the main binary, which makes
cross-compilation trivial; isolates an engine crash in a subprocess rather than
taking down the app with the user's file open; and makes every engine a
uniformly managed component in the tier system rather than a special case.

**Cost.** One process spawn per job, which is negligible next to the runtime of
the work itself.

## D3 — Pure-Go image pipeline, with FFmpeg as the exotic-format tier

**Decision.** Diverges from the original plan of libvips. Core image work uses
`golang.org/x/image`, `disintegration/imaging` and `nativewebp` — all pure Go.
HEIC, AVIF and other codecs that have no pure-Go implementation are handled by
the FFmpeg tier.

**Why.** libvips means cgo, which means D2's `CGO_ENABLED=0` guarantee is lost
and every platform needs a bundled shared library built and shipped. The
pure-Go set covers JPEG, PNG, GIF, BMP, TIFF and WebP, which is the
overwhelming majority of real input. HEIC — genuinely common, since phones
shoot it — is the one important gap, and FFmpeg decodes it well.

**Consequence.** HEIC conversion prompts for the media component the first
time. That is honest and visible, which is the tier system working as designed,
not a workaround. Recorded in `KNOWN_GAPS.md` as a candidate for a pure-Go HEIC
decoder later.

## D4 — AGPL-3.0

**Decision.** Lathe itself is AGPL-3.0. Component licenses are catalogued in
`LICENSES.md`.

**Why.** FFmpeg static builds are typically GPL. Because Lathe downloads them at
runtime rather than bundling them, it is arguably not a derivative work — but
"arguably" is an uncomfortable place to stand when the alternative costs
nothing. AGPL is compatible with every component in the tree, and it prevents
the work being re-hosted as a paid SaaS, which is precisely the model Lathe
exists as an alternative to.

**Consequence.** The runtime-download architecture stays. Bundling a GPL FFmpeg
build into the installer would change this analysis and must not be done
without revisiting this entry.

## D5 — Frameless on Windows and macOS, native title bar on Linux

**Decision.** Custom window chrome on Windows and macOS; the window manager's
own title bar on Linux.

**Why.** A grey OS title bar above a near-black app looks unfinished, and
frameless is well-trodden on both Windows and macOS. Linux window managers vary
too much — dragging, snapping and maximise behaviour all differ — to own that
surface in v1.

**Consequence.** The window looks different on Linux. That is stated in the
README rather than papered over.

## D6 — Pure-Go PDF rasterization

**Decision.** PDF page rendering goes through pdfcpu's content extraction plus
an internal rasterizer rather than pdfium.

**Why.** pdfium means either cgo or a large WebAssembly runtime shipped in the
core tier. Both conflict with D2 and the 60 MB installer budget. Where
full-fidelity rasterization is genuinely required — OCR of a scanned PDF — the
already-present engines handle it.

**Consequence.** Page rendering fidelity is lower than pdfium's for pages with
complex vector content. Recorded honestly in `KNOWN_GAPS.md`.
