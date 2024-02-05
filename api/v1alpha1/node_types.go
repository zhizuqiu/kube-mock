/*
Copyright 2024.

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
	"k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// NodeSpec defines the desired state of Node
type NodeSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	Suspend bool `json:"suspend,omitempty"`

	Image            string                        `json:"image"`
	ImagePullPolicy  corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	Tolerations      []corev1.Toleration           `json:"tolerations,omitempty"`
	NodeSelector     map[string]string             `json:"nodeSelector,omitempty"`
	Affinity         *corev1.Affinity              `json:"affinity,omitempty"`
	SecurityContext  *corev1.PodSecurityContext    `json:"securityContext,omitempty"`

	Resources             v1.ResourceRequirements `json:"resources,omitempty"`
	KubeconfigSecret      string                  `json:"kubeconfigSecret,omitempty"`
	ApiserverCertSecret   string                  `json:"apiserverCertSecret,omitempty"`
	ApiserverKeySecret    string                  `json:"apiserverKeySecret,omitempty"`
	StatusUpdatesInterval string                  `json:"statusUpdatesInterval,omitempty"`

	NodeConfig NodeConfig `json:"nodeConfig,omitempty"`
}

type NodeConfig struct {
	Cpu     resource.Quantity `json:"cpu,omitempty"`
	Memory  resource.Quantity `json:"memory,omitempty"`
	Pods    resource.Quantity `json:"pods,omitempty"`
	Address string            `json:"address,omitempty"`
	Port    int32             `json:"port,omitempty"`
	// todo other Resource

	StartupTimeout string `json:"startupTimeout,omitempty"`
	LogLevel       string `json:"logLevel,omitempty"`
	KLogV          string `json:"kLogV,omitempty"`

	Labels map[string]string `json:"labels,omitempty"`
	Taints []v1.Taint        `json:"taints,omitempty"`
}

// NodeStatus defines the observed state of Node
type NodeStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

type PodState struct {
	Name          string          `json:"name,omitempty"`
	Role          string          `json:"role,omitempty"`
	Phase         corev1.PodPhase `json:"phase,omitempty"`
	HostIP        string          `json:"hostIP,omitempty"`
	PodIP         string          `json:"podIP,omitempty"`
	ContainerPort int32           `json:"containerPort,omitempty"`
	PodIPs        []corev1.PodIP  `json:"podIPs,omitempty"`
	StartTime     *metav1.Time    `json:"startTime,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Node is the Schema for the nodes API
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeSpec   `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// NodeList contains a list of Node
type NodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Node `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Node{}, &NodeList{})
}
