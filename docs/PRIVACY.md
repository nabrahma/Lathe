# Privacy

## The guarantee

Lathe never uploads your files. There is no server, no account, no sync and no
telemetry. Not anonymous, not aggregated, not opt-out.

You can verify this in ten seconds without reading any code: disconnect from
the internet and use the app. Everything except a one-time component download
works exactly the same.

## Everything that touches the network

Two things, both of which you start deliberately.

### 1. Downloading a component

When a task needs software Lathe does not ship, currently only FFmpeg, it
tells you what it needs and how large the download is, and waits for you to
agree. Nothing downloads in the background and nothing downloads silently.

The request is an ordinary HTTPS GET to a fixed URL compiled into the binary.
It carries a user agent of `lathe/<version>` and nothing else: no identifier,
no file names, no information about your machine.

The downloaded file is checked against a SHA-256 fingerprint compiled into the
binary. A file that does not match is deleted rather than installed.

### 2. The update check

Off unless you turn it on, and you are asked once.

When on, Lathe asks the GitHub releases API whether a newer version exists. The
request sends the app's own version number and nothing else, and Lathe never
downloads or installs an update by itself. It only tells you one exists.

Turn it off and Lathe makes no network request at all, ever, except a component
download you explicitly start.

## How this is enforced

Not by policy. By tests that fail the build.

**No package may import network code.** Every package outside `internal/deps`
and `internal/update` is forbidden from importing `net`, `net/http`,
`golang.org/x/net` or anything under them. Checked by `scripts/boundary` on
every commit, and again from the outside in
[`test/isolation`](../test/isolation).

**Conversions run with the network blocked.** The isolation suite intercepts
every outbound name lookup, fails it, counts the attempts, and asserts the
count is zero while the full conversion suite runs. A conversion that reached
for the network would fail the build.

**No analytics library exists in the dependency tree.** Not Sentry, not
PostHog, not "anonymous usage statistics". Check `go.mod` yourself.

## What Lathe stores on your machine

| What | Where | Contains |
|---|---|---|
| Settings | `<config>/Lathe/settings.json` | Your preferences. No file names, no history. |
| Components | `<config>/Lathe/components/` | Downloaded software such as FFmpeg. |
| Temporary work | System temp, under `lathe/` | Deleted when the job finishes, and swept on the next start if the app was killed. |

`<config>` is `%AppData%` on Windows, `~/Library/Application Support` on macOS
and `$XDG_CONFIG_HOME` (usually `~/.config`) on Linux.

Lathe keeps no history of what you converted. Nothing is logged to disk about
your files.

## Your original files

Never modified, under any circumstance, including a crash. Every result is
written to a temporary file beside its destination and atomically renamed into
place, and an existing file is never overwritten: `report.pdf` becomes
`report (1).pdf`.

This is enforced by [`test/integrity`](../test/integrity), which hashes every
input before and after every task, and separately kills the process at
randomised points mid-conversion to confirm both the input and the destination
survive it.

## Removing a password from a PDF

"Unlock PDF" removes a password from a document you can already open. You have
to supply the correct password. Lathe never attempts to crack, brute-force or
bypass encryption, and there is no code in the repository that could.

It is a legitimate feature, since people encrypt their own documents and later
want them plain, but the boundary is worth stating rather than leaving
ambiguous.
