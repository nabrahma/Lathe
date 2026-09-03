# Adding a task

A task is one entry in a registry and one case in an engine. Nothing else
changes: the home screen, the search, the drag-to-filter, the CLI subcommand
and the job queue all pick it up from the registry.

This is on purpose. A converter project grows by people adding the conversion
they personally needed, and that should take an evening, not a weekend.

## 1. Describe it

Add an entry to the right group in `internal/task/catalog.go`.

```go
{
    ID: "pdf.extract-pages", Name: "Extract pages", Description: "Pull certain pages into a new PDF",
    Category: CategoryPDF, Icon: "split", Verb: "Extract",
    Accepts: pdfIn, MinInputs: 1, MaxInputs: 1,
    Engine: EnginePDF, RequiredTier: TierCore,
    Options: []Option{
        {ID: "pages", Label: "Pages", Type: OptionPageRange, Default: "", Placeholder: "e.g. 1-3, 8"},
    },
},
```

The fields that need thought:

- **`Description`** is one fragment, sentence case, no full stop. It goes under
  the name on the card. Say what the person gets, not how it works.
- **`Verb`** is the primary button. `Extract`, not `Submit`. Store it in
  sentence case; the interface uppercases chrome, so other locales can leave it
  alone.
- **`Accepts`** decides which tasks appear when someone drags a file onto the
  home screen. Get it right and the categories stop mattering.
- **`RequiredTier`** decides whether the card wears a `+111 MB` badge, and
  whether the pipeline prompts before running.

## 2. The interface rules, which are enforced

`task.Validate` runs at registry construction, so breaking one of these fails
at startup and in the tests rather than in front of a user.

- **At most three options outside Advanced.** Every extra control on a task
  screen makes the app harder for the people it is aimed at. Default well and
  hide the rest behind `Advanced: true`.
- **At most four choices in a primary Choice option.** Longer lists belong in
  Advanced.
- **No jargon in a primary label.** Not codec, bitrate, DPI, colorspace or
  container. Those words are welcome in Advanced, where the people who want
  them will look.
- **Every option needs a default**, except a password. A task screen has to be
  usable with nothing changed.

`TestEveryTaskRespectsTheInterfaceRules` checks all of this, plus that a
description is a fragment and a verb is not stored shouting.

## 3. Implement it

Add a case to the engine's `Execute` switch. The contract is short:

- Read inputs. **Never write to one**, for any reason, including tasks that
  feel in-place like rotate.
- Write every output into `req.Workspace`, using `UniqueName` so a batch where
  three inputs share a stem produces three results rather than one.
- Report progress with a real fraction where the engine knows one, and
  `engine.Indeterminate("stage")` where it genuinely does not. Never invent a
  percentage.
- Return errors as they come; the pipeline translates them. Raise a
  `usererr.Error` yourself only when you can say something more specific than
  the generic mapping would.

```go
func extractPages(req engine.Request, progress func(engine.Progress)) (*engine.Response, error) {
    in := req.Inputs[0]

    total, err := pageCount(in, req.Options.String("password", ""))
    if err != nil {
        return nil, err
    }
    pages, err := engine.ParsePages(req.Options.String("pages", ""), total)
    if err != nil {
        return nil, usererr.Wrap(err, usererr.CodeInvalidOptions,
            capitalise(err.Error())+".", usererr.ActionChangeOption)
    }

    progress(engine.Progress{Fraction: -1, Stage: fmt.Sprintf("Extracting %d pages", len(pages))})

    out := req.Workspace.UniqueName(outputName(in, "-extracted"))
    if err := api.CollectFile(in, out, pages.Strings(), conf("")); err != nil {
        return nil, err
    }
    return &engine.Response{Outputs: []string{out}}, nil
}
```

## 4. Test it

Add a case to `test/matrix/matrix_test.go` — the coverage guard fails the build
without one. Add both shapes:

```go
{"extract a range", "pdf.extract-pages", []string{pdf("five-page.pdf")},
    map[string]any{"pages": "2-4"}, wantSuccess},
{"extract past the end", "pdf.extract-pages", []string{pdf("five-page.pdf")},
    map[string]any{"pages": "40-50"}, wantMappedError},
```

The matrix asserts the input hash is unchanged and that any failure is a mapped
error with a next action, so those two lines cover more than they look like.

If the task is worth protecting against a crash, add it to `cases()` in
`test/integrity` as well.

```
make check
go test ./test/matrix/ -run TestConversionMatrix -v
```

## 5. What you get for free

- A card on the home screen, in the right group
- A place in the search results and in drag-to-filter
- `lathe pdf-extract-pages input.pdf --pages 2-4 -o out.pdf`, with generated
  `--help`
- Queueing, cancellation, progress reporting and native notifications
- Atomic writes, collision-safe naming and error translation

## Adding a whole engine

Rarer, but no harder. Implement `engine.Engine` — `ID`, `Available`,
`Execute` — in `internal/engine/<name>engine`, and add it to
`internal/engines/engines.go`.

Two rules apply to any engine that shells out:

1. **Spawn through `internal/exec` and nothing else.** It is the audited
   chokepoint for argument handling, timeouts, output capping and killing a
   process tree. `os/exec` anywhere else fails the boundary check in CI.
2. **Translate the tool's errors.** Add its distinctive phrases to the table in
   `internal/usererr`. The matrix test will fail if a library name, an exit
   code or an uppercase message reaches a user.

If the engine needs software Lathe does not ship, add a `Component` to
`internal/deps/manifest.go` — with a real SHA-256, since the installer refuses
anything else. See [BUNDLING.md](BUNDLING.md).
