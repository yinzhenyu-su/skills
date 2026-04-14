## ADDED Requirements

### Requirement: rclone 密钥派生 (scrypt)
系统必须能够根据用户提供的密码和盐值，通过 scrypt 算法导出 80 字节的密钥序列（Data Key, Name Key, Name Tweak）。

#### Scenario: 成功派生密钥
- **WHEN** 输入正确的 rclone 密码和默认盐值
- **THEN** 系统生成与 rclone 配置文件一致的 80 字节原始密钥

### Requirement: EME-AES 文件名解密
系统必须能够将 Base32 编码的加密文件名解密为原始明文，兼容 rclone 的 EME 模式。

#### Scenario: 解密文件名
- **WHEN** 接收到网盘端的 Base32 密文文件名
- **THEN** 系统返回正确的明文文件名（例如 `README.md`）

### Requirement: NaCl SecretBox 内容解密
系统必须支持读取 8 字节魔数和 24 字节 Nonce 后的加密分块，并使用 XSalsa20 + Poly1305 进行解密。

#### Scenario: 分块解密
- **WHEN** 给出 65552 字节的密文块（64KB 数据 + 16B MAC）
- **THEN** 系统返回解密后的 65536 字节原始明文
