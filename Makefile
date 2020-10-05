check-env:
ifndef GITHUB_APP_ID
	$(error GITHUB_APP_ID is undefined -> Run `direnv allow`)
endif
ifndef GITHUB_PRIVATE_KEY
	$(error GITHUB_PRIVATE_KEY is undefined -> Run `direnv allow`)
endif
ifndef GITHUB_WEBHOOK_SECRET
	$(error GITHUB_WEBHOOK_SECRET is undefined -> Run `direnv allow`)
endif

build: check-env
	rm -f bin/*
	env GOOS=linux go build -ldflags="-s -w" -o bin/handler main.go

deploy: check-env
	sls deploy --stage=prod