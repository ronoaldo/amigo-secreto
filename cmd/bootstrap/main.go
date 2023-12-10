package main

import (
	"github.com/aws/aws-lambda-go/lambda"
	amigosecreto "github.com/ronoaldo/amigo-secreto"
)

func main() {
	lambda.Start(amigosecreto.Health)
}
