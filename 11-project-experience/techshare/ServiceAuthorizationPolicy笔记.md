# Service Authorization Policy in Meican

> 技术分享：美餐内部服务间授权策略

## 目录

1. [背景与目标](#背景与目标)
2. [Basic Auth](#basic-auth)
3. [OpenID Connect (OIDC)](#openid-connect-oidc)
4. [Istio AuthorizationPolicy](#istio-authorizationpolicy)
5. [Best Practices](#best-practices)
6. [KeyPair 补充](#keypair-补充)

---

## 背景与目标

服务授权策略解决的核心问题：
- **验证调用方身份**（Check caller identity）
- **保护集群内部服务**（Protect app inner-cluster）
- ~~mTLS~~（本次不涉及 mTLS）

---

## Basic Auth

### 基本原理

```
Authorization: Basic base64(username:password)
```

- 用于 titan 项目
- 需要额外开发：Interceptor / Middleware + RPC calls
- 实现相对简单直接

### 流程（Activity Diagram）

```
Service A（调用方）:
  请求 + header Authorization: id:secret
        ↓
Service B（被调用方）:
  Interceptor/Middleware  ←── rpc check ──  titan/cmdb
        ↓
      check
     /     \
  Pass    Fail → Unauthorized → 终止
   ↓
 Response
```

### id:secret 的来源

凭证通过以下链路注入到 Pod 中：

```
titan/cmdb
    ↓
nerds/tower
    ↓
Injection（注入）
  - Env vars（环境变量）
  - Credentials file（凭证文件）   ←── 同时从 Nacos 获取配置
    ↓
Kubernetes → Create Pod
  - Application Container
  - Istio proxy（Sidecar）
```

凭证文件默认路径：
```
/var/run/secrets/tower/tower_resource_schema.toml
```

环境变量：`APP_CREDENTIALS`

### Credentials 数据结构

`tower_resource_schema.toml` 关键字段：

| 字段 | 说明 |
|------|------|
| `slug` | 应用唯一标识，相当于 username |
| `uniqueID` | 与 Secret 配合用于 Basic Auth |
| `secret` | 应用的"密码" |
| `privateKey` | RSA 私钥 |
| `publicKey` | RSA 公钥 |
| `group` | 应用所属 group |
| `project` | 应用所属 project |
| `subset` | 子集，可选，影响 Slug 值 |

> `uniqueID` + `Secret` → Basic Auth 的 `username:password`

### 常见错误示例

```bash
# 错误：使用了简短的随机 token（不是正确的 base64(uniqueID:secret)）
curl -H 'Authorization: Basic YTpi' http://cmdb-server...
# → {"code":16, "message":"basic auth failed: application subset..."}

# 正确：使用完整的 base64(uniqueID:secret)
curl -H 'Authorization: Basic Yjk1ZmU0M...' http://cmdb-server...
# → {"team":{"name":"titan",...}}
```

---

## OpenID Connect (OIDC)

### 概念

- **AuthN（Authentication）**：验证用户或服务的**身份**
- **AuthZ（Authorization）**：决定**访问权限**

### OIDC 是什么

- 构建在 **OAuth2** 之上的身份层
- 支持 **SSO**（Single Sign-On）
- 通过 **Connector** 连接 IDP（Identity Provider）
- IDP 支持多种协议：SAML、OAuth2、WebServiceFederation

使用场景：SSO (Apollo)、Istio、AWS+Kubernetes、`meican.x/authorizer`

### OIDC 工作流（3方）

```
Relying Party          OpenID Provider          End User
     |                       |                      |
     |── 1. AuthN request ──>|                      |
     |                       |<── 2. AuthN & AuthZ →|
     |                       |         request       |
     |<── 3. AuthN Response ─|                      |
     |── 4. UserInfo Request→|                      |
     |<── 5. UserInfo Resp. ─|                      |
```

### JWT（JSON Web Token）

JWT 由三部分组成（Base64URL 编码，`.` 分隔）：
1. **Header**：算法 + Token 类型，如 `{"alg":"RS256","kid":"...","typ":"JWT"}`
2. **Payload**：数据，如 `{"iat":..., "iss":"server.cmdb.titan@...", "sub":"client.istio-jwt-demo.nerds@..."}`
3. **Signature**：`RSASHA256(base64(header) + "." + base64(payload), privateKey)`

### JWKS（JSON Web Key Set）

- RFC 7517 标准
- 包含一组公钥，用于验证 JWT 签名
- 支持多个 key（old + current），实现密钥轮换

```json
{
  "keys": [
    {
      "use": "sig",
      "kty": "RSA",
      "kid": "b2141e833a1ac0718ea116d429...",  // Old
      "alg": "RS256",
      "n": "...",
      "e": "AQAB"
    },
    {
      "use": "sig",
      "kty": "RSA",
      "kid": "7822d935f4788107a29cb8ebb7b6a72fcb88e357",  // Current
      "alg": "RS256",
      "n": "...",
      "e": "AQAB"
    }
  ]
}
```

### OIDC JWKS-JWT 交互流程

```
OIDC Provider     OIDC Client(Server)      Client
     |                   |                    |
     |<── 1. Request JWKS|                    |
     |── 2. Response JWKS→                    |
     |            3. Cache until expire       |
     |<── 4. Sign my payload ─────────────────|
     |── 5. JWT ─────────────────────────────>|
     |                   |<── 6. Make request ─|
     |            7. Verify by JWKS            |
     |                   |── 8. Response ─────>|
```

---

## Istio AuthorizationPolicy

### 特点

- Service Mesh 层面隐式启用
- **无需额外业务代码开发**
- **无需 RPC 调用**
- 基于 OpenID Connect

### 流程（Activity Diagram）

```
Service A:
  Request + header Authorization: jwt
      ↓
Service B:
  Sidecar（Envoy）←── policy ── Istiod
      ↓
    check
   /     \
  Pass   Fail → Denied → 终止
   ↓
 Container → Response
```

### Istio Authz 内部结构（Dive into the policy）

```
Istiod:
  ┌─────────────────────────────────┐
  │ RequestAuthentication  ──────────────implement──→  Policy Storage (Envoy)
  │ Conditions             ──────────────provide───→   Authorization Policy (Envoy)
  └─────────────────────────────────┘
                                            ↓
                                          check
                                            ↓
                                        Container → Response
```

### Istio Authz 详细时序（Sequence Diagram）

```
titan/cmdb      Envoy(Sidecar)      Container      Client
     |                |                  |              |
     |<── 1. Request JWKS                |              |
     |── 2. Response JWKS ──>            |              |
     |          3. Cache until expire    |              |
     |<── 4. Sign my payload ────────────|              |
     |── 5. JWT ─────────────────────────────────────->|
     |                |<── 6. Make request ────────────|
     |          7. Verify by JWKS        |              |
     |                |── 9. Proxy the request ────────>|
```

### JWT 的来源

与 Basic Auth 相同，通过 `nerds/tower` 注入到 `tower_resource_schema.toml`：

```toml
[identifier]
jwt = "eyJhbGciOiJSUzI1Ni..."   # ← JWT 字段
privateKey = "-----BEGIN RSA PRIVATE KEY-----..."
```

- 凭证文件路径同 Basic Auth：`/var/run/secrets/tower/tower_resource_schema.toml`
- 需要 `nerds/app` 版本 **>= v0.10.6**

JWT Subject 命名规则：
- 若 subset 未定义：`[project].[group]@[domain]`
- 若 subset 已定义：`[subset].[project].[group]@[domain]`
- Issuer 格式：`server.cmdb.titan@[domain]`

### Kubernetes CRD 配置

#### 1. RequestAuthentication（Policy Storage）

```yaml
apiVersion: security.istio.io/v1beta1
kind: RequestAuthentication
metadata:
  name: istio-jwt-demo-server-req-auth
  namespace: app-nerds
spec:
  selector:
    matchLabels:
      app: istio-jwt-demo   # 匹配目标 Pod
      subset: server
  jwtRules:
    - issuer: "server.cmdb.titan@ntrnl-eks-fan.meican"
      jwksUri: "http://cmdb-server-grpcgateway.app-titan.ntrnl-eks-fan.meican/oidc/keys"
```

#### 2. AuthorizationPolicy（Conditions & Rules）

```yaml
apiVersion: security.istio.io/v1beta1
kind: AuthorizationPolicy
metadata:
  name: istio-jwt-demo-server-auth-policy
  namespace: app-nerds
spec:
  selector:
    matchLabels:
      app: istio-jwt-demo
      subset: server
  action: ALLOW
  rules:
    - from:
        - source:
            requestPrincipals:
              - "server.cmdb.titan@ntrnl-eks-fan.meican/client.istio-jwt-demo.nerds@ntrnl-eks-fan.meican"
      to:
        - operation:
            ports: ["18888"]   # 排除 18888 端口（metrics 采集用）
```

`requestPrincipals` 格式：`{issuer}/{subject}`

### 行为验证（重要！）

| 场景 | 只有 RequestAuthentication | RequestAuthentication + AuthorizationPolicy |
|------|---------------------------|---------------------------------------------|
| 无 Authorization Header | ✅ 200 OK | ❌ 403 Forbidden（RBAC: access denied） |
| 错误 JWT | ❌ 401 Unauthorized | ❌ 401 Unauthorized |
| 正确 JWT | ✅ 200 OK | ✅ 200 OK |

**结论：**
- 只有 `RequestAuthentication`：只拒绝错误 JWT，**无 JWT 的请求直接放行**
- 加上 `AuthorizationPolicy`：强制要求 JWT，**无 JWT 的请求返回 403**

---

## Best Practices

接入 Istio Authz 的推荐步骤：

1. **阅读文档**（内部 Wiki）
2. **配置 ArgoCD Ephemeral Metadata**：为 canary/stable Pod 打不同 label
3. **先应用 RequestAuthentication（canary 阶段）**
   - 仅拒绝携带错误 JWT 的请求
   - 不携带 JWT 的请求**仍然放行**（便于灰度验证）
4. **验证请求的 Authorization Header 是否正确**
5. **应用 AuthorizationPolicy（stable 阶段）**
   - 此时无 JWT 的请求也会被拒绝（403）
6. **再次验证请求的 Authorization Header**

### ArgoCD Ephemeral Metadata

用于在 canary 部署中为不同阶段的 Pod 自动打 label：

```yaml
# ArgoCD Rollout 策略配置
spec:
  strategy:
    canary:
      stableMetadata:
        labels:
          stage: stable
      canaryMetadata:
        labels:
          stage: canary
```

在 Istio CRD selector 中利用这些 label：

```yaml
spec:
  selector:
    matchLabels:
      app: istio-jwt-demo
      subset: server
      stage: canary   # 精确匹配 canary Pod
```

---

## KeyPair 补充

- 类型：**RSA 2048** 私钥/公钥对
- 格式：**PKCS #1**（已为 Java 做适配转换）
- 通过 `nerds/app-cli` 命令生成

使用方式：
- 将**公钥**通过 gRPC/HTTP 交换给 `titan/cmdb`
- 用**私钥**对请求 payload 进行签名（生成 JWT）

---

## 两种方案对比

| 维度 | Basic Auth | Istio AuthorizationPolicy |
|------|-----------|--------------------------|
| 实现成本 | 需开发 Interceptor/Middleware + RPC 调用 | 无需业务代码，配置 K8s CRD 即可 |
| 验证方式 | 每次请求 RPC 到 titan/cmdb 校验 | Envoy Sidecar 本地验证（JWKS 缓存） |
| 性能 | 每次都有远程调用开销 | 本地验证，无 RPC 开销 |
| 凭证来源 | uniqueID + secret（from tower_resource_schema.toml） | JWT（from tower_resource_schema.toml） |
| 适用场景 | titan 项目，已有基础设施 | 新项目，Service Mesh 环境 |

---

## 参考资料

- Istio 安全文档：https://istio.io/latest/docs/concepts/security/#authorization-policies
- JWKS RFC 7517：https://www.rfc-editor.org/rfc/rfc7517
- ArgoCD Ephemeral Metadata：https://argoproj.github.io/argo-rollouts/features/ephemeral-metadata/
