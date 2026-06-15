# S-UI 使用 nftables 进行流量统计：两种方案区别与其他可选方案

> 目标：在不依赖 sing-box 内核统计（StatsTracker）的前提下，改用 Linux nftables 对 **入站端口**做上下行流量统计，并对接现有 UI（入站/端点统计图、用户管理里的 up/down 展示等）。

---

## 1. 你截图里的“统计图”到底在用什么

现状：
- 前端 `Stats.vue` 调用：`GET /app/api/stats?resource=...&tag=...&limit=...`
- 后端：`service/stats.go -> StatsService.GetStats()` 从 `Stats` 表读取（resource/tag/direction/traffic/dateTime）
- `Stats` 表数据由定时任务每 10 秒写入（`StatsService.SaveStats`），来源是 sing-box 内核的 `core/StatsTracker`

因此：如果完全不跑内核统计，`Stats` 表就必须由 **nftables 采集器**来写入，UI 才会继续有数据。

---

## 2. 方案 A vs 方案 B：核心区别

### 方案 A：仅按“入站端口”统计（推荐你当前选择）

**定义**
- 每个入站（Inbound）有一个 `listen_port`。
- 用 nftables 分别统计：
  - 入方向（客户端 -> 服务器监听端口）字节数：记为 up（上传）
  - 出方向（服务器监听端口 -> 客户端）字节数：记为 down（下载）
- 把采集到的增量写入 `Stats` 表：`resource="inbound" tag=<inbound.tag> direction=(up/down)`。

**优点**
- 和你的需求一致：只关心“端口级别”的上下行。
- 入站管理页面（resource=inbound）的统计图可以做到准确、连续。
- 删除入站时可以同步删除该端口的 nftables 规则，避免“已不用的端口仍在统计/残留规则”。

**缺点 / 必须接受的点**
- 无法严格区分“同一入站端口下的不同用户（client）”流量。
- 用户管理（Clients.vue）里原本 `resource="user"` 的统计图将无法保持原语义（因为 nftables 并不知道应用层的 user/tag）。
- 如果一个入站端口被多个 client 共享，那么“按端口汇总”必然会把别人的流量也统计进来（除非你做 1 端口 1 用户/组的设计）。

**如何满足你提出的“选择性的汇总”**
- 用户管理里一个 client 选择了多个入站标签（inbounds），你可以把该 client 的 up/down 展示定义为：
  - `sum(所选入站端口的 up/down)`
- 当用户取消某个入站标签时，就不再纳入该 client 的汇总。
- 注意：如果同一个入站标签同时属于多个 client，则会出现“同一份端口流量被多个 client 同时显示”的现象（这是端口粒度的天然限制）。

---

### 方案 B：按用户统计：每个用户/客户端拥有独立端口（1 client = 1 inbound/port）

**定义**
- 每个 client 都对应一个独立入站（Inbound），拥有独立 `listen_port`。
- nftables 按端口统计就等价于按用户统计。
- UI 的 `client.up/down` 和 `resource="user"` 的统计图都可以“严格正确”。

**优点**
- 可以完全替代目前内核的 `resource=user` 统计语义。
- 不存在“别人的流量混在一起”的问题。

**缺点**
- 维护成本高：client 数量 = 入站数量 = 监听端口数量。
- 面板逻辑会更复杂：
  - 新增/删除 client 会引发新增/删除 inbound、分配端口、生成订阅、同步规则
  - 大量端口监听对系统资源与运维有压力
- 对你目前“client 选择多个 inbound 标签”的模型会产生冲突（因为你已经把 inbound 当作可复用的标签/分组）。

---

## 3. 还有哪些其他方案（按可落地程度排序）

### 方案 C：保持“端口统计”，但把 client 的展示改为“端口汇总”（你目前的方向）

- 入站统计图（resource=inbound）准确。
- Clients 列表/弹窗的 up/down 改为：
  - 仅显示“端口汇总”或显示 “-”
  - 或显示 `sum(该 client 选择的入站端口)`（会有共享端口的重复计数问题）
- `Clients.vue` 的 Stats 图（resource=user）建议隐藏/禁用，或改为 resource=inbound + 多标签汇总（前端改造更大）。

**这是最符合“改动小、能上线”的方案。**

---

### 方案 D：nftables + eBPF/TC 做更细粒度归因（可做到“共享端口仍按用户区分”，但复杂）

思路：
- 仅靠 nftables 很难把流量归因到“应用层 user”。
- 如果能在内核层拿到更细粒度的关联（例如：按 socket/cgroup/fd/mark），就可能做到：
  - 同端口不同连接按规则归类
- 典型做法：
  - eBPF kprobe/tc hook 统计 socket bytes，并按 key（pid/uid/cgroup id/mark/tuple）聚合
  - Go 程序读取 BPF map
- 代价：
  - 开发量大，内核版本/发行版适配成本高
  - 仍然需要一个“把 user 与 key 对应起来”的机制（例如 sing-box 在每个 user 的连接上设置 mark），这又回到了“要改内核/核心转发层”。

---

### 方案 E：继续用 sing-box 统计，但只把“统计模块”替换/隔离（折中）

如果你所谓“屏蔽内核”是指不想暴露/依赖 sing-box 内部 API 或某些行为，那么可以考虑：
- 仍让 sing-box 运行与监听端口（必须，否则服务不存在）
- 统计仍用 StatsTracker，但把落库/对外接口做隔离（例如独立 service、可开关）
- 这样可以保持 `resource=user` 语义完全不变

这不是 nftables 方案，但对现有 UI 兼容性最好。

---

## 4. nftables 端口统计的实现建议（面向你这个项目）

### 4.1 规则组织（建议）
创建独立 table/chain，避免污染用户规则：

- table：`inet s_ui`
- chain：`input`（hook input priority filter）
- chain：`output`（hook output priority filter）

对每个入站端口创建两条规则（TCP + UDP，按你的协议需求可选）：
- input：`tcp dport <port> counter comment "s-ui inbound <tag> tcp in"`
- output：`tcp sport <port> counter comment "s-ui inbound <tag> tcp out"`
- UDP 同理

统计方向映射（与现有 UI 保持一致的一个常用约定）：
- input 方向（客户端 -> 服务端端口）= up
- output 方向（服务端端口 -> 客户端）= down

### 4.2 规则生命周期（你强调的重点：针对性创建/删除）
挂载点建议放在 `service/inbounds.go -> InboundService.Save()`：
- act=new：入站保存成功后（已确定 tag/port），创建 nftables 规则
- act=edit：如果 `listen_port` 变化：
  - 删除旧端口规则
  - 创建新端口规则
- act=del：删除入站前/后都可以（建议删除入站后再删规则也行），删除该 tag 对应的规则

**确保：入站删除后，不再保留端口统计规则。**

> 注意：nftables 只负责统计，不负责“监听”。端口监听由 sing-box 入站决定。
> 你要保证“没用的端口不监听”，仍需要 `corePtr.RemoveInbound(tag)` 生效（当前代码已做）。

### 4.3 采集与对接 UI
你项目 UI 统计图读的是 `Stats` 表，所以 nftables 采集器应当每 10 秒：
1. 读取规则 counter 的 bytes（最好是 JSON 输出）
2. 计算与上次相比的增量（delta）
3. 写入 `Stats` 表（dateTime=now, resource="inbound", tag=<inbound.tag>, direction, traffic=deltaBytes）

这样 `Stats.vue` 完全不用改，入站统计图直接有数据。

---

## 5. 总结：你问“这两个的区别是什么”

- **方案 A（端口统计）**：统计粒度是“入站端口”，适合入站管理统计图；用户管理只能做端口汇总或留空，无法严格区分同端口多用户。
- **方案 B（用户=独立端口）**：统计粒度等价于“用户”，能完全替换现有 client.up/down 和 resource=user 图表，但需要大改模型（端口、入站数量暴涨）。

其他方案中，最现实可落地的是：
- **方案 C：端口统计 + client 仅显示端口汇总/留空**（你当前选的那条）。
