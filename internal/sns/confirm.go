package sns

import (
	"context"
	"errors"
)

var ErrUntrustedURL = errors.New("subscribe url is not an sns endpoint")

func (v *Verifier) Confirm(ctx context.Context, m Message) error {
	u, ok := v.TrustedURL(m.SubscribeURL)
	if !ok || m.Type != TypeSubscriptionConfirmation {
		return ErrUntrustedURL
	}
	_, err := v.get(ctx, u.String(), 64<<10)
	return err
}
