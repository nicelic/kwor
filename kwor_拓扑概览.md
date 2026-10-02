# Kwor 原始项目全量全局物理目录树拓扑图谱（总览）

> **项目根目录**: `E:\111111\kwor\kwor`  
> **文档定位**: 本文档为 Kwor 原始项目唯一一体化全局物理目录树拓扑总集。彻底打破分卷隔离，将 1 至 10 号分卷涉及的全量 480+ 个源码文件及构建运维资产全部聚合于单一完整的系统拓扑树中。  
> **涵盖架构**: Go 后端双代理内核（Sing-Box & Mihomo）、Linux Nftables 原子防火墙、ACME 与证书资产仓储、多协议反向代理集群、系统引导与 OS 底层调优、用户客户端治理、API 路由控制层、订阅分发、SQLite 30 个数据模型、流式压缩引擎、Vue 3 前端解耦控件库及跨平台 DevOps 构建管线。

---

## 全量全局物理目录树拓扑

```text
kwor/                                            # 【Kwor 原始项目根目录】(Go 1.26 + Vue 3 混合架构)
├── main.go                                      # 【系统入口总装】(GC微调/内存硬限额24MB/SIGHUP热重载/信号拦截)
├── Dockerfile                                   # 【多阶段生产级容器镜像构建定义】(Node/Go双阶段交叉编译/Alpine最小化)
├── docker-compose.yml                           # 【多架构生产级容器编排定义】(Host网络命名空间/CAP_NET_ADMIN权限绑定)
├── install.sh                                   # 【Linux生产环境全自动安装与自愈升级脚本 (1287行)】(跨发行版/进程防误杀/故障回滚)
├── entrypoint.sh                                # 【Docker容器启动引导与子命令分发脚本】(自动执行DB迁移/环境初始化/接管PID 1)
├── build.bat                                    # 【Windows环境Linux多架构发布包构建脚本 (301行)】(amd64/arm64交叉编译/tar验证)
├── build.sh                                     # 【Linux/macOS极简本地快速编译脚本】(前端构建与Linux amd64交叉编译)
├── kwor.service                                 # 【标准Linux systemd守护进程单元模板】(生命周期前置守护/句柄突破/自动重启)
├── .gortex.yaml                                 # 【Gortex增量编译与代码监听配置】(防抖配置/代码全文检索索引)
│
├── app/                                         # 【应用状态机与热重载装配】
│   └── app.go                                   # 【APP状态机与热重载装配】(数据库恢复/双内核装配/运行时所有权指纹)
│
├── config/                                      # 【全局静态配置与运行时路径计算中心】
│   ├── config.go                                # 【基础路径/日志级别/数据库迁移辅助与路径越界防护】
│   ├── name                                     # 【服务标识元数据】(嵌入常量 "kwor")
│   ├── version                                  # 【面板发行版本号元数据】(嵌入常量 "1.6.33")
│   └── runtime_support_files.go                 # 【运行时附属脚本迁移引擎】(install.sh/kwor.service自动收拢)
│
├── logger/                                      # 【日志持久化与环形缓冲管理】
│   └── logger.go                                # 【定长环形内存日志缓冲与50MB磁盘配额滚动管理】
│
├── cmd/                                         # 【命令行 CLI 运维控制与生命周期网关】
│   ├── cmd.go                                   # 【CLI子命令分发总线与Systemd守护单元动态构建】
│   ├── admin.go                                 # 【管理员账号CLI重置与更新执行器】
│   ├── setting.go                               # 【面板网络参数查看与多源公网IP并发竞速探测器】
│   ├── resetadmin_uninstall.go                  # 【交互式与面板二次确认卸载及登录状态紧急重置】
│   ├── docker_bootstrap.go                      # 【Docker容器首次无交互引导与自签名证书颁发】
│   └── migration/                               # 【SQLite数据库分级升级迁移引擎】
│       ├── main.go                              # 【数据库版本比对与事务级按序迁移流水线】
│       ├── 1_1.go                               # 【v1.1 客户端模型文本转JSON与凭证表结构迁移】
│       ├── 1_2.go                               # 【v1.2 Inbound/Outbound/TLS架构升级与分流重构】
│       └── 1_3.go                               # 【v1.3 DNS卡片化重组与AnyTLS协议支持迁移】
│
├── network/                                     # 【底层网络监听通道、反嗅探与TLS虚拟化引擎】
│   ├── auto_http_conn.go                        # 【HTTP端口协议前瞻与TLS握手特征探测封杀包装流】
│   ├── auto_http_listener.go                    # 【自动反嗅探HTTP网络监听器】
│   ├── auto_https_conn.go                       # 【HTTPS端口明文HTTP嗅探识别与静默断开包装流】
│   ├── auto_https_listener.go                   # 【自动反嗅探HTTPS网络监听器】
│   ├── fake504.go                               # 【伪装Nginx 504 Gateway Time-out延迟熔断响应器】
│   ├── http_tls_config.go                       # 【ALPN (h2/http/1.1) 协议协商TLS配置生成器】
│   ├── managed_tls_conn.go                      # 【托管TLS连接状态跟踪、代数管理与指纹优雅排障】
│   ├── managed_tls_listener.go                  # 【托管TLS监听器包装】
│   └── tls_sni.go                               # 【严格SNI/SAN证书校验器与叶子证书解析器】
│
├── web/                                         # 【Web 门面服务与前端静态资源嵌入】
│   ├── web.go                                   # 【Gin引擎装配/SPA路由/动态证书负载均衡/VSCode调试网关】
│   └── antiprobe.go                             # 【Web层反嗅探假504桥接适配器】
│
├── middleware/                                  # 【HTTP 管道过滤与网络安全中间件】
│   ├── antiprobe.go                             # 【并发受限型 (32槽) 延时3s假504防扫描中间件】
│   ├── domainValidator.go                       # 【域名白名单约束与直连IP放行验证中间件】
│   └── localhost.go                             # 【本地回环地址白名单匹配器 (127.0.0.1/::1)】
│
├── api/                                         # 【RESTful API 控制平面与业务路由分发】
│   ├── apiHandler.go                            # 【API v1 请求路由分发与停机状态写屏障保护】
│   ├── apiService.go                            # 【API核心业务服务总线 (聚合40+底层业务服务调用)】
│   ├── apiV2Handler.go                          # 【API v2 Token认证安全路由分发器】
│   ├── cache_control.go                         # 【防反向代理与浏览器缓存控制 (no-store/no-cache)】
│   ├── core_api_shared.go                       # 【双内核控制层解耦架构设计说明】
│   ├── ddns_api.go                              # 【动态域名DDNS规则与云厂商凭证管理API】
│   ├── dns_api.go                               # 【DNS记录集、收藏域名与解析账户管理API】
│   ├── mihomo_core_api.go                       # 【Mihomo内核启停/下载/版本轮转/配置更新API】
│   ├── mihomo_dns_api.go                        # 【Mihomo DNS配置原子合并与CAS版本冲突控制API】
│   ├── mihomo_route_api.go                      # 【Mihomo路由规则编辑与CAS乐观锁保存API】
│   ├── panel_time_api.go                        # 【面板时区与Linux主机时区原子同步及回滚API】
│   ├── port_check_bind.go                       # 【端口与UDP端口范围占用校验及复杂表单解析器】
│   ├── request_limits.go                        # 【严格API请求体Payload内存配额限制器 (4KB~512MB)】
│   ├── ruleset_probe_api.go                     # 【规则集在线探测与测速API】
│   ├── session.go                               # 【双层Session架构 (25槽内存热环+SQLite防溢出/代数/安全Cookie)】
│   ├── settings_api.go                          # 【面板高级设置打补丁、CAS乐观锁校验与时区事务API】
│   ├── singbox_basics_api.go                    # 【Sing-box基础配置参数与CAS乐观锁保存API】
│   ├── singbox_core_api.go                      # 【Sing-box内核生命周期/下载/自动更新设置API】
│   ├── singbox_dns_api.go                       # 【Sing-box DNS卡片化配置与CAS乐观锁保存API】
│   ├── singbox_route_api.go                     # 【Sing-box路由规则、分流策略与CAS乐观锁保存API】
│   └── utils.go                                 # 【API统一JSON响应封装 (Msg/pureJsonMsg/checkLogin)】
│
├── service/                                     # 【核心业务逻辑与服务治理层 (全量 216 个源文件)】
│   │
│   │── 【Sing-Box 核心服务与出入站路由子群】
│   ├── singbox_basics.go                        # 【Sing-Box基础配置】(NTP/实验特性/CAS版本控制/热重载)
│   ├── singbox_config_json.go                   # 【Sing-Box配置生成】(日志剔除/DNS语法净化/版本兼容熔断)
│   ├── singbox_core_auto_update.go              # 【内核自动更新】(版本比对/GitHub Release精确匹配/状态机重试)
│   ├── singbox_core_download_preferences.go      # 【内核下载偏好】(平台架构/Libc探测/自定义URL/历史兼容迁移)
│   ├── singbox_core_download_task.go            # 【内核下载任务】(异步任务管线/并发去重/取消控制/进度透传)
│   ├── singbox_core_local_detect.go             # 【本地内核探针】(ELF分析/BuildInfo反推/发布通道探测/LRU缓存)
│   ├── singbox_dns.go                           # 【Sing-Box DNS系统】(DNS全局参数/递归规则树/服务器卡片/原子CAS)
│   ├── singbox_inbound_meta.go                  # 【入站元数据】(多用户认证模式/UI绑定策略/协议能力推导)
│   ├── singbox_inbound_references.go            # 【入站引用守护】(路由规则与DNS规则入站引用检测/删除安全栅栏)
│   ├── singbox_outbound_references.go           # 【出站引用守护】(Detour链路/分组引用/自引用死锁拦截/删除防护)
│   ├── singbox_payload_validation.go            # 【载荷强校验】(防止JS精度溢出/WireGuard保留字节/端口边界规整)
│   ├── singbox_port_hop.go                      # 【Hysteria跳频】(端口跳跃范围正则化/时间间隔校验/NAT映射约束)
│   ├── singbox_route.go                         # 【路由编排引擎】(分流规则树/规则集定义/预算控制/入站别名重写)
│   ├── singbox_runtime_outbounds.go             # 【出站运行时清洗】(Mihomo专有字段剔除/证书库模式提取/协议规整)
│   ├── singbox_runtime_tags.go                  # 【运行时标签系统】(虚拟节点展开/ShadowTLS伴生Tag/标签冲突检测)
│   ├── singbox_service_references.go            # 【后台服务引用】(DERP/SSM-API等跨模块出入站引用及DNS关联)
│   ├── singbox_users.go                         # 【运行时用户转换】(面板用户模型至各协议内核User对象的投影规范化)
│   ├── inbounds.go                              # 【入站核心服务】(CRUD/ShadowTLS入站裂变/用户批量绑定/NFT动作)
│   ├── outbounds.go                             # 【出站核心服务】(CRUD/ShadowTLS出站配对/RawOutbound透明传递)
│   ├── outboundgroups.go                        # 【出站分组与订阅】(订阅拉取/并发控制/多节点去重/排序与级联删除)
│   ├── outbound_edit_merge.go                   # 【Schema编辑合并】(双命名空间/树形属性保护/避免导入高级参数丢失)
│   ├── route_inbound_normalize.go               # 【入站别名重定向】(ShadowTLS内部入站别名映射/路由规则Tag纠偏)
│   ├── ruleset_probe.go                         # 【规则集嗅探引擎】(SSRF硬防护/SRS与MRS解压探测/智能语法分类)
│   ├── ruleset_registry.go                      # 【规则集源注册中心】(全球主流GEO/Domain/IP规则集URL模板字典)
│   ├── endpoints.go                             # 【端点核心服务】(WARP注册/WireGuard端点/Tailscale监听/引用守护)
│   │
│   │── 【Mihomo / Clash Meta 核心服务与协议适配子群】
│   ├── mihomo_manager.go                        # 【Mihomo全局服务配置组装与生命周期渲染】(整机配置合成/YAML安全导出/剪枝)
│   ├── mihomo_config.go                         # 【Mihomo基础配置读取、存储与事务校验】(SQLite持久化/DNS与路由边界预检)
│   ├── mihomo_client.go                         # 【Mihomo客户端用户管理与多协议链接生成】(流量配额/月度重置/失效封锁/批量绑定)
│   ├── mihomo_sync.go                           # 【Mihomo订阅出站双向同步与标签映射】(Sub-Manager统一资产联动/前缀归一化)
│   ├── mihomo_core_manager.go                   # 【Mihomo内核下载、安装、多架构调度与守护进程】(Systemd/Direct双模式/解压校验)
│   ├── mihomo_core_auto_update.go               # 【Mihomo内核自动更新巡检与版本判定】(定时调度/Semver比对/自愈回滚)
│   ├── mihomo_core_download_task.go             # 【Mihomo异步下载任务状态机与生命周期】(并发句柄/进度采样/超时控制/取消回调)
│   ├── mihomo_core_download_preferences.go      # 【Mihomo架构首选项与自定义下载源管理】(GOOS/GOARCH/AMD64级别推导/镜像源)
│   ├── mihomo_core_local_detect.go              # 【本地Mihomo内核二进制探测与缓存校验】(BuildInfo提取/版本识别/合规性缓存)
│   ├── mihomo_dns.go                            # 【Mihomo DNS配置校验、清洗与文档构建】(Fake-IP/Nameserver/Fallback/安全防御)
│   ├── mihomo_dns_patch.go                      # 【Mihomo DNS差异补丁更新与事务审计】(乐观锁版本比对/原子更新/配置重载触发)
│   ├── mihomo_helpers.go                        # 【Mihomo代理配置清洗、Multiplex与多协议转换】(gRPC流控/BBR配额/传输层清理)
│   ├── mihomo_inbounds.go                       # 【Mihomo入站监听器管理、用户注入与生命周期】(多认证模式合并/Snell密文/Nft动作)
│   ├── mihomo_inbound_meta.go                   # 【Mihomo入站元数据生成与认证模式定义】(Runtime快照/用户管理模式分类/持久化)
│   ├── mihomo_inbound_references.go             # 【Mihomo路由入站标签依赖与引用校验】(拓扑解构/无主规则拦截/循环指向检测)
│   ├── mihomo_inbound_support.go                # 【Mihomo入站协议能力判定与遗留协议过滤】(运行时白名单/废弃协议拦截)
│   ├── mihomo_listener_compat.go                # 【Mihomo监听器跨协议兼容性归一化】(AnyTLS/Hysteria2/TUIC/Sudoku高级转译)
│   ├── mihomo_mieru_port_range.go               # 【Mieru协议端口跳跃范围提取与重定向规范】(单段范围收敛/选项与JSON一致性清洗)
│   ├── mihomo_nftables.go                       # 【Linux Nftables防火墙流量重定向与透明代理】(TProxy/Redirect表链原子装配/句柄)
│   ├── mihomo_nft_snapshot.go                   # 【Nftables规则句柄快照与链规则完整性校验】(链规则内存索引/失效句柄自愈/多表比对)
│   ├── mihomo_outbounds.go                      # 【Mihomo出站节点管理与订阅节点转换】(节点持久化/ShadowTLS外置封装/重命名联动)
│   ├── mihomo_outboundgroups.go                 # 【Mihomo策略组管理、排序与远程订阅导入】(Select/URL-Test策略组/Clash订阅拉取)
│   ├── mihomo_outbound_references.go            # 【Mihomo出站引用约束检查与拓扑删除保护】(依赖拓扑环分析/级联安全切断)
│   ├── mihomo_port_hop.go                       # 【Hysteria2端口跳跃规范、范围校验与间隔标准化】(多段端口展开/配额阈值/随机间隔)
│   ├── mihomo_proxy_convert.go                  # 【全协议出站到Clash Meta代理节点转换引擎】(14种代理协议转换/RAW节点保真/Dialer链)
│   ├── mihomo_raw_yaml_render.go                # 【Mihomo原始YAML保持渲染与拼接引擎】(保持用户自定义键位/Flow样式Sudoku紧凑序列化)
│   ├── mihomo_route_render.go                   # 【Mihomo路由规则生成、分流匹配与Sub-Rules编译器】(GeoIP/Port分流展开/子规则编译)
│   ├── mihomo_route_patch.go                    # 【Mihomo路由补丁安全更新与热加载】(分流策略动态微调/嗅探器Sniffer注入/审计)
│   ├── mihomo_route_limits.go                   # 【Mihomo路由边界限制、容量配额与上下文构建】(规则上限/笛卡尔积爆炸防护/类型审查)
│   ├── mihomo_route_sanitize.go                 # 【Mihomo路由配置与规则清洗过滤器】(空规则过滤/协议字段规范化/非法动作修正)
│   ├── mihomo_route_tag.go                      # 【Mihomo入站路由生效标签推导器】(原生Detour优先透传/分流入站别名判定)
│   ├── mihomo_shadowquic.go                     # 【ShadowQUIC协议专有入站清洗与JLS代理】(QUIC参数/JLS前置转发/双向凭据自愈)
│   ├── mihomo_sudoku_shared_uuid.go             # 【Sudoku连通分量共享UUID分配与同步】(无向图广度搜索/跨客户端与入站多对多密码保持)
│   ├── mihomo_tls.go                            # 【Mihomo TLS配置生命周期、模式验证与安全边界】(Reality/ECH/uTLS/ShadowTLS强校验)
│   ├── mihomo_tls_outbound_references.go        # 【Mihomo TLS出站代理引用收集与目标规范化】(链式代理依赖遍历/缺失目标阻断/标签对齐)
│   ├── clash_raw_yaml.go                        # 【Clash原始YAML词法切分与块保真保留】(Node AST解析/逐行缩进检测/原生节点保留)
│   │
│   │── 【Linux 底层防火墙、Nftables 与端口转发子群】
│   ├── firewall.go                              # 【防火墙总控服务】(安全规则生命周期/CRUD/安全基线/原子脚本渲染)
│   ├── firewall_connection_stats.go             # 【防火墙连接状态采集】(/proc/net/tcp 探针/并发与异常状态采集)
│   ├── firewall_geoip.go                        # 【防火墙GeoIP规则总控】(国家/地区封禁策略管理/异步规则拉取与缓存)
│   ├── firewall_geoip_nft.go                    # 【GeoIP Nftables规则集合渲染】(CIDR聚合集合注入/大规则集分片下发)
│   ├── firewall_geoip_parser.go                 # 【GeoIP规则多格式解析引擎】(SRS/MRS/JSON/TXT解析与IPSet预算构建)
│   ├── firewall_geoip_sources.go                # 【GeoIP规则上游源分发管理】(多厂商规则源下载/URL路由与镜像回退)
│   ├── firewall_listener_state.go               # 【系统监听器状态探针与属主嗅探】(/proc/net/tcp与/proc/[pid]/fd关联及IPv6识别)
│   ├── firewall_nftables_install.go             # 【Nftables环境自愈与自动安装】(多发行版包管理适配/镜像源修复/异步任务)
│   ├── firewall_runtime_ports.go                # 【防火墙运行时服务端口提供者】(面板/订阅动态端口注册与解耦)
│   ├── firewall_scan.go                         # 【外部防火墙规则扫描器】(nft list ruleset 外部表链嗅探与镜像对齐)
│   ├── firewall_ssh.go                          # 【SSH端口防护与防自锁管理器】(sshd_config安全覆写/防失联恢复与自锁防御)
│   ├── nftables.go                              # 【Nftables核心底层驱动】(原子脚本执行/回滚控制/表链底层封装)
│   ├── nft_capabilities.go                      # 【Nftables内核与工具链能力探针】(版本矩阵嗅探/原生inet与兼容ip/ip6适配)
│   ├── nft_layout_transition.go                 # 【Nftables拓扑动态无缝迁移】(原生inet与兼容性ip/ip6架构双向平滑切换)
│   ├── nft_lifecycle.go                         # 【Nftables生命周期协调器】(开机自启同步/关机规则清理/核心运行时联锁)
│   ├── nftables_client_block.go                 # 【单端口/端口段客户端阻断控制器】(基于nft drop规则的违规端口快速断流)
│   ├── nftables_client_limit.go                 # 【客户端单端口限速与整形器】(基于nft limit rate速率约束与状态持久化)
│   ├── nftables_comment_cleanup.go              # 【Nftables规则注释与废弃句柄清理器】(基于前缀特征的孤儿规则深度GC)
│   ├── nftables_mihomo_client_block.go          # 【Mihomo代理客户端端口拦截控制器】(针对Mihomo专用入站端口的阻断策略对齐)
│   ├── nftables_mihomo_client_limit.go          # 【Mihomo代理客户端带宽限速控制器】(针对Mihomo节点的流量速率限制与整形)
│   ├── nftables_rule_comment.go                 # 【Nftables规则规范化注释构造器】(全链路rule comment格式统一与上下文语义标记)
│   ├── nftables_traffic.go                      # 【Nftables入站流量监控与数据统计服务】(规则级Byte/Packet采样/增量累加/跳频适配)
│   ├── port_check.go                            # 【本地与远程端口连通性及占用检测器】(/proc端口扫描/UDP范围冲突与TCP校验)
│   ├── port_forward.go                          # 【端口转发业务总控与生命周期编排】(NAT转发规则CRUD/冲突校验/原子对齐与回滚)
│   ├── port_forward_kernel_state.go             # 【Linux内核IP转发开关协调器】(net.ipv4.ip_forward探测与恢复)
│   ├── port_forward_listener_claims.go          # 【端口转发与系统监听冲突仲裁器】(面板端口/代理入站/三方服务排他性声明)
│   ├── port_forward_nft.go                      # 【端口转发Nftables规则底层渲染】(PREROUTING DNAT/MASQUERADE/流量命名计数器)
│   ├── port_forward_nft_batch.go                # 【端口转发原子事务脚本批处理引擎】(全量规则原子替换脚本生成/Meter限速注入)
│   ├── port_forward_nft_snapshot.go             # 【端口转发底层Nftables状态快照分析器】(规则完整性校验/命名计数器瞬时读取/Diff)
│   ├── port_forward_runtime_conflicts.go        # 【端口转发运行时冲突探测与告警】(与/proc实时监听对比/冲突进程定位与告警缓存)
│   ├── port_forward_traffic.go                  # 【端口转发流量限额与统计审计中心】(月度流量配额超额熔断/到期封禁/定时重置)
│   ├── port_forward_validation.go               # 【端口转发参数合规与拓扑交叉验证器】(本地环回合法性/目标IP解析/CIDR重叠校验)
│   ├── network_interface.go                     # 【Linux网络接口与路由拓扑探测器】(物理网卡/虚拟网卡/容器网卡与默认路由嗅探)
│   ├── conntrack_flush.go                       # 【Linux内核连接跟踪表刷新与流断开器】(conntrack -F触发/内核网络流表原子刷新)
│   │
│   │── 【证书安全体系与反向代理集群子群】
│   ├── acme_service.go                          # 【ACME证书总控】(Let's Encrypt/ZeroSSL申请/DNS挑战/自动续期/临时防火墙)
│   ├── acme_runtime.go                          # 【ACME运行时沙箱】(acme.sh独立临时环境/CA运行态持久化/凭据隔离)
│   ├── acme_runtime_migration.go                # 【ACME运行时迁移】(旧版全局配置与遗留表数据向资产库平滑迁移)
│   ├── acme_task.go                             # 【ACME异步任务编排】(串行任务队列/实时日志流式会话/超时与取消控制)
│   ├── acme_account_identity.go                 # 【ACME账户与凭据标识】(DisplayID序列分配/防碰撞校验/资源标识生成)
│   ├── acme_certificate_record.go               # 【ACME记录与物料入库】(证书物料解析/重签默认值合并/资产库同步)
│   ├── certificate_inventory.go                 # 【全量证书资产仓储】(统一纳管ACME/自签/导入证书/配额管控/引用感知)
│   ├── certificate_tls_binding.go               # 【内核TLS热绑定同步】(双内核TLS预设实时热重载/证书指纹强校验联动)
│   ├── certificate_binding_usage.go             # 【证书引用与删除防护】(反代/Sing-Box/Mihomo/面板最低保有量删除拦截)
│   ├── certificate_core_restart.go              # 【内核安全平滑热重启】(双内核防抖合并窗口/快速退避重试/多进程实例注入)
│   ├── panel_certificate_inspection.go          # 【面板证书合规性探针】(X.509深度解析/系统根证书信任链探测/自签判定)
│   ├── panel_certificate_balance.go             # 【面板证书软负载均衡】(最小连接数分配/LRU淘汰/无锁选择/诊断状态维护)
│   ├── panel_sqlite_cert_store.go               # 【面板SQLite证书物料库】(面板自签物料持久化/运行时装载器/向资产库迁移)
│   ├── panel_tls_assignment.go                  # 【面板/订阅证书绑定治理】(多证书分配/动态排空优雅停机宽限期)
│   ├── panel_self_signed_cert.go                # 【面板自签证书自动生成】(公网IPv4/IPv6探测/内置SAN填充与续期配置)
│   ├── panel_self_signed_cert_paths.go          # 【面板自签证书存储拓扑】(面板与订阅证书隔离拆分/路径规范与有效性校验)
│   ├── panel_legacy_settings_cert_migration.go  # 【旧版面板证书平滑迁移】(旧配置路径提取入统一资产库)
│   ├── panel_cert_notice.go                     # 【面板证书状态告警通知】(过期/失效登录警告状态原子通信)
│   ├── self_signed_service.go                   # 【自签CA证书签发中心】(内置/自定义CA签发/证书链构建/系统信任链管理)
│   ├── self_signed_renew_config.go              # 【自签证书续期配置模型】(自动续期周期与时间单位编解码)
│   ├── tls.go                                   # 【通用TLS配置治理】(入站TLS规则清洗/证书物料校验/引用变动影响分析)
│   ├── tls_cert_templates.go                    # 【主流CA风格证书模板】(Let's Encrypt/DigiCert/Cloudflare模板矩阵)
│   ├── tls_path_file.go                         # 【本地TLS证书物理探针】(物理文件指纹嗅探与哈希比对)
│   ├── reverse_proxy.go                         # 【多协议反向代理网关】(HTTP/H2/gRPC/WS/TCP/UDP多协议分流与解密)
│   ├── reverse_proxy_certificate_balance.go     # 【反向代理多证书SNI均衡】(分片LRU缓存/精准与通配泛域名匹配)
│   ├── reverse_proxy_compression.go             # 【反向代理流式压缩适配】(Zstd/Brotli/Gzip双向内容协商与重写)
│   ├── reverse_proxy_dns.go                     # 【多协议智能分流DNS网关】(DoH/DoT/DoQ/DoH3/UDP分流与ECS治理)
│   ├── reverse_proxy_dns_http.go                # 【DoH上游连接池与压缩客户端】(HTTP/1.1/H2/H3批量DNS隧道)
│   ├── reverse_proxy_dns_listener.go            # 【DNS多协议监听与访问控制】(套接字限流/CIDR过滤/DoQ治理)
│   ├── reverse_proxy_http2_connect.go           # 【RFC 8441 HTTP/2扩展CONNECT】(Go运行时底层链接补丁)
│   ├── reverse_proxy_memory.go                  # 【反向代理租约式内存池】(防OOM内存租借/响应重写配额控制器)
│   ├── reverse_proxy_resources.go               # 【反代集群资源配额协调】(并发限制/规则版本号乐观锁/配额视图)
│   ├── reverse_proxy_runtime_worker.go          # 【反代运行时常驻守护看门狗】(配置变更自动感知/热同步心跳)
│   │
│   │── 【系统生命周期、OS 底层调优与监控子群】
│   ├── system_boot_lifecycle.go                 # 【系统引导/开机自启完整编排】(内核参数与服务拓扑拉起)
│   ├── system_linux_dns_optimization.go         # 【Linux本地DNS解析栈调优】(resolv.conf重写与防篡改)
│   ├── system_log_optimization.go               # 【生产级日志清理】(journald限制与rsyslog轮转优化)
│   ├── system_managed_file_rewrite.go           # 【配置文件原子写与防篡改】(chattr +i与金样板巡检)
│   ├── system_mtu_optimization.go               # 【网卡MTU自适应探测】(持久化脚本与systemd单元注入)
│   ├── system_optimization_command.go           # 【运维调优指令执行基座】(上下文超时控制与标准流捕获)
│   ├── system_optimization_limits.go            # 【limits.conf文件描述符与线程极限调优】(nofile/nproc调优)
│   ├── system_optimization_startup.go           # 【开机系统优化综合启动器】(并发编排与幂等性校验)
│   ├── system_optimization_watcher.go           # 【独占物理线程文件反篡改看门狗】(金样板定时比对与自愈)
│   ├── system_platform.go                       # 【跨平台环境嗅探】(LXC/Docker/KVM虚拟化与发行版识别)
│   ├── system_sysctl_optimization.go            # 【生产级内核网络栈参数调优】(BBR/fq/TCP缓冲区/连接复用)
│   ├── system_timezone.go                       # 【宿主机系统时区同步与时钟校准守护】
│   ├── systemd_core_startup.go                  # 【systemd核心服务注入、自启使能与依赖反压机制】
│   ├── kernel_manager.go                        # 【多源Linux内核安装管理】(XanMod/BBRPlus/官方源/启动项清洁)
│   ├── kernel_reboot.go                         # 【操作系统原子重启多阶降级规划器】(systemd-run/setsid/init 6)
│   ├── kernel_reboot_linux.go                   # 【Linux专用底层重启系统调用与IPC执行逻辑】
│   ├── kernel_reboot_other.go                   # 【非Linux平台重启桩代码与安全降级】
│   ├── lifecycle.go                             # 【面板服务全生命周期状态机】(优雅停机反压与热迁移中枢)
│   ├── lifecycle_linux.go                       # 【Linux宿主专用systemd单元控制与nftables资源清理】
│   ├── lifecycle_other.go                       # 【非Linux平台生命周期兼容层】
│   ├── coreManager.go                           # 【核心网络进程 (sing-box) 进程总管与状态感知中枢】
│   ├── managed_core_binary.go                   # 【核心二进制元数据读取、版本嗅探与架构合法性校验】
│   ├── managed_core_process.go                  # 【核心进程启动/停止/重启/平滑升级与PID验伪】
│   ├── managed_download_task.go                 # 【通用核心下载任务执行器与断点续传状态机】
│   ├── managed_runtime_hooks.go                 # 【核心运行时生命周期钩子容器与安全回调执行栈】
│   ├── core_auto_check_schedule.go              # 【核心内核版本定期巡检调度器与智能休眠】
│   ├── core_download_progress.go                # 【核心下载进度流式分发器与观察者广播】
│   ├── core_download_task.go                    # 【核心下载任务状态跟踪与生命周期模型封装】
│   ├── core_install_workspace.go                # 【核心安全安装工作区与原子重命名覆盖】
│   ├── core_layout.go                           # 【核心运行时文件系统路径布局与目录拓扑规范】
│   ├── core_log_level.go                        # 【核心日志级别动态映射与运行时热修改】
│   ├── core_runtime_cleanup.go                  # 【核心旧版本与废弃工作区垃圾回收中枢】
│   ├── stats.go                                 # 【运行态系统性能与实时流量监控结构定义】
│   ├── stats_storage.go                         # 【流量统计高频聚合存储与60s压缩存储桶分级归档】
│   ├── traffic_overview.go                      # 【vnStat网络接口流量全周期核算与月结日回滚中枢】
│   ├── traffic_runtime_journal.go               # 【极速ACID日志事务审计中枢与Seqlock分代持久化】
│   ├── runtime_file_store.go                    # 【核心运行时配置SQLite存储与零磁盘损耗临时物化器】
│   ├── runtime_mode.go                          # 【运行模式枚举定义与动态自适应切换】
│   ├── runtime_performance.go                   # 【宿主机负载、CPU/内存/连接数多维性能采集器】
│   ├── runtime_sampler_signal.go                # 【采样中枢高频信号驱动器与定时时钟发生器】
│   ├── dashboard_runtime.go                     # 【运维仪表盘全局运行态聚合展示中枢】
│   ├── host_ownership.go                        # 【宿主资源唯一归属判定与多实例冲突仲裁】
│   ├── host_ownership_lock_linux.go             # 【Linux基于/run/kwor/lifecycle.lock的flock文件排他锁】
│   ├── host_ownership_lock_other.go             # 【非Linux平台内存互斥锁回退保障】
│   ├── ddns_runtime_worker.go                   # 【DDNS物理线程调度中枢与快慢双通道隔离工作池】
│   ├── ddns_service.go                          # 【动态域名解析规则编排与34家云商API对接】
│   ├── dns_servers.go                           # 【sing-box DNS服务卡片全生命周期管理与动态注入】
│   ├── dns_service.go                           # 【全功能云厂商DNS域名记录远程管理服务】
│   │
│   │── 【用户治理、面板设置、ProManager 与订阅编排子群】
│   ├── user.go                                  # 【系统管理用户鉴权与Token生命周期】(用户登录/密码哈希/会话)
│   ├── client.go                                # 【代理节点客户端治理与限额熔断】(客户端CRUD/配额/到期封禁/重置)
│   ├── client_access_policy.go                  # 【客户端访问控制与月度账期策略】(用量超额评估/跨月日边界计算)
│   ├── client_block_ranges.go                   # 【客户端封禁端口范围解析引擎】(Mieru跳频/端口段标准化/NFT封禁计算)
│   ├── client_limit_ranges.go                   # 【客户端限速端口与范围展开计算】(入站限速端口继承/端口段排重展开)
│   ├── setting.go                               # 【面板系统全局配置注册与持久化】(全量设置键值注册表/TTL穿透防护)
│   ├── settings_patch.go                        # 【设置原子增量修补与版本冲突防护】(CAS版本锁检查/变更审计/副作用联动)
│   ├── settings_validation.go                   # 【系统设置输入清洗与边界防御校验】(路由路径/监听IP/FQDN规范化)
│   ├── panel.go                                 # 【面板控制层进程控制与信号派发】(平台重启能力嗅探/SIGHUP平滑重载)
│   ├── panel_stop_marker.go                     # 【面板停止仅标记文件状态控制】(停止标志落盘/防自启状态标记消费)
│   ├── panel_time.go                            # 【面板时区时钟同步与日历边界引擎】(IANA时区探测/时钟单调对齐)
│   ├── panel_time_runtime.go                    # 【面板时钟运行时重载解耦接口】(Cron调度器时区重载注册器/解耦)
│   ├── panel_uninstall.go                       # 【面板原生与容器化卸载任务调度】(卸载前置健康检查/Systemd瞬态托管)
│   ├── panel_uninstall_linux.go                 # 【Linux原生卸载独立工作进程启动器】(独立会话组Setsid脱离/PGID追踪)
│   ├── panel_uninstall_other.go                 # 【非Linux平台卸载防护桩】(跨平台编译桩/阻止非Linux误执行卸载)
│   ├── panel_update.go                          # 【面板版本生命周期与自解压原子升级】(GitHub Release检索/Tar.gz自解压)
│   ├── promanager.go                            # 【Sing-Box核心配置监听与聚合编排】(事件驱动/防抖批处理/DB聚合生成单文件)
│   ├── warp.go                                  # 【Cloudflare Warp WireGuard接口集成】(公私钥生成/TOS注册/Reserved字节)
│   ├── subgroups.go                             # 【订阅分组治理与远程源节点拉取】(订阅分组CRUD/排序/远程JSON提取)
│   ├── subgroups_auto_update.go                 # 【订阅分组多源定时自动更新引擎】(双源轮询/重试退避/订阅节点原子落盘)
│   ├── subgroups_dual_import.go                 # 【Clash配置双向转换与节点合并引擎】(Clash Proxy解析/跨格式深层合并)
│   ├── suboutbounds.go                          # 【订阅出站节点渲染与选择组拓扑】(出站节点持久化/层级选择组构建/URLTest)
│   ├── sub_json_file_guard.go                   # 【订阅文件命名冲突探测与防覆盖守卫】(客户端与分组文件基名冲突预警)
│   ├── sub_json_render.go                       # 【Sing-Box客户端订阅JSON规范化渲染】(默认选择组编排/延迟测试/TLS链注入)
│   ├── sub_selector_tag_migration.go            # 【订阅选择组Legacy Emoji标签平滑迁移】(历史Emoji标签过滤/无损清洗升级)
│   ├── sub_sync_block.go                        # 【客户端入站订阅同步黑名单防误推】(手动删除出站黑名单持久化/熔断联动)
│   ├── subscription_extensions.go               # 【订阅模板扩展校验与YAML/JSON防御】(Clash与JSON扩展结构合规性/防DOS)
│   ├── subscription_initial_state.go            # 【订阅模板出厂基线快照与重置安全锁】(不可变初始基线隔离/出厂配置还原)
│   ├── subscription_runtime_settings.go         # 【订阅运行时缓存与动态代际追踪】(动态Generation单调递增/高并发读缓存)
│   ├── subscription_tls_notice.go               # 【订阅TLS证书告警通知共享容器】(跨模块登录告警传递/原子值并发存取)
│   ├── subscription_tls_path_watch.go           # 【订阅TLS证书物理文件哈希监控巡检】(证书公私钥PEM解析/变更Diff追踪)
│   ├── syncService.go                           # 【客户端入站变动至订阅出站双向同步】(客户端与订阅节点Tag映射/证书链补全)
│   ├── server.go                                # 【服务器硬件度量与TLS密钥对生成引擎】(CPU/内存/磁盘监控/自签与ECH密钥对)
│   ├── services.go                              # 【Sing-Box Services扩展服务治理】(内部反代服务/DNS服务/关联性校验)
│   ├── config.go                                # 【全局配置事务网关与NFT/同步总控】(复合保存入口/NFT流量与封禁规则刷新)
│   ├── login_notice.go                          # 【控制台登录提示多源告警汇聚器】(证书过期告警与订阅TLS警告联合拼装)
│   ├── ipdetect.go                              # 【多网卡与公网出口IP嗅探探测器】(网络接口遍历/公网多API并发竞速解析)
│   ├── http_response.go                         # 【HTTP响应安全限长读取与JSON流控】(防内存溢出OOM防御读取/超限熔断)
│   ├── command_timeout.go                       # 【操作系统命令超时包装器与Systemctl工具】(带Context硬超时子进程执行)
│   └── uninstall.go                             # 【宿主机全量资产接管与自毁式清理引擎】(资源Manifest追踪/防火墙回滚/移除)
│
├── sub/                                         # 【订阅服务分发、多格式渲染与防探测中枢】
│   ├── sub.go                                   # 【HTTPS/TLS独立服务监听器】(证书热加载/SNI负载均衡分流)
│   ├── subService.go                            # 【客户端基础订阅聚合服务】(节点过滤/流量抬头装配)
│   ├── subHandler.go                            # 【HTTP订阅端点路由控制器】(并发限制/SingleFlight防雪崩)
│   ├── subManagerSubService.go                  # 【订阅管理器与节点分组分发】(多协议出站转换/Group聚合)
│   ├── linkService.go                           # 【节点链接解析与外链抓取】(流量标识混淆注入/Base64转码)
│   ├── clashService.go                          # 【Clash Meta/Mihomo订阅转换引擎】(复杂协议策略组生成)
│   ├── mihomo_subscription.go                   # 【Mihomo专用订阅数据加载】(协议OutJson清洗/规范化)
│   ├── jsonService.go                           # 【Sing-Box JSON订阅生成器】(分流规则组/DNS路由拓扑编排)
│   ├── raw_clash_render.go                      # 【Clash原生YAML节点流式合并】(Sudoku流混淆紧凑渲染)
│   ├── clash_proxy_sanitize.go                  # 【Mihomo Clash Proxy节点清洗】(面板内部敏感字段剥离)
│   ├── antiprobe.go                             # 【防主动嗅探与探测抑制中间件】(延迟伪装504响应)
│   └── subscription_tls_refresh.go              # 【订阅出站TLS动态凭据刷新】(ShadowTLS/Restls/JLS封装装配)
│
├── cronjob/                                     # 【Robfig Cron 定时任务与后台调度器】
│   ├── cronJob.go                               # 【定时任务主调度引擎】(非重叠执行保护/生命周期休眠栅栏)
│   ├── acmeAutoRenewJob.go                      # 【ACME自动化证书续期检查】(证书生命周期到期扫描/轮询续签)
│   ├── autoUpdateCoreJob.go                     # 【Sing-Box内核静默自动升级】(定时触发最新发布版资产拉取)
│   ├── autoUpdateMihomoCoreJob.go               # 【Mihomo内核静默自动升级】(定时触发最新发布版资产拉取)
│   ├── certificateCoreRestartJob.go             # 【证书变更内核软重启检查】(防抖合并窗口/延迟软重启执行)
│   ├── checkCoreJob.go                          # 【Sing-Box远程版本检查】(GitHub/Gitee最新版本元数据同步)
│   ├── checkMihomoCoreJob.go                    # 【Mihomo远程版本检查】(GitHub/Gitee最新版本元数据同步)
│   ├── delStatsJob.go                           # 【历史流量审计数据定期清理】(按保留天数轮转清理过期记录)
│   ├── depleteJob.go                            # 【用户配额耗尽/到期自动熔断】(流量超额判定/自动禁用客户端)
│   ├── firewallSyncJob.go                       # 【Linux Nftables防火墙周期同步】(端口规则一致性健康维护)
│   ├── mihomoNftCoreSyncJob.go                  # 【Mihomo流量拦截与规则同步】(底层拦截链一致性巡检与自愈)
│   ├── nftCoreSyncJob.go                        # 【Sing-Box流量拦截与规则同步】(底层拦截链一致性巡检与自愈)
│   ├── panelCertificateBalanceSyncJob.go        # 【面板证书软负载均衡状态刷新】(孤儿连接清理/权重健康扫描)
│   ├── portForwardSyncJob.go                    # 【Linux内核端口转发链同步】(DNAT规则与流量统计定期刷新)
│   ├── runtime_sampler.go                       # 【高性能异步时序采样器】(时隙对齐/批量合并刷盘/防竞争)
│   ├── statsJob.go                              # 【双内核流量指标周期聚合】(出入站流量读取/累加器持久化)
│   ├── subGroupAutoUpdateJob.go                 # 【外部订阅节点定时自动拉取】(远端订阅更新/节点拓扑重构)
│   └── tlsPathSyncJob.go                        # 【证书磁盘物理路径强校验】(证书文件存在性检查与索引维护)
│
├── database/                                    # 【数据访问层与数据库模型】
│   ├── db.go                                    # 【SQLite/GORM数据访问层】(自动迁移/单连接池/PRAGMA加固)
│   ├── backup.go                                # 【数据库热备份与在线迁移引擎】(VACUUM镜像/热替换回滚状态机)
│   ├── db_hooks.go                              # 【数据库生命周期钩子容器】(备份前/还原后广播事件处理总线)
│   ├── ddns_db.go                               # 【独立DDNS/DNS数据访问层】(按需懒加载实例/动态隔离存储)
│   └── model/                                   # 【全量 30 个数据实体模型】(索引策略与关系映射)
│       ├── acme_account.go                      # 【ACME CA账户凭据实体】(Let's Encrypt/ZeroSSL私钥)
│       ├── acme_certificate.go                  # 【ACME证书跟踪实体】(挑战参数/申请状态/过期周期)
│       ├── acme_dns_account.go                  # 【ACME DNS API鉴权凭据实体】(DNS-01验证提供商密钥)
│       ├── certificate_record.go                # 【统一证书档案资产实体】(私钥/公钥/指纹/SAN列表)
│       ├── ddns.go                              # 【DDNS/DNS账户/规则与收藏实体】(域名解析动态更新)
│       ├── dns_servers.go                       # 【上游递归/分流DNS配置实体】(DoH/DoT/UDP/TCP端点)
│       ├── endpoints.go                         # 【WireGuard/Tailscale端点实体】(底层网络打通配置)
│       ├── firewall.go                          # 【Nftables防火墙自定义规则实体】(端口/协议/动作控制)
│       ├── firewall_geo_rule.go                 # 【GeoIP/GeoSite国家地域防火墙实体】(地理位置分流规则)
│       ├── inbounds.go                          # 【Sing-Box入站监听器实体】(多协议配置/TLS绑定/监听端口)
│       ├── login_session.go                     # 【Web管理认证会话实体】(管理员登录Session与令牌管理)
│       ├── mihomo.go                            # 【Mihomo专有入站/出站/客户端模型】(全套独立协议栈实体)
│       ├── model.go                             # 【系统全局设置/状态版本/客户端核心】(系统配置键值模型)
│       ├── network_interface.go                 # 【物理与虚拟网络接口信息实体】(IP/MAC/状态快照)
│       ├── outboundgroups.go                    # 【Sing-Box出站分流策略组实体】(Selector/URLTest/Fallback)
│       ├── outbounds.go                         # 【Sing-Box原生出站节点配置实体】(代理出站参数定义)
│       ├── panel_certificate_balance_state.go   # 【面板自签证书轮询负载状态实体】(多证书平滑分流记录)
│       ├── panel_certificates.go                # 【Web管理面板独立TLS证书实体】(面板安全传输凭据)
│       ├── port_forward.go                      # 【Linux内核端口转发规则实体】(DNAT/SNAT链与流控配置)
│       ├── reverse_proxy.go                     # 【反向代理主机分流规则实体】(多协议路由/重写/TLS绑定)
│       ├── reverse_proxy_certificate_balance_state.go # 【反代多证书软均衡状态实体】(反向代理SNI分流记录)
│       ├── reverse_proxy_settings.go            # 【反向代理全局资源配额单例模型】(内存池/防OOM策略)
│       ├── self_signed_authority.go             # 【内置根CA证书认证机构实体】(本地根CA私钥与公钥)
│       ├── services.go                          # 【后台守护进程与系统服务实体】(服务启停/运行态元数据)
│       ├── singbox_config_state.go              # 【Sing-Box配置版本控制实体】(配置热加载与修订跟踪)
│       ├── sub_sync_block.go                    # 【订阅同步黑名单规则实体】(节点自动过滤排除规则)
│       ├── subgroups.go                         # 【外部订阅源分组管理实体】(分组拉取/定时更新配置)
│       ├── suboutbounds.go                      # 【订阅分发专有节点实体】(托管节点/原始YAML缓存)
│       ├── system_platform.go                   # 【宿主机OS架构平台快照实体】(操作系统/CPU架构固化)
│       └── traffic.go                           # 【多维流量监控与限流熔断实体】(入站/客户端/端口转发分级流量)
│
├── util/                                        # 【专属协议栈适配与通用算法支撑层】
│   ├── base64.go                                # 【Base64编解码与安全自适应识别】(标准/URLSafe兼容解码)
│   ├── genLink.go                               # 【全协议客户端分享链接生成器】(URI装配/混淆参数格式化)
│   ├── linkToJson.go                            # 【全协议分享链接反向解析器】(URI反向转化为Outbound配置)
│   ├── outJson.go                               # 【入站模板生成与变量替换引擎】(客户端配置OutJson自动填充)
│   ├── subInfo.go                               # 【Subscription-Userinfo响应头构造】(流量与过期时间格式化)
│   ├── subscription_host.go                     # 【订阅节点域名/IP自动匹配回源器】(IPv6方括号转义与主机解析)
│   ├── subscription_outbound_variants.go        # 【节点多端口/双栈地址裂变引擎】(克隆多变量版本展开)
│   ├── subscription_support.go                  # 【订阅协议特性支持检测矩阵】(内核协议兼容性过滤)
│   ├── hysteria_quic.go                         # 【Hysteria 1/2 QUIC协议流控适配】(接收窗口规范化与清洗)
│   ├── mieru.go                                 # 【Mieru专属抗封锁协议出站配置】(端口范围解析与配置展开)
│   ├── mihomo_realm_opts.go                     # 【Mihomo Realm选项命名适配器】(snake_case与kebab-case转换)
│   ├── shadowquic.go                            # 【ShadowQUIC协议原生字段清洗器】(TLS剥离/官方标准校验)
│   ├── shadowtls.go                             # 【ShadowTLS V1/V2/V3协议栈适配】(前置握手/Detour配对构建)
│   ├── singbox_outbound_config.go               # 【Sing-Box标准出站字段校验器】(合法协议字段白名单校验)
│   ├── sudoku.go                                # 【数独混淆 (Sudoku) 协议适配器】(混淆表注入与参数规范化)
│   ├── sudoku_yaml_flow.go                      # 【数独混淆YAML流式紧凑序列化】(自定义表压缩单行排版)
│   ├── trusttunnel.go                           # 【TrustTunnel专属可信隧道适配器】(密钥/UUID映射与出站构建)
│   ├── vless_encryption.go                      # 【VLESS流控加密与Reality拓展】(mlkem768等高级算法握手解析)
│   ├── inbound_ids.go                           # 【入站ID集合解析器】(JSON数组/逗号分隔字符串类型解析)
│   ├── inbound_order.go                         # 【入站节点保序排序算法】(依据入站绑定原始顺序重排)
│   └── common/                                  # 【底层共享通用函数库】
│       ├── array.go                             # 【泛型切片集合通用算法库】(去重/包含/差异比对)
│       ├── err.go                               # 【统一错误包装与格式化工具】(错误追踪与安全消息返回)
│       └── random.go                            # 【密码学安全随机发生器】(UUID/随机密钥/Hex安全生成)
│
├── compression/Compression-algorithm/           # 【高性能多算法流式压缩与编解码库】
│   ├── Brotli_Codec.go                          # 【Google Brotli高压缩比编解码器】(动态滑窗/质量等级适配)
│   ├── Compression_codecs.go                    # 【统一压缩编解码器工厂】(解压炸弹防护/窗口大小动态计算)
│   ├── Compression_http.go                      # 【HTTP响应动态流式压缩ResponseWriter】(Header劫持/按需压缩)
│   ├── Compression_negotiation.go               # 【RFC 7231内容编码协商引擎】(固定权重优先级调度/q值解析)
│   ├── Compression_types.go                     # 【压缩算法常量与错误定义】(支持算法枚举/默认压缩等级)
│   ├── Deflate_Codec.go                         # 【RFC 1951 Deflate原生流式编解码器】(标准zlib/flate包装)
│   ├── Gzip_Codec.go                            # 【RFC 1952 Gzip标准流式编解码器】(通用Gzip包装与排空)
│   ├── README.md                                # 【压缩子系统架构设计与规范指引】(算法基准/安全防御原则)
│   ├── S2_Codec.go                              # 【Klauspost S2超高速流式编解码器】(吞吐优先/扩展编码支持)
│   ├── Snappy_Codec.go                          # 【Google Snappy极速内存流编解码器】(Framing格式/无级别压缩)
│   └── Zstd_Codec.go                            # 【Facebook Zstandard极致吞吐编解码器】(32MiB滑窗/级别8优化)
│
├── scripts/                                     # 【DevOps 自动化运维、发布构建与环境验收脚本】
│   ├── docker-release.mjs                       # 【GitHub Actions与GHCR清单校验引擎】(自适应代理通道/多架构Manifest)
│   ├── docker-release.test.mjs                  # 【容器发布流水线自动化测试套件】(标签解析/工作流过滤/清单断言)
│   ├── release-publish.mjs                      # 【GitHub自动化发布生产级流水线 (731行)】(单源同步/Draft暂存/资产上传)
│   ├── sync-version.mjs                         # 【全局语义化版本单源同步工具】(config/version同步至package/lock/docker)
│   ├── sync-web-assets.mjs                      # 【静态资产热同步与字体瘦身裁剪器】(dist同步至web/html/剔除冗余字体)
│   ├── verify-vnstat-linux.sh                   # 【Debian/Ubuntu宿主机只读预检与验收脚本】(APT源元数据检查/10步验收序列)
│   └── vscode-f5-dev.bat                        # 【VSCode一键F5本地开发调试编排批处理】(前端调试编译/后端无优化构建)
│
├── windows/                                     # 【Windows 专属服务编排与本地构建套件】
│   ├── README.md                                # 【Windows开发与服务管理技术说明】(使用指南/构建与运维说明)
│   ├── install-windows.bat                      # 【Windows企业级全自动向导安装脚本 (212行)】(WinSW下载/网络配置/ACL加固)
│   ├── kwor-windows.xml                         # 【WinSW核心服务生命周期定义规范】(日志10MB轮转/崩溃阶梯自愈/TCP依赖)
│   ├── kwor-windows.bat                         # 【Windows终端交互式控制台菜单脚本 (247行)】(启停重载/日志查看/浏览器直达)
│   ├── uninstall-windows.bat                    # 【Windows彻底卸载与数据安全清理批处理】(服务注销/注册表抹除/防火墙撤销)
│   ├── kwor-windows-build.bat                   # 【Windows CMD原生极简一键构建脚本】(前端构建/资产同步/无CGO编译)
│   └── kwor-windows-build.ps1                   # 【Windows PowerShell自动化构建脚本】(参数化架构/日志/文件体积指纹)
│
└── temp_frontend/                               # 【前端 SPA 核心工程】(Vue 3.4 + TypeScript + Vuetify 3 + Pinia + Vite)
    ├── package.json                             # 【前端包声明与流水线定义】(版本对齐、打包与校验脚本)
    ├── tsconfig.json                            # 【TypeScript编译器配置】(严格类型校验、路径别名映射)
    ├── vite.config.mts                          # 【Vite现代构建引擎配置】(散列去重、Sass现代编译、反向代理)
    └── src/
        ├── App.vue                              # 【前端根容器组件】(全局加载蒙层/运行重试提示/确认对话框/标题绑定)
        ├── main.ts                              # 【前端引导入口】(Vue实例创建/插件装配/Notivue通知/i18n挂载)
        │
        ├── router/                              # 【路由导航与权限生命周期体系】
        │   └── index.ts                         # 【路由定义与守卫调度引擎】(双内核数据轮询/会话自适应保活/可见性感知)
        │
        ├── store/                               # 【Pinia 响应式状态管理模型】
        │   ├── index.ts                         # 【Pinia根实例入口】(全局状态树单例创建与导出)
        │   ├── uiNamespace.ts                   # 【双内核命名空间抽象契约】(default/mihomo API端点与核心配置映射)
        │   └── modules/                         # 【子状态机模块】
        │       ├── data.ts                      # 【Sing-box全局状态机】(增量同步/节点防重/DNS与基础配置乐观并发)
        │       └── mihomoData.ts                # 【Mihomo全局状态机】(Clash架构轻量轮询/路由与DNS补丁/双向数据共享)
        │
        ├── types/                               # 【TypeScript 类型契约与数据接口体系】(全量 18 个协议与实体契约)
        │   ├── brutal.ts                        # 【Brutal拥塞控制接口】(速率限制与使能契约)
        │   ├── clients.ts                       # 【用户与客户端契约模型】(多协议凭证映射/UUID碰撞防护/流量换算)
        │   ├── config.ts                        # 【Sing-box核心配置模型】(NTP/Route/RuleSet/Experimental/ClashAPI)
        │   ├── ddns.ts                          # 【DDNS动态域名契约】(Provider元数据/账号凭据/解析规则/接口探测)
        │   ├── dial.ts                          # 【底层网络连接Dial契约】(网络接口绑定/路由标记/TCP&UDP调优)
        │   ├── dns.ts                           # 【Sing-box DNS体系契约】(12种DNS服务/分流规则/缓存与反向映射)
        │   ├── dnsManage.ts                     # 【DNS域名管理契约】(服务商账号/DNS解析记录/常用域名收藏夹)
        │   ├── endpoints.ts                     # 【接入端点Endpoint契约】(WireGuard/Warp/Tailscale隧道与Peer)
        │   ├── inbounds.ts                      # 【入站节点协议契约】(24种协议/双端模板/端口跳跃/伪装配置)
        │   ├── multiplex.ts                     # 【多路复用Multiplex契约】(smux/yamux/h2mux协议栈/连接流限制)
        │   ├── outboundgroups.ts                # 【出站分组模型】(Selector/URLTest测速与选择器契约)
        │   ├── outbounds.ts                     # 【出站节点协议契约】(21种出站代理/底层Dial套接字/TLS镜像参数)
        │   ├── reverseProxy.ts                  # 【高性能反向代理契约】(反代规则/监听与上游协议/并发限制/内存池)
        │   ├── rules.ts                         # 【路由分流规则契约】(条件组合/Mihomo资源限额/逻辑树深度校验)
        │   ├── services.ts                      # 【系统底层服务契约】(DERP中继/Resolved代理/SSM-API契约)
        │   ├── subgroups.ts                     # 【订阅分组SubGroup契约】(订阅源URL/自动定时抓取/Clash与JSON兼容)
        │   ├── tls.ts                           # 【TLS与高级伪装契约】(TLS/Reality/ShadowTLS/Restls/JLS 5模式适配)
        │   └── transport.ts                     # 【传输层Transport契约】(WS/gRPC/H2/HTTPUpgrade/XHTTP协议栈)
        │
        ├── views/                               # 【核心业务表现视图】(全量 20 个功能模块页面)
        │   ├── Home.vue                         # 【仪表盘宿主视图】(挂载Main实时监控与多维状态图表)
        │   ├── Login.vue                        # 【管理员鉴权视图】(表单校验/安全认证/国际化语言与主题切换)
        │   ├── Admins.vue                       # 【管理员凭据管理视图】(登录时间格式化/密码重置/审计日志/API Token)
        │   ├── Basics.vue                       # 【Sing-box基础配置视图】(NTP同步/缓存文件/Clash API/V2Ray统计/版本锁)
        │   ├── Clients.vue                      # 【Sing-box用户管理视图】(多维度过滤/分页卡片/限速/重置周期/批量操作)
        │   ├── MihomoClients.vue                # 【Mihomo用户管理门面】(注入mihomo命名空间代理Clients视图)
        │   ├── Inbounds.vue                     # 【Sing-box入站管理视图】(内核启停控制/多协议卡片/端口跳跃监控/删除)
        │   ├── MihomoInbounds.vue               # 【Mihomo入站管理门面】(注入mihomo命名空间代理Inbounds视图)
        │   ├── Outbounds.vue                    # 【Sing-box出站管理视图】(出站节点卡片/引用安全校验删除/分组入口)
        │   ├── MihomoOutbounds.vue              # 【Mihomo出站管理门面】(注入mihomo命名空间代理Outbounds视图)
        │   ├── Rules.vue                        # 【Sing-box路由管理视图】(默认出口与网卡/规则拖拽排序/逻辑规则/规则集)
        │   ├── MihomoRules.vue                  # 【Mihomo路由管理门面】(注入mihomo命名空间代理Rules视图)
        │   ├── Dns.vue                          # 【Sing-box DNS视图】(12类服务器/DoH&DoT/分流规则/缓存容量/反向解析)
        │   ├── MihomoDns.vue                    # 【Mihomo专有DNS视图】(服务端内置DNS/Direct与ProxyNameserver/格式校验)
        │   ├── Tls.vue                          # 【Sing-box TLS视图】(证书与伪装卡片/入站与服务反向引用追踪/克隆/删除)
        │   ├── MihomoTls.vue                    # 【Mihomo TLS门面视图】(注入mihomo命名空间代理Tls视图)
        │   ├── Services.vue                     # 【Sing-box服务管理视图】(DERP中继网关/Resolved服务/SSM-API卡片)
        │   ├── Endpoints.vue                    # 【接入端点管理视图】(WireGuard/Warp/Tailscale接入隧道卡片与在线探针)
        │   ├── SubManager.vue                   # 【订阅管理调度中心】(节点卡片/外部源导入/全量清空/删除防反向同步)
        │   └── Settings.vue                     # 【系统全局治理中心】(14大设置选项卡全面调度：网络/防火墙/优化/反代等)
        │
        ├── layouts/                             # 【骨架布局与全量交互弹窗模态框】
        │   ├── default/                         # 【基础骨架布局组件 (4个文件)】
        │   │   ├── Default.vue                  # 【全局默认布局框架】(断点检测/自适应侧边抽屉收放联动)
        │   │   ├── AppBar.vue                   # 【全局顶部导航栏】(路由标题多语言映射/singbox前缀规范/主题切换)
        │   │   ├── Drawer.vue                   # 【全局侧边栏导航抽屉】(PC端悬浮展折/移动端抽屉覆盖/权限过滤/注销)
        │   │   └── View.vue                     # 【核心路由视图挂载容器】(页面外边距与滚动视口管理)
        │   │
        │   └── modals/                          # 【业务模态与交互弹窗库 (全量 28 个交互模态框)】
        │       ├── Admin.vue                    # 【管理员密码修改弹窗】(旧密码验证与新凭证更新)
        │       ├── Backup.vue                   # 【数据库备份与恢复弹窗】(热备份下载/备份文件恢复/重连轮询)
        │       ├── Changes.vue                  # 【系统审计日志弹窗】(操作记录过滤/差异比对查看)
        │       ├── Client.vue                   # 【用户全属性编辑弹窗】(基础/协议凭证/外链3大Tab/限速与到期)
        │       ├── ClientBulk.vue               # 【批量添加用户弹窗】(前缀与连续区间/并发防重/初始入站绑定)
        │       ├── ConfirmDialog.vue            # 【全局确认模态对话框】(info/warning/danger 3级定制/防误触焦点控制)
        │       ├── Dns.vue                      # 【Sing-box DNS服务器编辑弹窗】(12种协议自适应配置/证书关联)
        │       ├── DnsRule.vue                  # 【Sing-box DNS分流规则弹窗】(客户端匹配/多域策略/规则集挂载)
        │       ├── Endpoint.vue                 # 【接入端点编辑弹窗】(WireGuard密钥对/MTU/Peer列表/Tailscale参数)
        │       ├── Inbound.vue                  # 【多协议入站节点编辑弹窗】(服务端与客户端Tab/24种协议/端口跳跃/伪装)
        │       ├── Logs.vue                     # 【节点实时日志查看弹窗】(滚动流/日志格式化)
        │       ├── MihomoCore.vue               # 【Mihomo内核管理弹窗】(双通道版本对比/下载解压进度/守护进程启停)
        │       ├── Outbound.vue                 # 【出站节点编辑弹窗】(21种代理协议/Dial套接字选项/TLS镜像生成)
        │       ├── OutboundGroup.vue            # 【出站分组管理弹窗】(拖拽排序/Selector与URLTest/外部订阅导入)
        │       ├── PortLogs.vue                 # 【端口跳跃监控日志弹窗】(实时跳跃流/时间戳解析/一键清空)
        │       ├── QrCode.vue                   # 【客户端订阅与节点分享弹窗】(单节点与综合订阅二维码/剪贴板复制)
        │       ├── Rule.vue                     # 【路由规则编辑弹窗】(逻辑规则/简单规则/级联匹配条件/动作分流)
        │       ├── Ruleset.vue                  # 【规则集维护弹窗】(local/remote/inline/file/http、mrs/yaml/binary)
        │       ├── Service.vue                  # 【系统服务配置弹窗】(DERP中继网关/Resolved监听参数)
        │       ├── SingboxCore.vue              # 【Sing-box内核管理弹窗】(多架构检测/GitHub Release拉取/启停控制)
        │       ├── Stats.vue                    # 【历史流量统计图表弹窗】(Chart.js折线图/多时间周期/单位格式化)
        │       ├── SubGroup.vue                 # 【订阅分组维护弹窗】(拖拽排序/自动定时拉取/双格式外部订阅解析)
        │       ├── SubGroupQrCode.vue           # 【订阅分组专属二维码弹窗】(分组订阅地址与扫码)
        │       ├── SubManagerQrCode.vue         # 【订阅管理节点二维码弹窗】(节点即时扫码连接)
        │       ├── SubOutbound.vue              # 【订阅专属出站节点编辑弹窗】(节点属性/协议转换/分组绑定)
        │       ├── Tls.vue                      # 【TLS与高级伪装编辑弹窗】(TLS/Reality/ShadowTLS/Restls/JLS 5模式)
        │       ├── Token.vue                    # 【API访问令牌管理弹窗】(Token生成/权限描述/过期回收)
        │       └── WgQrCode.vue                 # 【WireGuard客户端配置二维码弹窗】(Conf配置文本/扫码直连)
        │
        ├── locales/                             # 【国际化本地化多语言体系】(6 国语言字典与类型定义)
        │   ├── index.ts                         # 【Vue I18n实例工厂】(语言包注册、持久化与降级策略)
        │   ├── portForwardFallback.ts           # 【端口转发字典浅层回退定义】(消除递归类型推导开销)
        │   ├── zhcn.ts                          # 【简体中文主字典】(千级运维/协议/系统词条全量定义)
        │   ├── zhtw.ts                          # 【繁体中文主字典】(港台地区专属术语映射)
        │   ├── en.ts                            # 【英语官方主字典】(全局默认回退基准语言包)
        │   ├── fa.ts                            # 【波斯语主字典】(中东地区支持，配合RTL布局引擎)
        │   ├── ru.ts                            # 【俄语主字典】(东欧地区网络协议与界面术语)
        │   └── vi.ts                            # 【越南语主字典】(东南亚本地化交互语言包)
        │
        ├── plugins/                             # 【前端插件库与核心基础设施】(网络/时间/协议/工具箱)
        │   ├── index.ts                         # 【Vue插件全局注册入口】(Vuetify实例挂载)
        │   ├── vuetify.ts                       # 【Vuetify 3样式与主题核心配置】(预设与深浅主题)
        │   ├── api.ts                           # 【Axios底层网络适配器】(动态BaseURL探测与GET去重引擎)
        │   ├── httputil.ts                      # 【统一HTTP交互网关】(响应解包、i18n翻译与会话失效阻断)
        │   ├── confirm.ts                       # 【全局异步安全确认弹窗控制器】(单例锁与防重复提交流程)
        │   ├── panelTime.ts                     # 【服务端基准时间与时区锚定系统】(单调时钟防时钟漂移)
        │   ├── portCheck.ts                     # 【端口占用探针网络服务】(TCP/UDP单端口与跳跃段探测)
        │   ├── portRange.ts                     # 【端口范围与跳跃表达式解析器】(区间合并/排序/4096硬限)
        │   ├── randomUtil.ts                    # 【加密安全伪随机数工具箱】(UUID/凭证密钥/Shadowsocks密码)
        │   ├── sessionNavigation.ts             # 【认证状态与平滑登出导航网关】(会话失效安全重定向)
        │   ├── shadowQuic.ts                    # 【ShadowQUIC协议参数校验与默认工厂】(BBR/回落地址规范化)
        │   ├── singboxDuration.ts               # 【Sing-box复合持续时间语法解析器】(多单位到秒级双向换算)
        │   ├── singboxInteger.ts                # 【Sing-box整数与字节列表解析器】(安全边界整型/3字节保留位)
        │   ├── subscriptionUri.ts               # 【订阅分发地址规范化与探测服务】(外部域名校验与斜杠补全)
        │   └── utils.ts                         # 【通用工具集】(深度比对、网络流量/数据包/时间自适应格式化)
        │
        └── components/                          # 【前端解耦控件库与业务组件集】(全量 96 个核心解耦组件)
            ├── Main.vue                         # 【控制台仪表盘主容器】(磁贴个性化配置/硬件指标/备份日志入口)
            ├── message.vue                      # 【自适应通知浮窗控件】(Notivue封装、Unicode双向RTL字符探测)
            ├── Editor.vue                       # 【代码级文本编辑器弹窗】(大文档行号熔断/强制LTR渲染/滚动对齐)
            ├── Users.vue                        # 【多协议用户与用户组选择器】(芯片化多选/单用户清洗/角色映射)
            ├── Listen.vue                       # 【通用入站监听配置卡片】(端口/TFO/MPTCP/UDP NAT/Detour)
            ├── Dial.vue                         # 【通用出站拨号配置卡片】(链式Detour/Bind IF/Routing Mark)
            ├── Multiplex.vue                    # 【TCP多路复用配置卡片】(smux/yamux/h2mux切换/TCP Brutal加速)
            ├── Addr.vue                         # 【服务器主机地址与备注表单】(地址输入/端口边界/集成OutTLS)
            ├── DateTime.vue                     # 【服务端时区关联日历控件】(Persian DatePicker适配与转换)
            ├── DnsRule.vue                      # 【Sing-box DNS分流规则编辑器】(后缀/正则/CIDR/查询类型全匹配)
            ├── Headers.vue                      # 【HTTP/WS自定义请求头键值编辑器】(动态增删/数组合并/空行抑制)
            ├── MihomoClientCommonFields.vue     # 【Mihomo客户端通用参数表单】(UDP开关/IP偏好/SMux/BBR Profile)
            ├── Network.vue                      # 【网络层传输类型快选控件】(TCP/UDP/双栈网络归一化)
            ├── OutJson.vue                      # 【节点出站导出模版适配器】(订阅出站参数注入/协议安全模式清洗)
            ├── Rule.vue                         # 【路由分流策略表单控件】(地理位置/域名/IP/入站源标签多路路由)
            ├── SimpleDNS.vue                    # 【轻量DNS上游服务器表单】(UDP/TCP/DoT/DoH/DoQ自适应)
            ├── Transport.vue                    # 【通用传输层聚合封装容器】(HTTP/H2/WS/gRPC/HTTPUpgrade/XHTTP)
            ├── UoT.vue                          # 【UDP over TCP协议版本切换器】(v1/v2协议切换与禁用控制)
            ├── WgPeer.vue                       # 【WireGuard对端Peer编排卡片】(公私钥配对/PSK/Reserved保留位)
            ├── SettingsPortForwardManage.vue    # 【端口转发视图呈现组件】(nftables规则卡片/冲突面板/流量条)
            ├── SettingsPortForwardManage.shared.ts # 【端口转发核心状态Composable】(规则CRUD/PID冲突探测)
            ├── SettingsReverseProxyManage.vue   # 【反向代理视图呈现组件】(Go原生反代规则卡片/证书匹配展示)
            ├── SettingsReverseProxyManage.shared.ts # 【反向代理状态驱动引擎】(HTTP/HTTPS/DNS规则/流式压缩配置)
            ├── SettingsTrafficManage.vue        # 【流量统计与vnstat控制视图】(物理网卡拓扑图/月度用量仪表)
            ├── SettingsTrafficManage.shared.ts   # 【流量与vnstat守护进程管理器】(安装探测/冲突判定/重置计算)
            ├── SettingsFirewallManage.vue       # 【系统防火墙管理视图组件】(nftables规则拓扑展示/国家阻断开关)
            ├── SettingsFirewallManage.shared.ts  # 【防火墙业务逻辑与状态驱动】(GeoIP规则集成/端口黑白名单下发)
            ├── SettingsFirewallGeoOptions.ts    # 【全球地理IP规则源与国家代码库】(SagerNet/MetaCubeX/Chocolate4U)
            ├── SettingsKernelManage.vue         # 【Linux宿主机内核管理弹窗】(XanMod/BBR探测/微架构包下载安装)
            ├── SettingsOptimizationManage.vue   # 【系统环境与性能优化弹窗】(journald/sysctl/resolv.conf加锁/MTU)
            ├── SettingsInterfaceManage.vue      # 【面板网络与安全会话配置弹窗】(WebListen/Port/Path/超时/平滑重启)
            ├── SettingsDdnsManage.vue           # 【DDNS动态解析管理视图组件】(解析任务卡片/接口状态/手动同步)
            ├── SettingsDdnsManage.shared.ts      # 【DDNS业务逻辑与驱动器】(Cloudflare/阿里云等API认证与调度)
            ├── SettingsDnsManage.vue            # 【内核级DNS策略管理视图】(服务器池/FakeIP/策略路由集中配置)
            ├── SettingsDnsManage.shared.ts       # 【内核级DNS状态编排引擎】(直接解析器与代理解析器逻辑绑定)
            ├── SettingsAcmeManage.vue           # 【ACME自动化证书管理弹窗】(Let's Encrypt/ZeroSSL自动化申请续签)
            ├── SettingsClashSubManage.vue       # 【Clash (Mihomo) 订阅模板管理弹窗】(可视化与Editor双模式配置生成)
            ├── SettingsJsonSubManage.vue        # 【Sing-box JSON订阅模板管理弹窗】(入出站规则可视化模版定制)
            ├── SettingsSubscriptionManage.vue   # 【订阅服务全局配置弹窗】(订阅端口/随机路径/外部域名/节点过滤)
            ├── SettingsLanguageManage.vue       # 【界面语言与时区持久化弹窗】(实时切换/LocalStorage本地同步)
            ├── SubClashExt.vue                  # 【Clash订阅扩展可视化表单】(策略组拖拽排序/规则提供商切换)
            ├── SubClashExtConstants.ts          # 【Clash订阅生成常量定义】(默认配置字典/FakeIP范围/规则源URL)
            ├── SubClashExtLogic.ts              # 【Clash订阅生成核心算法 (4484行)】(节点拓扑生成/YAML AST转换)
            ├── SubJsonExt.vue                   # 【JSON订阅扩展可视化表单】(Sing-box规则集选择/DNS服务器分组)
            ├── SubJsonExtConstants.ts           # 【JSON订阅生成常量与默认拓扑】(基础配置模板/默认Outbound链)
            ├── SubJsonExtCustomDnsPlugin.ts     # 【Sing-box自定义DNS插件逻辑】(DNS规则注入与上游劫持逻辑)
            ├── SubJsonExtLogic.ts               # 【JSON订阅生成核心算法】(全量Sing-box规范JSON树递归构建器)
            ├── subscriptionRuleSetProbe.ts      # 【订阅规则集远程可达性探针服务】(批量探测规则提供商源URL状态)
            │
            ├── protocols/                       # 【业务解耦协议表单控件集】(31 个协议专属表单与参数交互控件)
            │   ├── AnyTls.vue                   # 【AnyTLS协议专属配置表单】(无感知TLS传输参数控制)
            │   ├── Direct.vue                   # 【直连Direct出站表单控件】(路由直出与绑定接口指定)
            │   ├── Http.vue                     # 【标准HTTP代理入站表单】(HTTP鉴权与透明代理配置)
            │   ├── Hysteria.vue                 # 【Hysteria v1协议入出站表单】(带宽/混淆/端口跳跃)
            │   ├── Hysteria2.vue                # 【Hysteria 2协议深度配置卡片 (1197行)】(BBR Profile/Masquerade)
            │   ├── Hysteria2Realm.vue           # 【Hysteria 2 Realm中介服务器控件】(房间号正则/IP连接上限)
            │   ├── Mieru.vue                    # 【Mieru混淆传输协议表单】(用户端口分配与加密通道管理)
            │   ├── Naive.vue                    # 【NaiveProxy入站配置卡片】(基于Chromium网络栈的抗检测代理)
            │   ├── OutNaive.vue                 # 【NaiveProxy出站订阅模版组件】(padding与鉴权导出参数)
            │   ├── OutShadowTls.vue             # 【ShadowTLS出站订阅配置表单】(密码、SNI与握手探测策略)
            │   ├── Selector.vue                 # 【手动出站节点选择器Selector表单】(Outbound节点手动切换池)
            │   ├── ShadowQuic.vue               # 【ShadowQUIC深度配置卡片 (753行)】(JLS回落/BBR拥塞优化)
            │   ├── Shadowsocks.vue              # 【Shadowsocks协议配置卡片】(AEAD密码算法/2022规范/插件绑定)
            │   ├── ShadowTls.vue                # 【ShadowTLS协议入站表单】(伪装TLS域名/v2/v3协议版本协商)
            │   ├── Snell.vue                    # 【Snell协议轻量代理表单】(PSK/版本控制与连接复用Reuse)
            │   ├── Socks.vue                    # 【标准SOCKS协议配置卡片】(SOCKS4/4a/5协议与密码认证)
            │   ├── Ssh.vue                      # 【SSH客户端出站代理表单】(公私钥认证/密码认证/HostKey校验)
            │   ├── SshInbound.vue               # 【SSH协议入站服务卡片】(主机密钥注入/终端会话限制)
            │   ├── Sudoku.vue                   # 【Sudoku新型数独协议表单 (691行)】(随机数独矩阵/纯下行通道)
            │   ├── Tailscale.vue                # 【Tailscale网格网络互联出站表单】(AuthKey/ControlURL/路由通告)
            │   ├── Tor.vue                      # 【Tor洋葱网络出站代理表单】(隔离标识/回路构建超时/混淆桥)
            │   ├── TProxy.vue                   # 【透明代理TProxy入站卡片】(Linux内核级透明重定向与路由标记)
            │   ├── Trojan.vue                   # 【Trojan协议入出站表单】(密码认证/UDP over TCP/多路复用)
            │   ├── TrustTunnel.vue              # 【TrustTunnel协议表单 (361行)】(QUIC/TCP切换/健康检查/连接复用)
            │   ├── Tuic.vue                     # 【TUIC v5协议高级卡片】(Congestion Control/0-RTT/UDP Relay)
            │   ├── Tun.vue                      # 【TUN虚拟网卡入站配置卡片】(MTU/网络栈切换/AutoRoute)
            │   ├── UrlTest.vue                  # 【自动测速优选出站UrlTest表单】(健康检查URL/容差门限/轮询间隔)
            │   ├── Vless.vue                    # 【VLESS协议深度配置卡片】(UUID/XTLS-rprx-vision/Packet Encoding)
            │   ├── Vmess.vue                    # 【VMess协议深度配置卡片】(AEAD安全模式/alterId兼容/全局填充)
            │   ├── Warp.vue                     # 【Cloudflare WARP专有出站表单】(节点密钥提取/保留位/LicenseKey)
            │   └── Wireguard.vue                # 【WireGuard协议完整出站卡片】(对端Peer编排/MTU/保留字节)
            │
            ├── services/                        # 【增值微服务配置卡片】
            │   ├── Derp.vue                     # 【Tailscale DERP中继中转服务器配置】(STUN绑定/VerifyClient/Mesh)
            │   └── SSMAPI.vue                   # 【Shadowsocks SIP008管理接口配置】(动态多端口/服务端路由表映射)
            │
            ├── tiles/                           # 【控制台仪表盘图表与实时磁贴】
            │   ├── Gauge.vue                    # 【CPU/内存/磁盘/Swap仪表盘百分比刻度卡片】(纯CSS旋转扇形/告警调色)
            │   └── History.vue                  # 【实时流量与硬件折线图磁贴】(Chart.js引擎/20点滑动窗口/无动画低开销)
            │
            ├── tls/                             # 【TLS安全层与拟态伪装扩展控件】
            │   ├── Acme.vue                     # 【内嵌ACME证书挑战表单】(HTTP-01/TLS-ALPN-01/DNS-01挑战参数)
            │   ├── Ech.vue                      # 【ECH加密客户端Hello配置卡片】(PQ混合后量子加密/外部Config注入)
            │   ├── InTLS.vue                    # 【入站TLS模版绑定下拉卡片】(支持标准TLS/Reality/Shadow-TLS/Restls)
            │   ├── MihomoJls.vue                # 【Mihomo专有JLS伪装握手层配置】(上游真实TLS嗅探与回落模拟)
            │   ├── MihomoRestls.vue             # 【Mihomo专有Restls伪装层控件】(Restls脚本注入与客户端指纹拟态)
            │   ├── MihomoShadowTls.vue          # 【Mihomo专有ShadowTLS封装表单】(混淆握手SNI与密钥配置)
            │   └── OutTLS.vue                   # 【出站TLS认证与伪装配置表单】(ALPN/SNI/证书指纹/不安全跳过)
            │
            └── transports/                      # 【底层传输层多路复用与载荷协议控件】
                ├── gRPC.vue                     # 【gRPC传输协议配置表单】(ServiceName路由指定/Multi-Mode控制)
                ├── H2.vue                       # 【HTTP/2原生传输层配置卡片】(Host数组映射与路径前缀控制)
                ├── Http.vue                     # 【HTTP传输通道载荷配置卡片】(Host与URL Path映射)
                ├── HttpUpgrade.vue              # 【HTTPUpgrade载荷协议配置卡片】(无标准WebSocket状态下的升级通道)
                ├── WebSocket.vue                # 【WebSocket双向传输通道配置卡片】(Path路由/Early Data/自定义Headers)
                └── XHTTP.vue                    # 【Xray-Core XHTTP (Split HTTP) 配置卡片】(XMUX乱序重组/并发流填充)
```
