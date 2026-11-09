package relay

import (
	"context"
	"slices"
	"strings"

	"forkemail/internal/domains"
)

type Identities struct {
	Allowed []string
}

func (i Identities) Create(ctx context.Context, name string) (domains.Identity, error) {
	return i.Get(ctx, name)
}

func (i Identities) Get(ctx context.Context, name string) (domains.Identity, error) {
	name = strings.ToLower(name)
	_, domain, _ := strings.Cut(name, "@")
	if slices.Contains(i.Allowed, name) || (domain != "" && slices.Contains(i.Allowed, domain)) {
		return domains.Identity{Status: "SUCCESS", VerifiedForSending: true}, nil
	}
	return domains.Identity{Status: "PENDING"}, nil
}

func (i Identities) Delete(ctx context.Context, name string) error {
	return nil
}
