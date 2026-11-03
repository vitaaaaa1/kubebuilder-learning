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

package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// ============================================================
// 🎯 实验目标：理解如何用 Webhook 保护子资源
// ============================================================
//
// 本实验演示：
// 1. ValidatingWebhook 拦截对 /status 的非法写入
// 2. 白名单校验：Phase 只能是 Pending/Running/Failed/Scaling
// 3. 校验 readyReplicas 不超过 spec.replicas
//
// 部署方式：
//   1. 生成 TLS 证书（见 deploy/ 目录）
//   2. 部署 Webhook Server 到集群
//   3. 创建 ValidatingWebhookConfiguration
//   4. 测试非法写入被拒绝
// ============================================================

// ScalableAppStatus 简化版（和 CRD 一致）
type ScalableAppStatus struct {
	Phase         string `json:"phase"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Selector      string `json:"selector"`
	Message       string `json:"message,omitempty"`
}

// ScalableAppSpec 简化版
type ScalableAppSpec struct {
	Replicas int32 `json:"replicas"`
}

// ScalableApp 简化版
type ScalableApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ScalableAppSpec   `json:"spec"`
	Status            ScalableAppStatus `json:"status"`
}

// StatusValidator Webhook Handler
type StatusValidator struct {
	decoder admission.Decoder
}

// AllowedPhases 白名单
var AllowedPhases = map[string]bool{
	"Pending":         true,
	"Running":         true,
	"Failed":          true,
	"Scaling":         true,
	"PendingApproval": true,
	"Approved":        true,
}

// Handle 处理 AdmissionReview 请求
func (v *StatusValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	var app ScalableApp
	if err := v.decoder.DecodeRaw(req.Object, &app); err != nil {
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("decode object: %w", err))
	}

	// ============================================================
	// 校验 1: Phase 白名单
	// ============================================================
	if !AllowedPhases[app.Status.Phase] {
		return admission.Denied(fmt.Sprintf(
			"Phase %q is not allowed. Allowed values: %v",
			app.Status.Phase, getPhaseList(),
		))
	}

	// ============================================================
	// 校验 2: readyReplicas 不能超过 spec.replicas
	// ============================================================
	if app.Status.ReadyReplicas > app.Spec.Replicas {
		return admission.Denied(fmt.Sprintf(
			"readyReplicas (%d) cannot exceed spec.replicas (%d)",
			app.Status.ReadyReplicas, app.Spec.Replicas,
		))
	}

	// ============================================================
	// 校验 3: readyReplicas 不能为负数
	// ============================================================
	if app.Status.ReadyReplicas < 0 {
		return admission.Denied("readyReplicas cannot be negative")
	}

	// ============================================================
	// 校验 4: Message 长度限制
	// ============================================================
	if len(app.Status.Message) > 1024 {
		return admission.Denied(fmt.Sprintf(
			"message length (%d) exceeds maximum (1024 characters)",
			len(app.Status.Message),
		))
	}

	// 全部通过
	return admission.Allowed("Status validation passed")
}

// StatusValidator implements admission.DecoderInjector.
func (v *StatusValidator) InjectDecoder(d admission.Decoder) error {
	v.decoder = d
	return nil
}

func getPhaseList() string {
	phases := make([]string, 0, len(AllowedPhases))
	for p := range AllowedPhases {
		phases = append(phases, p)
	}
	return fmt.Sprintf("[%v]", phases)
}

// DeepCopyObject 实现 runtime.Object 接口
func (in *ScalableApp) DeepCopyObject() runtime.Object {
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

// ============================================================
// 启动入口
// ============================================================

func main() {
	// 初始化日志
	opts := zap.Options{Development: true}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	// 构建 Webhook Server
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: runtime.NewScheme(),
		WebhookServer: webhook.NewServer(webhook.Options{
			Host:    "0.0.0.0",
			Port:    9443,
			CertDir: "/tmp/k8s-webhook-server/serving-certs", // 证书目录
			TLSOpts: []func(*tls.Config){},
		}),
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// 注册 Webhook Handler
	// 路径必须和 ValidatingWebhookConfiguration 中的 path 一致
	mgr.GetWebhookServer().Register("/validate-status", &webhook.Admission{
		Handler: &StatusValidator{},
	})

	setupLog.Info("🚀 Starting Status Validation Webhook Server.")
	setupLog.Info("📝 Validating: Phase whitelist, readyReplicas range, message length")
	setupLog.Info("🔗 Endpoint: https://0.0.0.0:9443/validate-status")

	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running webhook server")
		os.Exit(1)
	}
}

// ============================================================
// 辅助工具：本地测试用（不部署到集群也能验证逻辑）
// ============================================================

// RunLocalTest 在本地模拟 AdmissionReview 请求，用于测试校验逻辑
func RunLocalTest() {
	fmt.Println("============================================================")
	fmt.Println("🧪 本地测试 Status Validator 逻辑")
	fmt.Println("============================================================")
	fmt.Println()

	testCases := []struct {
		name       string
		status     ScalableAppStatus
		spec       ScalableAppSpec
		shouldPass bool
	}{
		{
			name:       "✅ 合法: Phase=Running",
			status:     ScalableAppStatus{Phase: "Running", ReadyReplicas: 2, Message: "OK"},
			spec:       ScalableAppSpec{Replicas: 3},
			shouldPass: true,
		},
		{
			name:       "❌ 非法: Phase=Hacked",
			status:     ScalableAppStatus{Phase: "Hacked", ReadyReplicas: 0, Message: "I own your cluster"},
			spec:       ScalableAppSpec{Replicas: 3},
			shouldPass: false,
		},
		{
			name:       "❌ 非法: readyReplicas > spec.replicas",
			status:     ScalableAppStatus{Phase: "Running", ReadyReplicas: 10, Message: "Too many"},
			spec:       ScalableAppSpec{Replicas: 3},
			shouldPass: false,
		},
		{
			name:       "❌ 非法: readyReplicas < 0",
			status:     ScalableAppStatus{Phase: "Running", ReadyReplicas: -1, Message: "Negative"},
			spec:       ScalableAppSpec{Replicas: 3},
			shouldPass: false,
		},
		{
			name:       "❌ 非法: Message 太长",
			status:     ScalableAppStatus{Phase: "Running", ReadyReplicas: 1, Message: string(make([]byte, 1025))},
			spec:       ScalableAppSpec{Replicas: 3},
			shouldPass: false,
		},
		{
			name:       "✅ 合法: Phase=PendingApproval",
			status:     ScalableAppStatus{Phase: "PendingApproval", ReadyReplicas: 0, Message: "Waiting"},
			spec:       ScalableAppSpec{Replicas: 2},
			shouldPass: true,
		},
	}

	validator := &StatusValidator{}

	passed := 0
	failed := 0

	for _, tc := range testCases {
		app := ScalableApp{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "test"},
			Spec:       tc.spec,
			Status:     tc.status,
		}

		raw, _ := json.Marshal(app)
		req := admission.Request{
			AdmissionRequest: admissionv1.AdmissionRequest{
				UID:       "test-uid",
				Name:      "test",
				Namespace: "test",
				Object:    runtime.RawExtension{Raw: raw},
				Operation: admissionv1.Update,
			},
		}

		resp := validator.Handle(context.Background(), req)

		if resp.Allowed == tc.shouldPass {
			fmt.Printf("✅ %s: 通过\n", tc.name)
			passed++
		} else {
			fmt.Printf("❌ %s: 失败\n", tc.name)
			fmt.Printf("   预期: allowed=%v, 实际: allowed=%v, 原因: %s\n",
				tc.shouldPass, resp.Allowed, resp.Result.Message)
			failed++
		}
	}

	fmt.Println()
	fmt.Printf("📊 测试结果: %d passed, %d failed\n", passed, failed)

	if failed > 0 {
		fmt.Println("❌ 有测试用例未通过，请检查校验逻辑")
	} else {
		fmt.Println("✅ 所有测试用例通过！校验逻辑正确")
	}
}

// init 函数：如果设置了 LOCAL_TEST=1，则运行本地测试而不是启动 Server
func init() {
	if os.Getenv("LOCAL_TEST") == "1" {
		RunLocalTest()
		os.Exit(0)
	}
}
