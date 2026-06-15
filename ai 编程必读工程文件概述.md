# AI 编程必读工程文件概述（kwor / s-ui-main）
> 目标：让 AI 或新开发者先分清“主源码在哪、哪些目录是运行期生成物、默认 sing-box 链和 mihomo 链怎么并行存在、改需求时该先看哪一层”。
>
> 扫描基线：`2026-03-20`
>
> 结论来源：以当前仓库源码为准，不以 README、旧脚本或历史文件名为准。

---

## 0. AI 必须遵守的开发约定

1. 先读这个文件再找代码，按这里的树状索引去定位关联文件。
2. 看到一个文件时，要联想到它的调用方、被调用方、配置落点、运行时副作用，不要只看当前文件。
3. 本项目不是 Git 仓库，不要把“git 工作流”当成默认前提。
4. 前端正式构建优先使用 `npm.cmd run build`；如果当前环境没有 `npm.cmd`，不要硬跑不存在的命令，改用已安装的 `node` 直接调用 `temp_frontend/node_modules` 里的 `vue-tsc` 和 `vite` 做等价验证。
5. 所有修改必须保持 UTF-8，不得引入乱码。
6. 不要默认跑 `go test ./service` 这类长测试；优先跑定向、短时、只覆盖当前改动的测试。即使定向测试已通过，也不得自行再补跑 `go test ./service` 之类的整包/全量回归。

---

## 1. 先读这 25 个文件

1. `main.go`
2. `app/app.go`
3. `cmd/cmd.go`
4. `config/config.go`
5. `database/db.go`
6. `web/web.go`
7. `sub/sub.go`
8. `sub/subHandler.go`
9. `api/apiHandler.go`
10. `api/apiService.go`
11. `service/setting.go`
12. `service/config.go`
13. `service/promanager.go`
14. `service/coreManager.go`
15. `service/mihomo_core_manager.go`
16. `service/mihomo_manager.go`
17. `service/mihomo_config.go`
18. `service/mihomo_route_render.go`
19. `service/mihomo_proxy_convert.go`
20. `database/model/mihomo.go`
21. `cronjob/cronJob.go`
22. `temp_frontend/src/router/index.ts`
23. `temp_frontend/src/store/uiNamespace.ts`
24. `temp_frontend/src/store/modules/data.ts`
25. `temp_frontend/src/store/modules/mihomoData.ts`
26. `service/firewall.go`
27. `service/firewall_nftables_install.go`
28. `service/firewall_scan.go`
29. `cronjob/firewallSyncJob.go`
30. `temp_frontend/src/components/SettingsFirewallManage.vue`
31. `service/firewall_geoip.go`
32. `service/firewall_geoip_parser.go`
33. `service/firewall_geoip_nft.go`
34. `temp_frontend/src/components/SettingsFirewallGeoOptions.ts`

如果只想最快摸清项目，读完上面这些文件基本就能建立正确地图。

---

## 2. 目录边界

### 2.1 当前主源码目录

- 后端主源码：`api/`、`app/`、`cmd/`、`config/`、`cronjob/`、`database/`、`middleware/`、`network/`、`service/`、`sub/`、`util/`、`web/`
- 前端主源码：`temp_frontend/src/`
- 前端构建产物：`web/html/`

### 2.2 运行期生成目录

- `Promanager_data/db/kwor.db`
- `Promanager_data/core/singbox/config.json`
- `Promanager_data/core/server.yaml`
- `Promanager_data/core/mihomo_inbounds_meta.json`
- `Promanager_data/core/singbox/sing-box` / `sing-box.exe`
- `Promanager_data/core/mihomo/mihomo` / `mihomo.exe`
- `Promanager_data/cert/*`
- `Promanager_data/Inbound/*.json`
- `Promanager_data/outbound/*.json`
- `Promanager_data/sub_json/*.json`

这些都不是手写业务源码，很多文件会在保存配置、同步或启动时被重写。

### 1.3 容易看错的目录

- `frontend/`
  - 仍然像历史子模块/发布输入目录。
  - 但当前本地开发构建脚本 `build.sh`、`build.bat` 实际使用的是 `temp_frontend/`。
- `web/html/`
  - 是嵌入到 Go 二进制里的前端编译结果。
  - 不要直接改这里。
- `db/`
  - 旧数据库位置兼容目录。
  - 真实运行路径现在是 `Promanager_data/db/kwor.db`。
- `backup/`
  - 历史备份，不是当前主链。
- `_tmp_mihomo_src/`、`_tmp_mieru_official/`
  - 参考源码或临时对照目录，不是当前产品主入口。

---

## 2. 项目当前形态

- 可执行程序名：`kwor`
- 模块名仍是：`github.com/alireza0/s-ui`
- 版本：`1.4.20`
- 后端：Go 1.25 + Gin + GORM + SQLite（`github.com/glebarez/sqlite`）
- 前端：Vue 3 + Vite + Vuetify + Pinia + TypeScript
- Session Cookie 名：`kwor`
- Web API：`<webPath>/api/*`
- Token API：`<webPath>/apiv2/*`

### 2.0 版本号修改速查（避免每次全仓库排查）

主页面「系统信息 -> S-UI (kwor)」显示链路：

`config/version` -> `config/config.go:GetVersion()` -> `service/server.go:GetSystemInfo().appVersion` -> `temp_frontend/src/components/Main.vue: appVersionLabel`

结论（默认强制流程）：

- 当需求是“只改主页面版本显示”时，只允许修改 `config/version` 这 1 个文件。
- 默认禁止全仓库版本字符串扫描（例如 `rg 1.x.x`）和无关目录排查。
- 默认不要改 `web/html/` 下打包产物（会被前端构建覆盖）。
- 默认不要改 `temp_frontend/package.json` 与 `temp_frontend/package-lock.json`（它们不是主页面版本显示必需项）。
- 修改后说明“需要重新编译并重启后生效”（`config/config.go` 通过 `//go:embed version` 读取版本）。

例外条件（仅满足其一才允许扩展扫描或多文件修改）：

- 用户明确要求“同步前端包版本/发版元信息”。
- 用户明确要求“做全仓版本一致性检查”。
- 构建脚本或发布流程报错，明确要求 `package.json` 版本必须与 `config/version` 一致。
- 版本显示链路被改动，`config/version -> GetVersion() -> appVersion` 链路不再成立。

建议自检：

- 打开主页面确认版本标签已变更（例如 `vX.Y.Z`）。
- 确认 `service/setting.go` 默认值里的 `version` 仍由 `config.GetVersion()` 提供，不要改成硬编码字符串。

当前不是“单一配置面板”，而是两条并行配置面：

### 2.1 默认链（sing-box / 面板 / 订阅）

- 数据对象：`config`、`clients`、`tls`、`inbounds`、`outbounds`、`outboundgroups`、`suboutbounds`、`subgroups`、`services`、`endpoints`
- 运行配置输出：`Promanager_data/core/singbox/config.json`
- 相关生成文件：`Promanager_data/Inbound/*`、`Promanager_data/outbound/*`、`Promanager_data/sub_json/*`

### 2.2 Mihomo 链

- 数据对象：`mihomo_config`、`mihomo_clients`、`mihomo_tls`、`mihomo_inbounds`、`mihomo_outbounds`、`mihomo_outboundgroups`
- 运行配置输出：`Promanager_data/core/server.yaml`
- 额外元数据输出：`Promanager_data/core/mihomo_inbounds_meta.json`

关键现实：

- `service/config.go` 里的 `corePtr` 仍然是 dummy，不是实际 core 生命周期入口。
- 真正的 sing-box 管理器在 `service/coreManager.go`。
- 真正的 mihomo 管理器在 `service/mihomo_core_manager.go`。

---

## 3. 启动方式与运行模式

### 3.1 无参数启动

链路：

`main.go -> runApp() -> app.Init() -> app.Start()`

`app.Init()` 当前会做这些事：

- 初始化数据库：`database.InitDB(config.GetDBPath())`
- 初始化运行期文件备份/落盘体系：`service.InitManagedRuntimeFileStore()`
- 读取 settings，并尝试拆分旧的 panel/sub 共用自签证书路径
- 初始化 `cronJob`、`webServer`、`subServer`
- 初始化 `ConfigService`
- 启动前先触发两条生成链：
  - `service.NewProManagerService(...).SaveInboundJson()`
  - `service.NewMihomoManagerService().RegenerateServerConfig()`

`app.Start()` 当前会：

- 启动 cron
- 启动 Web Server
- 启动 Sub Server
- 启动 `PanelCertManager`

### 3.2 命令模式

链路：

`main.go -> cmd.ParseCmd()`

当前支持：

- `start`
- `stop`
- `resetadmin`
- `uninstall`
- `admin`
- `setting`
- `uri`
- `migrate`

### 3.3 `kwor start` 和正常运行默认值不是一套

正常运行的权威默认值来自 `service/setting.go`：

- 面板端口：`8888`
- 面板路径：`/app/`
- 订阅端口：`22780`
- 订阅路径：`/sub/`

`kwor start` 首次向导默认值来自 `cmd/cmd.go`：

- 面板端口：`8888`
- 面板路径：`/app/`
- 管理员：`admin/admin`
- 首次启动会尝试分别生成 panel/sub 自签证书

高频坑：

- `temp_frontend/src/views/Settings.vue` 里的本地占位默认 `subPort` 仍是 `2096`。
- 但后端真实默认值已经是 `22780`。
- 以 `service/setting.go` 为准，不要以表单初始值为准。

---

## 4. Web / API / Sub 主链路

### 4.1 Web 服务

入口：`web/web.go`

职责：

- 读取 `webPath`
- 注册 session：`sessions.Sessions("kwor", ...)`
- 挂载静态资源：`<webPath>assets/`
- 挂载 API：
  - `<webPath>api/*`
  - `<webPath>apiv2/*`
- 处理 SPA fallback
- 对错误路径做 anti-probe
- 根据证书存在情况自动走 HTTPS 或 HTTP fallback

### 4.2 Sub 服务

入口：`sub/sub.go` + `sub/subHandler.go`

当前路由：

- 客户端订阅（默认链）：
  - `GET /q/client`
  - `HEAD /q/client`
  - `GET /:subid`
  - `HEAD /:subid`
- 客户端订阅（Mihomo 链）：
  - `GET /q/mihomo`
  - `HEAD /q/mihomo`
  - `GET /mihomo/:subid`
  - `HEAD /mihomo/:subid`
- SubManager 订阅：
  - `GET /q/sm`
  - `GET /sm/:tag`
- SubGroup 订阅：
  - `GET /q/group`
  - `GET /group/:groupName`

`?format=json|clash` 仍然有效。

### 4.3 面板 API

入口：

- Session API：`api/apiHandler.go`
- Token API：`api/apiV2Handler.go`

当前规律：

- Session API 最全。
- Token API 只覆盖一部分读取与保存接口，没有完整复刻 mihomo 专用接口。

### 4.4 `api/save` 是总分发入口

入口：`api/apiService.go -> ConfigService.Save(...)`

默认对象：

- `clients`
- `tls`
- `inbounds`
- `outbounds`
- `outboundgroups`
- `suboutbounds`
- `subgroups`
- `services`
- `endpoints`
- `config`
- `settings`

Mihomo 对象：

- `mihomo_clients`
- `mihomo_tls`
- `mihomo_inbounds`
- `mihomo_outbounds`
- `mihomo_outboundgroups`
- `mihomo_config`

### 4.5 保存后的副作用

入口：`service/config.go`

特点：

- 先事务保存数据库
- 再提交事务
- 然后跑 `managed runtime hooks`
- 最后触发生成文件和 nftables 后处理

默认链常见副作用：

- 重生成 `Inbound/*`
- 重生成 `outbound/*`
- 重生成 `core/singbox/config.json`
- 重生成 `sub_json/*`
- 必要时更新 subgroup / suboutbound 文件
- 必要时更新 sing-box 侧 nftables 状态

Mihomo 链常见副作用：

- 任意 `mihomo_*` 保存成功后重生成 `Promanager_data/core/server.yaml`
- 同时重写 `Promanager_data/core/mihomo_inbounds_meta.json`
- 必要时更新 mihomo 侧 nft redirect 状态

---

## 5. 默认 sing-box 生成链

### 5.1 配置来源

`service/ConfigService.GetConfig()`

它不是直接把 `settings.config` 原样返回，而是把：

- `settings.config`
- DB 中的 inbounds
- DB 中的 outbounds
- DB 中的 services
- DB 中的 endpoints

重新拼成完整 sing-box 配置对象。

### 5.2 真实生成器

入口：`service/promanager.go`

核心职责：

- 生成每个入站的单文件配置
- 生成每个出站的单文件配置
- 生成完整 core 配置
- 生成订阅 JSON 文件

当前输出：

- `Promanager_data/Inbound/*.json`
- `Promanager_data/outbound/*.json`
- `Promanager_data/core/singbox/config.json`
- `Promanager_data/sub_json/*.json`

### 5.3 ProManager 不是纯监听器

虽然 `service/promanager.go` 里有事件总线和批处理逻辑，但当前更常见的触发方式是：

- 启动时直接调用 `SaveInboundJson()`
- `ConfigService.Save()` 在提交后直接调用 `regenerate*`

所以出问题时，优先看保存链和生成函数本身，不要只盯事件队列。

### 5.4 实际 core 生命周期

入口：`service/coreManager.go`

职责：

- 下载 sing-box
- 读取本地版本
- 查询 GitHub 远程版本
- 管理自动检查更新
- Linux 下创建/删除 `kwor-singbox` systemd service（`ExecStartPre` 会通过 `kwor materialize-core-config singbox` 从托管库刷新 `core/singbox/config.json`，`cleanup-core-config singbox` 不删除这份持久运行配置）
- 启动/停止/重启 sing-box

当前 sing-box 链保持历史目录布局：

- 内核程序：`Promanager_data/core/singbox/sing-box`
- 运行配置：`Promanager_data/core/singbox/config.json`
- `core/singbox/config.json` 保存配置时会真实落盘，systemd 启动前仍会用托管库内容刷新该文件；`cleanup-core-config singbox` 不再删除这份持久运行配置。

所以：

- `service/config.go` 里的 dummy core 不是实际运行体。
- 真正运行控制和下载升级都在 `service/coreManager.go`。

---

## 6. Mihomo 并行链

### 6.1 配置来源不是另一份 `config.json`

Mihomo 基础配置来源：

- `settings.mihomo_config`

但最终运行文件是：

- `Promanager_data/core/server.yaml`
- Mihomo 内核和运行时缓存位于 `Promanager_data/core/mihomo/`。

入口：

- `service/mihomo_config.go`
- `service/mihomo_manager.go`

### 6.2 Mihomo 配置保存会先清洗

前端清洗：

- `temp_frontend/src/types/rules.ts`
- `temp_frontend/src/types/tls.ts`

后端清洗：

- `service/mihomo_route_sanitize.go`
- `service/mihomo_config.go`
- `database/model/mihomo.go`

当前明确行为：

- 逻辑规则会被丢弃
- 只保留 `action=route` 或 `action=reject`
- 一批 sing-box 专有字段会被删除
- mihomo 不支持的 TLS 字段会被删除

### 6.3 Mihomo YAML 渲染主链

入口：`service/mihomo_manager.go -> GenerateServerDocument()`

当前流程：

1. 读取 `mihomo_config`
2. 复制通用配置，提取 route 通用项
3. 读取 `mihomo_outbounds`
   - 订阅导入的 Clash 客户端节点会额外保存在 `mihomo_outbounds.RawOutbound`
   - 如果来源是 Clash 订阅，还会把每个 proxy 的原始 YAML 文本保存到 `mihomo_outbounds.RawClashYAML`
   - 写 `server.yaml` 时，`proxies:` 段会优先回放这份原始 proxy YAML；只有缺失时才 fallback 到结构化渲染
4. 用 `service/mihomo_proxy_convert.go` 转成 `proxies` 和 `proxy-groups`
5. 读取 `route.rule_set`，转成 `rule-providers`
6. 读取 `mihomo_inbounds`
7. 用 inbound 的 `route_tag` / detour / final 生成 `listeners` 与 `sub-rules`
8. 用 `service/mihomo_route_render.go` 把 route.rules 展平成最终 `rules`
9. 归一化 `sniffer`
10. 输出 `server.yaml`

### 6.4 Mihomo 当前允许的 listener 类型

后端过滤入口：`service/mihomo_route_render.go -> filterSupportedMihomoListeners(...)`

当前允许：

- `mixed`
- `socks`
- `http`
- `redirect`
- `tproxy`
- `tun`
- `shadowsocks`
- `shadowtls`
- `vmess`
- `vless`
- `trojan`
- `anytls`
- `tuic`
- `hysteria2`
- `mieru`

前端 `temp_frontend/src/layouts/modals/Inbound.vue` 在 `mihomo` namespace 下当前隐藏的是：

- `direct`
- `naive`
- `hysteria`

所以旧结论“mihomo 前端隐藏 shadowtls / mieru”已经不成立。

### 6.5 Mihomo outbound 不是 UI 里能建就一定能落到 YAML

权威转换入口：`service/mihomo_proxy_convert.go`

当前明确行为：

- `selector`、`urltest` 会转成 `proxy-groups`
- `direct` 归一成 `DIRECT`
- `tor`、`ssh` 会被标记 unsupported
- `shadowtls` 不是普通独立 proxy
  - 只有 `shadowsocks + detour=shadowtls` 或运行时可还原成 ShadowTLS 组合时，才会正确折算

前端对应限制：

- `temp_frontend/src/layouts/modals/Outbound.vue` 在 `mihomo` namespace 下隐藏 `tor`、`ssh`

### 6.6 Mihomo 当前没有完整在线/流量统计链

入口：`api/apiService.go -> getMihomoData()`

当前返回：

- `enableTraffic = false`
- `onlines = { inbound: [], outbound: [], user: [] }`

所以 mihomo 页面与默认链当前并不等价。

---

## 7. 数据库与模型层

### 7.1 数据库路径

入口：`config/config.go`

当前默认：

- `<可执行文件目录>/Promanager_data/db/kwor.db`

兼容迁移：

- 如果发现旧路径 `<可执行文件目录>/db/kwor.db`
- 会自动复制到新路径并清理旧 sidecar 文件

### 7.2 自动建表

入口：`database/db.go`

除了默认链表，还会自动迁移这些 Mihomo 表：

- `mihomo_tls`
- `mihomo_inbounds`
- `mihomo_outbounds`
- `mihomo_outbound_groups`
- `mihomo_clients`
- `mihomo_inbound_redirect_state`

### 7.3 Mihomo 模型的现实意义

入口：`database/model/mihomo.go`

重要点：

- `MihomoInbound` / `MihomoOutbound` 复用默认 `Inbound` / `Outbound` 的编解码逻辑
- 保存时会删掉 mihomo 不支持的字段
- `MihomoTls.Sanitize()` 会主动清理一批 TLS 字段
- mihomo outbound 还会按类型清理 `utls`、`reality` 等字段

---

## 8. 前端主链

### 8.0 前端验证命令（Windows / Codex 环境）

优先使用项目脚本：

```powershell
cd temp_frontend
npm.cmd run build
```

如果当前环境没有 `npm.cmd`，但 `node` 和 `temp_frontend/node_modules` 已存在，使用本地 CLI 直接验证：

```powershell
cd temp_frontend
node ..\scripts\sync-version.mjs
node .\node_modules\vue-tsc\bin\vue-tsc.js --noEmit
node .\node_modules\vite\bin\vite.js build
```

说明：
- `temp_frontend/package.json` 的 `build` 实际等价于 `sync-version + vue-tsc --noEmit + vite build`。
- 这种 fallback 只用于当前环境缺少 `npm.cmd` 时的验证，不代表要修改 `package.json` 脚本。
- `vite build` 会生成 `temp_frontend/dist/`；如果只是验证改动，不要把这个临时产物当源码提交或纳入后续修改。
- Vite 当前可能出现大 chunk warning；只要构建 exit code 为 0，且没有新增错误，通常按既有警告处理。

### 8.1 路由与菜单

入口：

- 路由：`temp_frontend/src/router/index.ts`
- 菜单：`temp_frontend/src/layouts/default/Drawer.vue`
- 设置页内子标签：`temp_frontend/src/views/Settings.vue`
- 设置页新增了完整的 `反向代理` 页签：`temp_frontend/src/components/SettingsReverseProxyManage.vue`

当前有 6 个 mihomo 页面：

- `/mihomo_inbounds`
- `/mihomo_clients`
- `/mihomo_outbounds`
- `/mihomo_tls`
- `/mihomo_rules`
- `/mihomo_dns`

默认链的 `/endpoints` 与 `/services` 页面当前保留源码和数据链，但已从左侧菜单与直接访问入口中暂时隐藏。

默认链里这 6 个页面（`inbounds`/`clients`/`outbounds`/`tls`/`rules`/`dns`）现在在：

- 左侧菜单（`Drawer.vue`）
- 顶部标题（`AppBar.vue`）

都统一加了 `singbox_` 前缀，避免菜单名和页头标题不一致。

### 8.2 前端每 10 秒同时轮询两套数据

入口：`temp_frontend/src/router/index.ts`

非登录页会定时执行：

- `Data().loadData()`
- `MihomoData().loadData()`

所以两套 store 是并行常驻的，不是临时切换。

### 8.3 namespace 分流是前端核心

入口：`temp_frontend/src/store/uiNamespace.ts`

它决定：

- 使用哪个 store
- 使用哪个 API endpoint
- 订阅二维码 URL 前缀
- 端口日志 localStorage key
- 是否在 Inbounds 页显示 core 控制按钮
- 使用哪套 core manager 元数据

当前 namespace 差异：

默认：

- `syncEndpoint = api/syncToSubManager`
- `inboundIpsEndpoint = api/inbound-ips`
- `subscriptionPathPrefix = ""`
- `supportsSubscriptionQr = true`
- `showCoreControlsOnInbounds = true`
- core config path = `Promanager_data/core/singbox/config.json`

Mihomo：

- `syncEndpoint = api/mihomoSyncToSubManager`
- `inboundIpsEndpoint = api/mihomo-inbound-ips`
- `subscriptionPathPrefix = "mihomo/"`
- `supportsSubscriptionQr = true`
- `showCoreControlsOnInbounds = true`
- core config path = `Promanager_data/core/server.yaml`

注意：

- 旧文档里“mihomo 不支持 subscription QR”已经过时。
- 现在 `QrCode.vue` 会给 mihomo 也显示订阅二维码，只是 URL 带 `mihomo/` 前缀。

### 8.4 `Mihomo*.vue` 只是薄包装

`MihomoInbounds.vue`、`MihomoClients.vue`、`MihomoOutbounds.vue`、`MihomoTls.vue`、`MihomoRules.vue`

本质上只是把：

- `namespace="mihomo"`

传给共用页面：

- `Inbounds.vue`
- `Clients.vue`
- `Outbounds.vue`
- `Tls.vue`
- `Rules.vue`

真正的业务分流点还是：

- `uiNamespace.ts`
- `data.ts`
- `mihomoData.ts`
- 共用页面里的 namespace 判断

补充：

- `MihomoDns.vue` 不是薄包装，它是独立页面，直接读写 `mihomo_config.dns` 相关字段。

### 8.5 `SingboxCore.vue` 现在是通用 core 弹窗

入口：`temp_frontend/src/layouts/modals/SingboxCore.vue`

虽然文件名还叫 `SingboxCore.vue`，但它实际会根据 namespace 读：

- sing-box 的版本、下载、状态接口
- 或 mihomo 的版本、下载、状态接口

所以不要被文件名误导。

### 8.6 sing-box 与 mihomo 的 core 控制入口现在都在 Inbounds 页

- sing-box：`temp_frontend/src/views/Inbounds.vue`（默认 namespace）
- mihomo：`temp_frontend/src/views/Inbounds.vue`（`namespace="mihomo"`）

`temp_frontend/src/views/SubManager.vue` 现在只保留订阅管理相关 UI，不再承载 sing-box core 顶部控制区。

---

## 9. 订阅、SubManager、同步链

### 9.1 三套系统不要混为一谈

系统 A：JSON 订阅扩展

- 前端：`Settings.vue` + `SubJsonExt.vue` + `SubJsonExtLogic.ts`
- 后端：`sub/jsonService.go`
- 存储：`settings.subJsonExt`

系统 B：Clash/Mihomo 订阅扩展

- 前端：`Settings.vue` + `SubClashExt.vue` + `SubClashExtLogic.ts`
- 后端：`sub/clashService.go`
- 存储：`settings.subClashExt`

当前订阅输出会按目标内核分别过滤协议：

- `?format=json` 只保留 sing-box 支持的 outbounds；例如 `mieru` 不会进入 JSON 订阅
- `?format=clash` 只保留 mihomo 可转换/可识别的 proxies；不支持协议不会进入 `proxies`

系统 C：Mihomo 路由页

- 前端：`Rules.vue(namespace="mihomo")`
- 后端：`service/mihomo_config.go` + `service/mihomo_route_*`
- 存储：`settings.mihomo_config`

这三套不是同一条链。

### 9.2 `Settings.vue` 保存前会先提交 UI 临时态

入口：`temp_frontend/src/views/Settings.vue`

保存前会先调用：

- `commitCustomRuleRows()`
- `commitDnsRouteRows()`
- `commitClashRuleRows()`
- `commitClashDnsPolicyRows()`

所以 `SubJsonExtLogic.ts` / `SubClashExtLogic.ts` 不是纯展示逻辑，而是最终保存前的结构整理器。

### 9.3 客户端同步到 SubManager 分两套

默认链：

- 前端按钮：`Clients.vue`
- 后端：`service/syncService.go`
- `source_type = "client"`

Mihomo 链：

- 前端按钮：`Clients.vue(namespace="mihomo")`
- 后端：`service/mihomo_sync.go`
- `source_type = "mihomo_client"`
- 同步出的 `suboutbound tag` 会加 `mihomo_` 前缀

### 9.4 `sub_json` 文件名现在有冲突保护

入口：`service/sub_json_file_guard.go`

会检查三类名字归一化后是否冲突：

- client subscription 文件名
- subgroup 文件名
- suboutbound 文件名

因此：

- 改 subgroup 名或 suboutbound tag 时，如果保存失败，要先怀疑文件名冲突而不是渲染器本身。

### 9.5 SubGroup 自动更新已经是正式链路

入口：

- 设置：`service/subgroups_auto_update.go`
- API：`api/subgroup-auto-update-info`、`api/subgroup-auto-update-settings`
- Cron：`NewSubGroupAutoUpdateJob()`

行为：

- 支持 JSON 与 Clash 两种来源
- 自动重试
- 会记录失败来源与错误
- JSON 来源的有效节点会按原始 JSON 节点保存到 `suboutbounds.RawOutbound`
- Clash 来源的有效节点会按原始 proxy YAML 文本保存到 `suboutbounds.RawClashYAML`
- 输出 Clash 订阅时会优先直接回放 `RawClashYAML`，避免重新 `yaml.Marshal` 改写 proxy 文本

---

## 10. 证书与面板 HTTPS 链

### 10.1 panel/sub 自签证书已分离

入口：

- `service/panel_self_signed_cert.go`
- `service/panel_self_signed_cert_paths.go`

现在会分别为：

- panel
- sub

生成独立证书路径。

### 10.2 启动后有独立证书管理器

入口：`app/panel_cert_manager.go`

职责：

- 检查 web/sub 当前在用证书
- 热重载证书
- 在自签证书临近过期时续签
- 在受信任证书过期且超过 grace window 时 fallback 到自签
- 整个过程不触碰 sing-box / mihomo core 生命周期

### 10.3 旧共用证书路径会自动拆分

入口：`service/SplitLegacySharedPanelSelfSignedCertificate(...)`

所以看到旧路径：

- `Promanager_data/cert/fullchain.pem`
- `Promanager_data/cert/privkey.pem`

不要再把它当当前标准结构。

### 10.4 证书列表现在是“统一证书仓库”（SQLite）

入口：

- 模型：`database/model/certificate_record.go`
- 迁移：`database/db.go`（`AutoMigrate(&model.CertificateRecord{})`）
- 统一服务：`service/certificate_inventory.go`
- ACME 接入：`service/acme_service.go`
- 自签/导入接入：`service/panel_sqlite_cert_store.go`、`service/setting.go`、`service/config.go`、`app/app.go`
- 前端页：`temp_frontend/src/components/SettingsAcmeManage.vue`
- 查看证书 API：`api/apiService.go: ViewAcmeCertificate` + `api/apiHandler.go: acme-view`

当前行为：

- 证书列表不再只等于 ACME 表，而是统一读取 `certificate_records`。
- 统一仓库里的数据库主键 `id` 只用于内部接口；列表另外使用永久显示 `displayId`。
- `displayId` 从 `1` 开始分配，删除后会优先复用当前最小空缺编号。
- `displayId` 只保存在 SQLite 元数据里，不会写回 PEM、证书正文、推送目录或应用证书。
- 列表排序使用 `list_order_at DESC, id DESC`，新证书在上，旧证书在下，和 `displayId` 大小无关。
- 来源类型通过 `sourceType` 区分：
  - `acme`
  - `self_signed`（SQLite 自签）
  - `imported`（设置页 `webCertFile/subCertFile` 导入路径）
- 列表删除是“删仓库记录”为主：
  - 对 `acme` 记录：同时删除 `acme_certificates` 对应记录
  - 对 `self_signed/imported`：仅删 `certificate_records` 记录
  - 已推送目录内的 `cert.pem/key.pem/fullchain.pem/chain.pem` 不会被删除
- 不再存在“默认推送目录”全局回填逻辑：
  - 签发页/自签页的 `pushDir` 仅在用户显式填写时生效
  - 证书列表“推送到目录”仅使用证书已有 `pushDir` 或用户手动输入的目录
  - “查看证书”弹窗统一展示：
    - 上：`fullchain.pem` 文本
    - 下：`key.pem` 文本
- 续签/强制续签/应用到面板/应用到订阅仅对 `acme` 来源可用，前后端双重限制。
- ACME 新签发在 `recordID == 0` 时必须新建 `acme_certificates` 源记录，不再按 `main_domain + use_ecc` 复用旧行；这解决了同一 IP 重复签发只显示一条的问题。

同步链路：

- ACME 签发/续签/推送/应用后会 upsert 到统一仓库。
- Panel 自签写入 SQLite 时会 upsert 到统一仓库。
- 路径导入证书在读取成功时 upsert，路径清空/无效时会清理对应 imported 记录。
- 应用启动时会做一次“从 ACME 表 + panel sqlite + settings 路径”到统一仓库的补齐同步。
- 每次 ACME overview 同步、以及应用启动同步后，都会补做一次 `displayId/list_order_at` 修复。
- 如果 ACME 源表里还有证书、但统一仓库缺行，overview 同步会自动补回并显示。
- 如果统一仓库里的证书记录缺少 `displayId`，会自动补绑后显示，避免“隐藏证书”长期滞留在库里。

---

## 11. Cron、nftables、端口跳跃、Core 更新

### 11.1 Cron 统一入口

入口：`cronjob/cronJob.go`

当前注册：

- 启动即执行一次：`TLSPathSyncJob`（goroutine 直接 `Run()`）
- 每 5 秒：`NftCoreSyncJob`
- 每 5 秒：`MihomoNftCoreSyncJob`
- 每 10 秒：`StatsJob`
- 每 10 秒：`PortHopRefreshJob`（仅 traffic 关闭时单独跑）
- 每 10 秒：`MihomoPortHopRefreshJob`
- 每 1 分钟：`DepleteJob`
- 每天：`DelStatsJob`（traffic 开启时）
- 每 1 分钟：`CheckCoreJob`
- 每 1 分钟：`CheckMihomoCoreJob`
- 每 1 分钟：`SubGroupAutoUpdateJob`
- 每 30 秒：`TLSPathSyncJob`

### 11.2 nftables 是双链并行

- 默认链：`service/nftables*.go`
- Mihomo 链：`service/mihomo_nftables.go`
- 面板防火墙链：`service/firewall.go` + `service/firewall_scan.go`

保存入站时不会“永远直接写死规则”，而是：

- 先写状态
- 由保存后处理和定时任务按 core 是否运行来同步实际规则

面板防火墙补充现实：

- 防火墙不是挂在现有 `inet kwor` 统计表里，而是单独维护自己的 `inet kwor_firewall` 表
- 真正的开关、规则渲染、默认 SSH / 面板 / 订阅保留规则，都在 `service/firewall.go`
- `service/firewall_nftables_install.go` 负责 Linux 下 `nft` 缺失检测、发行版/包管理器探测、`manualCommands/reason` 状态建模，以及自动安装命令选择
- 对“外部程序自己写进 nftables 的放行规则”的扫描、展示与可选清理，在 `service/firewall_scan.go`
- 外部扫描结果默认不参与面板自己的放行链；真正下发到 `inet kwor_firewall` 的只有系统保留规则、面板手工规则以及 GeoIP 规则
- 定时同步入口是 `cronjob/firewallSyncJob.go`
- 设置页 UI 入口在 `temp_frontend/src/components/SettingsFirewallManage.vue`
- `api/firewall-overview` 现在会额外返回 `nftables` 状态对象；`api/firewall-nftables-install` 是防火墙页点击“下载 nftables”后的安装入口
- 防火墙页顶部总开关左侧现在会在“Linux 且未检测到 `nft` 二进制”时显示“下载 nftables”按钮；安装成功后按钮会立即隐藏
- 自动安装现在会在安装 `nftables` 包后尝试 `enable --now nftables`（或 fallback 到 `service nftables start`）；即使服务启动分支未命中，真正的规则接管仍由 `service/firewall.go` 与 `cronjob/firewallSyncJob.go` 继续完成
- 权限策略固定为：`kwor` 进程本身是 `root` 就直接执行；非 `root` 且存在 `sudo` 时走 `sudo -n` 无交互提权；没有 `sudo`、包管理器不支持、或命中 `rpm-ostree` / `transactional-update` 这类事务式系统时，只返回明确原因和可复制的手动命令
- 当前实现重点是“入站放行”场景，默认会额外保留已建立连接、回环，以及 ICMP / ICMPv6 的必要控制报文；`ping` 请求需要显式 ICMP 规则
- 面板防火墙生效方式是直接执行 `nft` 命令批量重建运行时表，规则立即生效，不需要重启 `nftables` 服务；只有改系统持久化配置时才涉及 reload / restart
- GeoIP 来源识别是独立于普通规则表的第二套规则链
- GeoIP 前端来源选项在 `temp_frontend/src/components/SettingsFirewallGeoOptions.ts`
- GeoIP 下载、缓存、更新周期和热更新入口在 `service/firewall_geoip.go`
- GeoIP 文件解析和 nft 分组渲染在 `service/firewall_geoip_parser.go`、`service/firewall_geoip_nft.go`
- GeoIP 缓存目录是 `Promanager_data/geoip`
- 默认来源顺序是“先 JSON，再 Clash，命中第一个可用源即停止”；只有用户显式填写完整 URL 时才会多源合并
- 即使防火墙总开关关闭，GeoIP 缓存仍会按更新周期继续刷新；真正开启后才会把最新缓存热更新进 nftables

### 11.3 端口占用检查现在前后端都接上了

后端入口：`api/portOccupancy` + `service/port_check.go`

前端入口：

- `temp_frontend/src/layouts/modals/Inbound.vue`
- `temp_frontend/src/views/Inbounds.vue`
- `temp_frontend/src/plugins/portCheck`

当前行为：

- `listen_port` blur 检查单端口占用
- `port_hop_range` blur 检查 UDP 范围占用
- Inbounds 页还会定时监控端口跳跃范围
- localStorage key 按 namespace 区分

### 11.4 两套 core manager 都带自动检查更新

- sing-box：`service/coreManager.go`
- mihomo：`service/mihomo_core_manager.go`
- Linux 子服务会在 `ExecStartPre` 通过 `kwor materialize-core-config <singbox|mihomo>` 刷新运行配置，避免 SQLite 托管配置与 systemd 独立启动链脱节。sing-box 的 `core/singbox/config.json` 是持久落盘文件；mihomo 的 `core/mihomo/server.yaml` 仍按临时实体化方式清理。

前端统一用：

- `SingboxCore.vue`

但具体接口由 `uiNamespace.ts` 切换。

---

## 12. 默认值与环境变量

### 12.1 后端权威默认值

入口：`service/setting.go`

重要默认值：

- `webPort = 8888`
- `webPath = /app/`
- `subPort = 22780`
- `subPath = /sub/`
- `trafficAge = 30`
- `timeLocation = Asia/Tehran`
- `sessionMaxAge = 0`
- `subUpdates = 12`
- `serverTlsStoreEnabled = true`
- `serverTlsStore = chrome`
- `clientTlsStoreEnabled = true`
- `clientTlsStore = chrome`
- `mihomo_config` 初始是最小 route 模板

### 12.2 当前运行期主要环境变量

入口：`config/config.go`

当前主代码直接读取：

- `KWOR_LOG_LEVEL`
- `KWOR_DEBUG`
- `KWOR_DB_FOLDER`

额外说明：

- `KWOR_BIN_FOLDER` 只在旧迁移逻辑 `cmd/migration/1_2.go` 中出现，不是当前正常运行主链的一部分。

---

## 13. 改需求时的最短路径

### 场景 A：改默认链的保存/生成行为

1. `api/apiService.go`
2. `service/config.go`
3. `service/promanager.go`
4. 如涉及 core 运行，再看 `service/coreManager.go`

### 场景 B：改 mihomo 的 `server.yaml` 输出

1. `service/mihomo_config.go`
2. `service/mihomo_manager.go`
3. `service/mihomo_proxy_convert.go`
4. `service/mihomo_route_render.go`
5. `database/model/mihomo.go`

### 场景 C：改 mihomo 路由编辑器行为

1. `temp_frontend/src/views/Rules.vue`
2. `temp_frontend/src/layouts/modals/Rule.vue`
3. `temp_frontend/src/layouts/modals/Ruleset.vue`
4. `temp_frontend/src/types/rules.ts`
5. `service/mihomo_route_sanitize.go`
6. `service/mihomo_route_render.go`

### 场景 D：改 TLS 页面行为

1. `temp_frontend/src/views/Tls.vue`
2. `temp_frontend/src/types/tls.ts`
3. 默认链看 `service/tls.go`
4. mihomo 链看 `service/mihomo_tls.go`
5. 模型清洗看 `database/model/mihomo.go`

### 场景 E：某个客户端同步到 SubManager 不对

1. `temp_frontend/src/views/Clients.vue`
2. 默认链看 `service/syncService.go`
3. Mihomo 链看 `service/mihomo_sync.go`
4. 继续看 `suboutbounds` / `subgroups` / `sub_json_file_guard.go`

### 场景 F：core 按钮、下载、版本列表不对

1. `temp_frontend/src/layouts/modals/SingboxCore.vue`
2. `temp_frontend/src/store/uiNamespace.ts`
3. 默认链看 `service/coreManager.go`
4. Mihomo 链看 `service/mihomo_core_manager.go`

### 场景 G：订阅输出不对

1. `sub/subHandler.go`
2. `sub/jsonService.go`
3. `sub/clashService.go`
4. `service/promanager.go`
5. `service/subgroups_auto_update.go`（如果是自动拉取来源）

### 场景 H：证书热更新、自签、面板 HTTPS 不对

1. `app/panel_cert_manager.go`
2. `service/panel_self_signed_cert.go`
3. `service/panel_self_signed_cert_paths.go`
4. `web/web.go`
5. `sub/sub.go`

### 场景 I：证书列表、查看证书、统一仓库不对

1. `database/model/certificate_record.go`
2. `service/certificate_inventory.go`
3. `service/acme_service.go`
4. `service/panel_sqlite_cert_store.go`
5. `service/setting.go`（`SyncPanelImportedCertificatesToInventory`）
6. `service/config.go`（settings 保存后的 post-commit hook）
7. `api/apiService.go`（`acme-view`）
8. `temp_frontend/src/components/SettingsAcmeManage.vue`

---

## 14. 高风险坑位（AI 必看）

1. 不要直接改 `web/html/*`
- 这是构建产物，源代码在 `temp_frontend/src/*`

2. 不要把 `Promanager_data/core/singbox/config.json` 当手写源文件
- 它会被 `ProManagerService` 重写

3. 不要把 `Promanager_data/core/server.yaml` 当手写源文件
- 它会被 `MihomoManagerService.RegenerateServerConfig()` 重写

4. 不要把 `frontend/` 当当前本地主前端
- 当前构建脚本实际用的是 `temp_frontend/`

5. `service/config.go` 里的 dummy core 不是实际 core
- 真正运行控制在 `service/coreManager.go` 和 `service/mihomo_core_manager.go`

6. `Mihomo*.vue` 不是独立业务实现
- 它们只是共用页面的薄包装

7. `SingboxCore.vue` 文件名具有误导性
- 它现在同时服务 sing-box 和 mihomo

8. Mihomo 订阅二维码现在是开启的
- `uiNamespace.ts` 里 `supportsSubscriptionQr = true`
- 旧文档里“mihomo 不支持 subscription QR”已经失效

9. Mihomo 前端当前并没有隐藏 `shadowtls` 和 `mieru`
- 当前隐藏的是 `direct`、`naive`、`hysteria`

10. Mihomo 路由编辑器是受限子集
- 逻辑规则会被清掉
- 只保留 route / reject

11. Mihomo 当前没有完整在线/流量统计链
- `enableTraffic = false`
- `onlines` 固定空

12. `Settings.vue` 的 `subPort` 本地默认值不是权威值
- 真正默认值在 `service/setting.go`

13. `Promanager_data/Inbound` 目录名大小写要注意
- 代码用的是大写 `Inbound`

14. `sub_json` 文件名会做冲突保护
- subgroup 名、suboutbound tag、client subscription 文件名冲突会直接阻止保存

15. `KWOR_BIN_FOLDER` 不是当前运行期核心配置
- 它只留在旧迁移逻辑里

---

## 15. 本文档的维护规则

出现以下变化时，必须同步更新本文档：

- 菜单或路由变化
- `uiNamespace.ts` 变化
- `Drawer.vue` / `AppBar.vue` 标题前缀逻辑变化
- `data.ts` / `mihomoData.ts` 对象映射变化
- `sub/subHandler.go` 路由入口变化（尤其 `/q/*` 别名）
- `api/save` 分发对象变化
- `coreManager` / `mihomo_core_manager` 行为变化
- `Promanager_data/core/singbox/config.json` 或 `server.yaml` 生成链变化
- Mihomo 支持/隐藏的 inbound/outbound 类型变化
- `supportsSubscriptionQr`、`subscriptionPathPrefix` 等 namespace 配置变化
- `sub_json` 目录命名规则变化
- nftables / port-hop / cron job 变化
- 设置页里的防火墙托管逻辑、系统保留端口策略、外部规则扫描/展示逻辑变化
- 证书热更新、自签证书路径策略变化
- Owner command-ban 规则变化（见下节）

---

## 16. Owner Hard Rule（必须遵守）

- **禁止运行：** `go test ./service`
- 如确需测试 `service`，只允许在用户明确同意后运行精确 `-run` 的定向测试命令。
- 不确定时先询问，不得默认执行任何全量或长时测试命令。
- 默认只运行与本次改动直接相关的小测试，不再自行尝试整包长测。
- 即使前面的定向测试已经通过，也**禁止**为了“更稳一点”“补回归”“顺手确认”而自行追加 `go test ./service`、`go test ./...` 或任何整包/全量长测试。

---

最后更新：`2026-05-03`
