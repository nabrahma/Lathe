# Bundling

This is the hardest problem in the project and the one that decides whether it
succeeds. Everything else is a matter of writing code carefully; this one is a
constraint you cannot code your way out of.

## The constraint

People abandon downloads. A 500 MB installer for "a file converter" loses most
of its audience before it starts. But LibreOffice genuinely is several hundred
megabytes, and there is no way around that if you want Word conversion.

The resolution is tiering: **install small, grow on demand, and be honest about
it.**

## What ships, and what does not

| Tier | Contents | Size | When |
|---|---|---:|---|
| **Core** | pdfcpu, the Go image codecs, the app | 18.1 MB | Always, in the download |
| **Text recognition** | Tesseract and its language data | 50 MB on Windows, detected elsewhere | First OCR task |
| **Video and photos** | FFmpeg | 111 MB | First video task, or first HEIC |
| **Office documents** | LibreOffice | detected | First Word or Excel task |
| **Stronger PDF compression** | Ghostscript | 65 MB on Windows, detected elsewhere | Never required |

Fifteen of the thirty tasks, every PDF and image operation, need nothing
beyond the core download. That is deliberate: the tasks people search for most
often are the ones that work the instant the app opens.

## Why some components are downloaded and others are detected

This is the part that diverges from the obvious design, and the reasoning
matters.

**Every download is checksum-verified against a SHA-256 compiled into the
binary.** A file that does not match is deleted, not used. This is not
belt-and-braces: Lathe is writing an executable onto someone else's machine,
and that is exactly the supply chain an attacker wants.

That rule constrains *what* can be fetched, not whether it has to be an
archive. Three shapes exist:

- **A portable archive.** FFmpeg publishes checksummed static builds for all
  three platforms, so Lathe downloads one, verifies it and installs it
  atomically into its own directory.
- **The publisher's own installer.** On Windows, Tesseract and Ghostscript ship
  a setup program and nothing portable. Lathe pins the exact setup file by
  SHA-256, exactly as it does an archive, and runs it into its own component
  folder rather than into the system. What runs is byte-for-byte the file the
  checksum was taken from, and removing the component is deleting a directory.
- **Neither.** LibreOffice everywhere, and Tesseract and Ghostscript on macOS
  and Linux, come from a package manager. Installing through one needs a root
  password, which Lathe has no business asking for, so it detects an existing
  installation and says exactly what to run when there is none.

Which shape applies is a property of the platform rather than of the project,
so it lives on the per-platform `Source` and not on the component.

### Running an installer, and the permission prompt

Both Windows installers ask for administrator rights in their manifests:
Tesseract requests the highest the account holds, Ghostscript requires
administrator outright. Two consequences fall out of that, and both are
load-bearing.

`os/exec` cannot start them at all. It calls `CreateProcess`, which refuses a
program that requests elevation with `ERROR_ELEVATION_REQUIRED` and never
offers the user a choice. Raising the prompt needs `ShellExecuteEx` with the
`runas` verb, which is why `installer_windows.go` exists rather than a
two-line `exec.Command`.

The prompt is then announced before it appears. An unexplained Windows
permission dialog, raised moments after someone pressed a download button, is
indistinguishable from the thing they have been taught to refuse, and the safe
response to that is No. The settings screen says Windows will ask, the button
reads "Download and install", and declining is treated as a decision rather
than an error: the component is simply not installed, and the message says how
to change your mind.

NSIS takes the destination as `/D=`, which has two rules that are easy to
break silently. It must be the final argument, which
`TestTheDestinationIsTheFinalInstallerArgument` enforces over the manifest, and
it must not be quoted even when the path contains a space, which is why the
arguments are passed as one string rather than through `os/exec` quoting. An
account named with a space would otherwise install into a directory whose name
begins with a quotation mark.

If a publisher starts shipping a portable archive, the component becomes a
download like FFmpeg and only the manifest entry changes.

A component already on the machine is always preferred. Someone who has FFmpeg
installed is never asked to download a second copy of it.

## Components that gate nothing

Ghostscript is the one entry that no task requires. It sits in its own tier so
that nothing ever waits on it: Compress PDF runs on the built-in path, and when
Ghostscript happens to be installed the same job also goes through it and the
smaller of the two results is kept.

That shape exists because Compress PDF is a core task. Making the most used
feature in the app depend on an external tool would be a worse product than
compressing less far, and prompting for a download in front of it would be
worse still. The cost is that results are machine-dependent, which
`KNOWN_GAPS.md` states plainly.

## What the manager guarantees

Each of these exists because of a specific failure people actually hit.

**Checksum verification is mandatory.** Not optional, not advisory. A component
with no published checksum is refused rather than installed.

**Downloads resume.** HTTP range requests, with the partial file deliberately
kept when a download is interrupted. Nobody restarts a 400 MB download from
zero; they quit.

**Installs are atomic.** Download to a staging directory, verify, extract, then
rename into place in one move. A partially extracted component must never be
visible as installed.

**Availability is proved by running the binary, not by checking it exists.** A
truncated download passes a file-exists check and then fails mysteriously in
the middle of someone's job.

**Archive extraction refuses to escape its destination.** Any entry whose path
would land outside the target directory is skipped, and symlinks and device
nodes are skipped entirely, because none of the components need them and each
is a way out of the sandbox.

**Space is checked first.** Free space is compared against the download plus
the unpacked size plus a margin, and a shortfall produces a clear message
rather than a full disk.

**One installer per component at a time.** A lock per component, so two windows
asking for the same thing do not race into the same directory.

## The prompt

When someone clicks a task that needs something they do not have, they see what
it is, what it costs, and that it is one-time, before they commit:

```
┌──────────────────────────────────────────────────┐
│  Video conversion needs a one-time download      │
│                                                  │
│  Converting video uses FFmpeg, a free            │
│  open-source media toolkit. It downloads once    │
│  and works offline afterwards.                   │
│                                                  │
│  Download size: 111 MB                           │
│  Disk space needed: 411 MB                       │
│                                                  │
│         [ Not now ]      [ Download ]            │
└──────────────────────────────────────────────────┘
```

Then a progress bar with a real percentage and a cancel button. Never a spinner
with no explanation, and never a silent background download.

The home screen is honest before the click too: a task needing a download wears
a `+111 MB` badge on its card.

## Keeping the manifest current

`internal/deps/manifest.go` holds every URL and checksum. Two are pinned to a
versioned URL and will not move. The Linux FFmpeg build is the exception: its
publisher offers only a rolling "latest release" URL with no versioned archive,
so its checksum goes stale whenever upstream publishes.

That failure is safe by construction, since a mismatch refuses the install
with a clear message rather than trusting the file, but it needs refreshing. The
`Rolling` flag on the source marks which entries need it.

## Size budget

| Platform | Target | Hard limit |
|---|---:|---:|
| Windows | 55 MB | 70 MB |
| macOS | 65 MB | 85 MB |
| Linux | 60 MB | 75 MB |

`scripts/size-budget.sh` fails CI when an artefact exceeds its limit. Size
regressions creep in silently and then cannot be undone, so the gate is
automatic rather than a thing somebody remembers to check.

Current Windows build: **18.1 MB**, well inside the target. That headroom is
what choosing Wails over Electron bought: there is no bundled browser, only
the WebView2 runtime the operating system already has.
