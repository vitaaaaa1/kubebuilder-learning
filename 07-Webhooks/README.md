# 07 Webhooks: Kubebuilder 模式

本章节记录如何使用 Kubebuilder 模式开发 Webhooks。

---

## 目录结构

```text
07-Webhooks/
├── cmd/
│   └── main.go                # 入口文件：启动 Manager 和 Webhook Server
├── api/
│   └── v1alpha1/
│       └── demoapp_types.go   # CRD 定义：包含 Webhook Markers
├── webhooks/
│   └── demoapp_webhook.go     # Webhook 逻辑：Defaulter 和 Validator
├── config/
│   └── webhook/
│       └── manifests.yaml     # 自动生成的 Webhook 配置
├── docs/
│   └── kubebuilder-webhook-patterns.md  # 理论文档
├── Makefile                   # 模拟 Kubebuilder 的 make 命令
└── README.md                  # 本文件
```

---

## 学习路径

1.  **阅读 [理论文档](./docs/kubebuilder-webhook-patterns.md)**：了解 Kubebuilder 如何生成 Webhook 代码。
2.  **查看 [API 定义](./api/v1alpha1/demoapp_types.go)**：重点看 `// +kubebuilder:webhook` markers。
3.  **阅读 [Webhook 实现](./webhooks/demoapp_webhook.go)**：
    *   `Default()`: 自动填充 Image 和 Label。
    *   `ValidateCreate()`: 检查 replicas >= 0。
    *   `ValidateUpdate()`: 检查 Image 是否被修改（不可变字段）。
4.  **运行验证**：
    ```bash
    # 本地运行（不带 Webhook，避免证书问题）
    make run
    
    # 或者直接用 go run
    ENABLE_WEBHOOKS=false go run cmd/main.go
    ```

---

## 核心知识点

*   **Markers**: Kubebuilder 使用 markers 告诉 `controller-gen` 生成相应的 YAML。
*   **Defaulter vs Validator**: 一个是修改（Mutating），一个是校验（Validating）。
*   **HTTPS**: K8s 要求 Webhook 必须使用 HTTPS，Kubebuilder 通过 `certmanager` 或 `kustomize` 解决证书问题。
