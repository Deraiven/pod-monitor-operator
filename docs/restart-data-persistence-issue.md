# Pod 重启数据持久化问题分析

## 问题描述

当 Pod Monitor Operator 重启后，之前记录的 Pod 重启信息会丢失，只能看到 Operator 重启后发生的新重启事件。

## 根本原因

### 1. Prometheus Gauge 的工作原理
```go
// 当前使用的是 GaugeVec
podLastTerminationInfo = prometheus.NewGaugeVec(...)

// Gauge 只保存当前值，不是历史时间序列
// 每次 Set() 会覆盖之前的值
```

### 2. 内存状态管理
```go
// 这个 map 在 Operator 重启后会被重新初始化为空
observedRestarts = make(map[string]int32)
```

当 Operator 重启后：
- `observedRestarts` map 为空
- 所有容器的 RestartCount 看起来都是"新的"
- 会重新记录所有已经存在的终止状态

### 3. 数据模型限制
当前设计只记录"最后一次"终止：
- 同一个容器的多次重启会互相覆盖
- 无法查看历史重启记录

## 解决方案

### 方案 1：使用 Counter 而不是 Gauge（推荐）
```go
// 使用 Counter 记录重启次数
podRestartTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "pod_monitor_container_restart_total",
        Help: "Total number of container restarts",
    },
    []string{"namespace", "pod", "container", "reason"},
)

// 每次检测到新重启时增加计数
podRestartTotal.With(prometheus.Labels{
    "namespace": pod.Namespace,
    "pod":       pod.Name,
    "container": cs.Name,
    "reason":    reason,
}).Inc()
```

**优点**：
- Counter 在 Prometheus 中会持久化
- 可以使用 `rate()` 或 `increase()` 函数查看重启频率
- 不会因 Operator 重启而丢失数据

### 方案 2：基于事件的记录
```go
// 创建一个新的 Gauge，每次重启创建新的时间序列
podRestartEvent = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "pod_monitor_container_restart_event",
        Help: "Container restart event with timestamp",
    },
    []string{
        "namespace", 
        "pod", 
        "container", 
        "reason", 
        "exit_code",
        "restart_count", // 添加重启次数作为标签
    },
)

// 记录时包含重启次数
podRestartEvent.With(prometheus.Labels{
    "namespace":     pod.Namespace,
    "pod":          pod.Name,
    "container":    cs.Name,
    "reason":       reason,
    "exit_code":    exitCode,
    "restart_count": fmt.Sprintf("%d", cs.RestartCount),
}).Set(float64(finishedAt))
```

### 方案 3：使用外部存储持久化状态
```go
// 使用 ConfigMap 或 CRD 来持久化 observedRestarts
type PodMonitorState struct {
    ObservedRestarts map[string]int32 `json:"observedRestarts"`
}

// 定期保存状态到 ConfigMap
func (r *PodMonitorReconciler) saveState(ctx context.Context) error {
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "pod-monitor-state",
            Namespace: r.Namespace,
        },
        Data: map[string]string{
            "state": jsonEncode(observedRestarts),
        },
    }
    return r.Update(ctx, cm)
}

// 启动时恢复状态
func (r *PodMonitorReconciler) restoreState(ctx context.Context) error {
    var cm corev1.ConfigMap
    if err := r.Get(ctx, types.NamespacedName{
        Name:      "pod-monitor-state",
        Namespace: r.Namespace,
    }, &cm); err == nil {
        jsonDecode(cm.Data["state"], &observedRestarts)
    }
    return nil
}
```

## 推荐实现

结合方案 1 和 2，同时使用 Counter 和事件记录：

```go
var (
    // Counter 用于统计总重启次数
    podRestartTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "pod_monitor_container_restart_total",
            Help: "Total number of container restarts by reason",
        },
        []string{"namespace", "pod", "container", "reason"},
    )
    
    // Gauge 用于记录最后一次重启的详细信息
    podLastRestartInfo = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "pod_monitor_container_last_restart_timestamp",
            Help: "Timestamp of the last container restart",
        },
        []string{"namespace", "pod", "container", "reason", "exit_code"},
    )
)

// 使用时
if cs.RestartCount > observedRestarts[containerKey] {
    // 增加 Counter
    podRestartTotal.With(prometheus.Labels{
        "namespace": pod.Namespace,
        "pod":       pod.Name,
        "container": cs.Name,
        "reason":    reason,
    }).Inc()
    
    // 更新最后一次重启信息
    podLastRestartInfo.With(prometheus.Labels{
        "namespace": pod.Namespace,
        "pod":       pod.Name,
        "container": cs.Name,
        "reason":    reason,
        "exit_code": exitCode,
    }).Set(float64(finishedAt))
}
```

## Prometheus 查询示例

使用 Counter 后可以这样查询：

```promql
# 查看过去1小时的重启次数
increase(pod_monitor_container_restart_total[1h])

# 查看重启率
rate(pod_monitor_container_restart_total[5m])

# 按原因分组的重启统计
sum by (reason) (increase(pod_monitor_container_restart_total[24h]))
```

## 总结

当前实现的主要问题是：
1. 使用 Gauge 而不是 Counter
2. 依赖内存状态而非 Prometheus 持久化
3. 数据模型只支持"最后一次"而非历史记录

建议采用 Counter + Gauge 的组合方案，这样既能统计总数，又能查看最新状态，且不会因为 Operator 重启而丢失数据。