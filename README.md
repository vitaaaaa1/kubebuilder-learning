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

## 常用命令

### 初始化项目

```bash
kubebuilder init --domain vitaaaaa.icu --repo github.com/vitaaaaa/kubebuilder-learning
```

### 创建 API

```bash
kubebuilder create api --group <group> --version <version> --kind <Kind>
```

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
