# Linkerd Certificate Monitoring

## 概述

Pod Monitor Operator 现在支持监控 Linkerd identity issuer 证书的有效期。这个功能会自动检测 `linkerd` 命名空间下的 `linkerd-identity-issuer` Secret 中的证书，并暴露相关的 Prometheus 指标。

## 支持的证书格式

- `.crt` 文件（如 `ca.crt`, `issuer.crt`）
- `.pem` 文件（如 `ca.pem`, `issuer.pem`, `crt.pem`）
- 实际的 Linkerd 使用 `crt.pem` 作为主要证书文件

## Prometheus 指标

### pod_monitor_certificate_expiration_timestamp_seconds

证书过期时间的 Unix 时间戳（秒）。

标签：
- `namespace`: Secret 所在的命名空间（linkerd）
- `secret_name`: Secret 名称（linkerd-identity-issuer）
- `cert_type`: 证书类型（ca.crt, issuer.crt, ca.pem, issuer.pem）

### pod_monitor_certificate_days_until_expiration

证书距离过期的剩余天数。

标签：
- `namespace`: Secret 所在的命名空间
- `secret_name`: Secret 名称
- `cert_type`: 证书类型

## 使用示例

### 查询证书剩余有效期

```promql
# 查看所有 Linkerd 证书的剩余有效天数
pod_monitor_certificate_days_until_expiration{namespace="linkerd"}

# 查看即将在 30 天内过期的证书
pod_monitor_certificate_days_until_expiration{namespace="linkerd"} < 30
```

### 配置告警

```yaml
groups:
- name: linkerd-certificate-alerts
  rules:
  - alert: LinkerdCertificateExpiringSoon
    expr: pod_monitor_certificate_days_until_expiration{namespace="linkerd"} < 30
    for: 1h
    labels:
      severity: warning
    annotations:
      summary: "Linkerd certificate expiring soon"
      description: "Certificate {{ $labels.cert_type }} in {{ $labels.namespace }}/{{ $labels.secret_name }} will expire in {{ $value }} days"
  
  - alert: LinkerdCertificateExpiryCritical
    expr: pod_monitor_certificate_days_until_expiration{namespace="linkerd"} < 7
    for: 10m
    labels:
      severity: critical
    annotations:
      summary: "Linkerd certificate expiring critically soon"
      description: "Certificate {{ $labels.cert_type }} in {{ $labels.namespace }}/{{ $labels.secret_name }} will expire in {{ $value }} days"
```

## 工作原理

1. Operator 会监听 `linkerd` 命名空间下的 `linkerd-identity-issuer` Secret 的变化
2. 当 Secret 创建或更新时，会解析其中的证书文件
3. 提取证书的过期时间并计算剩余有效天数
4. 将这些信息作为 Prometheus 指标暴露
5. 每小时会自动重新检查证书状态

## 注意事项

- Operator 需要有读取 `linkerd` 命名空间下 Secret 的权限（已在 RBAC 配置中包含）
- 只会处理 PEM 格式的 X.509 证书
- 私钥文件（如 `issuer.key`）会被忽略，不会被处理