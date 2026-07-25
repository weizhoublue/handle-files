# 文件处理

Go 二进制是主入口；`compress_mp4.py` 和 `sync_check.py` 保留为行为参考，不作为推荐运行方式。

## 构建 macOS 二进制

需要 Go（版本见 `go.mod`）。以下命令构建 Apple Silicon 和 Intel 的四个二进制文件：

```bash
make build-macos
```

输出位于 `dist/macos-arm64/` 和 `dist/macos-amd64/`，按 Mac 架构选择对应目录。

## compress-vedio

递归处理 MP4 文件。启动时同时使用可执行文件查找验证 `ffmpeg`，并运行 `ffmpeg -version`；任一检查失败都会终止。macOS 可通过 Homebrew 安装：

```bash
brew install ffmpeg
```

```text
compress-vedio --dir/-d <directory> [--execute/-x] [--yes/-y] [--ff-option/-f "<ffmpeg options>"]
```

| 选项 | 说明 |
| --- | --- |
| `--dir`, `-d` | 必填，递归扫描的目录。 |
| `--execute`, `-x` | 执行压缩；省略时仅预览，不修改文件或调用 ffmpeg。 |
| `--yes`, `-y` | 仅限执行模式；跳过每个文件的确认提示。 |
| `--ff-option`, `-f` | 传给 ffmpeg 的编码选项字符串。默认值为 `-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k`。 |
| `--help`, `-h` | 显示帮助。 |

`--yes` 必须和 `--execute` 一起使用。`--ff-option` 支持引号和反斜杠转义，且不会经由 shell 执行。

```bash
# 预览
dist/macos-arm64/compress-vedio --dir /Volumes/Data/Videos

# 无人值守执行
dist/macos-arm64/compress-vedio --dir /Volumes/Data/Videos --execute --yes
```

输出为原文件名加 `_output` 后缀。成功后删除原文件；`*_output.mp4` 会跳过；失败时保留原文件并清理不完整输出。

## check-copy

按相对路径和文件大小递归比较目录。`--copy` 会复制源端独有文件，并以源文件覆盖同路径但更小的目标文件；目标端独有文件和更大文件仅报告，不删除。

```text
check-copy --source/-s <directory> --destination/-d <directory> [--copy/-c]
```

| 选项 | 说明 |
| --- | --- |
| `--source`, `-s` | 必填，基准源目录。 |
| `--destination`, `-d` | 必填，待比较或复制的目标目录。 |
| `--copy`, `-c` | 执行复制；省略时仅报告差异。 |
| `--help`, `-h` | 显示帮助。 |

```bash
dist/macos-arm64/check-copy --source /Volumes/red/1 --destination /Volumes/black/1 --copy
```

源目录中仅大小写不同的路径属于大小写冲突组。在复制模式中，冲突组内每个源路径都会跳过，其他非冲突复制继续。全部处理完成后，程序会发出一条结构化警告，报告跳过的冲突组数和文件数。

## 输出

两个命令都向控制台输出结构化日志和每文件进度；不创建日志文件。信息和进度记录写入标准输出，验证失败和警告写入标准错误。

## Python 行为参考

`compress_mp4.py` 和 `sync_check.py` 保留以便核对旧行为。使用 Go 二进制进行新的处理工作；Python 脚本不在此 README 的运行接口范围内。
