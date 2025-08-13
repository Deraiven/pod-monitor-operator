# 持久化重启监控指南

## 概述

Pod Monitor Operator 现在支持持久化的容器重启监控。即使 Operator 重启，历史的重启记录（包括时间、原因、退出码）仍然可以查询。

## 新增指标

### 1. pod_monitor_container_restart_total (Counter)

**描述**: 容器重启的总次数（累计值）

**标签**:
- `namespace`: Pod 命名空间
- `pod`: Pod 名称
- `container`: 容器名称  
- `reason`: 终止原因

**特点**:
- 持久化在 Prometheus 中
- Operator 重启不会丢失
- 只增不减

### 2. pod_monitor_container_restart_events (Gauge)

**描述**: 每次重启的独立事件记录

**标签**:
- `namespace`: Pod 命名空间
- `pod`: Pod 名称
- `container`: 容器名称
- `reason`: 终止原因（如 OOMKilled, Error, Completed）
- `exit_code`: 退出码（如 137, 1, 0）
- `restart_count`: 重启次数（作为唯一标识符）

**值**: Unix 时间戳（终止时间）

**特点**:
- 每次重启创建新的时间序列
- 保留完整的历史记录
- 可以精确查询每次重启的信息

### 3. pod_monitor_container_last_termination_info (Gauge)

**描述**: 最后一次终止的信息（保持向后兼容）

## 查询示例

### 查看所有历史重启事件

```promql
# 查看特定 Pod 的所有重启历史
pod_monitor_container_restart_events{pod="my-app-xxx"}

# 结果示例：
# {namespace="default", pod="my-app-xxx", container="app", reason="OOMKilled", exit_code="137", restart_count="1"} 1609459200
# {namespace="default", pod="my-app-xxx", container="app", reason="Error", exit_code="1", restart_count="2"} 1609462800
# {namespace="default", pod="my-app-xxx", container="app", reason="OOMKilled", exit_code="137", restart_count="3"} 1609466400
```

### 查看重启总次数

```promql
# 查看每个容器的总重启次数
pod_monitor_container_restart_total

# 查看特定原因的重启次数
pod_monitor_container_restart_total{reason="OOMKilled"}

# 查看过去1小时的重启增量
increase(pod_monitor_container_restart_total[1h])
```

### 按时间过滤重启事件

```promql
# 查看最近24小时的重启事件
pod_monitor_container_restart_events > (time() - 86400)

# 查看特定时间范围的重启
pod_monitor_container_restart_events > 1609459200 < 1609545600
```

### 统计分析

```promql
# 按原因分组统计重启次数
sum by (reason) (pod_monitor_container_restart_total)

# 查看重启最频繁的 Pod
topk(10, sum by (namespace, pod) (rate(pod_monitor_container_restart_total[1h])))

# 查看 OOMKilled 的容器列表
pod_monitor_container_restart_events{reason="OOMKilled"}
```

### 时间转换

```promql
# 将 Unix 时间戳转换为可读时间（在 Grafana 中）
pod_monitor_container_restart_events{pod="my-app-xxx"}

# 在 Grafana 中可以使用时间格式化：
# Value mappings: 
# - Type: Value to text
# - Value: $__value
# - Display text: {{ $__value | date "2006-01-02 15:04:05" }}
```

## Grafana Dashboard 示例

### Panel 1: 重启历史表格

```yaml
Panel Type: Table
Query: pod_monitor_container_restart_events{namespace="$namespace", pod=~"$pod"}
Transform: 
  - Labels to fields
  - Organize fields:
    - namespace
    - pod  
    - container
    - restart_count
    - reason
    - exit_code
    - Value (as Timestamp)
```

### Panel 2: 重启趋势图

```yaml
Panel Type: Graph
Query: increase(pod_monitor_container_restart_total[$__interval])
Legend: {{namespace}}/{{pod}}/{{container}} - {{reason}}
```

### Panel 3: 重启原因饼图

```yaml
Panel Type: Pie Chart
Query: sum by (reason) (increase(pod_monitor_container_restart_total[24h]))
```

## 重要说明

1. **数据持久性**: 新的指标会持久化在 Prometheus 中，不受 Operator 重启影响

2. **历史保留**: 
   - `podRestartTotal`: 永久累计
   - `podRestartEvents`: 根据 Prometheus 保留策略（默认 15 天）

3. **性能考虑**: 
   - 每次重启创建新的时间序列
   - 在高重启率环境中可能产生较多时间序列
   - 建议配置合理的 Prometheus 保留策略

4. **向后兼容**: 
   - 保留了原有的 `pod_monitor_container_last_termination_info` 指标
   - 现有的 Dashboard 和告警规则无需修改

## 故障排查

### 问题：看不到历史重启记录

检查：
1. Prometheus 是否正常抓取指标：`up{job="pod-monitor"}`
2. 指标是否注册：`curl http://pod-monitor:8080/metrics | grep restart`
3. 时间范围是否正确

### 问题：重启次数不准确

原因：
- Pod 被重建（而非容器重启）会重置 restart_count
- 建议同时查看 `pod_monitor_container_restart_total` 获取准确的累计值

### 问题：内存使用增长

如果时间序列过多：
1. 减少标签基数（如去掉 exit_code）
2. 配置 Prometheus 记录规则聚合数据
3. 调整保留策略