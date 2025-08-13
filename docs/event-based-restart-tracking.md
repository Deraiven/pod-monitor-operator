# 基于事件的容器重启追踪方案

## 需求分析

需要精确记录每次容器重启的：
- 重启时间
- 退出码 (exit code)
- 重启原因 (reason)

并且这些记录需要在 Operator 重启后仍然可查询。

## 解决方案

### 方案 1：使用唯一标识的 Gauge 指标（推荐）

为每次重启创建唯一的指标实例：

```go
// 使用包含时间戳或重启次数的标签来区分每次重启
podRestartEvent = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "pod_monitor_container_restart_event",
        Help: "Container restart event with unique identifier",
    },
    []string{
        "namespace",
        "pod",
        "container",
        "reason",
        "exit_code",
        "restart_id",    // 唯一标识符，可以是 "restartCount_timestamp"
        "container_id",  // 容器 ID 也可以作为唯一标识
    },
)

// 记录重启事件
restartID := fmt.Sprintf("%d_%d", cs.RestartCount, time.Now().Unix())
podRestartEvent.With(prometheus.Labels{
    "namespace":    pod.Namespace,
    "pod":         pod.Name,
    "container":   cs.Name,
    "reason":      reason,
    "exit_code":   exitCode,
    "restart_id":  restartID,
    "container_id": cs.ContainerID,
}).Set(float64(finishedAt))
```

### 方案 2：使用 Histogram 记录时间分布

```go
// 使用 Histogram 记录重启时间，同时保留原因和退出码
podRestartTime = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "pod_monitor_container_restart_time",
        Help:    "Container restart times with reasons",
        Buckets: prometheus.DefBuckets,
    },
    []string{"namespace", "pod", "container", "reason", "exit_code"},
)

// 记录时使用 Observe
podRestartTime.With(prometheus.Labels{
    "namespace": pod.Namespace,
    "pod":       pod.Name,
    "container": cs.Name,
    "reason":    reason,
    "exit_code": exitCode,
}).Observe(float64(time.Since(lastState.FinishedAt.Time).Seconds()))
```

### 方案 3：创建 Info 类型的指标

```go
// Info 指标专门用于记录文本信息
podRestartInfo = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Name: "pod_monitor_container_restart_info",
        Help: "Container restart information",
    },
    []string{
        "namespace",
        "pod",
        "container",
        "reason",
        "exit_code",
        "finished_at",     // 终止时间作为标签
        "started_at",      // 启动时间
        "restart_count",   // 重启次数
    },
)

// 使用时
podRestartInfo.With(prometheus.Labels{
    "namespace":     pod.Namespace,
    "pod":          pod.Name,
    "container":    cs.Name,
    "reason":       reason,
    "exit_code":    exitCode,
    "finished_at":  lastState.FinishedAt.Format(time.RFC3339),
    "started_at":   lastState.StartedAt.Format(time.RFC3339),
    "restart_count": fmt.Sprintf("%d", cs.RestartCount),
}).Set(1) // Info 指标通常设置为 1
```

## 推荐实现：组合方案

结合多个指标类型以满足不同查询需求：

```go
var (
    // 1. Counter: 统计总重启次数
    podRestartTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "pod_monitor_container_restart_total",
            Help: "Total number of container restarts",
        },
        []string{"namespace", "pod", "container", "reason"},
    )

    // 2. Gauge: 记录每个重启事件（带唯一标识）
    podRestartEvents = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "pod_monitor_container_restart_events",
            Help: "Individual restart events with timestamps",
        },
        []string{
            "namespace",
            "pod",
            "container",
            "reason",
            "exit_code",
            "restart_count", // 用重启次数作为唯一标识
        },
    )

    // 3. Info: 记录详细的重启信息
    podRestartDetails = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "pod_monitor_container_restart_details",
            Help: "Detailed restart information",
        },
        []string{
            "namespace",
            "pod",
            "container",
            "restart_count",
            "finished_time",
            "reason",
            "exit_code",
            "duration", // 容器运行时长
        },
    )
)

// 实现逻辑
if cs.RestartCount > observedRestarts[containerKey] && cs.LastTerminationState.Terminated != nil {
    lastState := cs.LastTerminationState.Terminated
    
    // 1. 增加计数器
    podRestartTotal.With(prometheus.Labels{
        "namespace": pod.Namespace,
        "pod":       pod.Name,
        "container": cs.Name,
        "reason":    reason,
    }).Inc()
    
    // 2. 记录事件（每个重启次数是唯一的）
    podRestartEvents.With(prometheus.Labels{
        "namespace":     pod.Namespace,
        "pod":          pod.Name,
        "container":    cs.Name,
        "reason":       reason,
        "exit_code":    exitCode,
        "restart_count": fmt.Sprintf("%d", cs.RestartCount),
    }).Set(float64(lastState.FinishedAt.Time.Unix()))
    
    // 3. 记录详细信息
    duration := lastState.FinishedAt.Time.Sub(lastState.StartedAt.Time)
    podRestartDetails.With(prometheus.Labels{
        "namespace":     pod.Namespace,
        "pod":          pod.Name,
        "container":    cs.Name,
        "restart_count": fmt.Sprintf("%d", cs.RestartCount),
        "finished_time": lastState.FinishedAt.Format(time.RFC3339),
        "reason":       reason,
        "exit_code":    exitCode,
        "duration":     duration.String(),
    }).Set(1)
}
```

## Prometheus 查询示例

```promql
# 1. 查看所有重启事件（包括历史）
pod_monitor_container_restart_events{namespace="default"}

# 2. 查看特定 Pod 的重启历史
pod_monitor_container_restart_events{pod="my-app-xxx"}

# 3. 查看 OOMKilled 的所有事件
pod_monitor_container_restart_events{reason="OOMKilled"}

# 4. 查看详细的重启信息
pod_monitor_container_restart_details{pod="my-app-xxx"}

# 5. 按时间排序查看重启
sort_desc(pod_monitor_container_restart_events)

# 6. 查看最近 24 小时的重启
pod_monitor_container_restart_events > (time() - 86400)
```

## 关键点

1. **使用 restart_count 作为唯一标识**：每个容器的重启次数是递增的，可以作为唯一 ID
2. **时间戳作为值**：Gauge 的值存储终止时间的 Unix 时间戳
3. **详细信息作为标签**：将 reason、exit_code 等作为标签，方便查询
4. **不依赖内存状态**：所有数据都持久化在 Prometheus 中

这样即使 Operator 重启，历史的重启记录仍然可以在 Prometheus 中查询到。