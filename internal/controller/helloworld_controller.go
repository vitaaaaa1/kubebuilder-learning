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

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	demov1alpha1 "github.com/vitaaaaa/kubebuilder-learning/api/v1alpha1"
)

var logger = logf.Log.WithName("controller-helloWorld")

// HelloWorldReconciler reconciles a HelloWorld object
type HelloWorldReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=demo.vitaaaaa.icu,resources=helloworlds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=demo.vitaaaaa.icu,resources=helloworlds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=demo.vitaaaaa.icu,resources=helloworlds/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the HelloWorld object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.25.0/pkg/reconcile
func (r *HelloWorldReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here
	log := logger.WithValues("Request.Namepsace", req.Namespace, "Request.Name", req.Name)
	log.Info("Reconcile Called")

	// 1.
	var helloWorld demov1alpha1.HelloWorld
	err := r.Get(ctx, req.NamespacedName, &helloWorld)
	if err != nil {
		return ctrl.Result{}, err
	}

	// 2. modify crd status
	log.Info("Step 02")
	meta.SetStatusCondition(&helloWorld.Status.Conditions, metav1.Condition{
		Type:               "Available",                    // 条件类型
		Status:             metav1.ConditionTrue,           // 状态值
		ObservedGeneration: helloWorld.Generation,          // 对应的版本号
		Reason:             "TestingStatus",                // 原因
		Message:            "I am testing status updates!", // 说明
	})

	// 3. update crd
	log.Info("Step 03")
	err = r.Status().Update(ctx, &helloWorld)
	if err != nil {
		return ctrl.Result{}, err
	}

	log.Info("Status updated successfully")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *HelloWorldReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&demov1alpha1.HelloWorld{}).
		Named("helloworld").
		Complete(r)
}
