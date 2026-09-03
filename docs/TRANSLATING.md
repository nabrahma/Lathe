# Translating Lathe

Lathe ships in English. Adding a language is one file and needs no knowledge of
Go beyond copying the shape below.

## Add a catalogue

Create `internal/i18n/lang_<code>.go`:

```go
package i18n

func init() {
	Register(Language{Code: "hi", Name: "Hindi", Endonym: "हिन्दी"}, Catalog{
		"home.search":   "आप क्या करना चाहते हैं?",
		"result.open":   "खोलें",
		"result.another": "एक और बदलें",
		// ...
	})
}
```

`Endonym` is the language written in itself. It is what a speaker of it
actually scans a list for; `Name` in English is only for people who do not
speak it.

You do not have to translate everything at once. Anything you leave out falls
back to English string by string, so a half-finished translation is usable
rather than broken.

## Find what is left

```go
i18n.Missing("hi")   // the ids not yet covered
```

The English catalogue in `i18n.go` is the source of truth, and every id is
dotted by where it appears (`task.choose`, `error.retry`), so you have some
context even without the app open in front of you.

## What to watch for

**Placeholders must survive.** `%d`, `%s` and `%%` mean the same thing in your
string as in the English one, and in the same order.

```
"task.atLeast": "At least %d files"    →  "कम से कम %d फ़ाइलें"
```

**Do not shout.** Store everything in sentence case. The interface uppercases
chrome itself, and it only does so where uppercase is legible. In scripts where
caps do not exist or read badly, the style adapts; a catalogue full of capitals
takes that choice away.

**Errors are read, not scanned.** Keep them complete sentences in plain
language. They are the strings most likely to be read by someone already
frustrated, and the one place a literal translation of a technical phrase does
real harm. If English says "This file is open in another program. Close it and
try again", the translation should be the sentence a person in that language
would actually say, not a word-for-word mapping.

**Say what the user gets.** Task descriptions are fragments without a full
stop, describing the outcome rather than the mechanism: "Make a PDF smaller",
not "Applies lossless stream optimisation".

## Non-Latin scripts

The interface uses Geist Mono for chrome and data. It has no Devanagari,
Bengali, Tamil, Arabic or CJK coverage, so those fall through to a fallback
stack declared in `frontend/src/styles/tokens.css` as `--fallback-script`.

If your script renders poorly, the fix belongs in that token rather than in
your catalogue. Add the font your platform actually ships, where `Nirmala UI`
on Windows and `Noto Sans <Script>` elsewhere are usually the right answers,
and say so in the pull request.

Right-to-left layout is not implemented yet. A catalogue for an RTL language is
still welcome and will read correctly; the layout mirroring is tracked in
[KNOWN_GAPS.md](KNOWN_GAPS.md).

## Testing it

```
go test ./internal/i18n/
```

Then run the app with the language selected and look at the four screens. The
strings that break layouts are always the long ones, German compounds and
Tamil verb forms in particular, and no test catches a button whose label has
outgrown it.
