// 版本 A：使用 client.New（apiReader，纯直连）
package main

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func main() {
	cfg := ctrl.GetConfigOrDie()
	httpClient, err := rest.HTTPClientFor(cfg)
	if err != nil {
		panic(err)
	}

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(err)
	}

	mapper, err := apiutil.NewDynamicRESTMapper(cfg, httpClient)
	if err != nil {
		panic(err)
	}

	// client.New：不挂 Cache → 全部直连
	c, err := client.New(cfg, client.Options{
		Scheme:     scheme,
		Mapper:     mapper,
		HTTPClient: httpClient,
	})
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	fmt.Println("========== 版本 A：client.New ==========")

	// 1) List
	var list corev1.ConfigMapList
	if err := c.List(ctx, &list, client.InNamespace("test")); err != nil {
		fmt.Println("List 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[List] test 命名空间 ConfigMap 数量: %d\n", len(list.Items))

	// 2) Get
	var cm corev1.ConfigMap
	if err := c.Get(ctx, client.ObjectKey{Namespace: "test", Name: "demo-config"}, &cm); err != nil {
		fmt.Println("Get 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[Get ] demo-config data 键数量: %d\n", len(cm.Data))

	// 3) Create
	tmp := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-client-anatomy", Namespace: "test"},
		Data:       map[string]string{"from": "client.New"},
	}
	if err := c.Create(ctx, tmp); err != nil {
		fmt.Println("Create 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[Create] 成功，UID=%s\n", tmp.UID)

	// 4) Delete
	if err := c.Delete(ctx, tmp); err != nil {
		fmt.Println("Delete 失败:", err)
		os.Exit(1)
	}
	fmt.Println("[Delete] 成功")
}
