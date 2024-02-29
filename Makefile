export DOCKER_BUILDKIT=1
LDFLAG_LOCATION := github.com/metal-toolbox/flasher/internal/version
GIT_COMMIT  := $(shell git rev-parse --short HEAD)
GIT_BRANCH  := $(shell git symbolic-ref -q --short HEAD)
GIT_SUMMARY := $(shell git describe --tags --dirty --always)
VERSION     := $(shell git describe --tags 2> /dev/null)
BUILD_DATE  := $(shell date +%s)
GIT_COMMIT_FULL  := $(shell git rev-parse HEAD)
GO_VERSION := $(shell expr `go version |cut -d ' ' -f3 |cut -d. -f2` \>= 16)
DOCKER_IMAGE  := "ghcr.io/metal-toolbox/flasher"
REPO := "https://github.com/metal-toolbox/flasher.git"

.DEFAULT_GOAL := help

## lint
lint: gen-mock
	golangci-lint run --config .golangci.yml

## Go test
test: lint
	CGO_ENABLED=0 go test -timeout 1m -v -covermode=atomic ./...

## Generate mocks
gen-mock:
	go get go.uber.org/mock@v0.4.0
	go install go.uber.org/mock/mockgen@v0.4.0
	go generate ./...
	go mod tidy

## build-osx
build-osx:
ifeq ($(GO_VERSION), 0)
	$(error build requies go version 1.17.n or higher)
endif
	  go build -o flasher \
	   -ldflags \
		"-X $(LDFLAG_LOCATION).GitCommit=$(GIT_COMMIT) \
         -X $(LDFLAG_LOCATION).GitBranch=$(GIT_BRANCH) \
         -X $(LDFLAG_LOCATION).GitSummary=$(GIT_SUMMARY) \
         -X $(LDFLAG_LOCATION).AppVersion=$(VERSION) \
         -X $(LDFLAG_LOCATION).BuildDate=$(BUILD_DATE)"


## Build linux bin
build-linux:
ifeq ($(GO_VERSION), 0)
	$(error build requies go version 1.16.n or higher)
endif
	GOOS=linux GOARCH=amd64 go build -o flasher \
	   -ldflags \
		"-X $(LDFLAG_LOCATION).GitCommit=$(GIT_COMMIT) \
         -X $(LDFLAG_LOCATION).GitBranch=$(GIT_BRANCH) \
         -X $(LDFLAG_LOCATION).GitSummary=$(GIT_SUMMARY) \
         -X $(LDFLAG_LOCATION).AppVersion=$(VERSION) \
         -X $(LDFLAG_LOCATION).BuildDate=$(BUILD_DATE)"


## build docker image and tag as ghcr.io/metal-toolbox/flasher:latest
build-image: build-linux
	@echo ">>>> NOTE: You may want to execute 'make build-image-nocache' depending on the Docker stages changed"
	docker build --rm=true -f Dockerfile -t ${DOCKER_IMAGE}:latest  . \
							 --label org.label-schema.schema-version=1.0 \
							 --label org.label-schema.vcs-ref=$(GIT_COMMIT_FULL) \
							 --label org.label-schema.vcs-url=$(REPO)

## tag and push devel docker image to local registry
push-image-devel: build-image
	docker tag ${DOCKER_IMAGE}:latest localhost:5001/flasher:latest
	docker push localhost:5001/flasher:latest
	kind load docker-image localhost:5001/flasher:latest

## push docker image
push-image:
	docker push ${DOCKER_IMAGE}:latest


## generate statemachine graphs and docs
gen-docs:
	go build -o flasher
	./docs/statemachine/generate.sh



# https://gist.github.com/prwhite/8168133
# COLORS
GREEN  := $(shell tput -Txterm setaf 2)
YELLOW := $(shell tput -Txterm setaf 3)
WHITE  := $(shell tput -Txterm setaf 7)
RESET  := $(shell tput -Txterm sgr0)


TARGET_MAX_CHAR_NUM=20
## Show help
help:
	@echo ''
	@echo 'Usage:'
	@echo '  ${YELLOW}make${RESET} ${GREEN}<target>${RESET}'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\\_0-9]+:/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")-1); \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			printf "  ${YELLOW}%-$(TARGET_MAX_CHAR_NUM)s${RESET} ${GREEN}%s${RESET}\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)
