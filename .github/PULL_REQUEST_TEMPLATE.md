## What this changes

<!-- One paragraph. What was wrong or missing, and what it does now. -->

## Why

<!-- Link the issue if there is one. If the change is not obvious, say what
     you tried first and why it did not work. -->

## How it was verified

<!-- Not "it works". What did you actually run or measure? -->

- [ ] `make check` passes
- [ ] Added or updated a test that fails without this change
- [ ] Tried it on Windows / macOS / Linux (say which)

## Checklist

- [ ] Input files are still opened read-only and never modified
- [ ] Outputs go through `internal/fsatomic`, not a direct write
- [ ] Any new user-facing error is mapped in `internal/usererr`, not raw Go text
- [ ] No new network access outside `internal/deps` and `internal/update`
- [ ] Prose contains no em-dashes
