package domains

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
)

var ErrIdentityNotFound = errors.New("identity_not_found")

type Identity struct {
	Status             string
	VerifiedForSending bool
	DkimTokens         []string
}

type Identities interface {
	Create(ctx context.Context, name string) (Identity, error)
	Get(ctx context.Context, name string) (Identity, error)
	Delete(ctx context.Context, name string) error
}

type Local struct{}

func (Local) Create(ctx context.Context, name string) (Identity, error) {
	tokens := make([]string, 3)
	for i := range tokens {
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		tokens[i] = hex.EncodeToString(b)
	}
	return Identity{Status: "SUCCESS", VerifiedForSending: true, DkimTokens: tokens}, nil
}

func (Local) Get(ctx context.Context, name string) (Identity, error) {
	return Identity{Status: "SUCCESS", VerifiedForSending: true}, nil
}

func (Local) Delete(ctx context.Context, name string) error {
	return nil
}
