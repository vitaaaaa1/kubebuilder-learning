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

// ============================================================
// 📝 CRD 类型定义：DemoApp（演示 /scale + /status 子资源）
// ============================================================
//
// 本文件定义了一个最小 CRD：DemoApp
// 它支持两个子资源：
//   1. /status — 分离 Status 与 Spec
//   2. /scale  — 让 HPA 能读写 replicas
//
// 关键 Marker：
//   +kubebuilder:subresource:status
//   +kubebuilder:subresource:scale:specPath=.spec.replicas,statusPath=.status.readyReplicas,selectorPath=.status.selector
//
// ⚠️ 注意：因为我们是手写实验（不用 kubebuilder 脚手架），
// 所以 Marker 注释在这里仅作为文档记录。
// 实际 CRD YAML 需要手动编写 subresources 字段。
// ============================================================

// DemoAppSpec 用户声明的"期望"
type DemoAppSpec struct {
	// Image: 用户想用的容器镜像
	Image string `json:"image"`
	// Replicas: 用户想用的副本数（HPA 会修改这个字段）
	Replicas int32 `json:"replicas"`
}

// DemoAppStatus Controller 回报的"现实"
type DemoAppStatus struct {
	// Replicas: 当前实际的 replicas 数量（/scale 子资源需要）
	Replicas int32 `json:"replicas"`
	// ReadyReplicas: 实际就绪的副本数（HPA 读取这个值）
	ReadyReplicas int32 `json:"readyReplicas"`
	// Selector: Label Selector 字符串（HPA 用来找到 Pod）
	Selector string `json:"selector"`
	// Phase: 当前阶段（Pending / Running / Failed）
	Phase string `json:"phase"`
	// Message: 人类可读的状态信息
	Message string `json:"message,omitempty"`
}

// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specPath=.spec.replicas,statusPath=.status.readyReplicas,selectorPath=.status.selector
// DemoApp 是我们的自定义资源，启用了 /status 和 /scale 子资源
type DemoApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DemoAppSpec   `json:"spec,omitempty"`
	Status DemoAppStatus `json:"status,omitempty"`
}

// DemoAppList 是 DemoApp 的列表类型
type DemoAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DemoApp `json:"items"`
}

// GVK 定义
var (
	SchemeGroupVersion = schema.GroupVersion{Group: "demo.vitaaaaa.icu", Version: "v1alpha1"}

	DemoAppKind = "DemoApp"
)

func AddKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion, &DemoApp{}, &DemoAppList{})
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// DeepCopyDemoApp 手写深拷贝（避免与 embedded ObjectMeta.DeepCopy 冲突）
func DeepCopyDemoApp(in *DemoApp) *DemoApp {
	if in == nil {
		return nil
	}
	out := new(DemoApp)
	out.TypeMeta = in.TypeMeta
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	out.Spec = in.Spec
	out.Status = in.Status
	return out
}

func (in *DemoApp) DeepCopyObject() runtime.Object {
	return DeepCopyDemoApp(in)
}

func (in *DemoAppList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(DemoAppList)
	out.TypeMeta = in.TypeMeta
	out.ListMeta = in.ListMeta
	if in.Items != nil {
		out.Items = make([]DemoApp, len(in.Items))
		copy(out.Items, in.Items)
	}
	return out
}
