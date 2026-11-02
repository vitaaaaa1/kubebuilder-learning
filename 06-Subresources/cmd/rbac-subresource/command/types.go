/*
Copyright 2026.

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

package command

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specPath=.spec.replicas,statusPath=.status.readyReplicas,selectorPath=.status.selector
// RBACDemoApp 用于演示子资源的 RBAC 隔离
type RBACDemoApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RBACDemoAppSpec   `json:"spec,omitempty"`
	Status RBACDemoAppStatus `json:"status,omitempty"`
}

type RBACDemoAppSpec struct {
	Replicas int32  `json:"replicas"`
	Image    string `json:"image"`
}

type RBACDemoAppStatus struct {
	ReadyReplicas int32  `json:"readyReplicas"`
	Selector      string `json:"selector"`
	Phase         string `json:"phase,omitempty"`
}

type RBACDemoAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []RBACDemoApp `json:"items"`
}

var (
	SchemeGroupVersion = schema.GroupVersion{Group: "demo.vitaaaaa.icu", Version: "v1alpha1"}
	RBACDemoAppKind    = "RBACDemoApp"
)

func AddKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &RBACDemoApp{}, &RBACDemoAppList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

func DeepCopyRBACDemoApp(in *RBACDemoApp) *RBACDemoApp {
	if in == nil {
		return nil
	}
	out := new(RBACDemoApp)
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec
	out.Status = in.Status
	return out
}

func (in *RBACDemoApp) DeepCopyObject() runtime.Object {
	return DeepCopyRBACDemoApp(in)
}

func (in *RBACDemoAppList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(RBACDemoAppList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]RBACDemoApp, len(in.Items))
		copy(out.Items, in.Items)
	}
	return out
}
