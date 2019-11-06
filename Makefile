all: build deploy-dev deploy-prod
.PHONY: all

build:
	rm -f bin/*
	env GOOS=linux go build -ldflags="-s -w" -o bin/handler main.go

deploy:
	sls deploy --stage=prod