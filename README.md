# 文件处理


## compress_mp4.py   对运动相机的 mp4 文件进行压缩

批量压缩指定目录下所有层级的 MP4 文件，使用 ffmpeg 进行有损压缩（H.264 + AAC），压缩后自动删除原文件。

### 依赖

- Python 3.7+
- [ffmpeg](https://ffmpeg.org/) （需在 PATH 中可用）

```bash
# macOS
brew install ffmpeg
```

### 用法

```bash
python compress_mp4.py <目录> [dry_run] [confirm]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `目录` | 要处理的根目录（递归扫描所有子目录） | 必填 |
| `dry_run` | `true` 只展示将要处理的文件，不执行压缩；`false` 真正执行 | `true` |
| `confirm` | `true` 每个文件执行前提示 y/n 确认；`false` 无人值守批量处理 | `true` |

### 示例

```bash
# 预览将要处理的文件（不做任何修改）
python compress_mp4.py /Volumes/Data/Videos

# 逐文件确认后压缩（默认开启确认）
python compress_mp4.py /Volumes/Data/Videos false

# 无人值守批量压缩，不逐文件询问
python compress_mp4.py /Volumes/Data/Videos false false
```

### 压缩参数

```
ffmpeg -i input.mp4 -c:v libx264 -crf 23 -preset slow -c:a aac -b:a 192k input_output.mp4
```

| 参数 | 说明 |
|------|------|
| `-c:v libx264` | 视频编码器：H.264 |
| `-crf 23` | 画质系数（0=无损，51=最差），23 为默认平衡值 |
| `-preset slow` | 编码速度越慢，压缩率越高 |
| `-c:a aac` | 音频编码器：AAC |
| `-b:a 192k` | 音频码率 192 kbps |

### 输出规则

- 压缩结果保存为 `<原文件名>_output.mp4`，与原文件同目录
- 压缩成功后原文件被删除
- 文件名已含 `_output` 后缀的文件会被自动跳过（幂等，可重复运行）
- ffmpeg 压缩失败时，不删除原文件，并清理不完整的输出文件

### 重入安全

脚本可重复运行：已压缩的 `*_output.mp4` 文件不会被再次处理，中途中断后重新运行会继续处理未完成的文件。


---

## sync_check.py   比较两个目录的文件差异并可选择性同步

递归比较源目录和目标目录下所有层级的文件，以**文件大小**为判断依据，识别三类差异，并可将需要补齐的文件从源目录复制到目标目录。

### 依赖

- Python 3.7+（无其他外部依赖）

### 用法

```bash
python sync_check.py <源目录> <目标目录> [copy]
```

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `源目录` | 基准目录（递归扫描所有子目录） | 必填 |
| `目标目录` | 待对比的目标目录 | 必填 |
| `copy` | `false` 仅报告差异，不做任何修改；`true` 将需要补齐的文件从源复制到目标 | `false` |

### 示例

```bash
# 仅报告差异（不修改任何文件）
python sync_check.py /Volumes/red/1 /Volumes/black/1

# 报告差异并执行复制
python sync_check.py /Volumes/red/1 /Volumes/black/1 true
```

### 差异类型说明

| 标签 | 含义 | copy=true 时的处理 |
|------|------|-------------------|
| `[MISS]` | 源有、目标无 | 复制到目标 |
| `[DIFF]` src 更大 | 两边都有，但源文件更大 | 用源文件覆盖目标 |
| `[DIFF]` dst 更大 | 两边都有，但目标文件更大 | 跳过（目标可能更完整） |
| `[EXTRA]` | 目标有、源无 | 仅报告，不删除 |

### 日志标签

| 标签 | 含义 |
|------|------|
| `[INFO]` | 启动信息、路径、运行模式 |
| `[SCAN]` | 扫描进度 |
| `[MISS]` | 源有目标无的文件（显示两边路径） |
| `[EXTRA]` | 目标有源无的文件（显示两边路径） |
| `[DIFF]` | 大小不一致的文件（显示两边路径和大小） |
| `[COPY]` | 正在复制的文件 |
| `[WARN]` | 无法读写的文件（跳过继续）或大小写冲突警告 |
| `[SKIP]` | copy=false 时提示有文件需要复制 |

### 大小写冲突警告

若源目录中存在仅大小写不同的同名文件（如 `IMG_5031.MOV` 与 `IMG_5031.mov`），脚本会在扫描后立即警告：

```
[WARN] Source has 2 case conflict(s) — these files differ only in
       case and cannot both exist on a case-insensitive destination filesystem:
  [WARN]   '/Volumes/red/1/IMG_5031.MOV' (50,073,416 B)  vs  '/Volumes/red/1/IMG_5031.mov' (10,574,727 B)
```

这种情况发生在源卷为大小写敏感（如 exFAT、Linux 格式）而目标卷为大小写不敏感（如 macOS 默认 HFS+/APFS）时，无法将两个文件同时完整复制到目标卷。

### 汇总输出示例

```
==================================================
Scan complete
  Source files      : 30296
  Destination files : 30296

Differences:
  Missing  (src only)       : 2
  Extra    (dst only)       : 0
  Size diff, src larger     : 1  (will copy)
  Size diff, dst larger     : 1  (skip)
  Consistent                : 30293

  Copied              : 3  (2 missing + 1 size diff src larger)
```
