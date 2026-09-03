# Contributing

Lathe is a desktop app that converts files without uploading them. Anything
that weakens that promise is out of scope; everything else is welcome.

## Before you start

Small fixes need no discussion. Open a pull request.

For anything larger, open an issue first. Two documents will save you time:
`docs/DECISIONS.md` records why the architecture is the way it is, and
`docs/KNOWN_GAPS.md` lists what is missing on purpose. If your idea appears in
the second one, the issue is the place to argue it should not.

## Setting up

You need Go 1.25 or newer and Node 22 or newer. Everything else installs into
the project folder and nothing lands on your system:

```sh
make tools   # wails, golangci-lint and friends into ./.tools
make deps    # go modules and frontend packages
```

Then:

```sh
make dev     # run the desktop app with live reload
make build   # produce build/bin
make check   # fmt, vet, boundary, test
```

`make check` is what CI runs. Run it before you push.

## Rules the CI enforces

These are not style preferences. Each one is checked by a script and will fail
your build.

**Import boundaries.** `scripts/boundary` refuses a network import outside
`internal/deps` and `internal/update`, and a Wails import outside
`internal/app` and the root package. The first keeps the offline guarantee
checkable rather than merely claimed; the second keeps the UI framework
replaceable.

**Input files are never opened for writing.** `test/integrity` hashes inputs
before and after every task and kills jobs at random points to prove a crash
leaves both the original and the destination untouched. This is the one
unforgivable bug.

**Outputs are published atomically.** Write through `internal/fsatomic`. Never
write directly to a path in the user's folder.

**Errors reach the user in plain language.** A raw Go error string in the UI is
a bug. Map it in `internal/usererr` with a cause, a consequence and something
the person can actually do.

**No network at runtime.** `test/isolation` runs conversions with DNS blocked.

**Bundle rules.** `frontend/scripts/check-bundle.mjs` rejects CDN hosts, HTML
file inputs, `box-shadow` and a handful of other things the design system does
not use.

## Style

Go code is `gofmt`ed and passes `golangci-lint` with the repository config.
Comments are sparse and short: explain why something is unusual, not what the
next line does. If a comment restates the code, delete it.

Prose in the README and in `docs/` uses no em-dashes.

Commit messages follow Conventional Commits, so `feat:`, `fix:`, `docs:`,
`test:`, `chore:`, with an optional scope. The subject line is a sentence, not
a label.

## Adding a task

`docs/ADDING_A_TASK.md` walks through it. A task is a registry entry, an engine
method and a row in the test matrix. If you find yourself touching the UI to
add one, something has gone wrong.

## Working with the GitHub MCP server

The repository ships a `.mcp.json` pointing at GitHub's hosted MCP server, so
an agent working in this checkout can read issues, pull requests and releases
without a local install. Export a token before you start the session:

```sh
export GITHUB_PERSONAL_ACCESS_TOKEN=ghp_your_token
```

The token is read from the environment and never written into the repository.
If you would rather run the server locally, Docker works too:

```sh
docker run -i --rm -e GITHUB_PERSONAL_ACCESS_TOKEN ghcr.io/github/github-mcp-server
```

## Translations

`docs/TRANSLATING.md`. A new language is a JSON file and a line in the
registry, and you do not need to build the app to contribute one.

## Licence

Contributions are accepted under AGPL-3.0, the same licence as the project.
