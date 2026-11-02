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
// ApprovableApp 是一个自定义 CRD，演示如何用 Annotation 模拟 /approve 子资源
type ApprovableApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ApprovableAppSpec   `json:"spec,omitempty"`
	Status ApprovableAppStatus `json:"status,omitempty"`
}

type ApprovableAppSpec struct {
	Image    string `json:"image"`
	Replicas int32  `json:"replicas"`
}

type ApprovableAppStatus struct {
	Phase   string `json:"phase"`
	Message string `json:"message,omitempty"`
}

type ApprovableAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ApprovableApp `json:"items"`
}

var (
	SchemeGroupVersion = schema.GroupVersion{Group: "demo.vitaaaaa.icu", Version: "v1alpha1"}
	ApprovableAppKind  = "ApprovableApp"
)

func AddKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &ApprovableApp{}, &ApprovableAppList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

func DeepCopyApprovableApp(in *ApprovableApp) *ApprovableApp {
	if in == nil {
		return nil
	}
	out := new(ApprovableApp)
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec
	out.Status = in.Status
	return out
}

func (in *ApprovableApp) DeepCopyObject() runtime.Object {
	return DeepCopyApprovableApp(in)
}

func (in *ApprovableAppList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ApprovableAppList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]ApprovableApp, len(in.Items))
		copy(out.Items, in.Items)
	}
	return out
}
