package ocrengine

import (
	"errors"
	"regexp"
	"sort"
	"strconv"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	lexec "github.com/nabrahma/lathe/internal/exec"
)

// pdfConfiguration aliases pdfcpu's config so the OCR engine's signatures stay
// readable.
type pdfConfiguration = model.Configuration

func pdfConf(password string) *pdfConfiguration {
	c := model.NewDefaultConfiguration()
	c.ValidationMode = model.ValidationRelaxed
	if password != "" {
		c.UserPW = password
		c.OwnerPW = password
	}
	return c
}

func asExit(err error, target **lexec.ExitError) bool {
	return errors.As(err, target)
}

var digits = regexp.MustCompile(`\d+`)

// sortPages orders page files numerically, so page 10 follows page 9 rather
// than page 1.
func sortPages(names []string) {
	sort.Slice(names, func(i, j int) bool {
		a, b := lastNumber(names[i]), lastNumber(names[j])
		if a != b {
			return a < b
		}
		return names[i] < names[j]
	})
}

func lastNumber(s string) int {
	found := digits.FindAllString(s, -1)
	if len(found) == 0 {
		return 0
	}
	n, err := strconv.Atoi(found[len(found)-1])
	if err != nil {
		return 0
	}
	return n
}
