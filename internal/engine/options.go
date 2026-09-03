package engine

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Options is a task's option values with typed accessors.
//
// Values arrive from three places (task defaults, the CLI as strings, and the
// UI as JSON, where every number is a float64) so every accessor coerces
// rather than type-asserting.
type Options map[string]any

// String returns a string option, or def when absent or empty.
func (o Options) String(key, def string) string {
	v, ok := o[key]
	if !ok || v == nil {
		return def
	}
	s, ok := v.(string)
	if !ok {
		s = fmt.Sprint(v)
	}
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// Int returns an integer option, accepting the float64 that JSON decoding
// produces and the string the CLI produces.
func (o Options) Int(key string, def int) int {
	switch v := o[key].(type) {
	case nil:
		return def
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// Float returns a floating-point option.
func (o Options) Float(key string, def float64) float64 {
	switch v := o[key].(type) {
	case nil:
		return def
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f
		}
	}
	return def
}

// Bool returns a boolean option, accepting the usual textual spellings.
func (o Options) Bool(key string, def bool) bool {
	switch v := o[key].(type) {
	case nil:
		return def
	case bool:
		return v
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b
		}
	case float64:
		return v != 0
	case int:
		return v != 0
	}
	return def
}

// Has reports whether an option was set to a non-empty value.
func (o Options) Has(key string) bool {
	v, ok := o[key]
	if !ok || v == nil {
		return false
	}
	if s, isStr := v.(string); isStr {
		return strings.TrimSpace(s) != ""
	}
	return true
}

// PageSet is a parsed page selection, one-based and ascending.
type PageSet []int

// Contains reports whether the set includes a page.
func (p PageSet) Contains(page int) bool {
	for _, n := range p {
		if n == page {
			return true
		}
	}
	return false
}

// Strings renders the pages as decimal strings, which is what page-oriented
// tools take on their command lines.
func (p PageSet) Strings() []string {
	out := make([]string, len(p))
	for i, n := range p {
		out[i] = strconv.Itoa(n)
	}
	return out
}

// ParsePages reads a human page selection such as "1-3, 8, 11-" against a
// document of total pages. An empty selection means every page.
//
// It is deliberately forgiving: people write ranges with spaces, en dashes and
// trailing commas, and none of that should be an error message.
func ParsePages(spec string, total int) (PageSet, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		all := make(PageSet, 0, total)
		for i := 1; i <= total; i++ {
			all = append(all, i)
		}
		return all, nil
	}
	if total < 1 {
		return nil, fmt.Errorf("the document has no pages")
	}

	// A range pasted out of a document often carries an en or em dash rather
	// than a hyphen, and refusing it would be pedantry.
	spec = strings.NewReplacer("\u2013", "-", "\u2014", "-", " to ", "-").Replace(spec)
	seen := make(map[int]bool)

	for _, part := range strings.FieldsFunc(spec, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		lo, hi, err := parseRange(part, total)
		if err != nil {
			return nil, err
		}
		for i := lo; i <= hi; i++ {
			seen[i] = true
		}
	}

	if len(seen) == 0 {
		return nil, fmt.Errorf("no pages were selected")
	}
	out := make(PageSet, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Ints(out)
	return out, nil
}

func parseRange(part string, total int) (lo, hi int, err error) {
	dash := strings.Index(part, "-")
	if dash < 0 {
		n, convErr := strconv.Atoi(strings.TrimSpace(part))
		if convErr != nil {
			return 0, 0, fmt.Errorf("%q is not a page number", part)
		}
		if n < 1 || n > total {
			return 0, 0, fmt.Errorf("page %d is outside this document, which has %d pages", n, total)
		}
		return n, n, nil
	}

	loText := strings.TrimSpace(part[:dash])
	hiText := strings.TrimSpace(part[dash+1:])

	lo, hi = 1, total
	if loText != "" {
		if lo, err = strconv.Atoi(loText); err != nil {
			return 0, 0, fmt.Errorf("%q is not a page number", loText)
		}
	}
	if hiText != "" {
		if hi, err = strconv.Atoi(hiText); err != nil {
			return 0, 0, fmt.Errorf("%q is not a page number", hiText)
		}
	}
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo < 1 {
		lo = 1
	}
	if hi > total {
		hi = total
	}
	if lo > total {
		return 0, 0, fmt.Errorf("page %s is outside this document, which has %d pages", loText, total)
	}
	return lo, hi, nil
}
