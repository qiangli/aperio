package benchmark

import (
	"fmt"
	"sync"
)

// AdapterConstructor creates an Adapter from a spec.
type AdapterConstructor func(spec *AdapterSpec) (Adapter, error)

var (
	registryMu   sync.RWMutex
	adapterRegistry = make(map[string]AdapterConstructor)
)

// RegisterAdapter registers an adapter constructor by name.
func RegisterAdapter(name string, ctor AdapterConstructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	adapterRegistry[name] = ctor
}

// GetAdapter looks up and constructs an adapter by name.
func GetAdapter(spec *AdapterSpec) (Adapter, error) {
	registryMu.RLock()
	ctor, ok := adapterRegistry[spec.Name]
	registryMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown adapter: %q (registered: %v)", spec.Name, registeredAdapterNames())
	}
	return ctor(spec)
}

// RegisteredAdapters returns the names of all registered adapters.
func RegisteredAdapters() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registeredAdapterNames()
}

func registeredAdapterNames() []string {
	names := make([]string, 0, len(adapterRegistry))
	for name := range adapterRegistry {
		names = append(names, name)
	}
	return names
}
