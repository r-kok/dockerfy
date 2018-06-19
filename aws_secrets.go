package main

import (
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
	"github.com/aws/aws-sdk-go/service/ssm/ssmiface"
)

//
// return a map of AWS secrets (from AWS System Manager Parameter Store)
//
// If fetchAWSSecrets succeeds, returns a map of ENVIRONMENT variables with secrets overwritten from ssm.
// Otherwise, returns a map of ENVIRONMENT variables only.
//

type Secret struct {
	Value string
	Key   string
}

func getAWSSecrets() map[string]string {
	var svc ssmiface.SSMAPI

	secrets := GetEnvMap()

	sess := session.Must(session.NewSession())
	svc = ssm.New(sess)

	prefix := string_template_eval(awsSecretsPrefixFlag)
	rawSecrets, err := fetchAWSSecrets(prefix, svc)
	if err != nil {
		log.Printf("Cannot fetch parameters from AWS Parameter store: %s", err.Error())
		return secrets
	}

	for _, rawSecret := range rawSecrets {
		key := strings.Replace(rawSecret.Key, prefix, "", 1)
		secrets[key] = rawSecret.Value
	}

	return secrets
}

func fetchAWSSecrets(prefix string, svc ssmiface.SSMAPI) ([]*Secret, error) {
	secrets := []*Secret{}

	var nextToken *string
	for {
		getParametersByPathInput := &ssm.GetParametersByPathInput{
			MaxResults:     aws.Int64(10),
			NextToken:      nextToken,
			Path:           aws.String(prefix),
			WithDecryption: aws.Bool(true),
		}

		resp, err := svc.GetParametersByPath(getParametersByPathInput)
		if err != nil {
			return nil, err
		}

		for _, param := range resp.Parameters {
			secret := &Secret{
				Value: *param.Value,
				Key:   *param.Name,
			}
			secrets = append(secrets, secret)
		}

		if resp.NextToken == nil {
			break
		}

		nextToken = resp.NextToken
	}

	return secrets, nil
}
