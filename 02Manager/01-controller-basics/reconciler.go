package main

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SimpleReconciler 是这一节唯一的“业务逻辑”。
//
// 它什么也不做，只打一行日志 —— 目的是看清：
//   - Reconcile 什么时候被调用
//   - 一次事件对应一次还是多次调用
//   - req 里到底有什么
type SimpleReconciler struct {
	client.Client
}

func (r *SimpleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// ctx 由 controller-runtime 注入，logger 里已经带好：
	//   controller / controllerGroup / controllerKind
	//   ConfigMap{name, namespace} / namespace / name / reconcileID
	// 所以不要再手动加 namespace、name，否则输出里会出现两组（实测过）。
	log := ctrl.LoggerFrom(ctx)

	log.Info("Reconcile 被调用")

	return ctrl.Result{}, nil
}
