## ADDED Requirements

### Requirement: Cipher 接口抽象
系统必须提供 `Cipher` 接口，封装所有加密操作，以便支持未来切换或拓展加密方案。

```go
type Cipher interface {
    EncryptSegment(plaintext string) string
    DecryptSegment(encrypted string) (string, error)
    EncryptBlock(plaintext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error)
    DecryptBlock(ciphertext []byte, blockIndex uint64, fileNonce [24]byte) ([]byte, error)
    EncryptedSize(size int64) int64
    DecryptedSize(size int64) (int64, error)
    GenerateRandomNonce() ([24]byte, error)
}
```

### Requirement: rclone 密钥派生 (scrypt)
系统必须能够根据用户提供的密码和盐值，通过 scrypt(N=16384, r=8, p=1) 算法导出 80 字节的密钥序列（Data Key[32], Name Key[32], Name Tweak[16]）。

#### Scenario: 成功派生密钥
- **WHEN** 输入正确的 rclone 密码和默认盐值
- **THEN** 系统生成与 rclone 配置文件一致的 80 字节原始密钥

### Requirement: EME-AES 文件名加密/解密
系统必须支持对文件名进行加密和解密，兼容 rclone 的 EME 模式。

#### Scenario: 加密文件名
- **WHEN** 用户创建名为 `README.md` 的文件
- **THEN** 系统使用 Name Key 和 Name Tweak 进行 EME-AES 加密
- **AND** 加密结果使用 Lowercase Base32 (无填充) 编码
- **AND** 该加密名用作夸克网盘的远程文件名

#### Scenario: 解密文件名
- **WHEN** 接收到网盘端的 Base32 密文文件名
- **THEN** 系统返回正确的明文文件名（例如 `README.md`）

#### Scenario: 冲突后缀处理
- **WHEN** 远程文件名包含夸克网盘自动追加的 `(N)` 冲突后缀
- **THEN** 系统使用正则 `^(.*?)\s*\(\d+\)$` 去除后缀后再进行解密

### Requirement: NaCl SecretBox 内容加密/解密
系统必须使用 XSalsa20-Poly1305 分块加解密文件内容，兼容 rclone crypt 的 `NaCl Secretbox` 方案。

#### Scenario: 文件结构
- **WHEN** 加密文件被创建
- **THEN** 文件头部包含 `RCLONE\x00\x00` (8B 魔数) + 24B File Nonce
- **AND** 后续每块为 16B MAC + 64KB 密文（由 64KB 明文加密得到）

#### Scenario: 分块解密
- **WHEN** 给出 65552 字节的密文块（64KB 数据 + 16B MAC）
- **THEN** 系统使用 File Nonce + 块索引派生块级 Nonce（累加算法）
- **AND** 使用 `secretbox.Open` 验证 MAC 并解密
- **AND** 返回解密后的 65536 字节原始明文

#### Scenario: 分块加密
- **WHEN** 给出 65536 字节的明文块
- **THEN** 系统使用 `secretbox.Seal` 加密
- **AND** 返回加密后的 16B MAC + 64KB 密文
