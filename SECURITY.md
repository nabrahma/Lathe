# Security policy

## Supported versions

The latest release. Lathe is a desktop app with no server component, so there
are no older deployments to patch.

## Reporting a vulnerability

Report privately through GitHub's advisory form:

https://github.com/nabrahma/Lathe/security/advisories/new

Please do not open a public issue for a vulnerability. You should get a first
reply within seven days.

Useful things to include: the version, the platform, a file or a sequence of
steps that triggers it, and what you believe the impact is.

## What counts

Lathe makes three promises, and a way to break any of them is a security bug:

1. **Your files stay on your machine.** Any code path that sends file contents,
   file names or usage data anywhere is a vulnerability, not a feature. The
   only network access in the entire program is the component downloader in
   `internal/deps` and the opt-in update check in `internal/update`, and
   `scripts/boundary` fails the build if that changes.
2. **Your input file is never modified.** Any way to make a task write to,
   truncate or delete an input is a vulnerability.
3. **Nothing outside the chosen output folder is touched.** Path traversal
   through a crafted file name, an archive entry or a task option counts here.

Also in scope: a crafted input file that reaches an unsafe code path in a
bundled component, tampering with a downloaded component that the SHA-256
verification in `internal/deps` fails to catch, and a way to make Lathe execute
a binary it did not verify.

## What does not count

- Crashing on a malformed input file, as long as the input is unharmed and
  nothing partial is left behind. Lathe is expected to refuse bad files.
- Vulnerabilities in FFmpeg, Tesseract or LibreOffice themselves. Report those
  upstream. If Lathe pins a version with a known issue, that is worth an issue
  here.
- Unsigned Windows and Linux builds. Known and documented in
  `docs/KNOWN_GAPS.md`.
- Findings from an automated scanner with no demonstrated impact.

## Verifying a download

Every release ships `SHA256SUMS` and a CycloneDX SBOM. Check what you
downloaded before you run it:

```sh
sha256sum -c SHA256SUMS
```
