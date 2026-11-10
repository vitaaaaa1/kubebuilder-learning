// Package shared 放两个版本共用的“计数器自增”逻辑。
package shared

import (
	"context"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const Name = "demo-conflict"

func Key() types.NamespacedName {
	return types.NamespacedName{Namespace: "test", Name: Name}
}

// Ensure 确保对象存在，并把计数归零。
func Ensure(ctx context.Context, c client.Client) error {
	cm := &corev1.ConfigMap{}
	err := c.Get(ctx, Key(), cm)
	if apierrors.IsNotFound(err) {
		return c.Create(ctx, &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: Name, Namespace: "test"},
			Data:       map[string]string{"count": "0"},
		})
	}
	if err != nil {
		return err
	}
	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data["count"] = "0"
	return c.Update(ctx, cm)
}

// BumpNaive：读 → 改 → 写，冲突直接返回错误。
func BumpNaive(ctx context.Context, c client.Client) error {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, Key(), cm); err != nil {
		return err
	}
	n, _ := strconv.Atoi(cm.Data["count"])
	cm.Data["count"] = strconv.Itoa(n + 1)
	return c.Update(ctx, cm)
}

// BumpWithRetry：冲突时重新 Get → 改 → 写，最多重试 backoff.Steps 次。
// 返回过程中遇到的冲突次数。
func BumpWithRetry(ctx context.Context, c client.Client, backoff wait.Backoff) (int, error) {
	conflicts := 0
	err := retry.RetryOnConflict(backoff, func() error {
		cm := &corev1.ConfigMap{}
		if err := c.Get(ctx, Key(), cm); err != nil {
			return err
		}
		n, _ := strconv.Atoi(cm.Data["count"])
		cm.Data["count"] = strconv.Itoa(n + 1)

		err := c.Update(ctx, cm)
		if apierrors.IsConflict(err) {
			conflicts++
		}
		return err
	})
	return conflicts, err
}

// Current 读回当前计数。
func Current(ctx context.Context, c client.Client) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := c.Get(ctx, Key(), cm); err != nil {
		return "", err
	}
	return cm.Data["count"], nil
}
