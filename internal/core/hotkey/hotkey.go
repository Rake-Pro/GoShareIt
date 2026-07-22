// Package hotkey defines the global-hotkey seam.
package hotkey

import "context"

// Manager registers global hotkeys and dispatches them to callbacks. The core
// stores the declarative bindings; a platform Manager performs registration.
type Manager interface {
	Register(id, keys string, fn func()) error
	Unregister(id string)
	Run(ctx context.Context) error
}
