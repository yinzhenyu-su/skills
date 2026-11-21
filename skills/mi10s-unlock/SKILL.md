---
name: mi10s-unlock
description: 小米 10S（thyme / M2102J2SC）解锁 Bootloader 并获取 Root 的完整流程。覆盖设备探测、HyperOS 社区资格限制的绕过、跨平台解锁（macOS/Linux 无需 Windows）、Magisk 打补丁刷入、救砖回滚。适用于用户提出「小米10S 解锁」「澎湃OS 解BL」「免答题解锁」「小米 root」「刷 Magisk」等需求；其中绑定绕过与解锁步骤同样适用于其它小米/红米机型，仅需替换 ROM 与 boot 分区。
metadata:
  openclaw:
    requires:
      bins: ["adb", "fastboot", "python3", "curl", "tar", "openssl"]
      env: []
---

# 小米 10S 解锁与 Root

一台国行小米 10S（`thyme` / `M2102J2SC`，HyperOS，Android 13）从"BL 锁定、无 root"到"解锁 + Magisk root"的完整流程。

**先读 [references/internals.md](references/internals.md) 里的风险与红线**，再动手。核心前提只有一条：

> 没有"不解锁 BL 就能拿到 root"的可靠方案。任何声称能做到的，要么是卖服务的引流，要么要求你本来就能解锁（详见 internals.md「不存在的方案」）。

所以流程永远是：**探测 → 解锁 BL（清空数据）→ 刷 Magisk 补丁的 boot → 验证**。

## 阶段 0：探测现状

```bash
scripts/probe.sh
```

输出机型 / ROM 版本 / 安全补丁 / `ro.boot.flash.locked` / 分区布局（`boot` 还是 `init_boot`）/ 是否已有 `su`。据此判断下一步：

| `flash.locked` | 含义 | 下一步 |
|---|---|---|
| `1` | BL 锁定 | 阶段 1 |
| `0` | 已解锁 | 直接跳阶段 3 |

## 阶段 1：解除"社区资格"门槛

HyperOS 机型走官方解锁要求**小米社区 5 级 + 实名 + 答题 ≥96 分**，审核通过后绑定账号再等 168 小时。若账号已具备资格，手机上直接「开发者选项 → 设备解锁状态 → 添加账号和设备」即可，跳到阶段 2。

没有资格时，可用 `scripts/bind-bypass.py` 绕过**资格校验**（绕不过 168 小时等待，也绕不过清数据）：

```bash
scripts/bind-bypass.py            # 抓包 → 解密 → 改写 → 询问后重放
scripts/bind-bypass.py --dry-run  # 只抓包和分析，不发送
```

流程：手机点一次「添加账号和设备」→ 脚本从 logcat（tag `CloudDeviceStatus`）截获绑定请求 → 把 `rom_version` 的 `V816`（澎湃）改成 `V14`（MIUI）→ 用 APK 内硬编码的 HMAC 密钥重新签名 → 从电脑重放。

判读返回码：

| code | 含义 |
|---|---|
| `0` | 绑定成功，看 Mi Unlock 里的等待时间 |
| `30001` | 设备被强制校验账号资格，这条路封了 |
| `401` | 账号凭据过期，重新登录小米账号 |
| `20086` | 设备凭据过期，重启手机 |
| `86015` | 设备签名无效 / 该报文已用过，重新抓包 |

抓包结果（含活的会话 Cookie）默认写到 `~/.local/state/mi10s-unlock/capture.json`，权限 600，可用 `MI10S_STATE` 改位置。**不要把它提交到仓库或贴到任何地方**；账号 ID 在输出里默认打码，只在需要核对时再自行比对。

## 阶段 2：解锁（macOS/Linux 亦可）

官方 MiFlashUnlock 只有 Windows 版。跨平台用 [MiUnlockTool](https://github.com/MiForge/MiUnlockTool)：

```bash
pip install miunlock
scripts/unlock-status.py     # 只做前置查询，不解锁（先确认账号/地区/绑定都对）
miunlock                     # 完整解锁：需手机处于 fastboot
```

`miunlock` 的交互里选 **浏览器登录**（账号+密码走网页），不要用扫码/免密——`unlockApi` 是高危服务，免密会话会在 `/sts` 固定返回 401。

关键返回码：

| code | 含义 |
|---|---|
| `20036` | 正常等待期，`notice` 里写明还差多少小时 |
| `30002` | 解锁环节仍要求社区授权（伪装只对绑定生效） |
| `20031` | 账号未绑定 |
| `10013` | 设备激活不足 7 天 / SIM 未激活 |
| `20045` | 账号地区与设备地区不匹配（国行机必须用国区账号） |
| `20030` | 本账号本月已取过一次 token，等次月 |

**注意**：等待期是按设备激活时间算的，老机器绑定后往往能立刻解锁，不一定要等满 168 小时——想省时间就先跑 `miunlock` 试一次，返回 `20036` 才是真的没到期（失败不消耗额度）。

解锁会清空数据，且 `fastboot oem unlock` 成功后设备立即重启进首次开机向导，USB 调试被重置。

## 阶段 3：刷 Magisk

```bash
scripts/root-magisk.sh --prep --rom <fastboot-rom.tgz> --md5 778cda43017aacde0d7a04ff85b872f8
# 按提示在手机上：Magisk → 安装 → 选择并修补一个文件 → Download/boot.img
scripts/root-magisk.sh --flash
scripts/root-magisk.sh --verify
```

要点：

- **ROM 版本必须与设备 `ro.build.version.incremental` 完全一致**，否则 boot.img 不匹配。从 [xmfirmwareupdater](https://xmfirmwareupdater.com/hyperos/thyme/) 取对应版本的 fastboot 包并核对 MD5。
- 只有 `boot_a/boot_b`、没有 `init_boot` 的机型（如 10S）打在 `boot`；有 `init_boot` 的打在 `init_boot`。脚本会自动判断。
- 补丁必须在手机上用 Magisk App 打，宿主机没有可用的 `magiskboot`。
- 线刷包同时是救砖底牌，别删。

已知可用的一组（实测）：

| 项 | 值 |
|---|---|
| ROM | `V816.0.4.0.TGACNXM`（Android 13，补丁 2024-03） |
| 线刷包 | `thyme_images_OS1.0.4.0.TGACNXM_20240605.0000.00_13.0_cn_778cda4301.tgz`（5.62 GiB） |
| MD5 | `778cda43017aacde0d7a04ff85b872f8` |
| Magisk | v31.1 |
| 内核 | `4.19.157-perf` |

## 阶段 4：验证与回滚

```bash
adb shell su -v          # 有版本号即成功
adb shell "su -c id"     # uid=0(root)
```

回滚：

```bash
# 只回滚 boot（保留数据，脚本会自己找 boot / init_boot）
scripts/root-magisk.sh --restore
# 整机恢复 + 重新上锁（会清数据）
fastboot oem lock
```

## 已知副作用

解锁后的持久状态，属于"接受或不做"的选择，不是可以修的 bug：

- `verifiedbootstate = orange`，开机第一屏有橙色 "Bootloader is unlocked" 提示
- 小米钱包、部分银行 App、Play Integrity 相关功能失效（可上 Shamiko / Zygisk 隐藏，治标）
- OTA 更新会覆盖 boot，升级后需重新打补丁
- 解锁状态不可逆地影响 TEE 相关能力（部分机型）
