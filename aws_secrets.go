package main

import (
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ssm"
)

//
// return a map of AWS secrets (from AWS System Manager Parameter Store)
//
func getAWS_Secrets() map[string]string {

	sess := session.Must(session.NewSession())
	svc := ssm.New(sess)

	secrets := fetchAWS_Secrets(svc, describeAWS_ParameterNames(svc))
	return asMap(secrets)
}

func asMap(parameters *ssm.GetParametersOutput) map[string]string {

	secrets := make(map[string]string)
	for i := 0; i < len(parameters.Parameters); i++ {
		name := *parameters.Parameters[i].Name
		name = strings.Replace(name, awsSecretsPrefixFlag, "", 1)
		secrets[name] = *parameters.Parameters[i].Value
	}
	return secrets
}

func fetchAWS_Secrets(svc *ssm.SSM, parameterNames []string) *ssm.GetParametersOutput {
	params := &ssm.GetParametersInput{
		Names:          aws.StringSlice(parameterNames),
		WithDecryption: aws.Bool(true),
	}
	resp, err := svc.GetParameters(params)

	if err != nil {
		log.Fatalf("cannot fetch AWS System Manager Parameters %s", err.Error())
	}
	return resp
}

func describeAWS_ParameterNames(svc *ssm.SSM) []string {
	criteria := &ssm.DescribeParametersInput{
		MaxResults: aws.Int64(10), // limited by API call GetParametersInput
	}
	resp, err := svc.DescribeParameters(criteria)
	if err != nil {
		log.Fatalf("cannot describe AWS Parameter Names %s", err.Error())
	}

	size := len(resp.Parameters)
	names := make([]string, size)

	for i := 0; i < size; i++ {
		names[i] = *resp.Parameters[i].Name
	}

	return names
}
