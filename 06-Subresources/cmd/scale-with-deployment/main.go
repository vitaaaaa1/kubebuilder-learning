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
	"example/command"
	"flag"
	"fmt"
	"os"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ScalableAppReconciler 管理 ScalableApp CRD → 创建/更新 Deployment
type ScalableAppReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ScalableAppReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var app command.ScalableApp
	if err := r.Get(ctx, req.NamespacedName, &app); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("📦 Reconcile triggered", "name", app.Name, "replicas", app.Spec.Replicas)

	// ============================================================
	// 1. 创建或更新 Deployment（实际的工作负载）
	// ============================================================
	deploy := &appsv1.Deployment{}
	deployName := types.NamespacedName{Name: app.Name, Namespace: app.Namespace}

	err := r.Get(ctx, deployName, deploy)
	if errors.IsNotFound(err) {
		// Deployment 不存在 → 创建
		deploy = r.buildDeployment(&app)
		if err := r.Create(ctx, deploy); err != nil {
			logger.Error(err, "Failed to create Deployment")
			return ctrl.Result{}, err
		}
		fmt.Println("✅ 创建 Deployment 成功!")
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		return ctrl.Result{}, err
	} else {
		// Deployment 已存在 → 更新 replicas 和 image
		if deploy.Spec.Replicas == nil || *deploy.Spec.Replicas != app.Spec.Replicas ||
			deploy.Spec.Template.Spec.Containers[0].Image != app.Spec.Image {

			*deploy.Spec.Replicas = app.Spec.Replicas
			deploy.Spec.Template.Spec.Containers[0].Image = app.Spec.Image
			if err := r.Update(ctx, deploy); err != nil {
				logger.Error(err, "Failed to update Deployment")
				return ctrl.Result{}, err
			}
			fmt.Println("✅ 更新 Deployment 成功!")
		}
	}

	// ============================================================
	// 2. 同步 Status 到 CRD（HPA 需要这个来确认缩放是否生效）
	// ============================================================
	patchBase := command.DeepCopyScalableApp(&app)
	patchBase.Status.ReadyReplicas = deploy.Status.ReadyReplicas
	patchBase.Status.Selector = fmt.Sprintf("app=%s", app.Name)
	if deploy.Status.ReadyReplicas == app.Spec.Replicas {
		patchBase.Status.Phase = "Running"
	} else {
		patchBase.Status.Phase = "Scaling"
	}

	if err := r.Status().Patch(ctx, patchBase, client.MergeFrom(&app)); err != nil {
		logger.Error(err, "Failed to patch status")
		return ctrl.Result{}, err
	}

	fmt.Printf("📊 同步 Status: ReadyReplicas=%d, Selector=%s, Phase=%s\n",
		patchBase.Status.ReadyReplicas, patchBase.Status.Selector, patchBase.Status.Phase)

	return ctrl.Result{}, nil
}

// buildDeployment 根据 ScalableApp 构建 Deployment 对象
func (r *ScalableAppReconciler) buildDeployment(app *command.ScalableApp) *appsv1.Deployment {
	replicas := app.Spec.Replicas
	label := map[string]string{"app": app.Name}

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      app.Name,
			Namespace: app.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "demo.vitaaaaa.icu/v1alpha1",
					Kind:       "ScalableApp",
					Name:       app.Name,
					UID:        app.UID,
					Controller: ptrBool(true),
				},
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: label,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: label, // ← 关键：Pod 必须带这个 Label，HPA 才能采集 CPU 指标
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "app",
							Image:   app.Spec.Image,
							Ports:   []corev1.ContainerPort{{ContainerPort: app.Spec.Port}},
							Command: []string{"sh", "-c", "sleep 3600"},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("64Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func ptrBool(b bool) *bool { return &b }

func main() {
	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	log.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	setupLog := ctrl.Log.WithName("setup")

	scheme := runtime.NewScheme()
	_ = command.AddKnownTypes(scheme)
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				"test": {},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// 使用 GenerationChangedPredicate 避免 Status 更新触发无限循环
	if err = ctrl.NewControllerManagedBy(mgr).
		For(&command.ScalableApp{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		Complete(&ScalableAppReconciler{Client: mgr.GetClient(), Scheme: scheme}); err != nil {
		setupLog.Error(err, "unable to create controller")
		os.Exit(1)
	}

	setupLog.Info("🚀 Starting ScalableApp Controller.")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
