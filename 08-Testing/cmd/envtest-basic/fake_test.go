package envtestbasic

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// 本文件是"反例集"：它用 fake client 证明 fake client 测不了什么。
//
// fake client 的价值：毫秒级、无依赖、适合测 Reconcile 里的分支逻辑。
// fake client 的盲区，恰好是 Operator 最核心的那几条语义：
//  1. 不分配 UID      → ownerReference 里 UID 为空：看着正常，其实残废
//  2. 没有 GC         → 级联删除永远不会发生
//  3. 不校验 schema   → 非法字段能存进去
//  4. 不跑 webhook    → 默认值与校验全部不生效
//  5. 不实现 RV 乐观锁 → 冲突分支测不出来
//
// 这些用例的断言是"fake 确实做不到"，而不是"代码有 bug"。

func newFakeClient() client.Client {
	return fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
}

// TestFake_OwnerReferenceIsIncomplete 展示第一个幻觉：
// 真实 apiserver 会自动分配 UID；fake 不会。
// 更阴险的是：fake 下 Reconcile **不会报错**，ownerReference 照样被写进去，
// 只是其中的 UID 是空字符串 —— 测试全绿，而写出来的引用在真实集群里是残废的
// （garbage collector 靠 UID 匹配 owner，空 UID 匹配不上，子对象会变孤儿）。
//
// 这就是"假绿"最典型的样子：断言写的是 Name 和 Kind，恰好都能过；
// 唯一有意义的那个字段，fake 给不出来。
func TestFake_OwnerReferenceIsIncomplete(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "fake-no-uid", Namespace: "default"},
		Data:       map[string]string{"owner": "fake"},
	}
	if err := c.Create(ctx, cm); err != nil {
		t.Fatalf("fake 创建 ConfigMap 失败: %v", err)
	}

	var stored corev1.ConfigMap
	if err := c.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &stored); err != nil {
		t.Fatalf("fake 读取 ConfigMap 失败: %v", err)
	}

	// 如果哪天 controller-runtime 的 fake 开始分配 UID，这条会红。
	// 红了说明这一节的说明需要更新 —— 而不是代码有 bug。
	if stored.UID != "" {
		t.Fatalf("fake client 开始分配 UID 了（%q），请更新本节文档的结论", stored.UID)
	}

	r := &ConfigMapReconciler{Client: c, Scheme: clientgoscheme.Scheme}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cm)}); err != nil {
		t.Fatalf("fake 下 Reconcile 居然报错了（与本节预期不符，请复核）: %v", err)
	}

	key := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}
	var secret corev1.Secret
	if err := c.Get(ctx, key, &secret); err != nil {
		t.Fatalf("fake 下子 Secret 未创建: %v", err)
	}

	ref := metav1.GetControllerOf(&secret)
	if ref == nil {
		t.Fatal("fake 下没有写出 ownerReference")
	}
	if ref.Name != cm.Name || ref.Kind != "ConfigMap" {
		t.Fatalf("ownerReference 基础字段不对: %+v", ref)
	}
	t.Log("✅ 表面看一切正常：Kind / Name 都对，测试可以断言通过")

	if ref.UID != "" {
		t.Fatalf("预期 fake 写出空 UID，实际为 %q —— 请复核本节结论", ref.UID)
	}
	t.Log("⚠️  但 ownerReference.UID 是空的：真集群里 garbage collector 靠 UID 匹配 owner")
	t.Log("   → 这份 ownerReference 在真实环境里等于没写，子对象会变成孤儿")
	t.Log("   → 而 fake 不会报错，也不会帮你发现这一点")
}

// TestFake_GarbageCollectionIsFake 展示第二个幻觉：
// 我们手动补一个假 UID，让 ownerReference 写得进去，测试看起来全绿；
// 但删掉 owner 之后，子对象依然躺在那儿 —— 因为 fake 没有 garbage collector。
//
// 换句话说：fake 能证明"我调用了 SetControllerReference"，
// 不能证明"删了父对象子对象会消失"。
func TestFake_GarbageCollectionIsFake(t *testing.T) {
	ctx := context.Background()
	c := newFakeClient()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake-with-uid",
			Namespace: "default",
			// 手动补 UID —— 真实集群里这一步由 apiserver 完成
			UID: "fake-uid-0001",
		},
		Data: map[string]string{"owner": "fake"},
	}
	if err := c.Create(ctx, cm); err != nil {
		t.Fatalf("fake 创建 ConfigMap 失败: %v", err)
	}

	r := &ConfigMapReconciler{Client: c, Scheme: clientgoscheme.Scheme}
	if _, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(cm)}); err != nil {
		t.Fatalf("补了 UID 之后 Reconcile 仍然失败: %v", err)
	}

	key := types.NamespacedName{Name: ChildSecretName(cm.Name), Namespace: cm.Namespace}
	var secret corev1.Secret
	if err := c.Get(ctx, key, &secret); err != nil {
		t.Fatalf("fake 下子 Secret 未创建: %v", err)
	}

	ref := metav1.GetControllerOf(&secret)
	if ref == nil || string(ref.UID) != "fake-uid-0001" {
		t.Fatalf("ownerReference 不正确: %+v", ref)
	}
	t.Log("✅ fake 下 ownerReference 看起来是对的（因为 UID 是我自己填的）")

	if err := c.Delete(ctx, cm); err != nil {
		t.Fatalf("fake 删除 ConfigMap 失败: %v", err)
	}

	var orphan corev1.Secret
	if err := c.Get(ctx, key, &orphan); err != nil {
		t.Fatalf("预期孤儿 Secret 仍然存在（fake 无 GC），实际读取失败: %v", err)
	}
	t.Log("✅ 已确认：fake 不会因为 owner 被删就回收子对象 —— 级联删除在这里测不出来")
}
