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
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ============================================================
// 📝 1. 日志初始化
// ============================================================

// log 用于记录 Webhook 相关的日志
var log = logf.Log.WithName("demoapp-resource")

// demoapplog 别名，保持与 api 包一致的命名
var demoapplog = log

// ============================================================
// 📝 2. SetupWebhookWithManager (注册入口)
// ============================================================

// SetupWebhookWithManager 将 Webhook 注册到 Manager 中
// 对应 kubebuilder 生成的代码结构
func (r *DemoApp) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// ============================================================
// 📝 3. Mutating Webhook (Defaulter)
// ============================================================

//+kubebuilder:webhook:path=/mutate-demo-vitaaaaa-icu-v1alpha1-demoapp,mutating=true,failurePolicy=fail,sideEffects=None,groups=demo.vitaaaaa.icu,resources=demoapps,verbs=create;update,versions=v1,name=mdemoapp.demo.vitaaaaa.icu,admissionReviewVersions=v1

var _ webhook.Defaulter = &DemoApp{}

// Default 实现 defaulter 接口，用于设置默认值
// 当用户创建或更新资源时，如果某些字段为空，Webhook 会自动填充
func (r *DemoApp) Default() {
	demoapplog.Info("Default", "name", r.Name)

	// 🟢 场景 1: 自动填充 Image
	// 如果用户没有指定镜像，我们默认使用 nginx:latest
	if r.Spec.Image == "" {
		r.Spec.Image = "nginx:latest"
		demoapplog.Info("Set default image", "image", r.Spec.Image)
	}

	// 🟢 场景 2: 自动注入 Label
	// 用于标识该资源被 Operator 管理
	if r.Labels == nil {
		r.Labels = make(map[string]string)
	}
	if _, ok := r.Labels["demo.vitaaaaa.icu/managed-by"]; !ok {
		r.Labels["demo.vitaaaaa.icu/managed-by"] = "demo-operator"
	}
}

// ============================================================
// 📝 4. Validating Webhook (Validator)
// ============================================================

//+kubebuilder:webhook:path=/validate-demo-vitaaaaa-icu-v1alpha1-demoapp,mutating=false,failurePolicy=fail,sideEffects=None,groups=demo.vitaaaaa.icu,resources=demoapps,verbs=create;update,versions=v1,name=vdemoapp.demo.vitaaaaa.icu,admissionReviewVersions=v1

var _ webhook.Validator = &DemoApp{}

// ValidateCreate 实现 validator 接口，用于校验创建请求
func (r *DemoApp) ValidateCreate() (admission.Warnings, error) {
	demoapplog.Info("ValidateCreate", "name", r.Name)

	// 复用通用的校验逻辑
	return nil, r.validateDemoApp()
}

// ValidateUpdate 实现 validator 接口，用于校验更新请求
func (r *DemoApp) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	demoapplog.Info("ValidateUpdate", "name", r.Name)

	// 🟡 场景：防止不可变字段被修改
	oldApp, ok := old.(*DemoApp)
	if !ok {
		return nil, apierrors.NewInternalError(fmt.Errorf("failed to parse old object"))
	}

	// 示例：假设 "Image" 一旦设定就不能修改（模拟不可变字段）
	if oldApp.Spec.Image != "" && r.Spec.Image != oldApp.Spec.Image {
		return nil, field.Invalid(
			field.NewPath("spec").Child("image"),
			r.Spec.Image,
			"field is immutable once set",
		)
	}

	return nil, r.validateDemoApp()
}

// ValidateDelete 实现 validator 接口，用于校验删除请求
func (r *DemoApp) ValidateDelete() (admission.Warnings, error) {
	demoapplog.Info("ValidateDelete", "name", r.Name)

	// 通常删除不需要校验，除非有"保护锁"逻辑
	return nil, nil
}

// ============================================================
// 📝 5. 通用校验逻辑
// ============================================================

// validateDemoApp 包含所有共通的校验规则
func (r *DemoApp) validateDemoApp() error {
	var allErrs field.ErrorList

	// 🔴 规则 1: Replicas 不能为负数
	if r.Spec.Replicas < 0 {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("replicas"),
			r.Spec.Replicas,
			"must be greater than or equal to 0",
		))
	}

	// 🔴 规则 2: 如果有 Image，必须符合特定格式 (简单检查包含冒号)
	if r.Spec.Image != "" && !strings.Contains(r.Spec.Image, ":") {
		allErrs = append(allErrs, field.Invalid(
			field.NewPath("spec").Child("image"),
			r.Spec.Image,
			"must include a tag (e.g., nginx:1.25)",
		))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(
		schema.GroupKind{Group: "demo.vitaaaaa.icu", Kind: "DemoApp"},
		r.Name,
		allErrs,
	)
}
