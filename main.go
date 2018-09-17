package main

import (
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/awslabs/aws-lambda-go-api-proxy/handlerfunc"
	"github.com/ruudk/github-review-label-bot/bot"
	"os"
	"strconv"
	"strings"
)

var b *bot.GithubApp

func init() {
	integrationId, err := strconv.Atoi(os.Getenv("GITHUB_INTEGRATION_ID"))
	if err != nil {
		panic(err)
	}

	webhookSecret := []byte(os.Getenv("GITHUB_WEBHOOK_SECRET"))
	privateKey := []byte(strings.Replace(os.Getenv("GITHUB_PRIVATE_KEY"), "*", "\n", -1))

	b = bot.New(integrationId, webhookSecret, privateKey)
}

func main() {
	lambda.Start(func (req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
		adapter := handlerfunc.New(b.HandlerFunc)

		return adapter.Proxy(req)
	})
}