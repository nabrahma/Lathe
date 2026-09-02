package officeengine

import (
	"errors"

	lexec "github.com/nabrahma/lathe/internal/exec"
)

func asExit(err error, target **lexec.ExitError) bool {
	return errors.As(err, target)
}
