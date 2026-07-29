# ProManager - sing-box 最终配置生成器

## 概述

`ProManagerService` 监听默认链配置变化，并从 SQLite 中的设置、入站、客户端、TLS、出站、服务和端点聚合最终 sing-box 配置。它只把逻辑路径 `core/singbox/config.json` 写入 Runtime Store 的 `managed_runtime_files` 表。

Core 启动或校验时会按需把该配置短暂物化到 `<binary_dir>/Promanager_data/core/singbox/config.json`，使用完成后删除磁盘临时文件。数据库仍是业务配置真源。

## 不再生成的副本

以下路径只作为历史清理范围保留，不再生成或读取：

- `Inbound/inbound.json`
- `Inbound/<tag>.json` 与 `Inbound/<tag>_meta.json`
- `outbound/<tag>.json` 与 `outbound/<tag>_meta.json`
- `sub_manager/<tag>*.json`
- `sub_json/*.json`

Runtime Store 初始化时会幂等删除上述四个根目录中的 `.json` 逻辑记录，并清理磁盘根目录下的普通 `.json` 文件。清理不跟随符号链接，不进入嵌套目录，也不删除其他扩展名；失败只记录日志，下一次初始化重试。

## 最终配置内容

`GenerateFullConfig()` 聚合：

- `settings.config` 中的日志、DNS、NTP、路由和实验性配置
- `InboundService.GetAllConfig()` 生成的入站、用户、TLS 和 ShadowTLS 组合入站
- `OutboundService.GetAllConfig()` 生成的出站和 ShadowTLS 组合出站
- services 与 endpoints
- 证书 Store、路由规则集、DNS 和运行期出站规范化结果

最终配置的唯一默认链 Runtime Store 路径是：

```text
Promanager_data/core/singbox/config.json
```

Mihomo 的 `core/mihomo/server.yaml` 与 `core/mihomo_inbounds_meta.json` 由 `MihomoManagerService` 独立维护，不属于 ProManager 的副本清理范围。

## 订阅按请求生成

客户端、单订阅节点和订阅分组不再预生成 `sub_json` 文件。HTTP 请求到达后直接读取 SQLite 并实时渲染：

| 类型 | 路由 | 生成服务 |
| --- | --- | --- |
| 默认链客户端 JSON / Clash / 链接 | `/q/client`、`/:subid` | `JsonService`、`ClashService`、`SubService` |
| 单订阅节点 JSON / Clash | `/q/sm`、`/sm/:tag` | `SubManagerSubService` |
| 订阅分组 JSON / Clash | `/q/group`、`/group/:groupName` | `SubManagerSubService` |

`SubOutboundService` 和 `SubGroupService` 在保存后仍会执行一次内存渲染并丢弃结果，以保留原有配置合法性检查。`sub_json` 文件名清洗与冲突校验也继续保留，避免改变既有保存规则，但不会产生文件。

## 事件系统

支持的事件源包括 `inbound`、`outbound`、`client`、`tls`、`dns`、`route`、`ruleset`、`service`、`endpoint` 和 `config`。事件处理器在 500ms 窗口内合并事件，统一刷新最终 Core 配置。

配置保存、证书续签/TLS 重新绑定、出站分组导入等同步链路仍会更新最终 Core 配置，并保留原有自动管理客户端同步行为。

## 兼容接口

```go
proManager := service.GetProManagerService(configService)

// 旧名称保留；现在只刷新最终 sing-box Core 配置。
proManager.SaveInboundJson()

// 只在内存中聚合并返回完整配置。
config, err := proManager.GenerateFullConfig()
```

`SaveInboundJson()`、Runtime Store 导出 API、订阅文件名校验 API、旧导出结构体以及 `managed_runtime_files` 表均保留，避免破坏现有 app、Core Manager、systemd 命令或潜在外部 Go 调用。

## 修改检查

修改默认链生成逻辑时至少检查：

1. `GenerateFullConfig()` 是否仍完整包含用户、TLS、普通/ShadowTLS 入站、出站、服务、端点和路由。
2. `ConfigService.Save()`、证书绑定和出站分组通知是否仍刷新最终 Core 配置及自动管理客户端。
3. JSON、Clash 和链接订阅是否继续实时读取 SQLite，且请求后没有新增废弃 Runtime Store 记录。
4. 文件名冲突校验是否保持原有拒绝规则。
5. Runtime Store 清理是否只作用于四个准确根目录中的 JSON，保留 Core 配置、Mihomo 元数据、未知文件、符号链接和嵌套目录。
