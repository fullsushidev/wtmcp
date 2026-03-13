package plugin

// NewManagerForTest creates a Manager with no dependencies for use
// in tests that need to populate manifests and handles directly.
func NewManagerForTest() *Manager {
	return &Manager{
		handles:   make(map[string]*Handle),
		manifests: make(map[string]*Manifest),
	}
}

// SetManifest sets a manifest in the manager for testing.
func (m *Manager) SetManifest(name string, manifest *Manifest) {
	m.manifests[name] = manifest
}

// SetHandle marks a plugin as loaded by creating a placeholder handle.
func (m *Manager) SetHandle(name string) {
	m.handles[name] = &Handle{}
}
