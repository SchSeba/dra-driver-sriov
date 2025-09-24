# Copyright 2023 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

##### Global variables #####
CONTAINER_TOOL ?= docker
MKDIR ?= mkdir
DISTRIBUTIONS := centos9
DOCKERFILE := $(CURDIR)/Dockerfile
GOARCH ?= amd64

include $(CURDIR)/../../common.mk

##### Public rules #####
.PHONY: $(DISTRIBUTIONS)

centos9: BASE_IMAGE = quay.io/centos/centos:stream9

# Use a generic build target to build the relevant images
$(DISTRIBUTIONS):
	DOCKER_BUILDKIT=1 \
		$(CONTAINER_TOOL) build --pull \
		--tag $(IMAGE_NAME):$(VERSION) \
		--build-arg GOLANG_VERSION="$(GOLANG_VERSION)" \
		--build-arg BASE_IMAGE="$(BASE_IMAGE)" \
		--build-arg VERSION="$(VERSION)" \
		--build-arg GOARCH="$(GOARCH)" \
		-f $(DOCKERFILE) \
		$(CURDIR)/../..

push:
	$(CONTAINER_TOOL) push $(IMAGE_NAME):$(VERSION)
