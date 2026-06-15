# ProManager - 配置监视器和生成器

## 概述

ProManager 是一个配置监视器和生成器系统，用于监听 s-ui 面板中的所有配置变化，并自动生成相应的配置文件。

## 功能

1. **监听配置变化**：监听入站、出站、用户管理、TLS、mux、多路复用、utls等配置变化
2. **自动生成配置**：当配置变化时，自动组合生成完整的配置文件
3. **分层存储**：按入站、出站、核心配置分别存储

## 目录结构

```
Promanager_data/
├── inbound/          # 单入站JSON (每个入站一个文件)
│   ├── vless-node.json
│   ├── vmess-node.json
│   └── ...
├── outbound/         # 出站JSON (每个出站一个文件)
│   ├── direct.json
│   ├── block.json
│   └── ...
├── sub_json/         # JSON订阅配置 (每个用户-入站组合一个文件)
│   ├── vless-node_user1.json
│   ├── shadowsocks-38266_APq5xE9m.json
│   └── ...
├── core/             # 完整版核心配置
│   └── config.json
└── Inbound/          # 兼容旧版目录
    └── inbound.json
```

## 配置文件格式

### 单入站配置 (inbound/*.json)

```json
{
  "inbound": {
    "type": "vless",
    "tag": "vless-in",
    "listen": "0.0.0.0",
    "listen_port": 443,
    "users": [
      {
        "name": "user1",
        "uuid": "xxx-xxx-xxx",
        "flow": "xtls-rprx-vision"
      }
    ],
    "tls": {
      "enabled": true,
      "server_name": "example.com"
    }
  },
  "metadata": {
    "id": 1,
    "tag": "vless-in",
    "type": "vless",
    "tls_id": 1,
    "user_count": 5,
    "updated_at": 1707500000
  }
}
```

### 单出站配置 (outbound/*.json)

```json
{
  "outbound": {
    "type": "direct",
    "tag": "direct"
  },
  "metadata": {
    "id": 1,
    "tag": "direct",
    "type": "direct",
    "updated_at": 1707500000
  }
}
```

### 核心完整配置 (core/config.json)

```json
{
  "log": {
    "level": "info"
  },
  "dns": {
    "servers": [],
    "rules": []
  },
  "inbounds": [...],
  "outbounds": [...],
  "services": [...],
  "endpoints": [...],
  "route": {...},
  "experimental": {}
}
```

## 事件系统

ProManager 使用事件驱动架构，支持以下事件源：

| 事件源 | 说明 |
|--------|------|
| `inbound` | 入站配置变更 |
| `outbound` | 出站配置变更 |
| `client` | 用户/客户端变更 |
| `tls` | TLS配置变更 |
| `dns` | DNS配置变更 |
| `route` | 路由配置变更 |
| `ruleset` | 规则集变更 |
| `service` | 服务配置变更 |
| `endpoint` | 端点配置变更 |
| `config` | 核心配置变更 |

事件类型：
- `create` - 创建
- `update` - 更新
- `delete` - 删除

## 使用方法

### 获取ProManager实例

```go
import "github.com/alireza0/s-ui/service"

// 获取单例实例
proManager := service.GetProManagerService(configService)
```

### 触发事件

```go
// 入站变更
proManager.OnInboundChange(service.EventCreate, "vless-in", 1)

// 出站变更
proManager.OnOutboundChange(service.EventUpdate, "direct", 1)

// 用户变更
proManager.OnClientChange(service.EventDelete, "user1", 1)

// TLS变更
proManager.OnTlsChange(service.EventUpdate, "my-tls", 1)

// 核心配置变更
proManager.OnConfigChange()
```

### 手动生成配置

```go
// 异步生成所有配置
proManager.SaveInboundJson()

// 同步生成完整配置
config, err := proManager.GenerateFullConfig()
```

## 工作原理

1. **事件接收**：当配置发生变化时，通过 `EmitEvent` 发送事件到事件队列
2. **批量处理**：事件处理器会在500ms内收集所有事件，然后批量处理
3. **增量更新**：根据事件类型，只重新生成需要更新的配置
4. **文件输出**：将生成的配置写入对应的目录

## 集成点

ProManager 已集成到以下位置：

1. **ConfigService.Save()**：在保存任何配置后自动触发
2. 可根据需要在其他地方添加事件触发

## 注意事项

1. ProManager 使用异步处理，不会阻塞主流程
2. 事件队列最大容量为100，超出会丢弃事件
3. 停止服务时会处理完剩余事件
4. 文件名会自动清理不安全字符

## 扩展开发

如需添加新的事件源或配置类型：

1. 在 `ConfigEventSource` 中添加新的常量
2. 在 `processBatchEvents` 中添加处理逻辑
3. 添加相应的便捷方法
