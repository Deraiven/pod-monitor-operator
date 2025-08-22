# 并发安全和内存泄漏修复说明

## 修复的问题

### 1. 并发问题：Race Condition

**问题描述**：
- `observedRestarts` map 被多个 goroutine 同时访问
- controller-runtime 会并行处理不同 Pod 的 reconciliation
- 没有锁保护会导致数据竞争和潜在的 panic

**修复方案**：
```go
// 添加读写锁
var restartsMutex sync.RWMutex

// 读取时使用读锁
restartsMutex.RLock()
observedCount := observedRestarts[containerKey]
restartsMutex.RUnlock()

// 写入时使用写锁
restartsMutex.Lock()
observedRestarts[containerKey] = cs.RestartCount
restartsMutex.Unlock()
```

### 2. 内存泄漏问题

**问题描述**：
- Pod 删除后，`observedRestarts` map 中的条目永不清理
- 在高流失率的集群中会导致内存无限增长
- 最终可能导致 OOM

**修复方案**：
```go
// Pod 删除时清理内存
if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
    if client.IgnoreNotFound(err) == nil {
        // Pod 已删除，清理相关数据
        prefix := fmt.Sprintf("%s/%s/", req.Namespace, req.Name)
        restartsMutex.Lock()
        for key := range observedRestarts {
            if strings.HasPrefix(key, prefix) {
                delete(observedRestarts, key)
            }
        }
        restartsMutex.Unlock()
    }
}
```

## 测试验证

### 1. 并发安全测试

使用 Go 的 race detector：
```bash
go test -race ./internal/controller/...
```

### 2. 内存泄漏测试

创建和删除大量 Pod，监控内存使用：
```bash
# 创建测试 Pod
for i in {1..100}; do
    kubectl run test-pod-$i --image=busybox --restart=Always -- sh -c "exit 1"
done

# 等待一些重启
sleep 60

# 删除所有测试 Pod
kubectl delete pods -l run=test-pod

# 检查 operator 内存使用
kubectl top pod -n pod-monitor-system
```

## 性能影响

1. **读写锁的选择**：
   - 使用 `sync.RWMutex` 而不是 `sync.Mutex`
   - 允许多个并发读取，只在写入时独占
   - 最小化性能影响

2. **清理操作的效率**：
   - 只在 Pod 删除时执行清理
   - 使用前缀匹配快速定位需要删除的条目
   - O(n) 复杂度，但 n 通常很小（每个 Pod 的容器数）

## 重要说明

- **Prometheus 数据不受影响**：清理只影响内存中的辅助 map，不影响已记录的指标
- **线程安全**：所有对 `observedRestarts` 的访问都受锁保护
- **向后兼容**：这些修复不改变外部行为，只修复内部实现问题