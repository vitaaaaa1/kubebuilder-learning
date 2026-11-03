# 子资源 Webhook 实验

## 快速测试（本地验证校验逻辑）

不需要部署到集群，直接运行：

```bash
cd cmd/subresource-webhook
LOCAL_TEST=1 go run main.go
```

这会运行 6 个测试用例，验证：
- ✅ Phase 白名单（只允许 Pending/Running/Failed/Scaling 等）
- ✅ readyReplicas 不能超过 spec.replicas
- ✅ readyReplicas 不能为负数
- ✅ Message 长度不能超过 1024 字符

## 部署到集群

### 1. 生成 TLS 证书

K8s Webhook 必须使用 HTTPS。生成自签名证书：

```bash
# 1. 生成 CA 私钥和证书
openssl req -x509 -newkey rsa:2048 -keyout ca.key -out ca.crt \
  -days 365 -nodes -subj "/CN=scalableapp-webhook-ca"

# 2. 生成 Server 私钥
openssl req -newkey rsa:2048 -keyout server.key -out server.csr \
  -nodes -subj "/CN=scalableapp-webhook-svc.test.svc"

# 3. 用 CA 签署 Server 证书
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out server.crt -days 365 \
  -extfile <(echo "subjectAltName=DNS:scalableapp-webhook-svc.test.svc,DNS:scalableapp-webhook-svc.test.svc.cluster.local")

# 4. 创建 Secret
kubectl create secret generic scalableapp-webhook-certs \
  -n test \
  --from-file=tls.crt=server.crt \
  --from-file=tls.key=server.key

# 5. 将 CA 证书写入 webhook.yaml
CA_BUNDLE=$(cat ca.crt | base64 | tr -d '\n')
sed -i.bak "s/caBundle: \"\"/caBundle: \"$CA_BUNDLE\"/" config/webhook.yaml
```

### 2. 构建并推送镜像

```bash
docker build -t scalableapp-webhook:latest .
kind load docker-image scalableapp-webhook:latest  # 如果用 kind
```

### 3. 部署

```bash
kubectl apply -f config/webhook.yaml
```

### 4. 测试

```bash
# ✅ 合法：Phase=Running
kubectl patch scalableapp my-app --subresource=status --type merge \
  -p '{"status":{"phase":"Running"}}'
# 应该成功

# ❌ 非法：Phase=Hacked（被 Webhook 拒绝）
kubectl patch scalableapp my-app --subresource=status --type merge \
  -p '{"status":{"phase":"Hacked"}}'
# 应该返回：Error: admission webhook denied the request: Phase "Hacked" is not allowed

# ❌ 非法：readyReplicas > spec.replicas
kubectl patch scalableapp my-app --subresource=status --type merge \
  -p '{"status":{"readyReplicas":999}}'
# 应该返回：Error: readyReplicas (999) cannot exceed spec.replicas (2)
```

### 5. 清理

```bash
kubectl delete -f config/webhook.yaml
kubectl delete secret scalableapp-webhook-certs -n test
```
