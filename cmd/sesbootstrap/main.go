package main

import (
	"context"
	"encoding/json"
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
	snstypes "github.com/aws/aws-sdk-go-v2/service/sns/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const eventDestinationName = "forkemail-sns"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sesbootstrap:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadAWS()
	if err != nil {
		return err
	}
	baseURL := os.Getenv("PUBLIC_BASE_URL")
	if err := config.ValidateBaseURL(baseURL, false); err != nil {
		return err
	}
	topicName := os.Getenv("SNS_TOPIC_NAME")
	if topicName == "" {
		topicName = "forkemail-ses-events"
	}
	endpoint := baseURL + "/webhooks/ses"

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	awsCfg, err := ses.LoadConfig(ctx, cfg.Region)
	if err != nil {
		return err
	}
	who, err := sts.NewFromConfig(awsCfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("credentials: %w", err)
	}
	account := aws.ToString(who.Account)
	sesClient := sesv2.NewFromConfig(awsCfg)
	snsClient := sns.NewFromConfig(awsCfg)

	if err := ensureConfigurationSet(ctx, sesClient, cfg.ConfigurationSet); err != nil {
		return err
	}
	topic, err := snsClient.CreateTopic(ctx, &sns.CreateTopicInput{Name: aws.String(topicName)})
	if err != nil {
		return fmt.Errorf("create topic: %w", err)
	}
	topicARN := aws.ToString(topic.TopicArn)
	fmt.Println("topic ready:", topicARN)
	if err := setTopicPolicy(ctx, snsClient, topicARN, account, cfg.Region, cfg.ConfigurationSet); err != nil {
		return err
	}
	if err := ensureEventDestination(ctx, sesClient, cfg.ConfigurationSet, topicARN); err != nil {
		return err
	}
	if _, err := sesClient.PutAccountSuppressionAttributes(ctx, &sesv2.PutAccountSuppressionAttributesInput{
		SuppressedReasons: []sestypes.SuppressionListReason{sestypes.SuppressionListReasonBounce, sestypes.SuppressionListReasonComplaint},
	}); err != nil {
		return fmt.Errorf("account suppression: %w", err)
	}
	fmt.Println("account suppression enabled for BOUNCE and COMPLAINT")

	if cfg.TopicARN != topicARN {
		fmt.Printf("\nAdd these to .env, restart the server, then run sesbootstrap again to subscribe the webhook:\n")
		fmt.Printf("SES_CONFIGURATION_SET=%s\nSNS_TOPIC_ARN=%s\n", cfg.ConfigurationSet, topicARN)
		return nil
	}
	if err := subscribe(ctx, snsClient, topicARN, endpoint); err != nil {
		return err
	}
	fmt.Printf("\nSES_CONFIGURATION_SET=%s\nSNS_TOPIC_ARN=%s\n", cfg.ConfigurationSet, topicARN)
	return nil
}

func ensureConfigurationSet(ctx context.Context, client *sesv2.Client, name string) error {
	_, err := client.GetConfigurationSet(ctx, &sesv2.GetConfigurationSetInput{ConfigurationSetName: aws.String(name)})
	var missing *sestypes.NotFoundException
	if errors.As(err, &missing) {
		_, err = client.CreateConfigurationSet(ctx, &sesv2.CreateConfigurationSetInput{ConfigurationSetName: aws.String(name)})
		if err == nil {
			fmt.Println("configuration set created:", name)
		}
		return err
	}
	if err == nil {
		fmt.Println("configuration set exists:", name)
	}
	return err
}

func setTopicPolicy(ctx context.Context, client *sns.Client, topicARN, account, region, configSet string) error {
	policy, err := json.Marshal(map[string]any{
		"Version": "2012-10-17",
		"Id":      "forkemail-ses-publish",
		"Statement": []map[string]any{{
			"Sid":       "AllowSESPublish",
			"Effect":    "Allow",
			"Principal": map[string]string{"Service": "ses.amazonaws.com"},
			"Action":    "sns:Publish",
			"Resource":  topicARN,
			"Condition": map[string]any{"StringEquals": map[string]string{
				"AWS:SourceAccount": account,
				"AWS:SourceArn":     fmt.Sprintf("arn:aws:ses:%s:%s:configuration-set/%s", region, account, configSet),
			}},
		}},
	})
	if err != nil {
		return err
	}
	if _, err := client.SetTopicAttributes(ctx, &sns.SetTopicAttributesInput{
		TopicArn: aws.String(topicARN), AttributeName: aws.String("Policy"), AttributeValue: aws.String(string(policy)),
	}); err != nil {
		return fmt.Errorf("set topic policy: %w", err)
	}
	fmt.Println("topic policy allows ses.amazonaws.com from account", account)
	return nil
}

func ensureEventDestination(ctx context.Context, client *sesv2.Client, configSet, topicARN string) error {
	dest := &sestypes.EventDestinationDefinition{
		Enabled: true,
		MatchingEventTypes: []sestypes.EventType{
			sestypes.EventTypeSend, sestypes.EventTypeDelivery, sestypes.EventTypeBounce,
			sestypes.EventTypeComplaint, sestypes.EventTypeReject, sestypes.EventTypeDeliveryDelay,
		},
		SnsDestination: &sestypes.SnsDestination{TopicArn: aws.String(topicARN)},
	}
	_, err := client.CreateConfigurationSetEventDestination(ctx, &sesv2.CreateConfigurationSetEventDestinationInput{
		ConfigurationSetName: aws.String(configSet), EventDestinationName: aws.String(eventDestinationName), EventDestination: dest,
	})
	var exists *sestypes.AlreadyExistsException
	if errors.As(err, &exists) {
		_, err = client.UpdateConfigurationSetEventDestination(ctx, &sesv2.UpdateConfigurationSetEventDestinationInput{
			ConfigurationSetName: aws.String(configSet), EventDestinationName: aws.String(eventDestinationName), EventDestination: dest,
		})
	}
	if err != nil {
		return fmt.Errorf("event destination: %w", err)
	}
	fmt.Println("event destination publishes Send, Delivery, Bounce, Complaint, Reject, DeliveryDelay to the topic")
	return nil
}

func subscribe(ctx context.Context, client *sns.Client, topicARN, endpoint string) error {
	subs, err := httpsSubscriptions(ctx, client, topicARN)
	if err != nil {
		return err
	}
	confirmed := false
	for _, s := range subs {
		arn := aws.ToString(s.SubscriptionArn)
		switch {
		case aws.ToString(s.Endpoint) == endpoint:
			confirmed = confirmed || arn != "PendingConfirmation"
		case arn != "PendingConfirmation":
			if _, err := client.Unsubscribe(ctx, &sns.UnsubscribeInput{SubscriptionArn: aws.String(arn)}); err != nil {
				return fmt.Errorf("remove old subscription %s: %w", aws.ToString(s.Endpoint), err)
			}
			fmt.Println("removed old subscription:", aws.ToString(s.Endpoint))
		}
	}
	if confirmed {
		fmt.Println("webhook already subscribed and confirmed:", endpoint)
		return nil
	}
	if _, err := client.Subscribe(ctx, &sns.SubscribeInput{
		TopicArn: aws.String(topicARN), Protocol: aws.String("https"), Endpoint: aws.String(endpoint),
	}); err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}
	fmt.Println("subscribed", endpoint, "- waiting for the server to confirm")
	for i := 0; i < 15; i++ {
		time.Sleep(2 * time.Second)
		subs, err := httpsSubscriptions(ctx, client, topicARN)
		if err != nil {
			return err
		}
		for _, s := range subs {
			if aws.ToString(s.Endpoint) == endpoint && aws.ToString(s.SubscriptionArn) != "PendingConfirmation" {
				fmt.Println("subscription confirmed")
				return nil
			}
		}
	}
	fmt.Println("subscription still pending: check the server is running with this SNS_TOPIC_ARN and reachable at", endpoint)
	return nil
}

func httpsSubscriptions(ctx context.Context, client *sns.Client, topicARN string) ([]snstypes.Subscription, error) {
	var out []snstypes.Subscription
	var next *string
	for {
		page, err := client.ListSubscriptionsByTopic(ctx, &sns.ListSubscriptionsByTopicInput{TopicArn: aws.String(topicARN), NextToken: next})
		if err != nil {
			return nil, fmt.Errorf("list subscriptions: %w", err)
		}
		for _, s := range page.Subscriptions {
			if aws.ToString(s.Protocol) == "https" {
				out = append(out, s)
			}
		}
		if next = page.NextToken; next == nil {
			return out, nil
		}
	}
}
