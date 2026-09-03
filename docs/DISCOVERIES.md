# Discoveries

Things that were not obvious until they were tried. Cross-platform desktop work
generates these constantly; writing them down is cheaper than rediscovering
them.

## Killing a process is not enough: you must kill its tree

`cmd.Process.Kill()` kills the process you spawned and orphans everything it
spawned. FFmpeg and LibreOffice both fork children, and LibreOffice in
particular leaves a `soffice.bin` running that holds a user-profile lock, so the
*next* conversion then hangs waiting for a lock held by a process the user
cannot see.

The fix is different on each platform and there is no portable API:

- **Unix.** Put the child in its own process group (`Setpgid: true`) and signal
  the negative pid, so the signal reaches the whole group.
- **Windows.** There are no process groups in the Unix sense. Create a Job
  Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, assign the child to it, and
  closing the job handle terminates the entire tree, including grandchildren
  spawned after assignment.

Implemented in `internal/exec/process_unix.go` and `process_windows.go`, and
tested by spawning a script that forks and asserting every descendant dies.

## Atomic rename requires the temp file to live in the destination directory

`os.Rename` is atomic only within a single filesystem. Writing to the system
temp directory and renaming into the user's Documents folder is frequently a
cross-device move, which Go implements as copy-then-delete. That is not
atomic, and it is
observable as a half-written file if the process dies mid-copy.

`fsatomic.WriteFile` therefore creates its temp file as a dotfile *beside* the
destination. The fallback path, when the destination directory is not writable,
is to fail early with a clear error rather than silently degrade to a
non-atomic copy.

## Extension-based type detection is wrong often enough to matter in practice

Not a theoretical concern. The three cases seen constantly:

- iOS shares a HEIC as `.jpg`, so the extension is a lie and every naive decoder
  fails with "invalid JPEG format".
- A failed download saves an HTML error page as `.pdf`.
- A `.docx` that is a legacy OLE `.doc` renamed by hand.

`internal/detect` reads magic bytes and reports the real type, and the UI says
"this is actually a HEIC image" rather than failing. The HEIC signature is at
byte offset 4 (`ftyp` box), not offset 0, which is why a naive prefix table
misses it.

## Windows file locking turns a successful write into a failed one

On Unix, renaming over a file another process has open succeeds. On Windows it
fails with `ERROR_SHARING_VIOLATION` if the target is open, and a PDF the user
still has open in a viewer is the common case, since they just looked at it
before converting.

Output naming never overwrites (`report.pdf` → `report (1).pdf`), which avoids
this for most flows. Where a rename does hit a sharing violation, the error is
mapped to "That file is open in another program. Close it and try again."
rather than surfaced as an `errno`.

## Windows rejects filename characters that Unix allows, so the corpus differs

The adversarial corpus is meant to include every hostile filename a user could
plausibly produce. On Unix that includes `"`, `?`, `*`, `|` and a literal
newline. Windows refuses to create any of them: `os.Create` fails with "The
filename, directory name, or volume label syntax is incorrect", not with a
permissions error, which makes it look like a bug in the generator.

`scripts/gencorpus` therefore emits a platform-dependent corpus, and
`MANIFEST.yaml` says which entries are Unix-only. The characters that *are*
portable (spaces, `'`, `;`, `$()`, backticks, `&`, non-ASCII, emoji and very
long names) carry the argument-escaping test on every platform.

## pdfcpu's ImportImagesFile appends to an existing output

`api.ImportImagesFile(imgs, out, ...)` treats an existing `out` as a document to
append to, not as a file to replace. Running the corpus generator twice
therefore produced PDFs with exactly double the intended page count, and the
page-range tests failed in a way that pointed at the page-range parser rather
than at the generator.

The generator now clears its output directory first. Any code path that imports
images into a fixed path must either write into a fresh workspace or delete the
target beforehand.

## On Windows, soffice.exe is a launcher that neither prints nor waits

`soffice.exe --version` on Windows exits 0 and prints nothing, and
`soffice.exe --convert-to ...` returns immediately without waiting for the
conversion. Both look like success. The component probe therefore reported
LibreOffice as broken, and once that was worked around, conversions "succeeded"
with no output file.

`soffice.com` beside it is the console front-end: it prints the version and
blocks until the conversion finishes. Lathe resolves `soffice.com` on Windows
via `Component.WindowsExt`, and the default `.exe` elsewhere.

The related trap: LibreOffice exits 0 whether or not it converted anything, so
the office engine judges success by whether an output file appeared, never by
the exit code.

## LibreOffice needs explicit filter names, and PDF needs an import filter

`--convert-to docx` fails with "no export filter for … found": the bare
extension is ambiguous, and the filter has to be named
(`docx:MS Word 2007 XML`). Opening a PDF at all additionally needs
`--infilter=writer_pdf_import`, since PDF import belongs to Draw rather than to
the format sniffer.

The filter names contain spaces. They need no quoting here only because every
argument reaches the process through an argv slice, the same property that
makes a filename containing `; rm -rf ~` inert.

## Every LibreOffice job gets its own user profile

LibreOffice holds a lock on its user profile. Two conversions sharing the
default profile serialise at best and deadlock at worst, and a killed run
leaves a stale lock that hangs every later job with no visible cause.

Each job passes `-env:UserInstallation=file:///…` pointing into its own
workspace, so the profile is created fresh and removed with the workspace. The
value must be a `file://` URL, and on Windows that means forward slashes with a
leading slash before the drive letter.

## Wails v2 reports screen sizes but not screen origins

Restoring a window to its last position needs a guard: if the monitor it was on
has since been unplugged, restoring the saved coordinates leaves a running app
with no visible window, which is indistinguishable from a crash.

The obvious implementation asks the runtime for each screen's bounds and checks
for an intersection. Wails v2's `runtime.Screen` carries `Size` and
`PhysicalSize` but no `X`/`Y`, so the arrangement of a multi-monitor desktop
cannot be reconstructed: two 1920-wide screens could be side by side, stacked,
or overlapping, and the API cannot tell you which.

`internal/app` therefore does the conservative thing: it rules out positions
beyond the largest extent those sizes could possibly produce, and lets anything
else through. Wrongly re-centring a window somebody deliberately placed is the
more irritating of the two failures, so the guard errs towards trusting the
saved position.

Wails v3 exposes screen bounds properly, and this becomes an exact check when
the migration happens.
