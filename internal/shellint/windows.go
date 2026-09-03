//go:build windows

package shellint

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Everything is written under HKEY_CURRENT_USER, so no elevation is needed and
// nothing is changed for other people who share the machine.
const (
	shellKey   = `Software\Classes\*\shell\LatheConvert`
	commandKey = shellKey + `\command`
)

type windowsIntegrator struct{}

func (w *windowsIntegrator) Status() Status {
	k, err := registry.OpenKey(registry.CURRENT_USER, commandKey, registry.QUERY_VALUE)
	if err != nil {
		return Status{Supported: true, Installed: false}
	}
	defer func() { _ = k.Close() }()

	command, _, err := k.GetStringValue("")
	if err != nil {
		return Status{Supported: true, Installed: false}
	}
	return Status{Supported: true, Installed: true, Detail: command}
}

func (w *windowsIntegrator) Install(executable string) error {
	if strings.TrimSpace(executable) == "" {
		return fmt.Errorf("no application path to register")
	}

	k, _, err := registry.CreateKey(registry.CURRENT_USER, shellKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("add the menu entry: %w", err)
	}
	defer func() { _ = k.Close() }()

	if err := k.SetStringValue("", MenuLabel); err != nil {
		return fmt.Errorf("name the menu entry: %w", err)
	}
	// Explorer reads the icon from the executable itself.
	if err := k.SetStringValue("Icon", executable); err != nil {
		return fmt.Errorf("set the menu icon: %w", err)
	}

	c, _, err := registry.CreateKey(registry.CURRENT_USER, commandKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("add the menu command: %w", err)
	}
	defer func() { _ = c.Close() }()

	// %1 is quoted so a path containing spaces arrives as one argument.
	return c.SetStringValue("", fmt.Sprintf(`"%s" "%%1"`, executable))
}

func (w *windowsIntegrator) Remove() error {
	// The command subkey has to go first: a key with children cannot be
	// deleted.
	_ = registry.DeleteKey(registry.CURRENT_USER, commandKey)
	if err := registry.DeleteKey(registry.CURRENT_USER, shellKey); err != nil {
		return fmt.Errorf("remove the menu entry: %w", err)
	}
	return nil
}

// linuxIntegrator is unreachable on Windows but keeps New's switch total.
type linuxIntegrator struct{ unsupported }
