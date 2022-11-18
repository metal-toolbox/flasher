export DOCKER_BUILDKIT=1
GIT_COMMIT_FULL  := $(shell git rev-parse HEAD)
GO_VERSION := $(shell expr `go version |cut -d ' ' -f3 |cut -d. -f2` \>= 16)
DOCKER_IMAGE  := "ghcr.io/metal-toolbox/flasher"
REPO := "https://github.com/metal-toolbox/flasher.git"

.DEFAULT_GOAL := help

## lint
lint:
	golangci-lint run --config .golangci.yml

## Go test
test:
	CGO_ENABLED=0 go test -timeout 1m -v -covermode=atomic ./...

## build osx bin
build-osx:
ifeq ($(GO_VERSION), 0)
	$(error build requies go version 1.17.n or higher)
endif
	  GOOS=darwin GOARCH=amd64 go build -o flasher


## Build linux bin
build-linux:
ifeq ($(GO_VERSION), 0)
	$(error build requies go version 1.16.n or higher)
endif
	GOOS=linux GOARCH=amd64 go build -o flasher

## build docker image and tag as ghcr.io/metal-toolbox/flasher:latest
build-image: build-linux
	@echo ">>>> NOTE: You may want to execute 'make build-image-nocache' depending on the Docker stages changed"
	docker build --rm=true -f Dockerfile -t ${DOCKER_IMAGE}:latest  . \
							 --label org.label-schema.schema-version=1.0 \
							 --label org.label-schema.vcs-ref=$(GIT_COMMIT_FULL) \
							 --label org.label-schema.vcs-url=$(REPO)


# build docker image, ignoring the cache
build-image--nocache: build-linux
	docker build --no-cache --rm=true -f Dockerfile.inband -t ${DOCKER_IMAGE}:latest  . \
							 --label org.label-schema.schema-version=1.0 \
							 --label org.label-schema.vcs-ref=$(GIT_COMMIT_FULL) \
							 --label org.label-schema.vcs-url=$(REPO)

## build devel docker image
build-image-devel: build-image
	docker tag ${DOCKER_IMAGE}:latest localhost:5000/flasher:latest
	docker push localhost:5000/flasher:latest
	kind load docker-image localhost:5000/flasher:latest

## push docker image
push-image:
	docker push ${DOCKER_IMAGE}:latest


## generate statemachine graphs and docs
docs: build-osx
	./flasher export statemachine --action | dot -Tsvg  > ./docs/statemachine/action_sm.svg
	./flasher export statemachine --task | dot -Tsvg  > ./docs/statemachine/task_sm.svg
	./flasher export statemachine --task --json > ./docs/statemachine/task-statemachine.json
	./flasher export statemachine --action --json > ./docs/statemachine/action-statemachine.json
	./docs/statemachine/generate_action_sm_docs.sh
	./docs/statemachine/generate_task_sm_docs.sh


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
