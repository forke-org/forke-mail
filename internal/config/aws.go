package config

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	regionPattern    = regexp.MustCompile(`^[a-z]{2}(-[a-z]+)+-\d$`)
	configSetPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	topicARNPattern  = regexp.MustCompile(`^arn:aws:sns:([a-z0-9-]+):\d{12}:[A-Za-z0-9_-]{1,256}$`)
)

type AWS struct {
	Region           string
	ConfigurationSet string
	TopicARN         string
}

func LoadAWS() (AWS, error) {
	var errs []error
	a := AWS{
		Region:           os.Getenv("AWS_REGION"),
		ConfigurationSet: os.Getenv("SES_CONFIGURATION_SET"),
		TopicARN:         os.Getenv("SNS_TOPIC_ARN"),
	}
	if !regionPattern.MatchString(a.Region) {
		errs = append(errs, errors.New("AWS_REGION must be set to an AWS region such as us-east-1"))
	}
	if !configSetPattern.MatchString(a.ConfigurationSet) {
		errs = append(errs, errors.New("SES_CONFIGURATION_SET must be 1-64 letters, digits, dashes or underscores"))
	}
	if a.TopicARN != "" {
		m := topicARNPattern.FindStringSubmatch(a.TopicARN)
		if m == nil || m[1] != a.Region {
			errs = append(errs, fmt.Errorf("SNS_TOPIC_ARN must be an SNS topic ARN in %s", a.Region))
		}
	}
	id, secret := os.Getenv("AWS_ACCESS_KEY_ID"), os.Getenv("AWS_SECRET_ACCESS_KEY")
	if (id == "") != (secret == "") {
		errs = append(errs, errors.New("set both AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY, or neither to use the default credential chain"))
	}
	if strings.ContainsAny(id+secret, " \t\r\n") {
		errs = append(errs, errors.New("AWS credentials must not contain whitespace"))
	}
	return a, errors.Join(errs...)
}
