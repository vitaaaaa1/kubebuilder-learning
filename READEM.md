Controller 的四大核心组件

    要让一个 Controller 跑起来，代码里其实发生了四件大事：

     1. Scheme (翻译官): 注册 Go 结构体，让程序能读懂 API Server 的 JSON。
     2. Manager (大管家): 建立集群连接，维护本地缓存 (Cache)，管理生命周期。
     3. Controller (执行者): 定义“我要监听什么”、“过滤什么”、“怎么处理”。
     4. Start (启动引擎): 启动所有组件，开始工作。