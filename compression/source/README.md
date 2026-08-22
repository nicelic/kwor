# 第三方压缩源码固定目录

这些目录是项目发布所需的本地第三方源码快照。根目录 `go.mod` 使用 `replace` 指向它们，因此发布、备份或迁移项目时必须连同整个 `compression/source` 一起保留。

| 目录 | 根模块版本 | 项目实际使用的包 | 说明 |
| --- | --- | --- | --- |
| `klauspost/compress` | `github.com/klauspost/compress v1.19.2` | `zstd`、`s2`、`flate`、`zlib`、`gzip` | 同一个上游模块的内部包有共享依赖，不能只复制单个 `zstd` 子目录。 |
| `brotli` | `github.com/andybalholm/brotli v1.2.2` | 根包的 reader/writer | 纯 Go 实现，包含静态字典表。 |
| `snappy` | `github.com/golang/snappy v1.0.0` | 根包的 framed reader/writer | 纯 Go 实现，同时包含 amd64/arm64 汇编文件。 |

## 维护规则

- 业务适配代码放在 `compression/Compression-algorithm/`，不要直接修改第三方源码来改变协商或等级策略。
- 更新第三方源码时，必须同步检查根 `go.mod` 的版本声明、`replace` 路径、许可证文件和 Linux amd64/arm64 的纯 Go 构建。
- `klauspost/compress` 的源码目录包含项目暂未使用的 `huff0`、`fse`、`zip` 等包；它们是该模块的内部依赖或完整源码快照，不代表都会进入最终二进制。
- 不要删除 `LICENSE`、`go.mod`、`go.sum`（如果该快照包含）或 amd64/arm64 的 `.s` 文件。
