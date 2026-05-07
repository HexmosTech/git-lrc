package story

import (
	"fmt"
	"sort"
)

type Registry struct {
	providers map[string]Provider
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		providerID := provider.ID()
		if _, exists := registry.providers[providerID]; exists {
			return nil, fmt.Errorf("duplicate story provider id: %s", providerID)
		}
		registry.providers[providerID] = provider
	}
	return registry, nil
}

func (r *Registry) Provider(id string) (Provider, error) {
	provider, ok := r.providers[id]
	if !ok {
		return nil, fmt.Errorf("unknown story provider: %s", id)
	}
	return provider, nil
}

func (r *Registry) ProviderIDs() []string {
	ids := make([]string, 0, len(r.providers))
	for id := range r.providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
