package generator

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/coalaura/minicraft/internal/game"
)

type Factory func() (game.Generator, error)

var (
	registryMx sync.RWMutex
	factories  = make(map[string]Factory)
)

func Register(name string, factory Factory) error {
	if name == "" {
		return fmt.Errorf("generator name is empty")
	}

	if factory == nil {
		return fmt.Errorf("generator %q factory is nil", name)
	}

	registryMx.Lock()
	defer registryMx.Unlock()

	if _, exists := factories[name]; exists {
		return fmt.Errorf("generator %q is already registered", name)
	}

	factories[name] = factory

	return nil
}

func MustRegister(name string, factory Factory) {
	err := Register(name, factory)
	if err != nil {
		panic(err)
	}
}

func New(name string) (game.Generator, error) {
	registryMx.RLock()
	factory, exists := factories[name]
	registryMx.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown world generator %q (available: %v)", name, Names())
	}

	generated, err := factory()
	if err != nil {
		return nil, fmt.Errorf("create world generator %q: %w", name, err)
	}

	if isNil(generated) {
		return nil, fmt.Errorf("world generator %q factory returned nil", name)
	}

	return generated, nil
}

func Names() []string {
	registryMx.RLock()
	defer registryMx.RUnlock()

	names := make([]string, 0, len(factories))

	for name := range factories {
		names = append(names, name)
	}

	sort.Strings(names)

	return names
}

func isNil(generated game.Generator) bool {
	if generated == nil {
		return true
	}

	value := reflect.ValueOf(generated)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
