// Copyright (c) 2020 Tigera, Inc. All rights reserved.

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

package v3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/bw-bmbarga/libcalico-go/lib/numorstring"
)

const (
	KindBGPConfiguration     = "BGPConfiguration"
	KindBGPConfigurationList = "BGPConfigurationList"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BGPConfiguration contains the configuration for any BGP routing.
type BGPConfiguration struct {
	metav1.TypeMeta `json:",inline"`
	// Standard object's metadata.
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// Specification of the BGPConfiguration.
	Spec BGPConfigurationSpec `json:"spec,omitempty"`
}

// BGPConfigurationSpec contains the values of the BGP configuration.
type BGPConfigurationSpec struct {
	// LogSeverityScreen is the log severity above which logs are sent to the stdout. [Default: INFO]
	LogSeverityScreen string `json:"logSeverityScreen,omitempty" validate:"omitempty,logLevel" confignamev1:"loglevel"`

	// NodeToNodeMeshEnabled sets whether full node to node BGP mesh is enabled. [Default: true]
	NodeToNodeMeshEnabled *bool `json:"nodeToNodeMeshEnabled,omitempty" validate:"omitempty" confignamev1:"node_mesh"`

	// ASNumber is the default AS number used by a node. [Default: 64512]
	ASNumber *numorstring.ASNumber `json:"asNumber,omitempty" validate:"omitempty" confignamev1:"as_num"`

	// ServiceLoadBalancerIPs are the CIDR blocks for Kubernetes Service LoadBalancer IPs.
	// Kubernetes Service status.LoadBalancer.Ingress IPs will only be advertised if they are within one of these blocks.
	ServiceLoadBalancerIPs []ServiceLoadBalancerIPBlock `json:"serviceLoadBalancerIPs,omitempty" validate:"omitempty,dive" confignamev1:"svc_loadbalancer_ips"`

	// ServiceExternalIPs are the CIDR blocks for Kubernetes Service External IPs.
	// Kubernetes Service ExternalIPs will only be advertised if they are within one of these blocks.
	ServiceExternalIPs []ServiceExternalIPBlock `json:"serviceExternalIPs,omitempty" validate:"omitempty,dive" confignamev1:"svc_external_ips"`

	// ServiceClusterIPs are the CIDR blocks from which service cluster IPs are allocated.
	// If specified, Calico will advertise these blocks, as well as any cluster IPs within them.
	ServiceClusterIPs []ServiceClusterIPBlock `json:"serviceClusterIPs,omitempty" validate:"omitempty,dive" confignamev1:"svc_cluster_ips"`

	// Communities is a list of BGP community values and their arbitrary names for tagging routes.
	Communities []Community `json:"communities,omitempty" validate:"omitempty,dive" confignamev1:"communities"`

	// PrefixAdvertisements contains per-prefix advertisement configuration.
	PrefixAdvertisements []PrefixAdvertisement `json:"prefixAdvertisements,omitempty" validate:"omitempty,dive" confignamev1:"prefix_advertisements"`

	// ListenPort is the port where BGP protocol should listen. Defaults to 179
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=65535
	ListenPort uint16 `json:"listenPort,omitempty" validate:"omitempty,gt=0" confignamev1:"listen_port"`
}

// ServiceLoadBalancerIPBlock represents a single allowed LoadBalancer IP CIDR block.
type ServiceLoadBalancerIPBlock struct {
	CIDR string `json:"cidr,omitempty" validate:"omitempty,net"`
}

// ServiceExternalIPBlock represents a single allowed External IP CIDR block.
type ServiceExternalIPBlock struct {
	CIDR string `json:"cidr,omitempty" validate:"omitempty,net"`
}

// ServiceClusterIPBlock represents a single allowed ClusterIP CIDR block.
type ServiceClusterIPBlock struct {
	CIDR string `json:"cidr,omitempty" validate:"omitempty,net"`
}

// Community contains standard or large community value and its name.
type Community struct {
	// Name given to community value.
	Name string `json:"name,omitempty" validate:"required,name"`
	// Value must be of format `aa:nn` or `aa:nn:mm`.
	// For standard community use `aa:nn` format, where `aa` and `nn` are 16 bit number.
	// For large community use `aa:nn:mm` format, where `aa`, `nn` and `mm` are 32 bit number.
	// Where, `aa` is an AS Number, `nn` and `mm` are per-AS identifier.
	// +kubebuilder:validation:Pattern=`^(\d+):(\d+)$|^(\d+):(\d+):(\d+)$`
	Value string `json:"value,omitempty" validate:"required"`
}

// PrefixAdvertisement configures advertisement properties for the specified CIDR.
type PrefixAdvertisement struct {
	// CIDR for which properties should be advertised.
	CIDR string `json:"cidr,omitempty" validate:"required,net"`
	// Communities can be list of either community names already defined in `Specs.Communities` or community value of format `aa:nn` or `aa:nn:mm`.
	// For standard community use `aa:nn` format, where `aa` and `nn` are 16 bit number.
	// For large community use `aa:nn:mm` format, where `aa`, `nn` and `mm` are 32 bit number.
	// Where,`aa` is an AS Number, `nn` and `mm` are per-AS identifier.
	Communities []string `json:"communities,omitempty" validate:"required"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BGPConfigurationList contains a list of BGPConfiguration resources.
type BGPConfigurationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []BGPConfiguration `json:"items"`
}

// New BGPConfiguration creates a new (zeroed) BGPConfiguration struct with the TypeMetadata
// initialized to the current version.
func NewBGPConfiguration() *BGPConfiguration {
	return &BGPConfiguration{
		TypeMeta: metav1.TypeMeta{
			Kind:       KindBGPConfiguration,
			APIVersion: GroupVersionCurrent,
		},
	}
}

// NewBGPConfigurationList creates a new zeroed) BGPConfigurationList struct with the TypeMetadata
// initialized to the current version.
func NewBGPConfigurationList() *BGPConfigurationList {
	return &BGPConfigurationList{
		TypeMeta: metav1.TypeMeta{
			Kind:       KindBGPConfigurationList,
			APIVersion: GroupVersionCurrent,
		},
	}
}
