package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"forkemail/internal/config"
	"forkemail/internal/ses"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	sestypes "github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sescheck:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAWS()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	awsCfg, err := ses.LoadConfig(ctx, cfg.Region)
	if err != nil {
		return err
	}
	who, err := sts.NewFromConfig(awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("credentials: %w", err)
	}
	client := sesv2.NewFromConfig(awsCfg)
	acct, err := client.GetAccount(ctx, &sesv2.GetAccountInput{})
	if err != nil {
		return fmt.Errorf("get account: %w", err)
	}
	mode := "sandbox"
	if acct.ProductionAccessEnabled {
		mode = "production"
	}
	fmt.Printf("region:                    %s\n", cfg.Region)
	fmt.Printf("account:                   %s\n", aws.ToString(who.Account))
	fmt.Printf("mode:                      %s\n", mode)
	fmt.Printf("production access enabled: %v\n", acct.ProductionAccessEnabled)
	fmt.Printf("sending enabled:           %v\n", acct.SendingEnabled)
	if q := acct.SendQuota; q != nil {
		fmt.Printf("max send rate:             %.0f/s\n", q.MaxSendRate)
		fmt.Printf("24h quota:                 %.0f sent of %.0f\n", q.SentLast24Hours, q.Max24HourSend)
	}

	fmt.Println("verified identities:")
	var next *string
	found := 0
	for {
		out, err := client.ListEmailIdentities(ctx, &sesv2.ListEmailIdentitiesInput{NextToken: next})
		if err != nil {
			fmt.Printf("  could not list identities: %v\n", err)
			found = -1
			break
		}
		for _, id := range out.EmailIdentities {
			if id.VerificationStatus == sestypes.VerificationStatusSuccess {
				found++
				fmt.Printf("  %s (%s)\n", aws.ToString(id.IdentityName), id.IdentityType)
			}
		}
		if next = out.NextToken; next == nil {
			break
		}
	}
	if found == 0 {
		fmt.Println("  none")
	}

	_, err = client.GetConfigurationSet(ctx, &sesv2.GetConfigurationSetInput{ConfigurationSetName: aws.String(cfg.ConfigurationSet)})
	var missing *sestypes.NotFoundException
	switch {
	case err == nil:
		fmt.Printf("configuration set:         %s exists\n", cfg.ConfigurationSet)
	case errors.As(err, &missing):
		fmt.Printf("configuration set:         %s MISSING (run sesbootstrap)\n", cfg.ConfigurationSet)
	default:
		fmt.Printf("configuration set:         could not check: %v\n", err)
	}

	if cfg.TopicARN == "" {
		fmt.Println("sns topic:                 SNS_TOPIC_ARN not set (run sesbootstrap)")
		return nil
	}
	if _, err := sns.NewFromConfig(awsCfg).GetTopicAttributes(ctx, &sns.GetTopicAttributesInput{TopicArn: aws.String(cfg.TopicARN)}); err != nil {
		fmt.Printf("sns topic:                 %s NOT FOUND (%v)\n", cfg.TopicARN, err)
		return nil
	}
	fmt.Printf("sns topic:                 %s exists\n", cfg.TopicARN)
	return nil
}
