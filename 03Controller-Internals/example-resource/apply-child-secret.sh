#!/bin/bash
# 自动获取 ConfigMap UID 并创建带有 OwnerReference 的 Secret
# 用法: ./apply-child-secret.sh

NAMESPACE="test"
CONFIGMAP_NAME="my-owner"

echo "🔍 正在查找 ConfigMap '$CONFIGMAP_NAME' 在 namespace '$NAMESPACE' 中的 UID..."
CM_UID=$(kubectl get configmap "$CONFIGMAP_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.uid}' 2>/dev/null)

if [ -z "$CM_UID" ]; then
  echo "❌ 未找到 ConfigMap '$CONFIGMAP_NAME'。请先创建它:"
  echo "   kubectl create configmap $CONFIGMAP_NAME -n $NAMESPACE"
  exit 1
fi

echo "✅ 找到 UID: $CM_UID"

echo "📝 正在创建带有 OwnerReference 的 Secret..."
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: Secret
metadata:
  name: my-child-secret
  namespace: $NAMESPACE
  ownerReferences:
  - apiVersion: v1
    kind: ConfigMap
    name: $CONFIGMAP_NAME
    uid: $CM_UID
type: Opaque
data:
  key: dmFsdWU=
EOF

echo "✨ Secret 已创建！请观察 Controller 日志是否触发 Reconcile。"
