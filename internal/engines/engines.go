// Package engines wires the engine adapters together.
//
// It exists so that cmd/ and internal/app both build the same set without
// either of them importing every adapter directly, and so an adapter can be
// added in exactly one place.
package engines

import (
	"github.com/nabrahma/lathe/internal/deps"
	"github.com/nabrahma/lathe/internal/engine"
	"github.com/nabrahma/lathe/internal/engine/ffmpegengine"
	"github.com/nabrahma/lathe/internal/engine/imageengine"
	"github.com/nabrahma/lathe/internal/engine/ocrengine"
	"github.com/nabrahma/lathe/internal/engine/officeengine"
	"github.com/nabrahma/lathe/internal/engine/pdfengine"
)

// Default returns every engine, wired to the given component manager.
func Default(m deps.Manager) *engine.Registry {
	return engine.NewRegistry(
		pdfengine.New().WithComponents(m),
		imageengine.New(),
		ocrengine.New(m),
		ffmpegengine.New(m),
		officeengine.New(m),
	)
}
