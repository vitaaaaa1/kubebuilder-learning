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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ============================================================
// 📝 1. API 类型定义
// ============================================================

// DemoAppSpec 定义了用户期望的状态
type DemoAppSpec struct {
	// Image 指定容器镜像。
	// 如果用户留空，Mutating Webhook 会自动填充默认值。
	// +optional
	Image string `json:"image,omitempty"`

	// Replicas 指定副本数量。
	// Validating Webhook 会检查此值不能为负数。
	Replicas int32 `json:"replicas"`
}

// DemoAppStatus 定义了系统的实际状态
type DemoAppStatus struct {
	// Phase 表示当前阶段（如 Pending, Running, Failed）。
	Phase string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ============================================================
// 📝 2. Webhook Markers (核心！)
// ============================================================
//
// 这些 Marker 会被 controller-gen 解析并生成 ValidatingWebhookConfiguration / MutatingWebhookConfiguration。
//
// Mutating Webhook Marker:
// +kubebuilder:webhook:path=/mutate-demo-vitaaaaa-icu-v1alpha1-demoapp,mutating=true,failurePolicy=fail,groups="demo.vitaaaaa.icu",resources=demoapps,verbs=create;update,versions=v1,name=mdemoapp.demo.vitaaaaa.icu,sideEffects=None,admissionReviewVersions=v1
//
// Validating Webhook Marker:
// +kubebuilder:webhook:path=/validate-demo-vitaaaaa-icu-v1alpha1-demoapp,mutating=false,failurePolicy=fail,groups="demo.vitaaaaa.icu",resources=demoapps,verbs=create;update,versions=v1,name=vdemoapp.demo.vitaaaaa.icu,sideEffects=None,admissionReviewVersions=v1

// DemoApp 是我们自定义的资源类型
type DemoApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DemoAppSpec   `json:"spec,omitempty"`
	Status DemoAppStatus `json:"status,omitempty"`
}

// DemoAppList 包含 DemoApp 的列表
// +kubebuilder:object:root=true
type DemoAppList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DemoApp `json:"items"`
}

var (
	// SchemeGroupVersion 是 GroupVersion 标识符
	SchemeGroupVersion = schema.GroupVersion{Group: "demo.vitaaaaa.icu", Version: "v1alpha1"}

	// SchemeBuilder 用于将类型注册到 Scheme 中
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme 用于将 SchemeBuilder 应用到 Scheme
	AddToScheme = SchemeBuilder.AddToScheme
)

// addKnownTypes 将类型添加到指定的 Scheme 中
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&DemoApp{},
		&DemoAppList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}

// ============================================================
// 📝 3. 辅助函数
// ============================================================

// Hub 方法使得 DemoApp 成为 conversion.Hub (如果是多版本 API)
// 这里我们只有一个版本，所以留空实现
func (r *DemoApp) Hub() {}

// DeepCopyObject 实现 runtime.Object 接口（模拟 controller-gen 生成的代码）
func (in *DemoApp) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(DemoApp)
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
	return out
}

// DeepCopyObject 实现 runtime.Object 接口
func (in *DemoAppList) DeepCopyObject() runtime.Object {
	if in == nil {
		return nil
	}
	out := new(DemoAppList)
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]DemoApp, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
	return out
}

// DeepCopyInto 辅助方法
func (in *DemoApp) DeepCopyInto(out *DemoApp) {
	if in == nil {
		return
	}
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	out.Status = in.Status
}
