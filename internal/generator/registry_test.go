package generator

import (
	"errors"
	"testing"

	"github.com/coalaura/minicraft/internal/game"
)

type testGenerator struct{}

type pointerGenerator struct{}

func (testGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.Air
}

func (*pointerGenerator) BlockAt(_ int64, _ game.BlockPosition) game.Block {
	return game.Air
}

func TestRegisterAndCreate(t *testing.T) {
	const name = "registry-test-valid"

	err := Register(name, func() (game.Generator, error) {
		return testGenerator{}, nil
	})

	if err != nil {
		t.Fatalf("register generator: %v", err)
	}

	generated, err := New(name)
	if err != nil {
		t.Fatalf("create generator: %v", err)
	}

	if _, ok := generated.(testGenerator); !ok {
		t.Fatalf("generator type = %T", generated)
	}

	err = Register(name, func() (game.Generator, error) {
		return testGenerator{}, nil
	})

	if err == nil {
		t.Fatal("duplicate generator registration succeeded")
	}
}

func TestNewRejectsFactoryFailure(t *testing.T) {
	tests := map[string]Factory{
		"registry-test-error": func() (game.Generator, error) {
			return nil, errors.New("factory failed")
		},
		"registry-test-nil": func() (game.Generator, error) {
			return nil, nil
		},
		"registry-test-typed-nil": func() (game.Generator, error) {
			return (*pointerGenerator)(nil), nil
		},
	}

	for name, factory := range tests {
		err := Register(name, factory)
		if err != nil {
			t.Fatalf("register %s: %v", name, err)
		}

		_, err = New(name)
		if err == nil {
			t.Fatalf("create %s succeeded", name)
		}
	}
}

func TestNewRejectsUnknownGenerator(t *testing.T) {
	_, err := New("registry-test-unknown")
	if err == nil {
		t.Fatal("create unknown generator succeeded")
	}
}
