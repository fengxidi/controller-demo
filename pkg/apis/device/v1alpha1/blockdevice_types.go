/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BlockDeviceSpec defines the desired state of BlockDevice
type BlockDeviceSpec struct {
	// 设备名称
	Name string `json:"name,omitempty"`
	// 设备所在节点
	Node string `json:"node,omitempty"`
	// 块设备容量
	Size string `json:"size,omitempty"`
	// 类型
	Tyep string `json:"type,omitempty"`
}

// BlockDeviceStatus defines the observed state of BlockDevice
type BlockDeviceStatus struct {
	// 设备状态
	Status string `json:"status,omitempty"`
	// 设备最后更新实际
	LastUpdateTime metav1.Time `json:"lastUpdateTIme,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:openapi-gen=true
// +kubebuilder:resource:scope=Cluster
//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="deviceName",type="string",JSONPath=".spec.name"
//+kubebuilder:printcolumn:name="size",type="string",JSONPath=".spec.size"
//+kubebuilder:printcolumn:name="NodeName",type="string",JSONPath=".spec.node"
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"

// BlockDevice is the Schema for the blockdevices API
type BlockDevice struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BlockDeviceSpec   `json:"spec,omitempty"`
	Status BlockDeviceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BlockDeviceList contains a list of BlockDevice
type BlockDeviceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BlockDevice `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BlockDevice{}, &BlockDeviceList{})
}
