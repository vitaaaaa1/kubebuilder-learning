# kubebuilder-learning

本项目用于学习 Kubernetes Operator 开发，基于 [kubebuilder](https://book.kubebuilder.io/) 框架。

## 项目信息

- **Domain**: `vitaaaaa.icu`
- **Go Module**: `github.com/vitaaaaa/kubebuilder-learning`
- **Kubebuilder Version**: 4.16.0
- **Layout**: `go.kubebuilder.io/v4`
- **Go Version**: 1.26.0

## 目录结构

```
├── cmd/
│   └── main.go              # 控制器入口，启动 manager
├── config/                  # Kubernetes 资源配置（kustomize）
│   ├── default/             # 默认 kustomize 配置
│   ├── manager/             # Controller Deployment 配置
│   ├── network-policy/      # 网络策略
│   ├── prometheus/          # Prometheus 监控配置
│   └── rbac/                # RBAC 角色和权限配置
├── hack/                    # 辅助脚本
├── test/                    # 测试代码
├── PROJECT                  # kubebuilder 项目元数据（自动生成）
├── Makefile                 # 构建、部署、测试命令
├── Dockerfile               # 容器镜像构建文件
├── go.mod                   # Go 依赖管理
└── README.md                # 项目说明文档
```

## 命令详解

### kubebuilder init

```bash
kubebuilder init --domain vitaaaaa.icu --repo github.com/vitaaaaa/kubebuilder-learning
```

**参数解释：**

| 参数 | 值 | 作用 |
|------|-----|------|
| `--domain` | `vitaaaaa.icu` | 设置 API group 的基础域名 |
| `--repo` | `github.com/vitaaaaa/kubebuilder-learning` | 设置 Go module 路径 |

**`--domain` 的作用：**

将来创建 API 时，CRD 的完整 group 会是 `<group>.<domain>`。

比如创建了 `--group demo` 的 API，最终的 API group 就是：
```yaml
apiVersion: demo.vitaaaaa.icu/v1alpha1
kind: HelloWorld
```

**`--repo` 的作用：**

- 设置 `go.mod` 中的 module 名称
- 决定所有 import 路径的前缀
- 通常与 Git 仓库路径一致

```go
// import 路径示例
import (
    demoapiv1alpha1 "github.com/vitaaaaa/kubebuilder-learning/api/v1alpha1"
)
```

**init 做了什么？**

1. 生成 `PROJECT` 文件（记录项目元信息）
2. 生成 `Makefile`（构建、部署命令）
3. 生成 `go.mod`（Go module 配置）
4. 生成 `cmd/main.go`（controller 入口）
5. 生成 `config/` 目录（kustomize 配置）
6. 生成 `Dockerfile`
7. 初始化 Go 代码框架

---

### kubebuilder create api

```bash
kubebuilder create api --group demo --version v1alpha1 --kind HelloWorld
```

**参数解释：**

| 参数 | 值 | 作用 |
|------|-----|------|
| `--group` | `demo` | API group 名称 |
| `--version` | `v1alpha1` | API 版本 |
| `--kind` | `HelloWorld` | 资源类型名称 |

**三个参数共同定义了 CRD 的身份：**

```yaml
apiVersion: <group>.<domain>/<version>
kind: <Kind>

# 实际效果：
apiVersion: demo.vitaaaaa.icu/v1alpha1
kind: HelloWorld
```

**`--group`（API group）**
- 逻辑上相关的 API 的分组
- 类似于 Kubernetes 内置的 `apps`、`batch`、`networking.k8s.io`
- 通常用项目/团队/功能名命名

**`--version`（API version）**
- 表示 API 的成熟度
- 常见版本：`v1alpha1`（实验性）、`v1beta1`（较稳定）、`v1`（稳定）

**`--kind`（资源类型）**
- 资源的名称，必须大写字母开头
- 使用 PascalCase（驼峰命名）
- 类似于内置的 `Pod`、`Deployment`、`Service`

**create api 做了什么？（两个问题都选 y）**

1. **生成 API 类型定义**：`api/v1alpha1/helloworld_types.go`
   - `HelloWorld` 结构体
   - `HelloWorldSpec`（Spec 定义）
   - `HelloWorldStatus`（Status 定义）

2. **生成 Controller 骨架**：`internal/controller/helloworld_controller.go`
   - `HelloWorldReconciler` 结构体
   - `Reconcile()` 方法（你需要实现业务逻辑）
   - `SetupWithManager()` 方法

3. **生成测试文件**：
   - `internal/controller/helloworld_controller_test.go`
   - `internal/controller/suite_test.go`

4. **生成 kustomize 配置**：
   - `config/crd/` 目录
   - `config/rbac/` 目录
   - `config/samples/` 目录（包含示例 CR）

5. **注册 API**：
   - 在 `api/v1alpha1/groupversion_info.go` 中注册 group/version
   - 在 `cmd/main.go` 中添加对 API 的引用

---

### 其他常用 create 命令

**只创建 Controller（不创建新 API）**
```bash
kubebuilder create api --group demo --version v1alpha1 --kind AnotherResource --controller=true --resource=false
```

**只创建 Resource（不创建 Controller）**
```bash
kubebuilder create api --group demo --version v1alpha1 --kind AnotherResource --controller=false --resource=true
```

**创建 Webhook**
```bash
kubebuilder create webhook --group demo --version v1alpha1 --kind HelloWorld --defaulting --programmatic-validation
```

## 常用命令

### 安装 CRD 到集群

```bash
make install
```

### 本地运行控制器

```bash
make run
```

### 构建并部署到集群

```bash
make docker-build docker-push IMG=<your-registry>/<your-image>:<tag>
make deploy IMG=<your-registry>/<your-image>:<tag>
```

### 运行测试

```bash
make test
```

### 生成 CRD manifests

```bash
make manifests
```

## 完整学习流程

```bash
# 1. 初始化项目
kubebuilder init --domain vitaaaaa.icu --repo github.com/vitaaaaa/kubebuilder-learning

# 2. 创建第一个 API
kubebuilder create api --group demo --version v1alpha1 --kind HelloWorld

# 3. 编辑 api/v1alpha1/helloworld_types.go 定义 Spec 和 Status

# 4. 编辑 internal/controller/helloworld_controller.go 实现 Reconcile 逻辑

# 5. 生成 CRD manifests
make manifests

# 6. 安装 CRD 到集群
make install

# 7. 本地运行 controller
make run

# 8. 创建 HelloWorld 实例测试
kubectl apply -f config/samples/demo_v1alpha1_helloworld.yaml
```

## 学习路径

- [ ] 项目初始化（已完成）
- [ ] 创建第一个 CRD API
- [ ] 定义 Spec 和 Status
- [ ] 实现 Controller Reconcile 逻辑
- [ ] 本地测试和调试
- [ ] 部署到 Kubernetes 集群

## 核心依赖

- `sigs.k8s.io/controller-runtime`: Kubernetes controller 核心运行时库
- `k8s.io/apimachinery`: Kubernetes API 对象类型
- `k8s.io/client-go`: Kubernetes API 客户端
- `github.com/onsi/ginkgo/v2`: BDD 测试框架

## 参考资源

- [Kubebuilder Book](https://book.kubebuilder.io/)
- [Controller Runtime Docs](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- [Kubernetes API Reference](https://kubernetes.io/docs/reference/)
