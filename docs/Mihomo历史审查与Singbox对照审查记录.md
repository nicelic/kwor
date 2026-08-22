# Mihomo 历史审查与 Sing-box 对照审查记录

整理日期：2026 年 8 月 18 日

## 1. 这份记录的目的

这不是一次重新扫描源码后倒推出来的变更说明，而是依据三份历史审查过程整理的工作记录。它回答四个问题：

1. 用户最初真正担心的是什么；
2. 每一轮审查实际追到了哪些页面和数据链；
3. 被确认的问题分别怎样修到前端、接口、数据库、回显、订阅和运行配置；
4. 这些经验怎样作为默认 sing-box 页面后续审查的检查标准。

历史材料对应的原始诉求依次为：

| 轮次 | 原始诉求 | 核心担忧 |
| --- | --- | --- |
| 第一轮 | “细细审查 mihomo_ 的页面的所有 UI 控件到数据库处理，有类似问题都修复。” | 不接受只看页面能否点击；每个控件必须能正确落库、回显并进入实际配置。 |
| 第二轮 | “mihomo_ 经过了大改，我担心有疏漏、错误，你细致审查然后修复。” | 大改后共享弹窗、旧数据、导入数据和订阅旁路可能没有随主入口一起更新。 |
| 第三轮 | “对 mihomo 进行第二次详细的审查与修复。” | 在已有修复之上继续找并发、局部保存、严格输入校验、运行配置失败恢复和命名空间错用。 |

因此，本记录中的“完成”不表示只完成了静态审阅；它表示历史过程已把某项问题修进明确的数据链，并在当时执行了相应的定向测试、前端类型检查或生产构建。没有历史记录支撑的结论不会补写成既成事实。

## 2. 原始问题被怎样界定

用户并非只要求修一个协议控件，也不是只要求 Mihomo YAML 能生成。真正的验收链是：

```text
UI 控件显示与默认值
  -> 条件开关、隐藏/清理与表单草稿
  -> 请求格式、对象名、命名空间和 API 错误语义
  -> 服务层合并、最终校验与 SQLite 事务
  -> 保存后重新读取、编辑回显与并发状态
  -> server.yaml / Core 运行配置
  -> 普通订阅、JSON 订阅、Clash 订阅与历史数据读取旁路
```

任何一层断开，都会形成用户最担心的“控件能填、保存看似成功、实际不生效”或“关闭了又复活”的问题。三轮 Mihomo 审查都以这个链路为单位，而不是以单个 `.vue` 文件或单个 Go service 为单位。

## 3. 第一轮：先建立页面到运行配置的完整链路

### 3.1 审查范围

第一轮先确认 Mihomo 不是一张独立页面，而是六个侧栏页面及其共享组件共同组成的数据域：入站、用户、出站、TLS、路由、DNS，另含 Core 管理与订阅输出的关联路径。审查特意覆盖了：

- 首次加载、编辑、删除、关闭条件控件、保存失败和重新打开时的回显；
- 默认链与 Mihomo 共用弹窗在 `namespace="mihomo"` 时是否真正走 Mihomo Store、Mihomo API 和 Mihomo 数据表；
- `options`、`out_json`、TLS 表、RawOutbound、客户端模板和最终 YAML 之间的字段归属；
- 历史 SQLite 记录、导入记录、订阅读取等不经过当前表单保存的旁路。

### 3.2 首批确认的问题与修复

| 问题 | 为什么会发生 | 历史修复结果 |
| --- | --- | --- |
| 路由页提交整份 `mihomo_config` | 路由页只拥有 route/sniffer，却以整份配置保存；另一窗口刚保存 DNS 或其他顶层字段时可能被旧草稿覆盖。 | 改为局部路由保存：提交 route 与用户明确改动的 sniffer，服务端在事务内合并最新配置，而不是用旧页面对象覆盖整份 Mihomo 配置。 |
| 数据库提交后运行配置生成失败，页面把结果当普通失败 | SQLite 已成功，但 YAML/运行配置未写入；前端没有回读，会把新增记录留在弹窗或误判为未保存。 | 补齐已提交状态的回读/恢复语义，后续轮次又统一成可重试的 `retryRuntime` 契约。 |
| TLS 模式切换留下 Reality/ECH 旧字段 | 条件控件消失不等于草稿和数据库中的旧字段被删除；生成器对某些组合不支持。 | 前端切换时同步收敛草稿，后端保存时清理不支持的 Reality/ECH 组合，避免旧数据绕过 UI 再次污染配置。 |
| 客户端入站绑定仅由前端下拉限制 | 并发删除入站或绕过 UI 调 API 时，可能把不存在或不可绑定的入站 ID 写进 `mihomo_clients`。 | 服务端二次验证“记录存在且允许用户绑定”；已有用户的入站不能被切换成不支持绑定的类型。 |
| Mihomo DNS 使用 JSON 绑定，前端仍按全局表单编码发送 | 后端 `ShouldBindJSON` 与实际 `Content-Type` 不匹配，DNS 补丁请求可能被视为无效 JSON。 | DNS 专用请求显式改为 JSON；随后扫描 Mihomo JSON 专用接口，确认路由已正确、DNS 是唯一遗漏项。 |

### 3.3 TLS、TrustTunnel 与运行期能力边界

第一轮继续发现，某些控件的“显示”与运行期“支持”并不一致：

- Reality 模式下的 ECH 会在最终转换中被丢弃；因此仅在页面隐藏并不够，保存和历史数据也必须清理。
- TrustTunnel 的 uTLS 后端已经支持，但组件没有正确传入 Mihomo namespace，相关控件没有显示；修复后前端入口、字段保存和配置转换一致。
- TrustTunnel 的拥塞控制“Default/清空”原本会被重新写为 `bbr`，用户无法真正删除字段。历史修复把“新建/切换类型时给默认值”与“编辑旧记录时尊重用户已清空”分开。
- Hysteria v1 中没有运行期投影的跳频输入被移除，避免界面继续承诺一个不会进入 YAML 的能力。

这部分确立了一条重要准则：**只要运行配置转换明确不支持某个字段组合，就不能只在最终渲染阶段静默丢弃；UI、保存、历史读取和订阅输出都要使用同一边界。**

### 3.4 出站、入站模板与订阅旁路

第一轮曾逐步发现并校正多个“主保存路径已经修了，旁路仍会复活”的问题：

- 出站 Dial 的 `detour` 下拉把面板订阅分组也提供成可选项，但这类分组不会生成 Mihomo `proxy-groups`，因此不是有效的运行期目标。修复后前端候选项只保留真实 Core 目标，服务端转换也拒绝手工绕过的无效目标，并处理直连别名归一化。
- 手工/导入 Mihomo 出站再次编辑时，受控字段合并白名单漏掉真实可编辑字段，造成新建正常、编辑后旧值悄悄保留。修复补齐了协议相关字段的受控合并与回归用例。
- 批量创建用户只规范限速，单用户保存则还会规范流量、到期日、重置周期、统计值和首次重置时间。历史修复将两条写入路径统一。
- 保存层虽然拒绝部分不能作为普通用户 listener 的类型，订阅读取仍可能从历史绑定投影它们。后来抽出共享的运行 listener 白名单，让保存、列表、YAML 和订阅统一判断。
- Hysteria2/TUIC 的 fast-open 控件属于运行时明确无效的能力。最终修复同时覆盖前端隐藏、打开旧记录时清理、入站 `out_json`、手工出站 `Options`、RawOutbound、历史 RawOutbound 读取、Mihomo 订阅与 Clash 输出。

### 3.5 第一轮验证与边界

历史过程记录了定向 Go 回归、前端类型检查、生产构建和 `web/html` 静态资源同步。最终复核重点检索了 fast-open 的三种历史拼写，以及手册中旧的 listener 白名单/fast-open 文字。历史结论是：DNS、路由、TLS、Core 管理、用户绑定和主协议控件的交叉检查未再发现第二个已证实的“UI 可设置但最终静默失效”问题。

这里的表述很重要：它不是“所有未来改动永久无风险”，而是第一轮在当时工程状态和已覆盖的链路内没有再找到可证明的问题。

## 4. 第二轮：收紧默认值、数值和历史数据旁路

### 4.1 为什么需要第二轮

用户随后明确指出 Mihomo 经历过大改，担心已有修复外仍有遗漏。第二轮没有重复“页面能否保存”的表面结论，而是把重点转向最容易被大改破坏的三类行为：

1. 用户主动清空的字段会不会在重新打开或重新挂载时被默认值复活；
2. 数字控件会不会用 `parseInt`、宽松转换或字符串转换把错误输入悄悄变成另一个值；
3. 旧 SQLite、导入原始 JSON、订阅读取和最终渲染会不会绕过新保存校验。

### 4.2 默认值复活与空对象残留

- TrustTunnel 的 `bbr` 只允许在新建或切换协议时初始化。编辑旧记录时，已删除的拥塞控制字段必须保持缺失。
- AnyTLS 等客户端默认值不能在用户明确清空后重新打开页面时悄悄写回。
- `smux: null`、`brutal: null` 不能作为“看起来有字段”的特殊状态存活；修复收紧为非对象即删除，防止空值进入数据库或后续投影。
- 关闭实验字段、TLS 子配置和协议专有块时，不能仅隐藏控件；必须清理草稿、保存体、模型回退和订阅读取中的失活字段。

### 4.3 严格数值语义：拒绝截断，不替用户猜值

第二轮将多个 UI 与后端路径从“可转换就保存”改为“只接受完整、范围正确的原始值”。覆盖的典型字段包括：

| 范畴 | 处理原则 |
| --- | --- |
| 端口、Reality 回落端口 | 仅接受完整的 1–65535 整数；拒绝小数、尾随文本、溢出和截断。 |
| Hysteria2 QUIC receive-window | UI、入站模板、手工出站、listener 和订阅投影使用同一严格整数规则。 |
| gRPC、SMUX/Brutal、Multiplex | 不再把 `1.5`、文本数字或空值静默处理为其它值；非对象和无效数字清理或拒绝。 |
| Routing Mark | 空输入代表删除字段，不再因为 `Number('') === 0` 被重写为 `0`；小数不能进入 RawOutbound。 |
| ShadowTLS 与共享端口输入 | 继续清理残留的截断解析，前端提供范围约束，后端保留最终防线。 |

这一轮特别强调：前端即时校验只是体验层；API 直写、旧库、导入、模型反序列化、订阅和最终渲染都必须保留同一条不可截断规则。

### 4.4 TLS / Fragment / Reality 与订阅输出

第二轮确认 Mihomo 手工出站中有 Fragment 字段不会被生成器使用。这不是“暂时不可用”的普通字段，而是会造成保存成功假象的无效字段，因此在以下层级一起处理：

- 从 Mihomo 出站 UI 移除；
- 保存时从 TLS/RawOutbound 清理；
- TLS 模板客户端 JSON 清理；
- 订阅读取时再次清理，避免旧记录不重新保存也能输出。

Reality 回落端口则由前端和数据层共同限制为严格端口。这样既不会把 `443.5` 悄悄改成 `443`，也不会允许非法旧值进入 `server.yaml`。

### 4.5 第二轮验证与结论

历史记录显示，第二轮对 service、sub、util、database/model 做了定向回归，并执行了前端类型检查和两次生产构建。构建产物同步到 `web/html`，且后来又检查订阅读取这一历史旁路。最终结论是：gRPC、SMUX/Brutal、Hysteria2 receive-window、Routing Mark、Reality 回落端口、Fragment、TLS 模板和订阅输出已使用一致的数据边界。

## 5. 第三轮：局部保存、并发状态与最终服务端校验

### 5.1 保存成功与运行配置失败必须分开表达

第三轮发现 DNS/路由在“SQLite 已提交但运行文件刷新失败”后存在恢复缺口：前端仍用一个布尔结果判断保存，下一次保存又会被服务端认为“没有变化”而跳过重建。修复将结果拆成结构化语义：

- 数据库是否已经提交；
- 运行配置是否已经刷新；
- 当前 revision 与 revision conflict；
- 是否允许只执行 `retryRuntime`，不重复写数据库、审计或重放旧草稿。

页面保存后会保留最新草稿和 revision；只有运行重建成功，才清除待重试状态。这避免把“内容保存成功”伪装成“全部失败”，也避免把“再次点击保存”误当成一份新的配置写入。

### 5.2 路由数值与局部写入

Mihomo 路由的端口/UID 文本控件此前仍可通过 `parseInt` 把 `80x`、`80.5` 变为 `80`。修复将 UI、服务层和渲染器统一为：

- 端口必须是完整 1–65535 整数；
- UID 必须是非负安全整数；
- 渲染器不再将小数端口二次截断；
- route/DNS 专用局部保存使用当前 revision，冲突只返回结构化信息，不自动用旧草稿覆盖新数据。

### 5.3 最终入站校验与 TUN 历史字段

第三轮不再只相信弹窗中的 `validate()`：确认 `Inbound.vue` 曾定义校验但保存路径没有调用，而服务端也缺少对空 tag、非法监听端口的统一最终拒绝。修复后：

- 服务端从原始 JSON 区分整数、浮点、字符串、缺失和 `null`，不依赖会截断的转换；
- 真实要求 TLS 的协议才强制 `tls_id`；
- TUN 可以没有监听端口，但历史 `listen`、`listen_port`、`port` 不能跟随它进入 listener 生成；
- 既有绑定相关错误的优先级得到保留，不能因新协议校验改变原有 API 错误契约；
- Mixed 是“可监听但不可作为用户订阅节点”的特殊情况，订阅过滤与运行白名单必须表达这层区别。

### 5.4 页面级 busy、确认框与命名空间

第三轮还发现很多风险不在数据库字段，而在保存期间的第二次操作：

- `loading` 属性不等价于按钮不可点击，因此保存按钮显式绑定禁用状态；
- ClientBulk、OutboundGroup、TLS 子弹窗、DNS 保存函数、入站/用户/出站列表删除和同步操作增加函数级防重入、页面级 busy 与 persistent 确认框；
- 删除或同步期间不能打开编辑、二维码或另一条删除确认；遮罩和 Esc 不能绕过写锁；
- OutboundGroup 曾在实际保存、回滚删除和确认删除时写死默认 `outboundgroups`。修复为真实 `mihomo_outboundgroups`，并同步 Mihomo Store 的提示映射。

这一层解决的是另一种“表单看起来正确但最终写错数据域”的问题：用户看到的是 Mihomo 页面，实际却可能向默认链对象写入，或在并行操作中用旧 payload 覆盖新状态。

### 5.5 第三轮验证

历史过程记录了：前端类型检查、生产构建、Mihomo/ShadowQUIC 相关 service、sub、util、database/model 定向测试，以及 `temp_frontend/dist/index.html` 与 `web/html/index.html` 的 SHA-256 和资源文件数一致性检查。历史过程没有启动服务或浏览器。

## 6. 三轮审查沉淀出的通用方法

| 层级 | 必问问题 | 已形成的处理方式 |
| --- | --- | --- |
| UI 控件 | 关闭后是否清理？默认值是否复活？数字是否被截断？ | 用显式草稿状态和条件字段清理；完整数值解析；新建默认值与编辑回显分离。 |
| API 契约 | 请求格式与绑定方式匹配吗？对象名/namespace 正确吗？错误可区分吗？ | JSON 接口显式 JSON header；结构化 conflict/committed/retryRuntime 返回；不以布尔值猜测状态。 |
| 服务层 | 绕过 UI 能写进无效数据吗？ | 服务端最终校验、引用校验、类型白名单和原始数字形态检查。 |
| SQLite 事务 | 局部页面会覆盖其它配置吗？ | 局部 mutation、CAS revision、短事务内读最新配置后替换拥有的片段。 |
| 回显 | 保存失败/运行失败后用户能知道真实结果吗？ | 成功后使用服务端返回快照；冲突重载而非重放旧草稿；已提交但运行失败提供纯重试。 |
| 运行与订阅 | 历史记录会绕过保存层吗？ | YAML、普通订阅、JSON、Clash、RawOutbound 和模型回退共用清洗/白名单。 |
| 并发交互 | 保存期间还有第二条入口吗？ | 页面级写锁、函数级 guard、persistent 确认框、输入冻结。 |
| 验证 | 是否只跑了表层构建？ | 针对服务、模型、订阅、API 和前端类型/构建分别做窄回归；需要时检查发布目录一致性。 |

## 7. 对照到默认 sing-box 的审查矩阵

Mihomo 的三轮经验不能原样复制协议字段，但必须复制审查方法。默认 sing-box 按页面划分如下：

| 页面 | 主要持久化边界 | 本次对照重点 |
| --- | --- | --- |
| `/basics` | `config.log`、`config.ntp`、`config.experimental` | 避免整份 `config` 覆盖 DNS/route；拨号、V2Ray stats、Clash detour 引用必须来自真实运行目标。 |
| `/dns` | `config.dns` 与 `dns_servers` 卡片表 | 顶层 DNS 与 DNS card 分开保存；`final` 实际注入的 server、规则引用、detour 和历史嵌入 server 不可脱节。 |
| `/rules` | `config.route` | CAS、规则集/出站/入站引用、运行文件失败重试、页面离开时的草稿边界。 |
| `/inbounds` | `inbounds` 表与客户端关联 | 协议、端口、TLS、绑定、最终 route tag 与客户端初始化的一致性。 |
| `/clients` | `clients` 表及受管订阅/NFT 副作用 | 单个/批量保存语义、入站可绑定性、删除和同步并发。 |
| `/outbounds` | `outbounds`、`endpoints`、分组与引用链 | tag 改名/删除、detour、route/DNS/experimental 引用、导入 RawOutbound 和最终 Core tag。 |
| `/tls` | TLS 数据表、入站/出站引用 | 条件模式切换、历史字段清理、证书删除/替换后的引用保护。 |

## 8. 本次 sing-box 对照审查：已确认并已修复的点

### 8.1 Basics 从全局 Store 直接写入改为独立快照

此前基础信息页的风险与第一轮 Mihomo 路由页同源：页面只拥有 log、NTP、experimental 三段，却可能依赖默认链完整 `config` 草稿并通过通用保存回写。现在基础信息页使用专用接口：

```text
GET  api/singbox-basics-editor-context
POST api/singbox-basics-save
```

专用上下文只返回：

- `singbox_config_states.revision`；
- `log`、`ntp`、`experimental` 草稿；
- 真实运行期 outbound/endpoint dial tag；
- 有效 inbound route tag、client name；
- 实际会注入运行配置的 DNS server tag。

页面不再直接修改 `Data().config`。它维护本地草稿与旧快照，`/basics` 标记为 `skipGlobalDataPolling`，避免 30 秒全局 `api/load` 将未保存草稿混入 Store 或覆盖页面状态。

### 8.2 局部保存、CAS 与运行配置重试

`singbox-basics-save` 的请求只包含：

```json
{
  "expectedRevision": 1,
  "basics": {
    "log": {},
    "ntp": {},
    "experimental": {}
  },
  "retryRuntime": false
}
```

服务端在短事务内比较 revision，只替换 `Setting['config']` 中的 `log`、`ntp`、`experimental`，保留 DNS、route 和其它顶层配置。写入成功后递增 revision 并记录摘要审计；revision 过期则返回 `revision_conflict` 和当前 revision。

若 SQLite 已提交、但 `RegenerateCoreConfig()` 失败，API 明确返回 `committed=true` 与 `retryRuntime=true`。页面保留刚保存的快照，下一次保存只重试运行配置生成，不重复写数据库、增加 revision 或追加审计。冲突时页面重新读取最新快照，不自动把旧草稿重放到别的窗口刚保存的数据上。

### 8.3 控件到服务端校验的补齐

基础页可见控件的后端处理已按以下边界收紧：

| 控件组 | 已落实的后端约束 |
| --- | --- |
| Log | `disabled`、`timestamp` 必须为布尔值；`level` 只能是 UI 提供的 sing-box 日志级别，清空等价于删除；`output` 限制为合法 UTF-8 字符串。 |
| NTP 基础字段 | `enabled` 为布尔值；端口是 1–65535 完整整数；`interval` 是正且不超过 7 天的 duration。 |
| NTP Dial | `detour` 只能引用实际运行的 outbound/endpoint tag；`domain_resolver` 只能引用实际注入的 DNS server；routing mark、bind interface、IPv4/IPv6 bind、reuse address、TFO、MPTCP、UDP fragment、connect timeout 都有类型或 duration 校验。 |
| Clash API | 下载 detour 只能引用真实运行期目标；CORS origin 是受限字符串数组；布尔和文本字段有类型/长度校验。 |
| V2Ray API | stats 的 inbound/outbound/user 分别只允许有效 inbound route tag、真实 outbound/endpoint tag 和真实 client name。 |

其中 NTP 的通用 Dial 控件曾存在一个与历史 Mihomo `parseInt` 问题相近的显示风险：`connect_timeout` 不再用截断读取；页面只把完整秒数转换为 duration，服务端仍保留 duration 的最终校验。因此历史值不会在用户编辑其它字段时被悄悄改成另一秒数。

### 8.4 默认链其它写入口与引用保护

专用 Basics 接口不是唯一可以改写默认配置的入口，所以通用 `config` 保存也纳入 Basics 引用校验。与此同时：

- 删除 outbound 会检查 `experimental.v2ray_api.stats.outbounds`；
- 删除 inbound 会检查 `experimental.v2ray_api.stats.inbounds`；
- 否则配置可能先删掉资源，直到 Core 生成阶段才暴露 V2Ray API 引用悬空。

这与 Mihomo 的 listener 白名单/订阅旁路修复使用同一个原则：**不能只保护当前页面的下拉框，还要保护其它页面、删除路径和手工 API 调用。**

### 8.5 DNS 与路由的既有专用边界一并复核

本次也按相同标准复核了默认链 DNS 与路由的专用路径：

- `/rules` 使用 route 草稿、revision/CAS 和 `retryRuntime`，不提交完整默认 config；
- `/dns` 使用 DNS 快照；顶层 DNS 配置与 DNS card 的新建/编辑/删除是独立 mutation；
- DNS card 只有 `dns.final` 实际选中的一张会注入最终 Core，因此可引用 DNS tag 的候选和后端校验必须以有效运行配置为准；
- `independent_cache` 已不属于 UI 可编辑契约，保存/生成会规范化掉旧字段；
- DNS、route、Basics 三页的冲突处理都选择“重载最新快照，不自动重放旧草稿”。

### 8.6 继续审查：WireGuard、端口、整数与运行配置恢复

在此前已收紧入站 `listen_port`、实体 ID、TLS 引用和编辑身份之后，本轮继续沿着“控件输入 -> 请求 JSON -> 模型反序列化 -> SQLite -> 运行配置”检查默认 sing-box 的数值路径，重点处理了两类容易被忽略的问题：一类是前端读写时对旧值的截断或错误换算，另一类是绕过 UI 直接调用 API 时模型层把小数转换成 `float64` 后再被无声截断。

| 范围 | 确认的问题 | 本轮处理 |
| --- | --- | --- |
| WireGuard peer | `reserved` 曾用 `parseInt` 拆分，`1.5`、空片段或超范围值可能成为错误字节；peer port/keepalive 只依赖 HTML `min`。 | 新增严格整数/三字节解析；保留字节必须恰好三个、每项 `0-255`，peer port 为 `1-65535`，保活为完整正整数。缺失 key 映射时也不再让编辑控件访问不存在的数组项而抛错。 |
| WireGuard/WARP endpoint | listen port、workers、MTU、peer port 允许通过直接请求或历史值携带小数、字符串整数或越界值。 | 前端改为显式整数读写；服务端按原始 JSON 验证并规范化 `listen_port`、`workers`、`mtu`、peer port、保活和 reserved。合法字符串整数写入数据库时变为 JSON 数字，非法小数在入库前拒绝。 |
| TUN inbound MTU | 订阅编辑器已限制为 `576-65535`，默认入站页面和通用整数白名单却仍允许更小值，直接请求可绕过订阅侧规则。 | TUN 控件、原始 JSON 最终校验和回归统一为 `576-65535`；WireGuard/WARP 的独立 MTU 语义仍保留原有范围，不被错误地一刀切。 |
| 通用 outbound 数值 | `server_port`、ShadowTLS handshake port、gRPC/协议嵌套的连接数等进入 `RawOutbound`，模型通用反序列化可能掩盖输入形态。 | 新增递归但字段白名单化的整数规范化：端口严格 `1-65535`，UI 已公开的计数/窗口/带宽/MTU/线程字段（包括 Hysteria server 带宽、URLTest tolerance、Naive 并发、WebSocket early-data、XHTTP post bytes、Sudoku handshake timeout）必须是完整安全整数；未知扩展字段不被擅自改写。 |
| 时长控件 | `parseInt('1m30s')`、`replace('s')` 等会把复合 duration 或非秒单位读成错误数值，用户改其它控件也可能回写错误值。 | 增加 sing-box duration 读写工具；Listen、TUN、gRPC、Direct、URLTest、WireGuard/WARP、Tailscale、OutTLS 等字段在页面显示指定单位时先做完整 duration 换算，再以合法 duration 写回。 |
| 已提交后的恢复 | 专用 Basics/DNS/route 已有 `retryRuntime`，普通 `api/save` 保存入站、出站、endpoint 等则只提示失败并关闭弹窗。 | 新增只重建默认 sing-box 运行配置的统一接口。普通保存收到 `committed=true` 后先回读数据库，再自动尝试一次纯运行配置重建；它不重放原始数据库 mutation，不新增审计，也不会重复创建/编辑记录。 |

服务端新增的数值校验不是为了替代 Core schema，而是为了保证面板自身不会在“已保存”阶段篡改用户原始数值。Core 的更深协议语义仍由最终生成和 Core 启动校验负责；面板在此之前保证小数不会被当作整数、端口不会溢出、字符串整数会得到一致的存储形式、历史合法 duration 不会被 UI 意外降精度。

### 8.7 收尾复核：规则集 duration、重试请求和新建身份

继续对照历史 Mihomo 的“不得截断、不得重放、不得错写身份”标准后，又补上三项容易在共享组件中遗漏的边界：

- 默认 sing-box `Ruleset.vue` 的 remote `update_interval` 改为直接编辑 duration 字符串，不再用“天数数字框”读取。`24h`、`1d`、`1h30m`、`1w` 等值在用户修改 URL、detour 或其它字段时保持原单位和精度；服务端继续校验完整 duration 语法，非法值不会落库。
- Hysteria 跳频秒数输入、Naive 并发、WebSocket `max_early_data`、XHTTP `sc_max_each_post_bytes`、Sudoku `handshake_timeout` 和 ShadowTLS 内嵌 multiplex/Brutal 数值均改为完整整数读写；共享跳频解析拒绝小数、超安全整数和不能整除秒的毫秒值，不再用 `Math.floor`、`parseInt` 或字符串 `.length` 把错误值改成另一项配置。
- `singbox-runtime-retry` 不接受原始保存 payload，只允许空 body 或空 JSON 对象，并使用 4 KiB 请求上限；服务层仅调用运行配置重建，不重放数据库 mutation。新建实体若携带复制来源的旧 `id` 会被服务端丢弃，`edit` 仍要求合法 id，避免导入/复制与误更新之间互相污染。
- Endpoint、Service 与 SubOutbound 弹窗的异步保存/链接转换、WireGuard 密钥生成和 peer 操作增加函数级 busy guard；请求期间弹窗保持打开、输入区和关闭/保存按钮冻结，避免重复写入、关闭后异步结果落到新草稿或 peer 数组越界。

这三项收尾都已覆盖到前端类型检查、服务/API 定向测试和最终构建；它们不是 Core schema 的替代，而是把共享控件、API 契约和 SQLite 身份边界固定下来。

### 8.8 续审：状态收敛、稳定身份与失败响应

2026 年 8 月 18 日的续审没有重新重复此前的全量扫描，而是沿已建立的默认链保存/回显/运行配置路径复核第一次改造后仍可能出现的状态错位。确认并修复了以下边界：

- 全局 `runtimeRetryPending` 不再只依赖“点击重试”清除。DNS、Basics、route 的成功 mutation 只要实际改动并完成运行配置重建，就会收敛旧的失败提示；普通 `api/save` 中确定会重建默认链运行配置的入站、出站、用户、service、endpoint、config 保存成功后也会清除旧标记。
- `/tls` 卡片同时统计入站和 service 的 `tls_id` 引用；两类引用都为空才显示删除入口，服务端仍保留事务内引用校验。保存期间确认框、关闭、复制和编辑入口统一冻结。
- `/dns` 的 DNS card 编辑从数组 index 改为数据库 `serverId`；DNS rule 编辑保留对象身份并在写回时重新定位。这样并发刷新、拖拽或列表重排不会把旧弹窗的内容写到另一张卡片/另一条规则。
- 默认链与 Mihomo 共用的 `OutboundGroup.vue`、默认订阅管理 `SubGroup.vue` 不再把请求失败降格为空数组。已有列表会保留，首次失败显示重试；订阅刷新结果统一规范为安全数组/文本，缺少字段不会让模板访问 `undefined.length`，`committed=true` 则显示已保存但运行配置未刷新的 warning。
- `api/save` 现在区分“数据库已提交但用于组装响应的 `LoadPartialData`/`LoadMihomoPartialData` 失败”和普通保存失败，返回 `committed=true`、`refreshFailed=true`、`retryRuntime=false`。前端只重新读取数据，不会重复提交同一个 mutation。
- transport/multiplex 开关关闭时删除对应字段，不再保留 `{}` 这种会在后续编辑或序列化中复活的空配置。

## 9. 本次验证记录

本次完成后应保持以下验证组合，而不是只看一次构建是否通过：

```text
service：Basics、DNS、route、outbound/inbound 引用、Client 相关定向测试
api：singbox 专用保存、请求体限制、revision conflict 响应
database/model、util、sub：与 sing-box/Mihomo 订阅投影相关的定向测试
frontend：vue-tsc --noEmit 与 npm run build
产物：temp_frontend/dist/index.html 与 web/html/index.html 哈希及资源数一致
```

本轮不启动服务或浏览器；因此运行中的 Core 实际启动、真实网络 listener 和浏览器交互属于部署环境的后续验收，而非本地静态/定向测试能够替代的结论。

本轮实际验证还包括：`go test ./api ./service` 的 sing-box/运行重试/规则集/数值定向集合通过；`npm.cmd exec vue-tsc -- --noEmit` 与 `npm.cmd run build` 通过；构建脚本已同步 `temp_frontend/dist` 到 `web/html`，两处 `index.html` SHA-256 当前均为 `D9ECA479AC9D92EB84D62A120FD17843BB03588BB3EA2B0007862E3DB06BDEF4`，assets 文件数均为 105。完整 `go test ./api ./service` 还会触发若干与本次 sing-box 链路无关的 ACME、nftables、反向代理和环境依赖测试，不能把这些环境失败误写成 sing-box 回归通过或失败。

续审（2026 年 8 月 18 日）实际执行：`go test ./api -run 'TestCommittedSaveFailureResponsePreservesRetryRuntimeContract|TestCommittedPartialLoadFailureResponsePreservesCommittedContract|TestSaveSingbox' -count=1`、`go test ./service -run 'TestSaveSingbox|TestMigrateLegacySingboxDNSServers|TestGenerateFullConfigIncludesOnlyFinalDNSServer|TestSingboxDNSServerDeleteRejectsStaleResolverReferences' -count=1`、`npm.cmd exec vue-tsc -- --noEmit` 与 `npm.cmd run build`，均通过。构建同步后 `temp_frontend/dist/index.html` 与 `web/html/index.html` 的 SHA-256 均为 `FA117EA290DB46FC07A47539925A534C1EDBD9F13E8A4AAD81DB42750134ABBD`，两侧 assets 文件数均为 104。

## 10. 剩余边界与后续维护规则

1. 新增 sing-box Basics 字段时，先判断它是这三个基础段的字段，还是独立数据库实体；只有前者才应进入 `singbox-basics-save`。不能为了方便重新把整份 `config` 交给基础页。
2. 新增一个可引用 tag 的控件，必须同时定义它使用的是“数据库展示 tag”还是“最终运行期 tag”；页面候选项、服务端验证、删除保护和 Core 生成要一致。
3. 新增数值控件时，不允许用 `parseInt`、宽松 `Number()` 或渲染时的隐式转换补救输入。先定义接受的原始类型、整数/小数语义、范围和清空语义，再在前端与服务端同时落实；若字段在 RawOutbound、endpoint 或嵌套 transport 内出现，还要用原始 JSON 校验避免模型 `float64` 路径截断。
4. 新增条件控件时，必须明确关闭时是删除对象、删除字段还是写 `false`，并覆盖重新打开编辑、历史数据读取、订阅输出和运行配置生成。
5. 新增独立页面保存接口时，必须有：局部 ownership、revision/CAS、结构化冲突、已提交但运行失败的重试、页面级写锁和定向测试。
6. 任何“最终生成器会丢掉”的字段都不能继续作为有效 UI 能力保留；需要在显示、保存、旧数据清理和输出旁路四层统一处理。

这套规则正是三轮 Mihomo 审查最有价值的产物：不是一张协议字段清单，而是一种防止页面、SQLite、订阅和运行期配置逐渐失去同步的工作方式。
