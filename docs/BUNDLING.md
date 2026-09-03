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
| **Text recognition** | Tesseract and its language data | detected | First OCR task |
| **Video and photos** | FFmpeg | 111 MB | First video task, or first HEIC |
| **Office documents** | LibreOffice | detected | First Word or Excel task |

Fifteen of the thirty tasks — every PDF and image operation — need nothing
beyond the core download. That is deliberate: the tasks people search for most
often are the ones that work the instant the app opens.

## Why some components are downloaded and others are detected

This is the part that diverges from the obvious design, and the reasoning
matters.

**Every download is checksum-verified against a SHA-256 compiled into the
binary.** A file that does not match is deleted, not used. This is not
belt-and-braces: Lathe is writing an executable onto someone else's machine,
and that is exactly the supply chain an attacker wants.

That rule then decides which components can be downloaded at all:

- **FFmpeg publishes portable, checksummed static builds for all three
  platforms.** So Lathe downloads it, verifies it, and installs it atomically.
- **Tesseract and LibreOffice ship platform installers, not portable archives.**
  There is no archive of LibreOffice with a published SHA-256 that Lathe could
  unpack into a private directory. The options were to bundle an unverified
  binary from a third party, which would make the checksum rule theatre, or to
  detect an existing installation and say plainly what to install when there is
  none. Lathe does the second.

If that changes upstream, these become downloads like FFmpeg; the machinery is
already there and only the manifest entry needs updating.

A component already on the machine is always preferred. Someone who has FFmpeg
installed is never asked to download a second copy of it.

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
nodes are skipped entirely — none of the components need them, and each is a
way out of the sandbox.

**Space is checked first.** Free space is compared against the download plus
the unpacked size plus a margin, and a shortfall produces a clear message
rather than a full disk.

**One installer per component at a time.** A lock per component, so two windows
asking for the same thing do not race into the same directory.

## The prompt

When someone clicks a task that needs something they do not have, they see what
it is, what it costs, and that it is one-time — before they commit:

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

That failure is safe by construction — a mismatch refuses the install with a
clear message rather than trusting the file — but it needs refreshing. The
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
what choosing Wails over Electron bought — there is no bundled browser, only
the WebView2 runtime the operating system already has.
