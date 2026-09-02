# Discoveries

Things that were not obvious until they were tried. Cross-platform desktop work
generates these constantly; writing them down is cheaper than rediscovering
them.

## Killing a process is not enough — you must kill its tree

`cmd.Process.Kill()` kills the process you spawned and orphans everything it
spawned. FFmpeg and LibreOffice both fork children, and LibreOffice in
particular leaves a `soffice.bin` running that holds a user-profile lock, so the
*next* conversion then hangs waiting for a lock held by a process the user
cannot see.

The fix is different on each platform and there is no portable API:

- **Unix** — put the child in its own process group (`Setpgid: true`) and signal
  the negative pid, so the signal reaches the whole group.
- **Windows** — there are no process groups in the Unix sense. Create a Job
  Object with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`, assign the child to it, and
  closing the job handle terminates the entire tree, including grandchildren
  spawned after assignment.

Implemented in `internal/exec/process_unix.go` and `process_windows.go`, and
tested by spawning a script that forks and asserting every descendant dies.

## Atomic rename requires the temp file to live in the destination directory

`os.Rename` is atomic only within a single filesystem. Writing to the system
temp directory and renaming into the user's Documents folder is frequently a
cross-device move, which Go implements as copy-then-delete — not atomic, and
observable as a half-written file if the process dies mid-copy.

`fsatomic.WriteFile` therefore creates its temp file as a dotfile *beside* the
destination. The fallback path, when the destination directory is not writable,
is to fail early with a clear error rather than silently degrade to a
non-atomic copy.

## Extension-based type detection is wrong often enough to matter in practice

Not a theoretical concern. The three cases seen constantly:

- iOS shares a HEIC as `.jpg` — the extension is a lie, and every naive decoder
  fails with "invalid JPEG format".
- A failed download saves an HTML error page as `.pdf`.
- A `.docx` that is a legacy OLE `.doc` renamed by hand.

`internal/detect` reads magic bytes and reports the real type, and the UI says
"this is actually a HEIC image" rather than failing. The HEIC signature is at
byte offset 4 (`ftyp` box), not offset 0, which is why a naive prefix table
misses it.

## Windows file locking turns a successful write into a failed one

On Unix, renaming over a file another process has open succeeds. On Windows it
fails with `ERROR_SHARING_VIOLATION` if the target is open — and a PDF the user
still has open in a viewer is the common case, since they just looked at it
before converting.

Output naming never overwrites (`report.pdf` → `report (1).pdf`), which avoids
this for most flows. Where a rename does hit a sharing violation, the error is
mapped to "That file is open in another program. Close it and try again."
rather than surfaced as an `errno`.
