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
// ScalableApp 是一个自定义 CRD，演示如何：
// 1. 管理一个真实的 Deployment
// 2. 启用 /scale 子资源让 HPA 控制 replicas
type ScalableApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ScalableAppSpec   `json:"spec,omitempty"`
	Status ScalableAppStatus `json:"status,omitempty"`
}

// ScalableAppSpec 用户期望的状态
type ScalableAppSpec struct {
	// Replicas: 期望副本数（用户可以手动改，HPA 也可以自动改）
	Replicas int32 `json:"replicas"`
	// Image: 容器镜像
	Image string `json:"image"`
	// Port: 容器端口（用于 Service 和 HPA 指标采集）
	Port int32 `json:"port,omitempty"`
}

// ScalableAppStatus 实际观察到的状态
type ScalableAppStatus struct {
	// ReadyReplicas: 实际就绪的副本数（HPA 读取这个值确认缩放生效）
	ReadyReplicas int32 `json:"readyReplicas"`
	// Selector: Label Selector 字符串（HPA 用来找到 Pod 并采集 CPU 指标）
	Selector string `json:"selector"`
	// Phase: 当前阶段
	Phase string `json:"phase,omitempty"`
}

// ScalableAppList 列表类型
type ScalableAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScalableApp `json:"items"`
}

var (
	SchemeGroupVersion = schema.GroupVersion{Group: "demo.vitaaaaa.icu", Version: "v1alpha1"}
	ScalableAppKind    = "ScalableApp"
)

func AddKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &ScalableApp{}, &ScalableAppList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// 手动深拷贝（避免 ObjectMeta 嵌入导致 DeepCopy 冲突）
func DeepCopyScalableApp(in *ScalableApp) *ScalableApp {
	if in == nil {
		return nil
	}
	out := new(ScalableApp)
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec
	out.Status = in.Status
	return out
}

func (in *ScalableApp) DeepCopyObject() runtime.Object {
	return DeepCopyScalableApp(in)
}

func (in *ScalableAppList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(ScalableAppList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]ScalableApp, len(in.Items))
		copy(out.Items, in.Items)
	}
	return out
}
