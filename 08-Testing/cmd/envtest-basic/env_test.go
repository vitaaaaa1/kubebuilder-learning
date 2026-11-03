package envtestbasic

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// uniqueName 生成"本次测试专属"的对象名。
//
// 为什么需要：整包共用一个 apiserver，用例之间必须互不干扰。
// 直接写死 "demo" 之类的名字，第二次跑或并行跑就会撞名。
//
// 注意：K8s 对象名必须是 RFC1123 subdomain —— 只允许小写字母、数字、'-'、'.'，
// 且首尾必须是字母或数字。而 Go 的测试名里带下划线（TestEnv_Creates...），
// 直接拿来当对象名会被 apiserver 以 "metadata.name: Invalid value" 拒绝。
// 这不是 envtest 挑剔，是 fake client 帮不了你的地方之一：它不校验名字。
func uniqueName(t *testing.T) string {
	t.Helper()

	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(t.Name()) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case !prevDash:
			b.WriteByte('-') // 斜杠、下划线、空格统统折叠成单个 '-'
			prevDash = true
		}
	}
	return fmt.Sprintf("%s-%d", strings.Trim(b.String(), "-"), time.Now().UnixNano())
}

func newReconciler() *ConfigMapReconciler {
	return &ConfigMapReconciler{Client: k8sClient, Scheme: testScheme}
}

// mustCreateConfigMap 建一个真实的 ConfigMap，并断言 apiserver 确实分配了 UID。
// 这个 UID 断言看似多余，其实是本章的核心：ownerReference 靠它工作。
func mustCreateConfigMap(t *testing.T, ctx context.Context, name string) *corev1.ConfigMap {
	t.Helper()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Data:       map[string]string{"owner": "envtest"},
	}
	if err := k8sClient.Create(ctx, cm); err != nil {
		t.Fatalf("创建 ConfigMap %q 失败: %v", name, err)
	}
	if cm.UID == "" {
		t.Fatal("apiserver 没有给 ConfigMap 分配 UID —— envtest 环境异常")
	}
	return cm
}

// TestEnv_CreatesSecretWithOwnerReference 是整个 08 章的第一条真断言。
// 它验证三件事：子对象被创建、内容正确、ownerReference 指向真实的 UID。
func TestEnv_CreatesSecretWithOwnerReference(t *testing.T) {
	ctx := context.Background()
	cm := mustCreateConfigMap(t, ctx, uniqueName(t))

	r := newReconciler()
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cm)}); err != nil {
		t.Fatalf("Reconcile 返回错误: %v", err)
	}

	var secret corev1.Secret
	key := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}
	if err := apiReader.Get(ctx, key, &secret); err != nil {
		t.Fatalf("期望子 Secret %q 存在，实际读取失败: %v", key.String(), err)
	}

	if got := string(secret.Data[dataKey]); got != cm.Name {
		t.Errorf("Secret data[%q] = %q, 期望 %q", dataKey, got, cm.Name)
	}

	ref := metav1.GetControllerOf(&secret)
	if ref == nil {
		t.Fatal("Secret 上没有 controller ownerReference —— 级联删除永远不会生效")
	}
	if ref.Kind != "ConfigMap" || ref.Name != cm.Name {
		t.Errorf("ownerReference = %s/%s, 期望 ConfigMap/%s", ref.Kind, ref.Name, cm.Name)
	}
	// 这条断言在 fake client 下无法成立（fake 不分配 UID），只能在这里做。
	if ref.UID != cm.UID {
		t.Errorf("ownerReference.UID = %q, 期望 %q（必须与源对象 UID 一致）", ref.UID, cm.UID)
	}
}

// TestEnv_Idempotent 证明"重复 Reconcile 不产生副作用"。
// 判据不是"有没有报错"，而是 ResourceVersion 有没有变化 —— RV 变了就说明发生了写入。
func TestEnv_Idempotent(t *testing.T) {
	ctx := context.Background()
	cm := mustCreateConfigMap(t, ctx, uniqueName(t))

	r := newReconciler()
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cm)}
	key := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("第一次 Reconcile 失败: %v", err)
	}

	var first corev1.Secret
	if err := apiReader.Get(ctx, key, &first); err != nil {
		t.Fatalf("读取子 Secret 失败: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := r.Reconcile(ctx, req); err != nil {
			t.Fatalf("第 %d 次重复 Reconcile 失败: %v", i+2, err)
		}
	}

	var second corev1.Secret
	if err := apiReader.Get(ctx, key, &second); err != nil {
		t.Fatalf("读取子 Secret 失败: %v", err)
	}

	if first.ResourceVersion != second.ResourceVersion {
		t.Errorf("重复 Reconcile 改写了对象: RV %q → %q（说明每轮都在无谓地 Update）",
			first.ResourceVersion, second.ResourceVersion)
	}
}

// TestEnv_RepairsDrift 验证"自愈"：有人手改了子对象，下一轮 Reconcile 应把它改回来。
func TestEnv_RepairsDrift(t *testing.T) {
	ctx := context.Background()
	cm := mustCreateConfigMap(t, ctx, uniqueName(t))

	r := newReconciler()
	req := ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cm)}
	key := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("首次 Reconcile 失败: %v", err)
	}

	var drifted corev1.Secret
	if err := apiReader.Get(ctx, key, &drifted); err != nil {
		t.Fatalf("读取子 Secret 失败: %v", err)
	}
	drifted.Data[dataKey] = []byte("tampered")
	// 注意：apiReader 的类型是 client.Reader，只读；写操作必须走 k8sClient。
	if err := k8sClient.Update(ctx, &drifted); err != nil {
		t.Fatalf("模拟漂移写入失败: %v", err)
	}
	rvAfterDrift := drifted.ResourceVersion

	if _, err := r.Reconcile(ctx, req); err != nil {
		t.Fatalf("修复 Reconcile 失败: %v", err)
	}

	var repaired corev1.Secret
	if err := apiReader.Get(ctx, key, &repaired); err != nil {
		t.Fatalf("读取修复后的 Secret 失败: %v", err)
	}
	if got := string(repaired.Data[dataKey]); got != cm.Name {
		t.Errorf("漂移未被修复: data[%q] = %q, 期望 %q", dataKey, got, cm.Name)
	}
	if repaired.ResourceVersion == rvAfterDrift {
		t.Error("修复居然没有产生写入（RV 未变化），说明修复分支没被执行")
	}
}

// TestEnv_IgnoresNotFound 覆盖"对象已被删除"这个高频分支。
// 期望：返回 nil 且不重试。返回 NotFound 错误会让控制器无限重试一个不存在的东西。
func TestEnv_IgnoresNotFound(t *testing.T) {
	ctx := context.Background()
	r := newReconciler()

	_, err := r.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{Name: "definitely-not-exist", Namespace: "default"},
	})
	if err != nil {
		t.Fatalf("对象不存在时必须返回 nil，实际返回: %v", err)
	}
}

// TestEnv_NoGarbageCollector 是本节的"边界声明"。
//
// 删掉 owner 之后，子对象并不会消失 —— 因为 envtest 只跑 apiserver+etcd，
// 没有 kube-controller-manager，也就没有 garbage collector。
// 所以"级联删除"这件事在 envtest 里只能验证 ownerReference 写对了，
// 真正的删除行为必须到 kind / 真实集群上验证（12 章）。
func TestEnv_NoGarbageCollector(t *testing.T) {
	ctx := context.Background()
	cm := mustCreateConfigMap(t, ctx, uniqueName(t))

	r := newReconciler()
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cm)}); err != nil {
		t.Fatalf("Reconcile 失败: %v", err)
	}

	key := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}
	if err := k8sClient.Delete(ctx, cm); err != nil {
		t.Fatalf("删除 ConfigMap 失败: %v", err)
	}

	var secret corev1.Secret
	if err := apiReader.Get(ctx, key, &secret); err != nil {
		t.Fatalf("预期子对象仍然存在（envtest 无 GC），实际读取失败: %v", err)
	}
	t.Log("✅ 已确认：envtest 没有 garbage collector，owner 删除后子对象不会被回收")
}
