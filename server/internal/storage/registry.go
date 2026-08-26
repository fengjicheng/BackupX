package storage

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"backupx/server/internal/apperror"
)

type Registry struct {
	mu        sync.RWMutex
	factories map[ProviderType]ProviderFactory
}

func NewRegistry(factories ...ProviderFactory) *Registry {
	registry := &Registry{factories: make(map[ProviderType]ProviderFactory)}
	for _, factory := range factories {
		registry.Register(factory)
	}
	return registry
}

func (r *Registry) Register(factory ProviderFactory) {
	if factory == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.factories == nil {
		r.factories = make(map[ProviderType]ProviderFactory)
	}
	r.factories[factory.Type()] = factory
}

func (r *Registry) Factory(providerType string) (ProviderFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.factories[providerType]
	return factory, ok
}

func (r *Registry) Types() []ProviderType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]ProviderType, 0, len(r.factories))
	for providerType := range r.factories {
		items = append(items, providerType)
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	return items
}

func (r *Registry) SensitiveFields(providerType string) []string {
	factory, ok := r.Factory(providerType)
	if !ok {
		return nil
	}
	return factory.SensitiveFields()
}

func (r *Registry) Create(ctx context.Context, providerType string, config map[string]any) (StorageProvider, error) {
	factory, ok := r.Factory(providerType)
	if !ok {
		return nil, apperror.BadRequest("STORAGE_PROVIDER_UNSUPPORTED", "不支持的存储类型", fmt.Errorf("unsupported storage provider type: %s", providerType))
	}
	if config == nil {
		config = map[string]any{}
	}
	provider, err := factory.New(ctx, config)
	if err != nil {
		return nil, apperror.BadRequest("STORAGE_TARGET_INVALID_CONFIG", "无法创建存储客户端", err)
	}
	return provider, nil
}
