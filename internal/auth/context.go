// Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
// Source-Available License (Non-Commercial / Fair Source).
// Open for inspection, learning, and non-commercial development.
// Commercial use, hosting, or resale without authorization is strictly prohibited.

package auth

import "context"

type Identity struct {
	UserID      string
	KeyID       string
	KeyPrefix   string
	Scope       string
	Role        string
	WorkspaceID string
	Plan        string
}

func (i Identity) Session() bool {
	return i.KeyID == "" && i.UserID != ""
}

type identityKey struct{}

func withIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, id)
}

func FromContext(ctx context.Context) Identity {
	id, _ := ctx.Value(identityKey{}).(Identity)
	return id
}
