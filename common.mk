# Copyright 2022 The Kubernetes Authors.
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

GOLANG_VERSION ?= 1.24.6

DRIVER_NAME := dra-driver-sriov
MODULE := github.com/SchSeba/$(DRIVER_NAME)

VERSION ?= v0.0.1

VENDOR := sriovnetwork.openshift.io
APIS := virtualfunction/v1alpha1

PLURAL_EXCEPTIONS  = DeviceClassParameters:DeviceClassParameters
PLURAL_EXCEPTIONS += VirtualfunctionClaimParameters:VirtualfunctionClaimParameters

ifeq ($(IMAGE_NAME),)
REGISTRY ?= local
IMAGE_NAME = $(REGISTRY)/$(DRIVER_NAME)
endif
