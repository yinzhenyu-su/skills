# macOS FUSE Release-before-Write 导致 0 字节上传

## 日期
2026-05-08

## 问题
批量上传 dist 目录时，`cp -r dist/ /mnt/qrypt/` 前几个文件（version.json, index.html, favicon.ico）被上传为 0 字节文件。

## 根因
macOS FUSE 内核在 Write 完成之前就调用了 Release。Release 触发 enqueueSync → syncFile，此时 staging 文件为空（n.size=0, stagingFileSize=0），syncFile 用 PlainSize=0 调用 Sync → UploadPre 创建 32 字节 placeholder（加密空文件）→ UploadCommit + UploadFinish → 服务器上文件为 0 字节。

## 日志证据
```
16:22:50  Create: /dist/index.html
16:22:50  syncFile: snapshotSize=0 stagingFileSize=0  ← Release 在 Write 之前触发
16:22:50  Write: /dist/index.html, len=8277           ← Write 在 syncFile 之后
16:22:50  Sync: plainSize=0 encSize=32                 ← 上传了 0 字节文件
```

## 修复
在 syncFile 中添加空 staging 检查：当 `n.size == 0 && stagingFileSize == 0` 时跳过上传，等 Write 完成后 Release 会再次触发 sync。

## 注意
此 bug 在 commit 6183938 中修复过（空 staging 跳过上传），但在 commit 7c66c40（Release-only sync 重构）中保护被移除。
