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
- 启动时会输出 `run config:`，包含 `source_dir`、`output_dir`、`execute_copy`、`remove_original` 和 `ffmpeg_args`。

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

指定 `--type/-t` 后，扫描、差异报告、统计和复制都只包含选中的类型；匹配最后一个扩展名，因此 `-t gz` 可匹配 `archive.tar.gz`，点文件 `.gitignore` 不视为 `gitignore` 类型。

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
check-copy --source/-s <directory> --destination/-d <directory> [--type/-t <extension>]... [--copy/-c]
```

| 选项 | 说明 |
| --- | --- |
| `--source`, `-s` | 必填，基准源目录。 |
| `--destination`, `-d` | 必填，待比较或复制的目标目录。 |
| `--type`, `-t` | 只处理指定的最后扩展名，可重复；忽略大小写，可写 `jpg` 或 `.jpg`。省略时处理全部常规文件。 |
| `--copy`, `-c` | 执行复制；省略时仅报告差异。每个失败的选中文件拷贝 has at most 5 total attempts, including the first, with a 1-second interval。某个文件在尝试耗尽后仍失败时，命令会在写完汇总后返回非零退出状态；其他候选文件会继续处理。 |
| `--help`, `-h` | 显示帮助。 |

复制模式下，遇到失败文件时会自动重试，直到用完最多 5 次总尝试次数（包括第一次）；重试间隔为 1 秒。单个文件尝试耗尽后不会中断其余候选文件的处理，但如果仍有文件最终失败，命令会在输出汇总后返回非零退出状态。

```bash
# 预览要拷贝哪些文件，但不会实施拷贝
check-copy -s /Volumes/red/1 -d /Volumes/black/1

# 只预览 JPG 文件
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg

# 只复制 JPG 和 MP4 文件
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -t jpg -t mp4 -c

# 复制所有文件类型
check-copy -s /Volumes/red/1 -d /Volumes/black/1 -c
```
