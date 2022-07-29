# by default running `make` will build the doctor program
# run it inside a dockerized host where a kava blockchain
# node is running, and execute the unit and integration
# tests using local sources
default: build run test

# declare non-file based targets to speed up target
# invocataions by letting make know it can skip checking
# for changes in a file
.PHONY: lint build cross-compile install run test stop refresh

# import environment file for setting or overriding
# configuration used by this Makefile
include .env

# source all variables in environment file
# This only runs in the make command shell
# so won't affect your login shell
export $(shell sed 's/=.*//' .env)

# Variables used for naming and finding program binaries
BINARY=doctor
VERSION=0.0.0

# Variables used for labelling dockerized host
# containing docker and kava binaries
CONTAINER_NAME=kava-node-doctor
IMAGE_NAME=kava-labs/doctor
LOCAL_IMAGE_TAG=local

# format and verify doctor source code and dependency tree
lint:
	go mod tidy
	go fmt ./...
	go vet ./...

# build a linux docker container that includes
# a binary and configuration for the doctor program
# using local sources along with the kava node program
# installed and ready to run
build:
	docker build ./ -t ${IMAGE_NAME}:${LOCAL_IMAGE_TAG}

# build a binary of the doctor using local sources
# that can run on the build host and place it in the
# GOBIN path for the current host
install:
	go install

# start dockerized host that has both the kava and doctor processes
# running for testing and development
run:
	docker run --name ${CONTAINER_NAME}-$$(date +%s) -d -it --env-file ./.env -p ${KAVA_RPC_DOCKER_HOST_PORT}:${KAVA_RPC_PORT} -p ${KAVA_API_DOCKER_HOST_PORT}:${KAVA_API_PORT} ${IMAGE_NAME}:${LOCAL_IMAGE_TAG}

# follows the logs for the docker host where
# the kava and doctor services are running
logs:
# only follow logs if the container is running
	docker ps | grep ${CONTAINER_NAME} | awk '{ print $$1 }'| xargs -r docker logs -f $$1

# execute unit tests locally and integration tests
# against docker host environment
test:
	go test -v ./...

# stop the doctor test and development container
stop:
# only stop the container if its running
# run the command recursively to work around false negatives
# that occur when trying to stop a container that is using
# significant host resources
	docker ps | grep ${CONTAINER_NAME} | awk '{ print $$1 }'| xargs -r docker kill -s SIGKILL $$1 || $(MAKE) stop

# rebuild and restart the test and development container
refresh: build stop run

# restart the test and development container
restart: stop run
