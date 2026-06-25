# AI 编程必读工程文件概述（kwor / s-ui-main）

> 目标：让 AI 编程助手或新接手开发的人，先分清 **源码在哪、运行期文件在哪、默认链与 Mihomo 链如何并行、改一处时要联动检查哪里**。
>
> 扫描基线：`2026-06-18`
>
> 适用场景：**Win10 + VSCode + Go + Node + npm.cmd 本地开发**，然后构建并发布到其他平台运行。
>
> 结论来源：以当前仓库源码、构建脚本、启动入口为准；**README、旧文档、历史目录、运行期生成文件都只能辅助参考，不能反过来覆盖源码事实。**

---

## 0. 先看这一页：这份文档怎么用

### 0.1 这不是产品介绍，而是“AI 任务导航图”

这个项目不是单一网页，也不是单一 Go 服务，而是下面几条链叠加起来的：

1. **Go 后端主程序**
2. **Vue 3 前端 SPA**
3. **内嵌前端资源的 Web 服务**
4. **订阅服务（普通 / JSON / Clash / Mihomo / SubManager / SubGroup）**
5. **默认链（sing-box）配置生成与运行控制**
6. **Mihomo 链配置生成与运行控制**
7. **系统级运维能力**：证书、防火墙、端口转发、反向代理、流量总览、系统监控、内核包管理等

所以接任务时，**不要把它当成“改一个 Vue 文件”或“改一个 Go 接口”就能结束的项目**。

### 0.2 当前 owner 协作约定（AI 必须遵守）

1. **交流、总结、解释统一使用中文。**
2. **任何修改都必须保持 UTF-8，不得引入乱码。**
3. **Windows 本地前端命令默认使用 `npm.cmd`，不要写成 PowerShell 专属调用方式。**
4. **不要默认跑 `go test ./service` 这类整包长测试。**
   - 优先做小范围、短时、与当前改动直接相关的验证。
   - 即使前面的小验证通过，也不要顺手补跑全量长测。
5. 改功能前，先判断需求属于哪一层：
   - 默认链（sing-box）
   - Mihomo 链
   - 前后端共享层
   - 运行期生成层
   - 构建 / 发布层
6. 看到一个文件时，要主动思考：
   - 它被谁调用
   - 它把数据交给谁
   - 保存后会重写哪些运行文件
   - 前端或后端是否还有另一条并行链也要一起改

### 0.3 权威来源优先级

遇到文档与代码冲突时，按下面优先级判断：

1. **源码实现**
2. **路径函数 / 默认值函数 / 构建脚本**
3. **启动入口与 API 路由**
4. **README**
5. **旧文档**
6. **运行期生成文件**

特别是以下文件，属于高权威来源：

- 路径与运行目录：`config/config.go`
- 默认值与设置项：`service/setting.go`
- 应用启动链：`main.go`、`app/app.go`
- API 真正入口：`api/apiHandler.go`、`api/apiService.go`
- 前端分流总开关：`temp_frontend/src/store/uiNamespace.ts`
- 默认链配置生成：`service/promanager.go`
- Mihomo YAML 生成：`service/mihomo_manager.go`
- Core 运行控制：`service/coreManager.go`、`service/mihomo_core_manager.go`

### 0.4 快速任务分类法（接任务先套这个）

如果用户一句话描述需求，先把它归到下面某一类，再开始查文件：

1. **页面 / 表单 / 交互问题**
   - 先看：`temp_frontend/src/views/*`、`temp_frontend/src/layouts/modals/*`
   - 再看：`temp_frontend/src/store/uiNamespace.ts`、对应 store
2. **保存后结果不对 / 运行文件没更新**
   - 先看：`api/apiService.go`、`service/config.go`
   - 再看：`service/promanager.go` 或 `service/mihomo_manager.go`
3. **URL / 端口 / 路径 / 登录跳转问题**
   - 先看：`service/setting.go`、`web/web.go`、`sub/sub.go`
   - 再看：`temp_frontend/index.html`、`router/index.ts`、`vite.config.mts`
4. **订阅内容 / 二维码 / 导入链接问题**
   - 先看：`sub/subHandler.go`、`sub/jsonService.go`、`sub/clashService.go`
   - 再看：`temp_frontend/src/layouts/modals/QrCode.vue`
5. **外部 core 下载 / 版本 / 启停问题**
   - 先看：`service/coreManager.go`、`service/mihomo_core_manager.go`
   - 再看：`temp_frontend/src/layouts/modals/SingboxCore.vue`、`temp_frontend/src/views/Inbounds.vue`
6. **系统功能（证书 / 防火墙 / 转发 / 反代 / 监控）问题**
   - 先看：对应 `Settings*.vue`、`api/apiHandler.go`、`api/apiService.go`
   - 再看：对应 `service/*.go`
7. **构建 / 安装 / 发布问题**
   - 先看：`build.bat`、`build.sh`、`windows/*`、`.github/workflows/*`
   - 再看：`install.sh`、`cmd/cmd.go`、`config/config.go`、`scripts/sync-version.mjs`、`web/web.go`

---

## 1. 5 分钟读图顺序

如果你刚接任务，只按下面顺序读，基本就能建立正确脑图。

### 1.1 第一轮：先搞清程序怎么启动

1. `main.go`
   - 看无参数启动应用、有参数走 CLI。
2. `app/app.go`
   - 看启动时初始化了哪些服务、哪些运行文件会被自动生成。
3. `config/config.go`
   - 看版本、程序名、`Promanager_data`、数据库路径等基础规则。
4. `service/setting.go`
   - 看面板端口、路径、订阅端口、随机订阅路径、默认配置等。

### 1.2 第二轮：搞清对外暴露了哪些服务

5. `web/web.go`
   - 看 Web 面板路径、静态资源、Session、API、SPA fallback、TLS。
6. `sub/sub.go`
   - 看订阅服务是如何按 `subPath` 启动的。
7. `sub/subHandler.go`
   - 看普通订阅、Mihomo 订阅、SubManager、SubGroup 的具体路由。
8. `api/apiHandler.go`
   - 看 Session API 与 GET/POST action 路由入口。
9. `api/apiService.go`
   - 看前端加载数据、保存配置、系统功能入口怎么串起来。

### 1.3 第三轮：搞清“保存后为什么会连锁重生成”

10. `service/config.go`
    - 这是 `/api/save` 的总分发中心，也是保存后副作用中心。
11. `service/promanager.go`
    - 看默认链（sing-box）运行文件如何生成。
12. `service/mihomo_manager.go`
    - 看 Mihomo 的 `server.yaml` 如何生成。
13. `service/core_layout.go`
    - 看运行期 `core/` 目录与配置文件真实路径。
14. `service/coreManager.go`
    - 看 sing-box 外部 core 的下载、版本、启动、systemd 控制。
15. `service/mihomo_core_manager.go`
    - 看 Mihomo 外部 core 的下载、版本、启动、systemd 控制。

### 1.4 第四轮：搞清前端为什么“一处修改会影响两条链”

16. `temp_frontend/index.html`
    - 看 `BASE_URL` 如何注入到前端。
17. `temp_frontend/src/main.ts`
    - 看 Vue App、router、store、i18n、Vuetify、Notivue 的启动顺序。
18. `temp_frontend/src/router/index.ts`
    - 看登录探测、10 秒轮询、Mihomo 页面入口。
19. `temp_frontend/src/store/uiNamespace.ts`
    - 看默认链与 Mihomo 链如何在前端分流。
20. `temp_frontend/src/store/modules/data.ts`
    - 看默认链的数据加载与保存。
21. `temp_frontend/src/store/modules/mihomoData.ts`
    - 看 Mihomo 链的数据加载与保存。
22. `temp_frontend/src/views/Settings.vue`
    - 看大量系统功能（流量、防火墙、反代、监控等）是如何挂在设置页里的。

### 1.5 真正的最短排查顺序（不要一上来全仓乱翻）

当你已经有具体问题时，推荐按下面顺序排查：

1. **先定层**：前端层 / API 层 / 保存层 / 生成层 / 运行层 / 构建层
2. **再定链**：默认链 / Mihomo 链 / 共享层 / 系统功能层
3. **再找入口**：
   - 页面问题先看 view 或 modal
   - 接口问题先看 `apiHandler.go`
   - 保存问题先看 `service/config.go`
   - 运行配置问题先看 `promanager.go` 或 `mihomo_manager.go`
4. **最后补联动**：
   - 这个字段是否进了 store
   - 这个保存是否会触发文件重生成
   - 这个页面是否有 Mihomo 包装页共用

如果你没先做这 4 步，就很容易出现：

- 找对文件但改错层
- 改对默认链却漏掉 Mihomo 链
- 前端表现改了，但保存后又被后端重写回去

---

## 2. 项目真实形态：先建立全局认知

### 2.1 当前项目不是“前端 + 一个简单后端”

它更接近下面这个结构：

- **面板后端**：Gin + GORM + SQLite
- **前端 SPA**：Vue 3 + Vite + Vuetify + Pinia + TypeScript
- **嵌入式前端资源**：构建后复制到 `web/html/`，再由 Go `embed` 进二进制
- **默认链（sing-box）**：数据库配置 → 运行期 JSON 文件 → 外部 core 运行
- **Mihomo 链**：数据库配置 → `server.yaml` → 外部 core 运行
- **系统能力层**：证书、nftables、防火墙、反代、转发、流量统计、监控等

### 2.2 当前产品名与模块名并不完全一致

- 程序名：`kwor`
- 版本来源：`config/version`
- Go module 仍是：`github.com/alireza0/s-ui`

因此看到 `s-ui`、`kwor` 混用时，不要立刻以为是两个项目，先确认它是历史命名残留还是当前功能语义。

### 2.3 当前主运行模式不是“内嵌 core”

虽然仓库里仍有 `core/` 相关代码，但**当前主链路里真正负责 sing-box / Mihomo 运行控制的，是 `service/coreManager.go` 和 `service/mihomo_core_manager.go`。**

也就是说：

- 真正的 core 下载 / 升级 / 启停：看 `service/*core_manager*.go`
- 真正的运行配置路径：看 `service/core_layout.go`
- 真正的配置生成：看 `service/promanager.go` 与 `service/mihomo_manager.go`

不要先把注意力放在 `core/` 目录，以免误判主链。

---

## 3. 目录边界：哪些是源码，哪些是生成物

### 3.1 目录总表

| 类别 | 位置 | 说明 | 是否应直接修改 |
| --- | --- | --- | --- |
| 后端主源码 | `api/` `app/` `cmd/` `config/` `cronjob/` `database/` `middleware/` `network/` `service/` `sub/` `util/` `web/` | Go 主业务代码 | 是 |
| 前端主源码 | `temp_frontend/src/` | 当前实际前端源码 | 是 |
| 前端入口模板 | `temp_frontend/index.html` | 注入 `BASE_URL`，dev 时回退 `/app/` | 是 |
| 前端构建输出 | `temp_frontend/dist/` | Vite 打包产物 | 否 |
| Go 内嵌前端产物 | `web/html/` | 前端复制后给 Go `embed` 使用 | 否 |
| 运行期数据目录 | `<binary_dir>/Promanager_data/` | DB、core 配置、证书、订阅 JSON 等 | 一般否 |
| 历史/非当前主前端 | `frontend/` | 不是当前本地主开发目录 | 一般否 |
| Windows 构建与安装 | `windows/` | Windows 打包、安装、服务相关脚本 | 按需 |
| 版本同步脚本 | `scripts/sync-version.mjs` | 同步 `config/version` 到前端包元数据 | 按需 |
| 文档目录 | `docs/` | 子系统文档，未必始终跟代码同步 | 辅助参考 |

### 3.2 最容易看错的几个目录

#### `temp_frontend/`
这是**当前真实前端项目根目录**。本地开发、构建、Vite 配置、Pinia、路由、页面，全在这里。

#### `web/html/`
这是**Go 运行时要嵌入的前端打包结果**，不是源码。改这里没有长期意义，下次构建会被覆盖。

#### `temp_frontend/dist/`
这是**Vite 构建输出目录**，同样不是源码。

#### `Promanager_data/`
这是**运行期目录**，真实位置由 `config.GetDataDir()` 决定：

- 规则：`<可执行文件所在目录>/Promanager_data`

因此它通常不固定在仓库根目录，而是跟着最终运行的 `kwor` / `kwor.exe` 走。这里的很多文件都是**启动时、保存配置后、定时任务中自动生成或重写的**。

#### `frontend/`
仓库里虽然有这个目录，但**当前前端构建链实际使用的是 `temp_frontend/`**。不要把 `frontend/` 当成本地主源码入口。

### 3.3 运行期生成物速查表

下面这些路径非常常见，但都不应该被当成“手写源码来源”：

| 路径 | 来源 | 何时更新 | 应该去改哪里 |
| --- | --- | --- | --- |
| `Promanager_data/db/kwor.db` | SQLite 主库 | 保存配置、运行时 | `database/` + `service/` |
| `Promanager_data/core/singbox/config.json` | 默认链配置生成器 | 启动时、默认链保存后 | `service/promanager.go` `service/config.go` |
| `Promanager_data/core/mihomo/server.yaml` | Mihomo 生成器 | 启动时、任意 `mihomo_*` 保存后 | `service/mihomo_manager.go` |
| `Promanager_data/core/mihomo_inbounds_meta.json` | Mihomo 入站元数据生成器 | Mihomo 配置重生成时 | `service/mihomo_manager.go` |
| `Promanager_data/Inbound/*.json` | 默认链入站文件生成器 | `inbounds` / `clients` / `tls` / `settings` 相关保存后 | `service/promanager.go` |
| `Promanager_data/outbound/*.json` | 默认链出站文件生成器 | `outbounds` / `outboundgroups` / `settings` 相关保存后 | `service/promanager.go` |
| `Promanager_data/sub_json/*.json` | 订阅 JSON 生成器 | 客户端、组、订阅输出相关保存后 | `service/promanager.go` `service/suboutbounds.go` `service/subgroups.go` |
| `Promanager_data/cert/*` | 证书中心 / 自签 / 导入 | 证书签发、应用、迁移时 | `service/acme_service.go` `service/panel_*` |
| `Promanager_data/install.sh` | Linux 安装脚本副本 | 安装/面板内升级后 | `install.sh` `service/panel_update.go` |
| `Promanager_data/kwor.service` | Linux service 模板副本 | 安装/面板内升级后 | `install.sh` `service/panel_update.go` |
| `temp_frontend/dist/*` | Vite build 结果 | 前端构建后 | `temp_frontend/src/*` |
| `web/html/*` | 拷贝后的嵌入前端 | 前端构建并复制后 | `temp_frontend/src/*` |

---

## 4. 启动链、服务链与关键入口

## 4.1 启动总入口

### 无参数启动

链路：

`main.go -> runApp() -> app.NewApp() -> app.Init() -> app.Start()`

### 有参数启动

链路：

`main.go -> cmd.ParseCmd()`

### `cmd` 常用子命令

在 `cmd/cmd.go` 里可以看到主要命令入口：

- `start`
- `stop`
- `resetadmin`
- `uninstall`
- `admin`
- `setting`
- `uri`
- `migrate`

另外还有给 systemd / 内部链路使用的辅助命令，例如：

- `materialize-core-config`
- `cleanup-core-config`

### `kwor start` 首次初始化要注意的事实

`cmd/cmd.go` 里的 `firstRunSetup()` **只会交互式询问下面几项**：

- 面板端口
- 面板路径
- 管理员用户名
- 管理员密码

并不会在首次交互里单独询问订阅端口或订阅路径。

也就是说：

- `subPort` 默认值来自 `service/setting.go`
- `subPath` 若为空，会在 `service/setting.go` 中自动生成随机路径

因此不要再把“首次启动默认订阅路径固定是 `/sub/`”当成当前事实。

## 4.2 `app.Init()` 做了什么

`app/app.go` 中，`Init()` 是真正的应用装配入口。当前它会做这些关键动作：

1. 初始化日志
2. 初始化数据库：`database.InitDB(config.GetDBPath())`
3. 初始化运行期托管文件存储：`service.InitManagedRuntimeFileStore()`
4. 初始化系统监控存储：`service.InitSystemMonitorStore()`
5. 确保 core 运行目录存在：`service.EnsureManagedCoreLayout()`
6. 读取全部 settings，并做若干启动修复 / 迁移动作
7. 同步 panel / sub 的证书分配关系
8. 准备 ACME 概览一致性
9. 清理启动前遗留的临时防火墙规则
10. 创建：
   - `cronJob`
   - `webServer`
   - `subServer`
   - `reverseProxy`
11. 注册 TLS 运行时应用器与防火墙运行时端口提供器
12. 创建 `ConfigService`
13. **启动前先主动生成两条配置链：**
   - 默认链：`service.NewProManagerService(...).SaveInboundJson()`
   - Mihomo 链：`service.NewMihomoManagerService().RegenerateServerConfig()`

重点：**程序启动前就会先把运行配置落盘。**

## 4.3 `app.Start()` 做了什么

`app/app.go` 中，`Start()` 会：

1. 启动时先同步托管 nftables 生命周期：`service.SyncManagedNftablesOnStartup()`
2. 读取时区和流量保留设置
3. 启动 cron
4. 启动 Web Server
5. 启动 Sub Server
6. 启动 Reverse Proxy 运行时
7. 启动流量总览 runtime probe
8. 启动系统监控 runtime probe
9. Linux 下延迟检查并自恢复 sing-box / Mihomo 托管 core

### Cron 当前主要注册了什么

`cronjob/cronJob.go` 当前会注册（含启动即跑的一次性执行）这些关键任务：

- nft core sync
- Mihomo nft core sync
- firewall sync
- port-forward sync
- reverse-proxy sync
- panel certificate balance sync
- TLS path sync
- stats job
- port hop refresh job
- deplete job
- sing-box core update check
- Mihomo core update check
- subgroup auto update
- ACME auto renew

所以遇到“为什么我没点保存，它也会自动改状态/改文件/改运行规则”时，**别忘了把 cron 也算进去。**

### 4.3.1 `app.Stop()` 与 `app.RestartApp()` 也值得关注

很多人只看启动，不看停止和重启，但这个项目里它们也有实际副作用：

- `app.Stop()` 会：
  - 停止 cron
  - 按条件清理托管 nftables 运行时规则
  - flush 流量总览快照
  - 停止 Sub / Web / Reverse Proxy 运行时
- `app.RestartApp()` 会：
  - 标记 panel-only stop
  - 重新创建 `webServer` 与 `subServer`
  - 再次调用 `Start()`

因此遇到“重启后状态不一致”“停服务后某些规则没了/没清理”“反代只在重启后生效”这类问题时，要把：

- `app/app.go`
- `service/nft_lifecycle.go`
- `service/reverse_proxy.go`
- `service/traffic_overview.go`

一起纳入排查。

### 4.3.2 托管 nftables 生命周期不是只属于 sing-box

`service/nft_lifecycle.go` 当前会在启动 / 停止时统一处理多类运行时规则：

- firewall
- port-forward
- traffic-cap
- 默认链 core 相关 nft 规则
- Mihomo core 相关 nft 规则

所以不要把 `nftables` 理解成“只给 sing-box 用的附属功能”；它其实是多个系统能力共享的运行时基础设施。

## 4.4 Web 服务入口：`web/web.go`

当前 Web 服务的关键事实：

- 使用 `SettingService.GetWebPath()` 作为面板基础路径
- Session Cookie 名是：`kwor`
- 静态资源挂载在：`<webPath>assets/`
- Session API 挂载在：`<webPath>api/*`
- Token API 挂载在：`<webPath>apiv2/*`
- 前端 HTML 模板来自内嵌的 `html/index.html`
- 对不符合规则的路径会做 SPA fallback 或 anti-probe 假响应
- 启动时会根据 `EnsurePanelTLSMaterials(...)` 决定是走 HTTPS 还是 HTTP fallback

### `BASE_URL` 是前后端协作关键点

后端在渲染 `index.html` 时，会把：

- `BASE_URL = webPath`

注入到前端，前端再用它做：

- 路由 history base
- 页面入口路径判断
- 登录跳转基准路径

因此改 `webPath` 不能只改一边。

## 4.5 Sub 服务入口：`sub/sub.go` + `sub/subHandler.go`

订阅服务会按 `subPath` 启动一个独立的 HTTP 服务组。

当前核心路由包括：

### 默认链客户端订阅

- `GET /q/client`
- `HEAD /q/client`
- `GET /:subid`
- `HEAD /:subid`

### Mihomo 链客户端订阅

- `GET /q/mihomo`
- `HEAD /q/mihomo`
- `GET /mihomo/:subid`
- `HEAD /mihomo/:subid`

### SubManager 订阅

- `GET /q/sm`
- `GET /sm/:tag`

### SubGroup 订阅

- `GET /q/group`
- `GET /group/:groupName`

### 订阅格式切换

通过查询参数：

- `?format=json`
- `?format=clash`

分别走 JSON / Clash 渲染链。

## 4.6 API 入口分成两套：Session API 与 Token API

### Session API：`api/apiHandler.go`

这是面板前端最常走的完整接口集合，特点是：

- 功能最全
- 覆盖保存、加载、系统功能、证书、监控、防火墙、反代、core 管理等大多数能力
- 依赖 Session 登录状态

### Token API：`api/apiV2Handler.go`

这是另一套基于 Token 的 API，特点是：

- 通过 `Token` 请求头鉴权
- 功能覆盖面比 Session API 小
- 主要提供：
  - `load`
  - 部分 partial load
  - `save`
  - `restartApp`
  - `restartSb`
  - `linkConvert`
  - `importdb`
  - `portOccupancy`

### 结论

如果用户说“接口没返回这个字段”“某个 API 能不能也支持某功能”，先确认他说的是：

- 面板 Session API
- 还是 Token API

不要默认以为两套接口能力完全一致。

---

## 5. 前端主链：入口、路由、双 Store、命名空间

## 5.1 前端启动链

链路：

`temp_frontend/index.html -> temp_frontend/src/main.ts -> temp_frontend/src/App.vue`

### `temp_frontend/index.html`

负责：

- 注入 `window.BASE_URL = "{{ .BASE_URL }}"`
- 在 dev fallback 场景下，如果没被后端模板替换，就回退到 `/app/`

### `temp_frontend/src/main.ts`

负责：

- 创建 Vue App
- 挂载 router
- 挂载 Pinia store
- 挂载 i18n
- 挂载 Notivue
- 挂载 Vuetify
- 注入全局 loading 状态

### `temp_frontend/src/App.vue`

负责：

- 全局 loading overlay
- message 组件
- `router-view`
- 页面标题基础设置

## 5.2 路由与登录探测：`temp_frontend/src/router/index.ts`

这个文件非常关键，因为它决定了：

1. 页面路由如何定义
2. 登录态怎么探测
3. 何时跳到 `/login`
4. 何时开始 10 秒轮询数据
5. Mihomo 页面如何挂到主菜单体系里

### 当前主要页面

默认链页面：

- `/`
- `/submanager`
- `/inbounds`
- `/clients`
- `/outbounds`
- `/rules`
- `/tls`
- `/dns`
- `/basics`
- `/admins`
- `/settings`

Mihomo 页面：

- `/mihomo_inbounds`
- `/mihomo_clients`
- `/mihomo_outbounds`
- `/mihomo_tls`
- `/mihomo_rules`
- `/mihomo_dns`

### 10 秒轮询不是只拉一套数据

进入已登录页面后，router 会定时执行：

- `Data().loadData()`
- `MihomoData().loadData()`

也就是：**默认链和 Mihomo 链两套 store 会常驻并行刷新。**

因此很多页面虽只显示一套内容，但后台两套数据都可能在更新。

## 5.3 命名空间分流总开关：`temp_frontend/src/store/uiNamespace.ts`

这是前端最关键的“共享页分流器”。

它决定：

- 该页面使用哪个 store
- 调哪个 API endpoint
- 订阅二维码 URL 前缀是什么
- 使用哪个 core manager 接口
- 使用哪个运行配置路径显示文本
- Inbounds 页是否显示 core 控制按钮

### 默认链 namespace 配置

- `syncEndpoint = api/syncToSubManager`
- `inboundIpsEndpoint = api/inbound-ips`
- `subscriptionPathPrefix = ""`
- `supportsSubscriptionQr = true`
- `showCoreControlsOnInbounds = true`
- core config path = `Promanager_data/core/singbox/config.json`

### Mihomo namespace 配置

- `syncEndpoint = api/mihomoSyncToSubManager`
- `inboundIpsEndpoint = api/mihomo-inbound-ips`
- `subscriptionPathPrefix = "mihomo/"`
- `supportsSubscriptionQr = true`
- `showCoreControlsOnInbounds = true`
- core config path = `Promanager_data/core/mihomo/server.yaml`

### 这意味着什么

- Mihomo 现在**支持订阅二维码**，只是链接前缀不同
- `SingboxCore.vue` 虽然名字像 sing-box 专属，但实际上会根据 namespace 切换到 Mihomo 接口
- 共享页面修改后，要先问自己：**默认链和 Mihomo 链会不会一起被影响？**

## 5.4 两套 Store：`data.ts` 与 `mihomoData.ts`

### 默认链 Store：`temp_frontend/src/store/modules/data.ts`

负责：

- `api/load` 拉取默认链数据
- `api/save` 保存默认链对象
- 保存对象名通常是：
  - `config`
  - `clients`
  - `tls`
  - `inbounds`
  - `outbounds`
  - `outboundgroups`
  - `suboutbounds`
  - `subgroups`
  - `services`
  - `endpoints`

### Mihomo Store：`temp_frontend/src/store/modules/mihomoData.ts`

负责：

- `api/mihomo-load` 拉取 Mihomo 链数据
- 保存时把前端对象名映射为：
  - `mihomo_config`
  - `mihomo_clients`
  - `mihomo_tls`
  - `mihomo_inbounds`
  - `mihomo_outbounds`
  - `mihomo_outboundgroups`

### 共享订阅管理数据要特别注意

`getMihomoData()` 虽然是 Mihomo 数据接口，但它仍会带回：

- `suboutbounds`
- `subgroups`

这说明：**Mihomo 页面并不是完全独立世界，订阅管理层仍有共享数据。**

## 5.5 哪些 Mihomo 页面只是“薄包装”

以下页面本质上只是把 `namespace="mihomo"` 传给共用页：

- `MihomoInbounds.vue` -> `Inbounds.vue`
- `MihomoClients.vue` -> `Clients.vue`
- `MihomoOutbounds.vue` -> `Outbounds.vue`
- `MihomoTls.vue` -> `Tls.vue`
- `MihomoRules.vue` -> `Rules.vue`

因此：

- 改 `Inbounds.vue`，默认链与 Mihomo 链都会受影响
- 改 `Clients.vue`，默认链与 Mihomo 链都会受影响
- 改 `Outbounds.vue`，默认链与 Mihomo 链都会受影响
- 改 `Tls.vue`，默认链与 Mihomo 链都会受影响
- 改 `Rules.vue`，默认链与 Mihomo 链都会受影响

### 例外：`MihomoDns.vue`

`MihomoDns.vue` 是独立页面，不是简单包装页。它直接围绕 `mihomo_config.dns` 相关字段工作。

### 共享页面 / 命名空间映射速查

| 页面 | 默认链实现 | Mihomo 链实现 | 说明 |
| --- | --- | --- | --- |
| Inbounds | `views/Inbounds.vue` | `views/MihomoInbounds.vue` -> 同一个 `Inbounds.vue` | 最典型的共享页 |
| Clients | `views/Clients.vue` | `views/MihomoClients.vue` -> 同一个 `Clients.vue` | 客户端管理共享 |
| Outbounds | `views/Outbounds.vue` | `views/MihomoOutbounds.vue` -> 同一个 `Outbounds.vue` | 出站管理共享 |
| Tls | `views/Tls.vue` | `views/MihomoTls.vue` -> 同一个 `Tls.vue` | TLS 页面共享 |
| Rules | `views/Rules.vue` | `views/MihomoRules.vue` -> 同一个 `Rules.vue` | 路由页共享 |
| Dns | `views/Dns.vue` | `views/MihomoDns.vue` | Mihomo DNS 是独立实现 |

### 菜单与页头标题还有一层“默认链前缀”规则

左侧菜单和页头并不只是直接显示路由名：

- `temp_frontend/src/layouts/default/Drawer.vue` 会给默认链页面标题加 `singbox_` 前缀
- `temp_frontend/src/layouts/default/AppBar.vue` 也会给默认链页头标题加 `singbox_` 前缀
- 默认链中的 `services` / `endpoints` 现在在 UI 里是暂时隐藏的，但路由和 API 仍然存在

这意味着：

- 路由存在 ≠ 菜单可见
- 页面标题 ≠ 路由原名
- 共享页改动时，别只看一个 view，菜单和页头也可能要联动

### 再补一个容易漏掉的点：首页也有两套 core 状态入口

`temp_frontend/src/components/Main.vue` 首页卡片里，会同时显示：

- Sing-Box 运行状态
- Mihomo 运行状态

并且都可以直接执行启动 / 停止 / 重启。

所以 core 相关改动不要只查 `Inbounds.vue` 和 `SingboxCore.vue`，还要联查：

- `temp_frontend/src/components/Main.vue`

## 5.6 设置页不是“小设置”，而是系统能力总入口

`temp_frontend/src/views/Settings.vue` 当前不仅有面板基础设置，还挂了很多大功能页签：

- 面板接口设置
- 订阅设置
- JSON 订阅扩展
- Clash 订阅扩展
- 流量总览
- 防火墙
- 端口转发
- 系统优化
- 证书管理
- 反向代理
- 内核包管理
- 系统监控

所以当用户说“改设置页”时，先问清楚是：

- 基础面板设置
- 订阅设置
- 还是某个系统功能子模块

### 设置页保存前不是原样直传

`Settings.vue` 保存前会先做几件整理动作：

- 端口空值 / 非法值会先按默认值归一化
  - `webPort -> 8888`
  - `subPort -> 22780`
- JSON 订阅页会先提交临时编辑行：
  - `commitCustomRuleRows()`
  - `commitDnsRouteRows()`
- Clash 订阅页会先提交临时编辑行：
  - `commitClashRuleRows()`
  - `commitClashDnsPolicyRows()`
- 证书绑定字段会在 payload 中主动删除，避免旧 UI 状态把当前证书分配回滚掉

因此“设置页看起来没改这个字段，为什么保存后结果变了”时，先别急着怀疑后端，前端保存前本身就有一层整理逻辑。

---

## 6. 两条配置链：默认链（sing-box）与 Mihomo 链

## 6.1 对照表

| 维度 | 默认链（sing-box） | Mihomo 链 |
| --- | --- | --- |
| 前端 store | `data.ts` | `mihomoData.ts` |
| 数据加载接口 | `api/load` | `api/mihomo-load` |
| 保存对象名 | `config` `clients` `tls` `inbounds` `outbounds` `outboundgroups` `suboutbounds` `subgroups` `services` `endpoints` | `mihomo_config` `mihomo_clients` `mihomo_tls` `mihomo_inbounds` `mihomo_outbounds` `mihomo_outboundgroups` |
| 前端分流开关 | `uiNamespace.ts` 默认配置 | `uiNamespace.ts` mihomo 配置 |
| 主配置生成器 | `service/promanager.go` | `service/mihomo_manager.go` |
| 运行配置路径 | `Promanager_data/core/singbox/config.json` | `Promanager_data/core/mihomo/server.yaml` |
| 额外生成物 | `Inbound/*.json` `outbound/*.json` `sub_json/*.json` | `Promanager_data/core/mihomo_inbounds_meta.json` |
| Core 管理器 | `service/coreManager.go` | `service/mihomo_core_manager.go` |
| 订阅 URL 前缀 | `""` | `"mihomo/"` |
| Inbounds 页 core 控制 | 显示 | 显示 |

## 6.2 默认链（sing-box）要记住的事实

- 数据源不只是 `settings.config`
- `ConfigService.GetConfig()` 会把：
  - `settings.config`
  - inbounds
  - outbounds
  - services
  - endpoints
  重新拼成完整 sing-box 配置对象
- 真正的配置生成逻辑在 `service/promanager.go`
- 真正的 core 下载 / 运行控制在 `service/coreManager.go`

## 6.3 Mihomo 链要记住的事实

- 数据源是 `settings.mihomo_config` + Mihomo 各对象表
- 最终不是生成 JSON，而是生成 `server.yaml`
- 真实主生成器是 `service/mihomo_manager.go`
- 真实运行控制是 `service/mihomo_core_manager.go`
- 运行配置真实路径是：
  - `Promanager_data/core/mihomo/server.yaml`
- 额外元数据路径是：
  - `Promanager_data/core/mihomo_inbounds_meta.json`

**不要再把 Mihomo 的权威运行配置路径写成旧式 `Promanager_data/core/server.yaml`。**

## 6.4 Mihomo 额外要注意的并行限制

Mihomo 链不是“默认链的 YAML 皮肤”，它还有自己的前端限制与后端裁剪逻辑：

- 前端某些 listener / outbound 类型在 Mihomo 语境下是受限的
- 后端会对规则、listener、outbound 做裁剪 / 转换 / 回退
- 不是所有 UI 上能建出来的对象，最后都能原样进入 `server.yaml`

所以改 Mihomo 时，不要只盯一个页面；要继续往后端转换层追：

- `service/mihomo_config.go`
- `service/mihomo_proxy_convert.go`
- `service/mihomo_route_render.go`
- `database/model/mihomo.go`

---

## 7. 保存链与生成链：为什么改一个对象会重写很多文件

## 7.1 总入口：`/api/save`

保存总链路是：

`前端 store.save()`
→ `api/save`
→ `api/apiService.go`
→ `service/config.go: ConfigService.Save()`

`ConfigService.Save()` 不只是“存数据库”，它还负责：

1. 开事务保存对象
2. 提交事务
3. 更新变更时间 / 变更记录
4. 跑 managed runtime hooks
5. 触发运行配置重生成
6. 应用 nftables / TLS / 自动同步等后置动作

它是全项目最需要建立全局意识的文件之一。

## 7.2 默认链保存后的常见副作用

| 保存对象 | 常见副作用 |
| --- | --- |
| `inbounds` | 重生成入站文件、重生成 sing-box 主配置、重生成订阅 JSON、可能应用 nft 动作 |
| `clients` | 重生成入站文件、主配置、订阅 JSON，并同步默认链自动托管客户端 |
| `tls` | 重生成入站文件、主配置、订阅 JSON |
| `outbounds` | 重生成出站文件、重生成主配置 |
| `outboundgroups` | 重生成出站文件、重生成主配置 |
| `services` `endpoints` | 重生成主配置 |
| `suboutbounds` | 重生成订阅 JSON / 相关共享订阅文件 |
| `subgroups` | 更新组订阅文件 |
| `config` | 重生成默认链整套运行文件 |
| `settings` | 重生成默认链整套运行文件，并应用 panel/sub TLS 运行时设置、防火墙同步等 |

## 7.3 Mihomo 保存后的常见副作用

当保存下面任一对象时：

- `mihomo_inbounds`
- `mihomo_outbounds`
- `mihomo_outboundgroups`
- `mihomo_clients`
- `mihomo_tls`
- `mihomo_config`

都会触发：

- `service.NewMihomoManagerService().RegenerateServerConfig()`
- 也就是重生成：
  - `Promanager_data/core/mihomo/server.yaml`
  - `Promanager_data/core/mihomo_inbounds_meta.json`

## 7.4 默认链生成器：`service/promanager.go`

ProManager 当前最重要的职责是：

- 生成每个入站的 JSON 文件
- 生成每个出站的 JSON 文件
- 生成默认链完整 core 配置
- 生成订阅 JSON 文件

典型输出：

- `Promanager_data/Inbound/*.json`
- `Promanager_data/outbound/*.json`
- `Promanager_data/core/singbox/config.json`
- `Promanager_data/sub_json/*.json`

## 7.5 Mihomo 生成器：`service/mihomo_manager.go`

Mihomo 当前最重要的职责是：

- 读取 `mihomo_config`
- 读取 Mihomo 出入站 / 组 / 客户端数据
- 把结构化对象转换为 Mihomo 可接受的 YAML 文档
- 落盘 `server.yaml`
- 生成入站元数据文件

因此改 Mihomo 路由、代理组、出站协议支持时，**一定要继续追到 `service/mihomo_manager.go`、`service/mihomo_proxy_convert.go`、`service/mihomo_route_render.go`。**

## 7.6 `sub_json` 文件名冲突保护是正式链路，不是边角料

`sub_json` 不是随便往目录里丢文件就行。当前它有正式的冲突保护：

- client subscription 文件名
- subgroup 文件名
- suboutbound tag

三者会经过规范化后相互校验。

这意味着：

- 如果你保存 subgroup 或 suboutbound 失败，先怀疑“文件名冲突”
- 不要先怀疑渲染器本身坏了
- 不要以为改一个名字只影响一张表，它可能会影响整个 `sub_json` 目录

### 7.7 `Changes` 变更记录不是装饰品

`service/config.go` 在保存后会写入 `model.Changes`，前端 `loadData()` / `loadMihomoData()` 又会通过 `lu` 去增量判断是否需要重新拉取。

所以如果你看到“前端明明轮询了，但页面数据没按预期刷新”，要联想：

- `LastUpdate`
- `model.Changes`
- `ConfigService.CheckChanges()`
- 前端 `lu` 参数

这一套是增量刷新机制的核心。

---

## 8. 运行路径与生成物：不要把“仓库里的文件路径”当成“程序真正运行路径”

## 8.1 数据根目录规则

`config/config.go` 中：

- `GetDataDir() = <binary_dir>/Promanager_data`
- `GetRuntimeInstallScriptPath() = <binary_dir>/Promanager_data/install.sh`
- `GetRuntimeServiceFilePath() = <binary_dir>/Promanager_data/kwor.service`

也就是说：

- 运行期数据目录是跟着**可执行文件所在目录**走的
- 不是固定跟着仓库根目录走

### 这意味着什么

- 如果你在仓库根目录直接运行 `kwor.exe`，那运行期目录可能出现在仓库旁边
- 如果你把二进制发布到别的目录，那 `Promanager_data` 也会跟着去那个目录
- 文档里写运行路径时，优先用“相对二进制”的表达法，而不是写死成仓库内路径

## 8.2 数据库路径规则

`config/config.go` 中：

- `GetDBPath() = <data_dir>/db/kwor.db`

即：

- `<binary_dir>/Promanager_data/db/kwor.db`

另外它还会尝试把旧的 `./db/kwor.db` 迁移到新位置。

同理，Linux 历史安装中如果把 `install.sh` / `kwor.service` 放在二进制同级目录，当前启动链也会尽量自动迁移到 `Promanager_data/` 下。

## 8.3 Core 目录与配置路径规则

`service/core_layout.go` 中：

- sing-box core 目录：`Promanager_data/core/singbox/`
- sing-box 配置路径：`Promanager_data/core/singbox/config.json`
- Mihomo core 目录：`Promanager_data/core/mihomo/`
- Mihomo 配置路径：`Promanager_data/core/mihomo/server.yaml`
- Mihomo 入站元数据：`Promanager_data/core/mihomo_inbounds_meta.json`

## 8.4 订阅最终 URL 不是简单拼字符串

`service/setting.go` 里的 `GetFinalSubURI(host)` 会按下面优先级决定订阅 URL：

1. 如果手工设置了 `subURI`，直接用它
2. 否则根据：
   - `subDomain`
   - `subPort`
   - `subPath`
   - 是否给订阅服务分配了证书（决定 `http` 还是 `https`）
   来自动拼接

所以改订阅 URL 行为时，不要只看前端展示字段，后端还有最终拼装逻辑。

## 8.5 系统监控是独立数据库，不在主库里

除了主库 `kwor.db` 外，系统监控还会使用独立数据库：

- `monitor.db`

真实路径由 `config.GetSystemMonitorDBPath()` 决定，通常也在：

- `Promanager_data/db/monitor.db`

### 这意味着什么

- 监控历史清空，不等于主业务数据清空
- `monitor.db` 体积变化，不代表 `kwor.db` 一定有问题
- 如果用户说“数据库占用很大”，要先问清楚是：
  - 主库 `kwor.db`
  - 还是监控库 `monitor.db`

---

## 9. 构建、嵌入与发布链

## 9.1 前端脚本：`temp_frontend/package.json`

当前前端脚本最重要的事实：

- `dev`：先跑 `scripts/sync-version.mjs`，再起 Vite
- `build`：先跑 `scripts/sync-version.mjs`，再 `vue-tsc --noEmit`，再 `vite build`
- `preview`：先同步版本
- `lint`：先同步版本

也就是说：**前端 build/dev/lint 都会先同步版本号。**

## 9.2 版本同步脚本：`scripts/sync-version.mjs`

这个脚本会把：

- `config/version`

同步到：

- `temp_frontend/package.json`
- `temp_frontend/package-lock.json`

所以如果你改版本号后发现前端包版本也变了，不是副作用 bug，而是脚本的设计行为。

## 9.3 Vite 开发环境要注意：2095 不是面板默认端口

`temp_frontend/vite.config.mts` 当前重要设置：

- dev server 端口：`3000`
- 代理：`/app/api -> http://localhost:2095`

这只是 **Vite 联调代理目标**，不是程序运行时默认面板端口。

真正的面板默认端口来自 `service/setting.go`：

- `webPort = 8888`

因此：

- `2095` 是开发代理目标
- `8888` 是默认运行面板端口

这两个数字不是一回事。

## 9.4 前端嵌入链

前端构建真正的嵌入链是：

`temp_frontend/src`（源码）
→ `temp_frontend/dist`（Vite 打包）
→ `web/html`（复制给 Go 内嵌）
→ `web/web.go` 的 `embed`（编进二进制）

### 结论

- 改前端源码：改 `temp_frontend/src/*`
- 不要直接改：
  - `temp_frontend/dist/*`
  - `web/html/*`

## 9.5 Windows 本地开发命令约定

本地 Windows 例子统一按下面写：

```bat
cd temp_frontend
npm.cmd run build
```

如需仅验证前端类型检查与构建，也应优先按当前项目脚本语义执行，不要自行切成 PowerShell 专属写法。

## 9.6 构建脚本职责分工

### 根目录 `build.bat`

职责：**在 Windows 上构建 Linux amd64 / arm64 发布产物**，输出到 `releases/`。

它不是“本地 Windows 运行入口”。

仓库根目录固定保留这两个手册源码文件：

- `User Manual.md`
- `使用手册.md`
发布前要检查的是：这两个文件在 `main` 根目录中存在，而不是把它们额外上传成 release 资产。

### `build.sh`

职责：构建 Linux amd64 的 `kwor`。

### `windows/kwor-windows-build.bat`

职责：**构建本地 Windows `kwor.exe`**。

### `windows/install-windows.bat`

职责：Windows 安装、服务包装、目录初始化、管理员配置等。

## 9.7 Linux 安装脚本职责：`install.sh`

当前 `install.sh` 的职责，不再是“自己重复实现一套 panel setting / admin 设置逻辑”，而是：

1. 探测 Linux 发行版与 CPU 架构
2. 下载 GitHub Release 里的 Linux 发布包
3. 优先按**运行中的 `kwor` 进程**定位旧安装目录
4. 若无运行进程，再按**`kwor.service` 的 `ExecStart` / `WorkingDirectory`** 反推旧安装目录
5. 若两者都没有，则按**默认新装目录 `/opt/kwor`**
6. 升级时调用现有二进制的 `stop`
7. 替换二进制后调用现有二进制的 `start`
8. 安装脚本副本与 `kwor.service` 模板副本统一写入 `Promanager_data/`，并兼容迁移旧的同级文件

因此当前 Linux 安装链要分清三层：

- 发布包下载与替换：`install.sh`
- 首次初始化、`systemd` 注册、首次登录信息：`cmd/cmd.go` 里的 `start`
- 运行期数据目录与数据库路径：`config/config.go`

另外要注意：

- `install.sh` 现在是**目录定位器 + release 下载器 + `start/stop` 包装层**
- 首次安装默认目录是 `/opt/kwor`
- 但如果脚本检测到旧实例正在运行，或检测到已有 `kwor.service`，就会沿用旧目录，不擅自迁移目录

---

## 10. 按任务找文件：最短定位路径

## 10.1 改“面板路径 / 面板端口 / 登录跳转 / 基础 URL”

先看：

1. `service/setting.go`
2. `web/web.go`
3. `temp_frontend/index.html`
4. `temp_frontend/src/router/index.ts`
5. `temp_frontend/vite.config.mts`

还要联查：

- `temp_frontend/src/plugins/httputil.ts`
- `cmd/cmd.go`（首次启动 / `setting` 命令）

## 10.2 改“设置页保存后没生效 / 保存后跳转异常 / 保存后被还原”

先看：

1. `temp_frontend/src/views/Settings.vue`
2. `api/apiService.go`
3. `service/config.go`
4. `service/setting.go`

重点留意：

- 前端保存前会先归一化端口默认值
- JSON / Clash 订阅页有自己的“提交临时行”步骤
- payload 会主动删除证书绑定字段
- 从 HTTP 切到 HTTPS 时，保存成功后前端可能主动跳转

这类问题经常是“前端保存整理 + 后端保存副作用 + 保存后跳转”三者叠加，不要只盯一个 POST 请求。

## 10.3 改“订阅路径 / 订阅端口 / 订阅链接 / 订阅格式输出”

先看：

1. `service/setting.go`
2. `sub/sub.go`
3. `sub/subHandler.go`
4. `sub/jsonService.go`
5. `sub/clashService.go`
6. `temp_frontend/src/views/Settings.vue`

## 10.3 改“API 字段 / 保存行为 / 某个 object 名”

先看：

1. `api/apiHandler.go`
2. `api/apiService.go`
3. `service/config.go`
4. `temp_frontend/src/store/modules/data.ts`
5. `temp_frontend/src/store/modules/mihomoData.ts`
6. `temp_frontend/src/store/uiNamespace.ts`

## 10.4 改“默认链 sing-box 运行配置输出”

先看：

1. `service/config.go`
2. `service/promanager.go`
3. `service/core_layout.go`
4. `service/coreManager.go`

## 10.5 改“Mihomo 的 server.yaml 输出”

先看：

1. `service/mihomo_manager.go`
2. `service/mihomo_config.go`
3. `service/mihomo_proxy_convert.go`
4. `service/mihomo_route_render.go`
5. `database/model/mihomo.go`
6. `service/mihomo_core_manager.go`

## 10.6 改“共享页面（Inbounds / Outbounds / Rules / Tls / Clients）”

先看：

1. `temp_frontend/src/views/Inbounds.vue` / `Outbounds.vue` / `Rules.vue` / `Tls.vue` / `Clients.vue`
2. `temp_frontend/src/store/uiNamespace.ts`
3. `temp_frontend/src/store/modules/data.ts`
4. `temp_frontend/src/store/modules/mihomoData.ts`
5. 对应 `temp_frontend/src/views/Mihomo*.vue`

先确认它是不是共用页面，再决定改动范围。

## 10.7 改“版本号 / 首页显示版本 / 前端版本同步”

先看：

1. `config/version`
2. `config/config.go`
3. `service/server.go`
4. `temp_frontend/src/components/Main.vue`
5. `scripts/sync-version.mjs`
6. `temp_frontend/package.json`

## 10.8 改“证书 / HTTPS / Panel/Sub TLS”

先看：

1. `web/web.go`
2. `sub/sub.go`
3. `app/app.go`
4. `service/panel_sqlite_cert_store.go`
5. `service/panel_self_signed_cert_paths.go`
6. `service/acme_service.go`
7. `service/certificate_inventory.go`
8. `temp_frontend/src/components/SettingsAcmeManage.vue`

## 10.9 改“防火墙 / 端口转发 / 反向代理”

先看：

1. `service/firewall.go`
2. `service/port_forward.go`
3. `service/reverse_proxy.go`
4. `cronjob/firewallSyncJob.go`
5. `cronjob/portForwardSyncJob.go`
6. `cronjob/reverseProxySyncJob.go`
7. `temp_frontend/src/views/Settings.vue`
8. `temp_frontend/src/components/SettingsFirewallManage.vue`
9. `temp_frontend/src/components/SettingsPortForwardManage.vue`
10. `temp_frontend/src/components/SettingsReverseProxyManage.vue`

## 10.11 改“二维码 / 订阅导入链接 / 为什么 Mihomo 也会显示订阅二维码”

先看：

1. `temp_frontend/src/layouts/modals/QrCode.vue`
2. `temp_frontend/src/store/uiNamespace.ts`
3. `service/setting.go`
4. `sub/subHandler.go`
5. `sub/jsonService.go`
6. `sub/clashService.go`

关键事实：

- 二维码弹窗不是写死链接，而是根据 namespace 动态拼接
- Mihomo 下会走 `q/mihomo?name=...`
- 默认链下会走 `q/client?name=...`
- `supportsSubscriptionQr = true` 时，Mihomo 也会显示订阅二维码

## 10.12 改“Token API 也要支持某能力 / 接口能力不一致”

先看：

1. `api/apiV2Handler.go`
2. `api/apiHandler.go`
3. `api/apiService.go`

重点留意：

- Session API 与 Token API 不是一模一样的镜像
- 很多系统级功能只在 Session API 中开放
- 需求如果是“也要开放给外部脚本 / Token 调用”，通常要补 `apiV2Handler.go`

---

## 11. 联动检查矩阵：改一处时别忘了这些地方

| 改动点 | 必须联查 |
| --- | --- |
| `webPath` / `webPort` / `webDomain` | `service/setting.go`、`web/web.go`、`temp_frontend/index.html`、`router/index.ts`、`vite.config.mts` |
| 设置页保存流程 | `Settings.vue`、`api/apiService.go`、`service/config.go`、`service/setting.go` |
| `subPath` / `subPort` / `subDomain` / `subURI` | `service/setting.go`、`sub/sub.go`、`sub/subHandler.go`、`GetFinalSubURI()`、设置页 |
| `/api/save` 的 object 名或字段 | `api/apiHandler.go`、`api/apiService.go`、`service/config.go`、两个 store、相关 views/modals/types |
| 共享页面逻辑 | `uiNamespace.ts`、`data.ts`、`mihomoData.ts`、对应 `Mihomo*.vue` 包装页 |
| 默认链运行配置路径 | `service/core_layout.go`、`service/promanager.go`、`service/coreManager.go`、前端 core 显示路径文本 |
| Mihomo 运行配置路径 | `service/core_layout.go`、`service/mihomo_manager.go`、`service/mihomo_core_manager.go`、`uiNamespace.ts` |
| 版本号 | `config/version`、`config/config.go`、`service/server.go`、`Main.vue`、`sync-version.mjs`、前端包元数据 |
| 前端构建输出或嵌入方式 | `temp_frontend/package.json`、`scripts/sync-version.mjs`、`build.bat`、`build.sh`、`windows/kwor-windows-build.bat`、`web/web.go` |
| 证书分配或 TLS 行为 | `web/web.go`、`sub/sub.go`、`app/app.go`、证书 inventory / ACME / self-signed 相关 service |
| 二维码 / 订阅导入链接 | `QrCode.vue`、`uiNamespace.ts`、`GetFinalSubURI()`、`sub/subHandler.go` |
| Token API 功能覆盖 | `api/apiV2Handler.go`、`api/apiHandler.go`、`api/apiService.go` |
| 设置页子功能 | `Settings.vue` + 对应 `Settings*.vue` 组件 + `api/apiHandler.go` + `api/apiService.go` + 对应 service |

---

## 11.1 专题地图：证书 / ACME / 自签 / Panel/Sub TLS

### 先看哪些文件

- 后端服务：
  - `service/acme_service.go`
  - `service/self_signed_service.go`
  - `service/certificate_inventory.go`
  - `service/panel_sqlite_cert_store.go`
  - `service/panel_self_signed_cert_paths.go`
- Web / Sub 入口：
  - `web/web.go`
  - `sub/sub.go`
  - `app/app.go`
- Cron：
  - `cronjob/acmeAutoRenewJob.go`
  - `cronjob/panelCertificateBalanceSyncJob.go`
  - `cronjob/tlsPathSyncJob.go`
- 前端：
  - `temp_frontend/src/components/SettingsAcmeManage.vue`
  - `temp_frontend/src/views/Settings.vue`
- API：
  - `api/apiHandler.go`
  - `api/apiService.go`

### 这个模块的真实分层

1. **证书签发与升级**：`AcmeService`
2. **统一证书仓库**：`CertificateInventoryService`
3. **自签证书体系**：`SelfSignedService`
4. **Panel / Sub 入口实际取材与运行时应用**：`panel_*` 相关 service + `web/sub/app`
5. **前端设置与查看**：`SettingsAcmeManage.vue`

### 最容易误判的点

- 证书列表不只等于 ACME 证书列表，它是统一 inventory 视图。
- Panel 和 Sub 是两套 target，不是同一份入口配置。
- 旧共享自签路径会被拆分迁移，不要再把 `Promanager_data/cert/fullchain.pem` 当唯一标准结构。
- 保存 settings 本身不会直接覆盖当前证书绑定 ID，前端 payload 会主动删掉这些字段。

### 改这个模块时常见联动

- 改证书列表展示：前端页 + `certificate_inventory.go`
- 改 ACME 签发 / 自动续签：`acme_service.go` + `acmeAutoRenewJob.go`
- 改 Panel/Sub 实际 HTTPS 行为：`web/web.go`、`sub/sub.go`、`app/app.go`、`panel_sqlite_cert_store.go`
- 改自签证书路径策略：`panel_self_signed_cert_paths.go`

## 11.2 专题地图：防火墙（nftables）

### 先看哪些文件

- 后端服务：
  - `service/firewall.go`
  - `service/firewall_scan.go`
  - `service/firewall_nftables_install.go`
  - `service/firewall_geoip.go`
  - `service/firewall_geoip_parser.go`
  - `service/firewall_geoip_nft.go`
- Cron：
  - `cronjob/firewallSyncJob.go`
- API：
  - `api/apiHandler.go`
  - `api/apiService.go`
- 前端：
  - `temp_frontend/src/components/SettingsFirewallManage.vue`
  - `temp_frontend/src/components/SettingsFirewallGeoOptions.ts`
  - `temp_frontend/src/views/Settings.vue`
- 启停生命周期：
  - `service/nft_lifecycle.go`

### 这个模块的真实分层

1. **规则模型与 overview**：`firewall.go`
2. **GeoIP 来源、缓存、渲染**：`firewall_geoip*.go`
3. **安装 nftables 命令与系统探测**：`firewall_nftables_install.go`
4. **扫描外部已存在规则**：`firewall_scan.go`
5. **定时同步运行时表**：`firewallSyncJob.go`

### 最容易误判的点

- 防火墙不是简单“写数据库”，真正生效依赖运行时同步。
- 外部规则扫描结果不等于面板自己的规则集。
- GeoIP 缓存刷新和实际规则启用不是同一件事。
- 这个模块只在 Linux 下真正执行 nft 管理。

### 改这个模块时常见联动

- 改 UI 展示：`SettingsFirewallManage.vue` + `api/firewall-overview`
- 改实际规则渲染：`firewall.go` + `firewall_geoip_nft.go`
- 改安装行为：`firewall_nftables_install.go`
- 改同步时机：`firewallSyncJob.go` + `nft_lifecycle.go`

## 11.3 专题地图：端口转发

### 先看哪些文件

- 后端服务：
  - `service/port_forward.go`
- Cron：
  - `cronjob/portForwardSyncJob.go`
- API：
  - `api/apiHandler.go`
  - `api/apiService.go`
- 前端：
  - `temp_frontend/src/components/SettingsPortForwardManage.vue`
  - `temp_frontend/src/components/SettingsPortForwardManage.shared.ts`
  - `temp_frontend/src/views/Settings.vue`
- 生命周期：
  - `service/nft_lifecycle.go`

### 这个模块的真实分层

1. **规则校验、概览、状态**：`port_forward.go`
2. **定时同步运行时规则**：`portForwardSyncJob.go`
3. **前端重型管理页**：`SettingsPortForwardManage.vue`
4. **共享文案 / 选项 / 逻辑抽取**：`SettingsPortForwardManage.shared.ts`

### 最容易误判的点

- 端口转发不是保存后永久静态生效，它依赖定时 sync 与运行时规则重建。
- 规则冲突不仅看端口，还要看 family / protocol / range。
- Linux 与非 Linux 的可用性判断不同。

## 11.4 专题地图：反向代理

### 先看哪些文件

- 后端服务：
  - `service/reverse_proxy.go`
- Cron：
  - `cronjob/reverseProxySyncJob.go`
  - `cronjob/panelCertificateBalanceSyncJob.go`
- API：
  - `api/apiHandler.go`
  - `api/apiService.go`
- 前端：
  - `temp_frontend/src/components/SettingsReverseProxyManage.vue`
  - `temp_frontend/src/components/SettingsReverseProxyManage.shared.ts`
  - `temp_frontend/src/views/Settings.vue`
- 应用启动 / 停止：
  - `app/app.go`

### 这个模块的真实分层

1. **规则模型、listener 运行时、TLS 证书选择、上游协议行为**：`reverse_proxy.go`
2. **页面逻辑抽取**：`SettingsReverseProxyManage.shared.ts`
3. **运行时启动与停止**：`app/app.go` 中 `StartRuntime()` / `StopRuntime()`
4. **周期性 reconcile**：`reverseProxySyncJob.go`

### 最容易误判的点

- 反向代理不是只在保存后生效，应用启动时也会启动 runtime。
- 证书选择和证书 balance 是这个模块的重要组成部分，不是外部附属逻辑。
- 它既依赖配置保存，也依赖 app 生命周期。

## 11.5 专题地图：流量总览

### 先看哪些文件

- 后端服务：
  - `service/traffic_overview.go`
- API：
  - `api/apiHandler.go`
  - `api/apiService.go`
- 前端：
  - `temp_frontend/src/components/SettingsTrafficManage.vue`
  - `temp_frontend/src/views/Settings.vue`
- Cron / 生命周期：
  - `cronjob/statsJob.go`
  - `service/nft_lifecycle.go`
  - `app/app.go`

### 这个模块的真实分层

1. **总览数据、vnstat 安装删除、限额状态**：`traffic_overview.go`
2. **设置页管理面板**：`SettingsTrafficManage.vue`
3. **定时采样与限额协调**：`statsJob.go`
4. **启动 / 停止时的快照 flush 与 cleanup**：`app/app.go` + `nft_lifecycle.go`

### 最容易误判的点

- 这个模块不只负责展示统计，它还会管：
  - vnstat 安装/移除
  - 月流量限额
  - 快照 flush
- 流量总览与 `enableTraffic`、StatsJob、运行时快照是联动的。

## 11.6 专题地图：系统监控

### 先看哪些文件

- 后端服务：
  - `service/system_monitor.go`
  - `service/system_monitor_store.go`
- API：
  - `api/apiHandler.go`
  - `api/apiService.go`
- 前端：
  - `temp_frontend/src/components/SettingsMonitorManage.vue`
  - `temp_frontend/src/views/Settings.vue`
- 应用启动：
  - `app/app.go`

### 这个模块的真实分层

1. **实时采样与 overview / history 查询**：`system_monitor.go`
2. **独立 monitor.db 存储与 rollup 表维护**：`system_monitor_store.go`
3. **设置页图表与采样/保留设置**：`SettingsMonitorManage.vue`
4. **应用启动时 runtime probe 准备**：`app/app.go`

### 最容易误判的点

- 系统监控不是走主库 `kwor.db`，而是独立 `monitor.db`。
- 这个模块不是只有实时值，还有 rollup 历史桶表。
- “数据库占用”在监控页里说的是监控库体积，不是主库体积。

---

## 12. 前端共享页面、包装页、modal 与协议组件关系图

1. **不要直接改 `web/html/*`**
   - 这是嵌入产物，不是前端源码。

2. **不要直接改 `temp_frontend/dist/*`**
   - 这是 Vite 产物，不是前端源码。

3. **不要把 `Promanager_data/*` 当手写业务源码**
   - 大量文件会在保存配置或启动时被重写。

4. **不要把 `frontend/` 当当前前端主目录**
   - 当前本地开发实际使用的是 `temp_frontend/`。

5. **不要把 `service/config.go` 里的 dummy core 当实际运行 core**
   - 真正的运行控制在：
     - `service/coreManager.go`
     - `service/mihomo_core_manager.go`

6. **不要把 `Mihomo*.vue` 全部当独立实现**
   - 很多只是给共用页传 `namespace="mihomo"` 的包装层。

7. **不要被 `SingboxCore.vue` 文件名误导**
   - 它现在同时服务默认链和 Mihomo 链。

8. **不要把 Vite 代理端口 `2095` 当成程序默认面板端口**
   - 默认面板端口来自 `service/setting.go`，是 `8888`。

9. **不要再把订阅默认路径写死成 `/sub/`**
   - 当前 `subPath` 会在首次需要时自动生成随机路径，除非用户手工设置。

10. **不要再把 Mihomo 配置路径写成旧式 `Promanager_data/core/server.yaml`**
    - 当前权威路径是 `Promanager_data/core/mihomo/server.yaml`。

11. **不要以为只改前端表单就算完成**
    - 这类项目经常还要补：
      - store
      - `/api/save`
      - service 保存逻辑
      - 运行配置生成
      - 实际落盘路径

12. **不要把设置页当“小配置页”**
    - 它实际上还挂着流量、防火墙、反代、监控、内核管理等系统级功能。

13. **Windows 本地前端命令优先写 `npm.cmd`**
    - 不要默认给出 PowerShell 专属写法。

14. **默认禁止跑 `go test ./service` 这类长测试**
    - 只做与当前修改直接相关的短验证。

15. **不要默认认为 Session API 和 Token API 完全等价**
    - `api/apiV2Handler.go` 当前只覆盖了部分能力。

16. **不要把 `monitor.db` 和 `kwor.db` 混为一谈**
    - 系统监控使用独立数据库。

17. **不要忽略 `sub_json` 文件名冲突保护**
    - client subscription / subgroup / suboutbound 三类名字会相互校验。

18. **不要只盯 Inbounds 页找 core 控制入口**
    - 首页 `Main.vue` 也能直接启动 / 停止 / 重启 sing-box 与 Mihomo。

19. **不要把 release 包文件名、手工重命名文件名、运行进程名混为一谈**
    - release 产物名可能是 `kwor-linux-amd64.tar.gz`
    - 手工部署文档里可能会把二进制改名成 `kwor_amd64`
    - 但程序内部 `kworServiceName`、`systemd` 服务名、主进程语义仍然以 `kwor` 为核心
    - 分析安装 / 升级问题时，一定要区分“压缩包名”“磁盘文件名”“进程名”“service 名”

---

## 12. 前端共享页面、包装页、modal 与协议组件关系图

## 12.1 页面分层：哪些是主页面，哪些是一层包装

### 主页面（真正承载业务逻辑）

- `views/Home.vue`
- `views/SubManager.vue`
- `views/Inbounds.vue`
- `views/Clients.vue`
- `views/Outbounds.vue`
- `views/Rules.vue`
- `views/Tls.vue`
- `views/Dns.vue`
- `views/Basics.vue`
- `views/Admins.vue`
- `views/Settings.vue`
- `views/MihomoDns.vue`
- `views/Login.vue`

### Mihomo 包装页（一层 namespace 包装）

- `views/MihomoInbounds.vue`
- `views/MihomoClients.vue`
- `views/MihomoOutbounds.vue`
- `views/MihomoRules.vue`
- `views/MihomoTls.vue`

这些文件本身几乎不承载业务，只是把 `namespace="mihomo"` 传给共享主页面。

### 隐藏但仍存在的页面

- `views/Services.vue`
- `views/Endpoints.vue`

当前路由和数据链仍在，但 UI 菜单里临时隐藏。

## 12.2 哪些 modal 是真正编辑器

下面这些 modal 不是“小弹窗”，而是实际业务编辑器：

- `layouts/modals/Inbound.vue`
- `layouts/modals/Outbound.vue`
- `layouts/modals/Client.vue`
- `layouts/modals/ClientBulk.vue`
- `layouts/modals/Tls.vue`
- `layouts/modals/Rule.vue`
- `layouts/modals/Ruleset.vue`
- `layouts/modals/Dns.vue`
- `layouts/modals/DnsRule.vue`
- `layouts/modals/SubOutbound.vue`
- `layouts/modals/SubGroup.vue`
- `layouts/modals/OutboundGroup.vue`
- `layouts/modals/Admin.vue`
- `layouts/modals/Service.vue`
- `layouts/modals/Endpoint.vue`
- `layouts/modals/Token.vue`
- `layouts/modals/SingboxCore.vue`

### 更偏查看 / 辅助 / 操作型的 modal

- `layouts/modals/QrCode.vue`
- `layouts/modals/SubManagerQrCode.vue`
- `layouts/modals/SubGroupQrCode.vue`
- `layouts/modals/Stats.vue`
- `layouts/modals/Logs.vue`
- `layouts/modals/Changes.vue`
- `layouts/modals/Backup.vue`
- `layouts/modals/PortLogs.vue`
- `layouts/modals/WgQrCode.vue`

## 12.3 哪些 protocol 组件只是局部协议片段

`temp_frontend/src/components/protocols/*.vue` 基本都不是独立页面，而是给 `Inbound.vue` / `Outbound.vue` / 相关编辑器嵌入的协议片段，例如：

- `Shadowsocks.vue`
- `Hysteria.vue`
- `Hysteria2.vue`
- `Tuic.vue`
- `Vless.vue`
- `Vmess.vue`
- `Trojan.vue`
- `Snell.vue`
- `Mieru.vue`
- `TrustTunnel.vue`
- `ShadowTls.vue`
- `OutShadowTls.vue`
- `Ssh.vue`
- `SshInbound.vue`
- `AnyTls.vue`
- `Tun.vue`
- `TProxy.vue`
- `Naive.vue`
- `OutNaive.vue`
- `Selector.vue`
- `UrlTest.vue`
- `Tor.vue`
- `Wireguard.vue`
- `Tailscale.vue`
- `Warp.vue`
- `Direct.vue`
- `Socks.vue`
- `Http.vue`
- `Sudoku.vue`

### 这意味着什么

- 协议字段改动，往往不能只改某个 protocol 组件
- 还要联查：
  - `layouts/modals/Inbound.vue` 或 `Outbound.vue`
  - 对应 `types/*.ts`
  - store save
  - 后端模型 / 生成器

## 12.4 设置页里的“系统功能大页”很多都还有 shared 逻辑层

典型例子：

- `SettingsReverseProxyManage.vue` + `SettingsReverseProxyManage.shared.ts`
- `SettingsPortForwardManage.vue` + `SettingsPortForwardManage.shared.ts`
- `SettingsFirewallManage.vue` + `SettingsFirewallGeoOptions.ts`

这类页面通常是：

- `.vue` 负责展示
- `.shared.ts` 或配套 `.ts` 负责常量、文案、表头、过滤项、组合逻辑

所以改系统功能页时，不要只搜 `.vue`。

---

## 13. 典型需求案例：按问题倒推文件

## 13.1 改一个“默认值”应该怎么查

### 例：面板端口、订阅端口、首次默认值不对

排查顺序：

1. `service/setting.go`
   - 看权威默认值
2. `cmd/cmd.go`
   - 看首次启动交互默认值是否另有一套
3. `temp_frontend/src/views/Settings.vue`
   - 看前端本地初始值与保存前归一化逻辑
4. `README.md` / 文档
   - 看是否需要同步说明

### 原则

- 后端默认值优先级高于前端占位值
- CLI 首次启动默认值不一定等于 Settings 页初始值
- 文档不要反过来定义事实

## 13.2 改一个 API 字段应该怎么查

### 例：前端要多一个字段 / 字段名要改 / Token API 也要支持

排查顺序：

1. `api/apiHandler.go`
   - 找具体 action
2. `api/apiService.go`
   - 找最终返回对象组装位置
3. `temp_frontend/src/store/modules/data.ts` / `mihomoData.ts`
   - 看前端是否接这个字段
4. 对应 `views/*.vue` / `layouts/modals/*.vue`
   - 看 UI 怎么消费这个字段
5. 如需外部 token 调用，再补：`api/apiV2Handler.go`

### 原则

- 改返回字段时，不要只改后端响应，还要看 store 是否会覆盖 / 忽略它
- Session API 改了，不代表 Token API 自动拥有同能力

## 13.3 改 Mihomo 也支持某功能应该怎么查

### 例：默认链已有能力，Mihomo 页面也要有

排查顺序：

1. 看这个能力是不是共享页面天然支持：
   - `views/Inbounds.vue` / `Clients.vue` / `Outbounds.vue` / `Rules.vue` / `Tls.vue`
2. 看 `uiNamespace.ts` 是否已经定义 Mihomo 对应 endpoint / 开关
3. 看 `mihomoData.ts` 是否已有对应保存对象映射
4. 看后端：
   - `api/apiService.go`
   - `service/config.go`
   - `service/mihomo_*.go`
5. 最后确认生成器是否真的支持进 `server.yaml`

### 原则

- 前端能显示 ≠ Mihomo 生成器能正确落盘
- UI 共享 ≠ 后端能力共享
- Mihomo 改动通常至少要查：页面 + namespace + store + 保存链 + YAML 生成链

## 13.4 改订阅二维码 / 订阅导入链接应该怎么查

### 例：二维码地址不对、Mihomo 链接不对、scan only 链接不对

排查顺序：

1. `temp_frontend/src/layouts/modals/QrCode.vue`
   - 看二维码 URL 如何拼接
2. `temp_frontend/src/store/uiNamespace.ts`
   - 看 namespace 是否支持订阅二维码、前缀是什么
3. `service/setting.go`
   - 看 `GetFinalSubURI()` 最终怎么拼 host / port / path / https
4. `sub/subHandler.go`
   - 看后端有没有对应的路由入口
5. `sub/jsonService.go` / `sub/clashService.go`
   - 看 format 输出是否匹配预期

### 原则

- 不要只改前端字符串
- 要确认后端 route 存在，最终订阅 URL 拼接也正确
- Mihomo 与默认链二维码是不同路径前缀

## 13.5 改一个系统功能页应该怎么查

### 例：防火墙 / 反代 / 转发 / 监控页显示或保存不对

排查顺序：

1. `Settings.vue`
   - 看页面是否真的挂在设置页中
2. 对应 `Settings*.vue`
   - 看展示与交互
3. 对应 `shared.ts` / 选项 `.ts`
   - 看隐藏的逻辑层
4. `api/apiHandler.go`
5. `api/apiService.go`
6. 对应 `service/*.go`
7. 如依赖运行时同步，再看 `cronjob/*SyncJob.go`

### 原则

- 系统功能页通常不是“单页 + 单接口”结构
- 往往是：页面 + shared 逻辑 + API + service + cron + 运行时状态

---

## 14. 常见误判与高风险坑位（AI 必看）

1. **不要直接改 `web/html/*`**
   - 这是嵌入产物，不是前端源码。

2. **不要直接改 `temp_frontend/dist/*`**
   - 这是 Vite 产物，不是前端源码。

3. **不要把 `Promanager_data/*` 当手写业务源码**
   - 大量文件会在保存配置或启动时被重写。

4. **不要把 `frontend/` 当当前前端主目录**
   - 当前本地开发实际使用的是 `temp_frontend/`。

5. **不要把 `service/config.go` 里的 dummy core 当实际运行 core**
   - 真正的运行控制在：
     - `service/coreManager.go`
     - `service/mihomo_core_manager.go`

6. **不要把 `Mihomo*.vue` 全部当独立实现**
   - 很多只是给共用页传 `namespace="mihomo"` 的包装层。

7. **不要被 `SingboxCore.vue` 文件名误导**
   - 它现在同时服务默认链和 Mihomo 链。

8. **不要把 Vite 代理端口 `2095` 当成程序默认面板端口**
   - 默认面板端口来自 `service/setting.go`，是 `8888`。

9. **不要再把订阅默认路径写死成 `/sub/`**
   - 当前 `subPath` 会在首次需要时自动生成随机路径，除非用户手工设置。

10. **不要再把 Mihomo 配置路径写成旧式 `Promanager_data/core/server.yaml`**
    - 当前权威路径是 `Promanager_data/core/mihomo/server.yaml`。

11. **不要以为只改前端表单就算完成**
    - 这类项目经常还要补：
      - store
      - `/api/save`
      - service 保存逻辑
      - 运行配置生成
      - 实际落盘路径

12. **不要把设置页当“小配置页”**
    - 它实际上还挂着流量、防火墙、反代、监控、内核管理等系统级功能。

13. **Windows 本地前端命令优先写 `npm.cmd`**
    - 不要默认给出 PowerShell 专属写法。

14. **默认禁止跑 `go test ./service` 这类长测试**
    - 只做与当前修改直接相关的短验证。

15. **不要默认认为 Session API 和 Token API 完全等价**
    - `api/apiV2Handler.go` 当前只覆盖了部分能力。

16. **不要把 `monitor.db` 和 `kwor.db` 混为一谈**
    - 系统监控使用独立数据库。

17. **不要忽略 `sub_json` 文件名冲突保护**
    - client subscription / subgroup / suboutbound 三类名字会相互校验。

18. **不要只盯 Inbounds 页找 core 控制入口**
    - 首页 `Main.vue` 也能直接启动 / 停止 / 重启 sing-box 与 Mihomo。

---

## 15. 文档维护规则

出现以下变化时，必须同步更新本文档：

- 路由变化
- `uiNamespace.ts` 变化
- `data.ts` / `mihomoData.ts` 对象映射变化
- `api/save` 分发对象变化
- `api/apiV2Handler.go` 的 Token API 覆盖面变化
- `service/core_layout.go` 路径规则变化
- `promanager.go` 输出结构变化
- `mihomo_manager.go` 输出结构变化
- `build.bat` / `build.sh` / Windows 构建脚本变化
- 订阅路由变化
- `Settings.vue` 保存前整理逻辑变化
- 二维码 / 订阅链接生成逻辑变化
- 设置页新增或删除大功能页签
- Core manager 接口 / 路径 / 版本同步规则变化
- 系统监控库路径或结构变化

## 15.1 提交、Tag、Release 规则

以后处理本项目提交 / 发版时，默认按下面规则执行，不要再临时猜测：

1. `main` 分支始终代表当前最新代码状态。
2. 只有当 `config/version` 发生变化时，才创建**新的版本 Tag** 与 **新的 GitHub Release**。
3. 新版本 Tag 统一使用：`v<version>`，例如：
   - `config/version = 1.5.10`
   - 对应 Tag = `v1.5.10`
4. 如果只是文档修正、说明补充、非版本号变更提交：
   - 正常提交到 `main`
   - **不要新建新的版本 Tag**
   - **不要新建新的 GitHub Release**
5. 如果版本号已经变更并且准备发版：
   - 先确认 `config/version`
   - 再同步前端版本元数据
   - 再创建新的 `v<version>` Tag
   - 再推送 Tag，让 GitHub Release / Actions 跟随新 Tag 产出
6. `bash <(curl -Ls https://raw.githubusercontent.com/nicelic/kwor/main/install.sh)` 默认安装逻辑：
   - 取的是 **GitHub Release latest**
   - 因此用户执行这条命令时，默认拿到的应始终是**最新已发布版本**
   - 这也是为什么“版本号变化 -> 新 Tag -> 新 Release”这条链必须保持严格一致
7. 如果版本号**没有变化**，但又希望当前版本标签也指向最新修正提交：
   - 可以把**同一个已有 Tag**重新移动到最新提交
   - 但这属于“修正已有版本标签指向”，不是“创建新版本”
   - 执行前要明确用户是否接受改写现有 Tag

## 15.2 下次提交时的默认处理原则

如果用户只说“提交并推送”，默认按下面判断：

1. 先看 `config/version` 有没有变化。
2. 如果版本号没变：
   - 只提交并推送 `main`
   - 不创建新 Tag
   - 不创建新 Release
3. 如果版本号变了：
   - 提交并推送 `main`
   - 创建新的 `v<version>` Tag
   - 推送新 Tag
   - 让 GitHub Release / Actions 生成新的版本产物
4. 如果用户明确要求“把当前已有版本 Tag 挪到最新提交”：
   - 这是**改写已有 Tag**
   - 不是“新增版本”
   - 要明确知道这会改变该版本在 GitHub 上对应的提交

---

最后更新：`2026-06-19`
