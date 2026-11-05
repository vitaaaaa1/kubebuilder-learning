// 版本 B：不使用 client.New，只用 rest.RESTClient 手写同样的四个操作
//
// 这一段就是 client.New → typedClient 内部真正做的事：
//
//	资源路径（configmaps）  ← mapper 负责，这里手工写
//	URL 拼接                ← rest.Request 负责
//	编解码                  ← serializer.CodecFactory 负责
package main

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
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

	// ───────── 等价于 client.New 内部的关键几步 ─────────

	// ① 编解码器（client.New 里叫 codecs）
	codecs := serializer.NewCodecFactory(scheme)

	// ② 指定 GroupVersion + 序列化器，造一个指向 core/v1 的 RESTClient
	restCfg := rest.CopyConfig(cfg)
	restCfg.GroupVersion = &corev1.SchemeGroupVersion
	restCfg.NegotiatedSerializer = codecs.WithoutConversion()
	// core 组走 /api，其他组走 /apis（controller-runtime 的 createRestConfig 也这么设）
	restCfg.APIPath = "/api"

	rc, err := rest.RESTClientForConfigAndClient(restCfg, httpClient)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	fmt.Println("========== 版本 B：手写 rest.RESTClient ==========")

	// 1) List —— 对应 client.List
	//    resource 名 "configmaps" 平时由 mapper 从 ConfigMap 类型翻译出来
	var list corev1.ConfigMapList
	if err := rc.Get().
		Namespace("test").
		Resource("configmaps").
		Do(ctx).
		Into(&list); err != nil {
		fmt.Println("List 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[List] test 命名空间 ConfigMap 数量: %d\n", len(list.Items))

	// 2) Get —— 对应 client.Get
	var cm corev1.ConfigMap
	if err := rc.Get().
		Namespace("test").
		Resource("configmaps").
		Name("demo-config").
		Do(ctx).
		Into(&cm); err != nil {
		fmt.Println("Get 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[Get ] demo-config data 键数量: %d\n", len(cm.Data))

	// 3) Create —— 对应 client.Create
	tmp := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-client-anatomy", Namespace: "test"},
		Data:       map[string]string{"from": "rest.RESTClient"},
	}
	var created corev1.ConfigMap
	if err := rc.Post().
		Namespace("test").
		Resource("configmaps").
		Body(tmp).
		Do(ctx).
		Into(&created); err != nil {
		fmt.Println("Create 失败:", err)
		os.Exit(1)
	}
	fmt.Printf("[Create] 成功，UID=%s\n", created.UID)

	// 4) Delete —— 对应 client.Delete
	if err := rc.Delete().
		Namespace("test").
		Resource("configmaps").
		Name("demo-client-anatomy").
		Do(ctx).
		Error(); err != nil {
		fmt.Println("Delete 失败:", err)
		os.Exit(1)
	}
	fmt.Println("[Delete] 成功")
}
