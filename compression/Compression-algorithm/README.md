# 反向代理内容编码适配

本目录只存放 kwor 的业务适配代码。第三方算法源码不放在这里，而是固定保存在：

- `compression/source/klauspost/compress`
- `compression/source/brotli`
- `compression/source/snappy`

根 `go.mod` 使用 `replace` 指向这些本地源码目录，发布或迁移项目时不能只复制 Go 业务文件而遗漏 `compression/source`。

## 协商顺序

客户端先通过 `Accept-Encoding` 声明支持的编码。项目把 `q=0` 视为明确拒绝，把任意合法的 `q>0` 都视为“支持”；选择时忽略 `q` 的大小，始终按下面的固定顺序选择：

```text
zstd > s2 > snappy > br > deflate > gzip
```

项目按上述六种算法统一参与协商和优先级选择；客户端未声明某种算法时，服务端不会主动选择该算法，客户端即使给出 `zstd;q=0.00001` 也仍会优先选择 zstd。普通浏览器通常不会声明 `s2` 或 `snappy`，因此会自然回退到 Brotli、DEFLATE、Gzip 或 identity。`q` 只用于判断是否拒绝：`q=0` 不参与选择，`q>0` 不再互相比较。

其中 `br`、`deflate`、`gzip` 和 `zstd` 是 HTTP 内容编码生态中有规范定义的编码；`s2`、`snappy` 在本项目中属于明确约定的扩展编码，不代表任意浏览器或公网 HTTP 服务都支持。代理只有在请求方声明接受，或请求使用 `*` 通配符时，才会选择这两种扩展编码；如果请求完全没有 `Accept-Encoding` 字段，则项目仍保持 identity，不能把“没有声明”推断成已声明支持六种算法。解析器只接受唯一的 `q=` 参数，非法参数项不会按默认 q=1 放行；合法 q>0 的大小不参与项目排序。反向代理向配置的上游发送六种编码时，列表排列和显式递减 q 值同时表达项目固定顺序：`zstd;q=1.000, s2;q=0.999, snappy;q=0.998, br;q=0.997, deflate;q=0.996, gzip;q=0.995`。这样既兼容按 q 选择的服务器，也尽量兼容只按列表先后选择的服务器；这仍然是对目标服务器的协商偏好，不能强迫完全忽略 `Accept-Encoding` 的外部服务。

当客户端通过 `identity;q=0` 或 `*;q=0` 拒绝 identity，且没有任何可用编码时，HTTP 响应返回 `406 Not Acceptable`，不会把原文静默当作 identity 发出。

## 默认等级

默认等级按算法分开，定义在 `Compression_types.go`：

- Zstandard：项目默认等级 `8`，映射到 `klauspost/compress/zstd` 的 `SpeedBetterCompression` 档位。
- Zstandard 的 HTTP 编码器和解码器将窗口限制为不超过 `32 MiB`。用户要求的 `36 MiB` 不能直接用于 zstd 编码器，因为底层窗口必须是 2 的幂；项目取不超过 36 MiB 的最大合法值。这个窗口上限和解压输出总量上限是两个不同的限制。
- 注意：窗口大于 8 MiB 可能不符合部分浏览器或严格 HTTP zstd 实现的互操作性预期；Codex/目标服务必须以实际客户端和上游能力验证，不能把 32 MiB 视为公网通用兼容值。
- Brotli、S2、DEFLATE/Gzip：项目默认等级 `6`。S2 的等级 `4-6` 映射到 `WriterBetterCompression`；Brotli 使用 quality `6`；DEFLATE/Gzip 使用 level `6`。
- Brotli 的标准窗口使用 `lgwin=24`，最大历史窗口约 `16 MiB`；底层 Writer 会按数据增长动态使用缓冲，不在创建时预分配完整窗口。
- Snappy：使用 framing stream，没有数值等级。
- Deflate/Gzip 的格式窗口固定约 `32 KiB`，不能改为 36 MiB。
- S2 的块大小和 Snappy 的 framing 块大小不是 zstd 窗口，当前不伪造 36 MiB 窗口设置。

## 接入边界

- 普通 HTTP、HTTPS/H2、H3 请求体保持不透明，不因压缩功能自动解码、删头或重新封装；只有 DoH/DoH3 的 DNS wire message 入口解码请求体。响应是否解码、改写或重新编码由反代规则和 API passthrough 边界决定。
- 目标 HTTP/HTTPS/H2/H3 连接显式发送按项目固定顺序排列的六种 `Accept-Encoding`，关闭标准库自动 gzip，收到响应后由本目录解码；目标侧选择与本地监听侧相互独立。
- 本地 DoH/DoH3 支持压缩的 POST 请求和响应；解压后的 DNS wire message 不得超过 `65535` 字节。DoH 请求使用不支持的 `Content-Encoding` 时返回 `415`，并带上六种可接受编码。
- 不支持的请求 `Content-Encoding` 返回 `415`，并通过响应 `Accept-Encoding` 告知可接受的六种编码；同名的重复编码头按逗号拼接解析。
- 目标 DoH/DoH3 使用项目自有的纯 Go upstream 适配，因为 `dnsproxy` 内置 DoH client 会关闭 HTTP 压缩。
- UDP、TCP、DoT、DoQ 传输的是 DNS wire message，不使用 HTTP `Content-Encoding`。
- WebSocket、Range、`Content-Range`、SSE、multipart 和未知扩展编码不会被强行重新编码；未知扩展编码在客户端明确拒绝、显式空 `Accept-Encoding` 或未匹配 `*` 时返回 `406`。若客户端完全没有 `Accept-Encoding` 字段，按 RFC 9110 视为接受任意内容编码，因此允许保留无法由本项目解码的上游扩展编码。
- 同一个编码在重复的 `Accept-Encoding` 字段中出现多次时按更严格的最小 q 值处理：只要合并结果仍大于 0 就表示支持，出现 q=0 就表示拒绝，q 的正数大小不参与优先级。

## 源码规模与构建边界

当前通过 `go.mod` 的本地 `replace` 固定源码版本：`klauspost/compress v1.19.2`、`andybalholm/brotli v1.2.2`、`golang/snappy v1.0.0`。三份源码都保存在本项目中，包含各自许可证文件；业务适配代码只引用需要的包，不会把整个算法目录都链接进最终二进制。

本地源码大致规模为：klauspost/compress 125 个 Go 文件、约 1.13 MB；Brotli 81 个 Go 文件、约 2.55 MB（其中静态字典表占很大比例）；Snappy 7 个 Go 文件、约 32 KB。源码体积不等于最终可执行文件增量，最终大小应以 Linux 发布构建产物为准。

目标运行平台为 Linux amd64/arm64，代码路径保持纯 Go、`CGO_ENABLED=0`；Windows 仅作为开发环境，不把 Windows 运行测试结果当作目标平台验证。

## 定向验证

```text
go test -mod=readonly ./compression/Compression-algorithm
go test -mod=readonly ./service -run 'TestReverseProxy(.*Compression|.*SSE|.*WebSocket|.*DoH).*' -count=1
```

不要用 `go test ./service` 或 `go test ./...` 代替上述定向验证。

交叉编译只能使用 `go test -c` 或 `go build` 验证，不能在 Windows 上直接执行 `GOOS=linux` 生成的测试二进制。例如：

```text
set CGO_ENABLED=0
set GOOS=linux
set GOARCH=amd64
go test -c ./compression/Compression-algorithm
```
