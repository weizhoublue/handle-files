# 文件处理


## 构建 macOS 二进制

需要 Go（版本见 `go.mod`）。以下命令构建 Apple Silicon 和 Intel 的四个二进制文件：

```bash
make build-macos
```

输出位于 `dist/macos-arm64/` 和 `dist/macos-amd64/`，按 Mac 架构选择对应目录。

## compress-vedio

递归压缩指定目录下的所有 MP4 文件
- 输出新文件名 = 原文件名 + `_output` 后缀。
- 未指定目标目录时，输出写回原文件所在目录。
- 指定 `--dest/-d` 时，输出保留源目录的相对层级；命令会创建缺失的嵌套输出目录，但目标根目录必须预先存在。
- 成功后默认删除原文件；`--remove false` 保留原文件。
- `*_output.mp4` 会跳过；失败时保留原文件并清理不完整输出。
- 执行压缩时会直接显示 FFmpeg 原始 stdout/stderr。
- 未加 `--execute/-x` 时仅预览输入/输出路径，不写入文件。

安装：
```bash
brew install ffmpeg

# ARCH="amd64"
ARCH="arm64"
curl -LO https://github.com/weizhoublue/handle-files/releases/latest/download/compress-vedio-macos-${ARCH}
chmod +x compress-vedio-macos-${ARCH}
mv compress-vedio-macos-${ARCH} compress-vedio
sudo rm -f /usr/local/bin/compress-vedio
sudo mv compress-vedio  /usr/local/bin/

```

```text
compress-vedio --source/-s <directory> [--dest/-d <directory>] [--remove/-r <true|false>] [--execute/-x] [--ff-option/-f "<ffmpeg options>"]
```

| 选项 | 说明 |
| --- | --- |
| `--source`, `-s` | 必填，递归扫描的源目录。 |
| `--dest`, `-d` | 可选，输出根目录；必须已经存在。 |
| `--remove`, `-r` | 是否在成功后删除源文件；默认值 `true`。 |
| `--execute`, `-x` | 直接压缩所有发现的文件；省略时仅预览，不修改文件。 |
| `--ff-option`, `-f` | 传给 ffmpeg 的编码选项字符串。默认值为 `-c:v libx264 -crf 26 -preset slow -c:a aac -b:a 192k`。 |
| `--help`, `-h` | 显示帮助。 |

未指定 `--dest/-d` 时，输出文件与源文件同目录。指定 `--dest/-d` 时，源目录内的相对路径会保留到目标目录下。`--execute/-x` 直接压缩所有发现的文件。`--ff-option` 支持引号和反斜杠转义，且不会经由 shell 执行。

```bash
# 仅预览输入与输出路径，不会写入文件
compress-vedio --source /Volumes/Data/Videos

# 直接在源目录旁生成 *_output.mp4，并在成功后删除源文件
compress-vedio --source /Volumes/Data/Videos --execute

# 写入已有目标目录，保留源目录相对层级，并保留源文件
compress-vedio --source /Volumes/Data/Videos --dest /Volumes/Data/Archive --remove false --execute
```


## check-copy

比较两个目录在下的文件，并实施拷贝
- `--copy` 选项会复制 源目录下的独有文件 到目的目录，并以 源文件 覆盖同路径但更小的目标文件；目标目录下的独有文件、更大文件，仅报告，不删除。

源目录中仅大小写不同的路径属于大小写冲突组。
- 在复制模式中，冲突组内每个源路径都会跳过，其他非冲突复制继续。
- 全部处理完成后，程序会发出一条结构化警告，报告跳过的冲突组数和文件数。

```bash
# ARCH="amd64"
ARCH="arm64"
curl -LO https://github.com/weizhoublue/handle-files/releases/latest/download/check-copy-macos-${ARCH}
chmod +x check-copy-macos-${ARCH}
mv check-copy-macos-${ARCH} check-copy
sudo rm -f /usr/local/bin/check-copy
sudo mv check-copy  /usr/local/bin/
```

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
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -c
```
