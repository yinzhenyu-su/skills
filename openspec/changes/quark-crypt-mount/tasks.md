## 1. 项目基础与 rclone 算法移植

- [x] 1.1 初始化 Go 模块和项目结构 (`cmd/`, `internal/`)
- [x] 1.2 引入核心依赖: `cgofuse`, `cobra`, `sqlite`
- [ ] 1.3 引入 `golang.org/x/crypto/scrypt` 和 `nacl/secretbox`
- [ ] 1.4 实现 rclone 兼容的 scrypt 密钥派生
- [ ] 1.5 实现 rclone 兼容的 EME-AES 文件名加解密逻辑
- [ ] 1.6 实现 rclone 兼容的 NaCl 分块解密逻辑

## 2. 夸克网盘驱动增强

- [x] 2.1 实现基于 Cookie/Token 的夸克网盘 API 登录认证
- [x] 2.2 实现网盘文件目录树的列举与元数据解析
- [ ] 2.3 实现 `PathResolver`：支持将人类可读路径（如 `/A/B`）递归解析为 `fid`
- [x] 2.4 实现基于 HTTP Range 头的分块并行下载逻辑

## 3. 本地 LRU 缓存系统

- [x] 3.1 初始化 SQLite 数据库，用于存储分块元数据和 LRU 访问时间
- [ ] 3.2 增加 `name_cache` 表，存储已解密的文件名映射，加速 `ls`
- [x] 3.3 实现本地磁盘分块文件的存取与管理逻辑
- [x] 3.4 实现基于高低水位线的后台 LRU 缓存清理机制

## 4. FUSE 挂载与 VFS 深度集成

- [x] 4.1 实现基于 `cgofuse` 的核心文件系统接口 (Open, Read, Readdir)
- [x] 4.2 在 VFS 访问层添加 macOS 特有隐藏文件（.DS_Store 等）的过滤逻辑
- [ ] 4.3 修改 VFS 读取逻辑：读取前解析 32 字节 rclone 文件头，处理 Nonce 累加
- [ ] 4.4 修改 `Readdir` 逻辑：结合 `rclone-crypt` 和 `name_cache` 进行文件名实时解密

## 5. 验证与文档

- [ ] 5.1 在 macOS 上挂载 rclone 已加密目录并验证文件可见性
- [ ] 5.2 进行 4K 视频大文件流式播放压力测试
- [x] 5.3 更新 `README.md`，添加 rclone 密钥配置说明
