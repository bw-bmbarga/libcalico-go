// Copyright (c) 2017-2019 Tigera, Inc. All rights reserved.

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package namespace

import (
	apiv3 "github.com/bw-bmbarga/libcalico-go/lib/apis/v3"
)

const (
	// Re-implement the model.KindKubernetesNetworkPolicy constant here
	// to avoid an import loop.
	KindKubernetesNetworkPolicy = "KubernetesNetworkPolicy"
)

func IsNamespaced(kind string) bool {
	switch kind {
	case apiv3.KindWorkloadEndpoint, apiv3.KindNetworkPolicy, apiv3.KindNetworkSet:
		return true
	case KindKubernetesNetworkPolicy:
		// KindKubernetesNetworkPolicy is a special-case resource. We don't expose it over the
		// v3 API, but it is used in the felix syncer to implement the Kubernetes NetworkPolicy API.
		return true
	default:
		return false
	}
}
