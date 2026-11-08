package ses

import (
	"context"
	"errors"

	"forkemail/internal/domains"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

type identityAPI interface {
	CreateEmailIdentity(ctx context.Context, in *sesv2.CreateEmailIdentityInput, optFns ...func(*sesv2.Options)) (*sesv2.CreateEmailIdentityOutput, error)
	GetEmailIdentity(ctx context.Context, in *sesv2.GetEmailIdentityInput, optFns ...func(*sesv2.Options)) (*sesv2.GetEmailIdentityOutput, error)
	DeleteEmailIdentity(ctx context.Context, in *sesv2.DeleteEmailIdentityInput, optFns ...func(*sesv2.Options)) (*sesv2.DeleteEmailIdentityOutput, error)
}

type Identities struct {
	Client identityAPI
}

func (s Identities) Create(ctx context.Context, name string) (domains.Identity, error) {
	out, err := s.Client.CreateEmailIdentity(ctx, &sesv2.CreateEmailIdentityInput{EmailIdentity: aws.String(name)})
	var exists *types.AlreadyExistsException
	if errors.As(err, &exists) {
		return s.Get(ctx, name)
	}
	if err != nil {
		return domains.Identity{}, err
	}
	id := domains.Identity{Status: "PENDING", VerifiedForSending: out.VerifiedForSendingStatus}
	if out.DkimAttributes != nil {
		id.DkimTokens = out.DkimAttributes.Tokens
	}
	return id, nil
}

func (s Identities) Get(ctx context.Context, name string) (domains.Identity, error) {
	out, err := s.Client.GetEmailIdentity(ctx, &sesv2.GetEmailIdentityInput{EmailIdentity: aws.String(name)})
	var missing *types.NotFoundException
	if errors.As(err, &missing) {
		return domains.Identity{}, domains.ErrIdentityNotFound
	}
	if err != nil {
		return domains.Identity{}, err
	}
	id := domains.Identity{Status: string(out.VerificationStatus), VerifiedForSending: out.VerifiedForSendingStatus}
	if out.DkimAttributes != nil {
		id.DkimTokens = out.DkimAttributes.Tokens
	}
	return id, nil
}

func (s Identities) Delete(ctx context.Context, name string) error {
	_, err := s.Client.DeleteEmailIdentity(ctx, &sesv2.DeleteEmailIdentityInput{EmailIdentity: aws.String(name)})
	var missing *types.NotFoundException
	if errors.As(err, &missing) {
		return nil
	}
	return err
}
