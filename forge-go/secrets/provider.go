package secrets

import (
	"context"
	"errors"
)

var ErrSecretNotFound = errors.New("secret not found")

type SecretProvider interface {
	Resolve(ctx context.Context, key string) (string, error)
}
