# Pod Monitor Operator 功能总结

## 项目概述

Pod Monitor Operator 是一个 Kubernetes Operator，用于监控集群中的 Pod 状态和证书过期情况，并通过 Prometheus 指标暴露相关信息。

## 主要功能

### 1. Pod 容器重启监控

监控所有 Pod 中容器的重启情况，并记录最后一次终止的详细信息。

**Prometheus 指标：**
- `pod_monitor_container_last_termination_info` - 容器最后一次终止的信息
  - 标签：`namespace`, `pod`, `container`, `reason`, `exit_code`
  - 值：终止时间的 Unix 时间戳

**功能特点：**
- 自动检测容器重启
- 记录终止原因（如 OOMKilled）
- 记录退出码
- 实时监控所有命名空间的 Pod

### 2. Linkerd 证书过期监控

专门监控 Linkerd 服务网格的身份证书过期情况。

**监控目标：**
- 命名空间：`linkerd`
- Secret 名称：`linkerd-identity-issuer`
- 支持的证书文件：`crt.pem`, `ca.crt`, `issuer.crt`, `ca.pem`, `issuer.pem`

**Prometheus 指标：**
- `pod_monitor_certificate_expiration_timestamp_seconds` - 证书过期时间戳
  - 标签：`namespace`, `secret_name`, `cert_type`
  - 值：过期时间的 Unix 时间戳（秒）

- `pod_monitor_certificate_days_until_expiration` - 证书剩余有效天数
  - 标签：`namespace`, `secret_name`, `cert_type`
  - 值：距离过期的天数

**功能特点：**
- 支持 PEM 格式的 X.509 证书
- 自动解析证书有效期
- 每小时定期检查
- Secret 更新时实时响应

## 技术架构

### 使用的技术栈
- **语言**: Go 1.23
- **框架**: Kubebuilder (controller-runtime v0.20.4)
- **监控**: Prometheus Client
- **部署**: Kubernetes CRD + Controller

### 核心组件
1. **Controller**: 处理 Pod 和 Secret 事件的主控制器
2. **CRD**: PodMonitor 自定义资源定义（当前未使用）
3. **RBAC**: 集群角色和权限配置
4. **Metrics**: Prometheus 指标暴露端点

## 部署和使用

### 部署方式
1. **Kustomize**: 使用 `make deploy` 部署
2. **Helm Chart**: 支持 Helm 部署（`pod-monitor/` 目录）

### 监控告警示例

```yaml
# Prometheus 告警规则示例
groups:
- name: pod-monitor-alerts
  rules:
  # 容器频繁重启告警
  - alert: ContainerRestartingTooOften
    expr: changes(pod_monitor_container_last_termination_info[1h]) > 5
    annotations:
      summary: "容器频繁重启"
      description: "容器 {{ $labels.container }} 在过去1小时内重启超过5次"
  
  # 证书即将过期告警
  - alert: LinkerdCertificateExpiringSoon
    expr: pod_monitor_certificate_days_until_expiration{namespace="linkerd"} < 30
    annotations:
      summary: "Linkerd 证书即将过期"
      description: "证书 {{ $labels.cert_type }} 将在 {{ $value }} 天后过期"
```

## 项目状态

当前版本：v0.1.0-alpha

### 已实现功能
- ✅ Pod 容器重启监控
- ✅ Linkerd 证书过期监控
- ✅ Prometheus 指标集成
- ✅ 基础 RBAC 配置
- ✅ Helm Chart 支持

### 待改进项
- ⚠️ 内存中的状态管理（需要持久化方案）
- ⚠️ RBAC 权限过于宽泛（需要细化）
- ⚠️ 缺少指标清理机制（Pod 删除后）
- ⚠️ 硬编码的 Linkerd 配置（需要可配置化）
- ⚠️ 缺少完整的端到端测试

## 相关文档

- [Linkerd 证书监控指南](docs/linkerd-certificate-monitoring.md)
- [README](README.md) - 安装和部署说明

## License

Apache License 2.0