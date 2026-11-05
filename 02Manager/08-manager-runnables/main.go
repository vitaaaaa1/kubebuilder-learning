// 观察 Manager 的 6 组 runnables：谁先启动、谁先停止、优雅关闭超时会怎样
//
//	go run .                       # 正常运行 6 秒后退出
//	go run . -hold 10s             # 运行 10 秒
//	go run . -slow 5s -grace 2s    # 某个组件停止很慢，触发优雅关闭超时
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var log = ctrl.Log.WithName("demo")

func main() {
	hold := flag.Duration("hold", 6*time.Second, "运行多久后主动退出")
	grace := flag.Duration("grace", 30*time.Second, "优雅关闭超时")
	slow := flag.Duration("slow", 0, "leader 组组件收到停止信号后再拖多久")

	logOpts := zap.Options{Development: true}
	logOpts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&logOpts)))

	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		Metrics:                 metricsserver.Options{BindAddress: "0"},
		Cache:                   cache.Options{DefaultNamespaces: map[string]cache.Config{"test": {}}},
		GracefulShutdownTimeout: grace,
	})
	if err != nil {
		log.Error(err, "创建 Manager 失败")
		os.Exit(1)
	}

	// ① Caches 组：实现 GetCache() 的 runnable 会进这里
	if err := mgr.Add(&cacheGroupRunnable{name: "caches组", mgr: mgr}); err != nil {
		panic(err)
	}

	// ② Others 组：NeedLeaderElection() == false
	if err := mgr.Add(&otherRunnable{name: "others组", slow: *slow}); err != nil {
		panic(err)
	}

	// ③ Warmup 组 + ④ LeaderElection 组：
	//    实现 Warmup() 会进 Warmup；NeedLeaderElection() == true 会进 LeaderElection
	if err := mgr.Add(&warmupRunnable{name: "warmup组/leader组", slow: *slow}); err != nil {
		panic(err)
	}

	// ⑤ LeaderElection 组：普通 Runnable 默认进这里（向后兼容）
	if err := mgr.Add(&plainRunnable{name: "leader组"}); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), *hold)
	defer cancel()

	fmt.Println("========== mgr.Start ==========")
	if err := mgr.Start(ctx); err != nil {
		fmt.Printf("========== mgr.Start 返回错误: %v ==========\n", err)
		return
	}
	fmt.Println("========== mgr.Start 正常返回 ==========")
}

// ───────── 各种 runnable ─────────

type cacheGroupRunnable struct {
	name string
	mgr  manager.Manager
}

func (r *cacheGroupRunnable) GetCache() cache.Cache { return r.mgr.GetCache() }

func (r *cacheGroupRunnable) Start(ctx context.Context) error {
	log.Info("START", "group", r.name)
	<-ctx.Done()
	log.Info("STOP ", "group", r.name)
	return nil
}

type otherRunnable struct {
	name string
	slow time.Duration
}

func (r *otherRunnable) NeedLeaderElection() bool { return false }

func (r *otherRunnable) Start(ctx context.Context) error {
	log.Info("START", "group", r.name)
	<-ctx.Done()
	log.Info("STOP ", "group", r.name)
	return nil
}

type warmupRunnable struct {
	name string
	slow time.Duration
}

func (r *warmupRunnable) NeedLeaderElection() bool { return true }

func (r *warmupRunnable) Warmup(ctx context.Context) error {
	log.Info("WARMUP（在成为 leader 之前执行）", "group", r.name)
	return nil
}

func (r *warmupRunnable) Start(ctx context.Context) error {
	log.Info("START", "group", r.name)
	<-ctx.Done()
	if r.slow > 0 {
		log.Info("停止中，故意变慢", "group", r.name, "耗时", r.slow)
		time.Sleep(r.slow)
	}
	log.Info("STOP ", "group", r.name)
	return nil
}

type plainRunnable struct {
	name string
}

func (r *plainRunnable) Start(ctx context.Context) error {
	log.Info("START", "group", r.name)
	<-ctx.Done()
	log.Info("STOP ", "group", r.name)
	return nil
}
