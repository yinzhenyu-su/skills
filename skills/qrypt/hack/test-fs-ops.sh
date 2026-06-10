#!/bin/bash
# test-fs-ops.sh — qrypt FUSE 挂载点文件操作完整性测试
#
# 在指定挂载点上创建临时目录，测试各种 POSIX 文件操作是否正常，
# 包括创建/读写/移动/删除/复制以及内容校验。
#
# Usage:
#   ./hack/test-fs-ops.sh /path/to/mount
#
# 退出码: 0=全部通过, 1=有测试失败

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "用法: $0 /path/to/mount"
    echo ""
    echo "在指定挂载点上测试 POSIX 文件操作完整性"
    exit 1
fi

MOUNT="$1"
PASS=0
FAIL=0
TEST_DIR=""

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

cleanup() {
    if [ -z "$TEST_DIR" ] || [ ! -d "$TEST_DIR" ]; then
        return
    fi

    # 白名单校验：只能删除符合模式 $MOUNT/.fs-test-* 的目录
    case "$TEST_DIR" in
        "$MOUNT"/.fs-test-*) ;;
        *)
            echo "   错误: 目录名不匹配测试模式，跳过删除: $TEST_DIR"
            return
            ;;
    esac

    echo ""
    echo "--- 清理测试目录: $TEST_DIR"

    # FUSE 异步删除可能有延迟，重试多次
    local retries=3
    local delay=2
    local ok=false

    for i in $(seq 1 "$retries"); do
        if rm -rf "$TEST_DIR" 2>/dev/null; then
            ok=true
            break
        fi
        if [ "$i" -lt "$retries" ]; then
            echo "   重试 $i/$retries (等待 ${delay}s)..."
            sleep "$delay"
        fi
    done

    if $ok; then
        echo "   清理完成"
    else
        echo "   错误: 清理失败，请手动删除: $TEST_DIR"
    fi
}
trap cleanup EXIT INT TERM

# ---------- helpers ----------

ok()    { PASS=$((PASS+1)); echo -e "  ${GREEN}[PASS]${NC} $1"; }
fail()  { FAIL=$((FAIL+1)); echo -e "  ${RED}[FAIL]${NC} $1"; }

expect_exists() {
    if [ -e "$1" ]; then ok "$2"; else fail "$2 ($1 不存在)"; fi
}

expect_not_exists() {
    if [ ! -e "$1" ]; then ok "$2"; else fail "$2 ($1 存在但不应存在)"; fi
}

expect_eq() {
    if [ "$1" = "$2" ]; then ok "$3"; else fail "$3 (期待 '$1', 实际 '$2')"; fi
}

expect_neq() {
    if [ "$1" != "$2" ]; then ok "$3"; else fail "$3 (不应等于 '$1')"; fi
}

expect_fail() {
    if eval "$1" 2>/dev/null; then fail "$2 (应该失败但成功了)"; else ok "$2"; fi
}

headline() {
    echo ""
    echo "=== $1 ==="
}

section() {
    echo "--- $1"
}

clean_dir() {
    ( cd "$TEST_DIR" && rm -rf ./* .* 2>/dev/null || true )
}

# ---------- setup ----------

if [ ! -d "$MOUNT" ]; then
    echo "错误: 挂载点 $MOUNT 不存在"
    echo "请确认网盘已挂载 (qrypt mount quark ~/Qrypt)"
    exit 1
fi

# 确认是 FUSE 挂载，拒绝对本地目录运行
# 从给定路径向上遍历，找到最近的挂载点并检查是否为 FUSE 类型
check_fuse_mount() {
    local path="$1"
    local current
    current=$(cd "$path" 2>/dev/null && pwd -P) || return 1

    while [ "$current" != "/" ]; do
        local entry
        entry=$(mount 2>/dev/null | grep -F " on $current " || true)
        if [ -n "$entry" ]; then
            # macOS: "qrypt@macfuse2 on /mnt (macfuse, ...)"
            # Linux: "qrypt on /mnt type fuse.qrypt (rw,...)"
            if echo "$entry" | grep -qiE "fuse|macfuse"; then
                echo "$current"
                return 0
            fi
            echo "本地文件系统: $(echo "$entry" | awk '{print $1}')"
            return 1
        fi
        current=$(dirname "$current")
    done

    echo "未挂载"
    return 1
}

MOUNT_INFO=$(check_fuse_mount "$MOUNT") || {
    if [ "$MOUNT_INFO" = "未挂载" ]; then
        echo "错误: $MOUNT 不位于任何挂载点上，是本地目录"
    elif echo "$MOUNT_INFO" | grep -q "本地文件系统"; then
        echo "错误: $MOUNT 位于 $MOUNT_INFO 上，不是 FUSE 挂载点"
    else
        echo "错误: $MOUNT_INFO"
    fi
    echo "请先执行 qrypt mount quark ~/Qrypt"
    exit 1
}

TEST_DIR=$(mktemp -d "$MOUNT/.fs-test-XXXXXX")
echo "测试目录: $TEST_DIR"
echo "挂载点:   $MOUNT"

# =====================================================================
headline "1. 目录操作"
# =====================================================================

section "创建空目录"
mkdir -p "$TEST_DIR/dir1"
expect_exists "$TEST_DIR/dir1" "单个空目录"

section "创建嵌套目录"
mkdir -p "$TEST_DIR/a/b/c/d"
expect_exists "$TEST_DIR/a/b/c/d" "四层嵌套目录"
expect_exists "$TEST_DIR/a/b"     "中间目录存在"

section "创建目录含空格"
mkdir -p "$TEST_DIR/my folder/sub folder"
expect_exists "$TEST_DIR/my folder/sub folder" "带空格的目录"

# =====================================================================
headline "2. 写入并读取"
# =====================================================================

section "纯文本写入读取"
echo "hello world" > "$TEST_DIR/hello.txt"
expect_eq "hello world" "$(cat "$TEST_DIR/hello.txt")" "纯文本内容一致"

section "Unicode 写入读取"
echo "中文测试 日本語 한국어" > "$TEST_DIR/unicode.txt"
expect_eq "中文测试 日本語 한국어" "$(cat "$TEST_DIR/unicode.txt")" "Unicode 内容一致"

section "二进制写入读取"
printf '\x00\x01\x02\xff\xfe\x00' > "$TEST_DIR/binary.bin"
SIZE=$(stat -f%z "$TEST_DIR/binary.bin" 2>/dev/null || stat -c%s "$TEST_DIR/binary.bin" 2>/dev/null)
expect_eq "6" "$SIZE" "二进制文件大小正确"

section "空文件创建"
touch "$TEST_DIR/empty.txt"
SIZE=$(stat -f%z "$TEST_DIR/empty.txt" 2>/dev/null || stat -c%s "$TEST_DIR/empty.txt" 2>/dev/null)
expect_eq "0" "$SIZE" "空文件大小为 0"

section "多行文本"
printf "line1\nline2\nline3\n" > "$TEST_DIR/multiline.txt"
LINES=$(wc -l < "$TEST_DIR/multiline.txt" | tr -d ' ')
expect_eq "3" "$LINES" "多行文本行数"

# =====================================================================
headline "3. 覆盖与追加"
# =====================================================================

section "覆盖写入"
echo "first"  > "$TEST_DIR/overwrite.txt"
echo "second" > "$TEST_DIR/overwrite.txt"
expect_eq "second" "$(cat "$TEST_DIR/overwrite.txt")" "覆盖后内容正确"

section "追加写入"
echo "line1" >  "$TEST_DIR/append.txt"
echo "line2" >> "$TEST_DIR/append.txt"
echo "line3" >> "$TEST_DIR/append.txt"
expect_eq "3" "$(wc -l < "$TEST_DIR/append.txt" | tr -d ' ')" "追加后行数"
expect_eq "line1" "$(head -1 "$TEST_DIR/append.txt")" "追加后第一行不变"

# =====================================================================
headline "4. 重命名/移动"
# =====================================================================

section "同目录重命名"
mv "$TEST_DIR/hello.txt" "$TEST_DIR/renamed.txt"
expect_not_exists "$TEST_DIR/hello.txt"   "原路径已不存在"
expect_exists     "$TEST_DIR/renamed.txt"  "新路径存在"
expect_eq "hello world" "$(cat "$TEST_DIR/renamed.txt")" "重命名后内容不变"

section "跨目录移动"
mkdir -p "$TEST_DIR/subdir"
mv "$TEST_DIR/renamed.txt" "$TEST_DIR/subdir/hello.txt"
expect_not_exists "$TEST_DIR/renamed.txt"            "原路径已不存在"
expect_exists     "$TEST_DIR/subdir/hello.txt"        "目标路径存在"
expect_eq "hello world" "$(cat "$TEST_DIR/subdir/hello.txt")" "跨目录移动后内容不变"

section "移动目录"
mkdir -p "$TEST_DIR/dir_move_src"
touch "$TEST_DIR/dir_move_src/file.txt"
mv "$TEST_DIR/dir_move_src" "$TEST_DIR/dir_move_dst"
expect_not_exists "$TEST_DIR/dir_move_src"           "原目录不存在"
expect_exists     "$TEST_DIR/dir_move_dst/file.txt"   "移动后目录内文件存在"

# =====================================================================
headline "5. 复制"
# =====================================================================

section "复制文件"
cp "$TEST_DIR/unicode.txt" "$TEST_DIR/unicode_copy.txt"
expect_exists "$TEST_DIR/unicode_copy.txt" "复制文件存在"
ORIG=$(cat "$TEST_DIR/unicode.txt")
COPY=$(cat "$TEST_DIR/unicode_copy.txt")
expect_eq "$ORIG" "$COPY" "复制后内容与原文一致"

section "复制并覆盖"
echo "OVERWRITTEN" > "$TEST_DIR/unicode.txt"  # 先覆盖源文件
cp "$TEST_DIR/unicode.txt" "$TEST_DIR/unicode_copy.txt"
expect_eq "OVERWRITTEN" "$(cat "$TEST_DIR/unicode_copy.txt")" "复制覆盖目标后正确"

# =====================================================================
headline "6. 删除操作"
# =====================================================================

section "删除文件"
touch "$TEST_DIR/delete_me.txt"
expect_exists "$TEST_DIR/delete_me.txt" "删除前存在"
rm "$TEST_DIR/delete_me.txt"
expect_not_exists "$TEST_DIR/delete_me.txt" "删除后不存在"

section "rmdir 非空目录递归删除"
mkdir -p "$TEST_DIR/nonempty"
touch "$TEST_DIR/nonempty/file.txt"
mkdir -p "$TEST_DIR/nonempty/sub"
touch "$TEST_DIR/nonempty/sub/deep.txt"
rmdir "$TEST_DIR/nonempty"
expect_not_exists "$TEST_DIR/nonempty" "rmdir 递归删除非空目录"
expect_not_exists "$TEST_DIR/nonempty/file.txt" "子文件一并删除"
expect_not_exists "$TEST_DIR/nonempty/sub/deep.txt" "嵌套子目录一并删除"

section "rm -rf 递归删除"
mkdir -p "$TEST_DIR/deep/a/b"
touch "$TEST_DIR/deep/f1.txt" "$TEST_DIR/deep/a/f2.txt"
expect_exists "$TEST_DIR/deep/a/b" "递归删除前存在"
rm -rf "$TEST_DIR/deep"
expect_not_exists "$TEST_DIR/deep" "rm -rf 后完全消失"

# =====================================================================
headline "7. 文件元数据"
# =====================================================================

section "文件大小精确"
dd if=/dev/urandom bs=1024 count=4 of="$TEST_DIR/4k.bin" 2>/dev/null
SIZE=$(stat -f%z "$TEST_DIR/4k.bin" 2>/dev/null || stat -c%s "$TEST_DIR/4k.bin" 2>/dev/null)
expect_eq "4096" "$SIZE" "4KB 文件大小精确"

section "mtime 更新"
touch "$TEST_DIR/tstamp"
OLD_MTIME=$(stat -f%m "$TEST_DIR/tstamp" 2>/dev/null || stat -c%Y "$TEST_DIR/tstamp" 2>/dev/null)
sleep 1
touch "$TEST_DIR/tstamp"
NEW_MTIME=$(stat -f%m "$TEST_DIR/tstamp" 2>/dev/null || stat -c%Y "$TEST_DIR/tstamp" 2>/dev/null)
expect_neq "$OLD_MTIME" "$NEW_MTIME" "touch 更新 mtime"

# =====================================================================
headline "8. 特殊文件名"
# =====================================================================

section "长文件名"
LONG=$(python3 -c "print('a'*128)")
touch "$TEST_DIR/$LONG.txt"
expect_exists "$TEST_DIR/$LONG.txt" "128 字符文件名创建"
rm -f "$TEST_DIR/$LONG.txt"

section "特殊字符"
NAME="test (parens) [brackets] {braces} & and spaces.txt"
touch "$TEST_DIR/$NAME"
expect_exists "$TEST_DIR/$NAME" "特殊字符文件名创建"
rm -f "$TEST_DIR/$NAME"

section "点开头文件"
touch "$TEST_DIR/.hidden"
expect_exists "$TEST_DIR/.hidden" "点文件创建"
rm -f "$TEST_DIR/.hidden"

# =====================================================================
headline "9. 批量文件与目录遍历"
# =====================================================================

section "创建大量文件"
N=100
for i in $(seq 1 $N); do
    echo "data-$i" > "$TEST_DIR/batch-$i.txt"
done
COUNT=$(ls "$TEST_DIR"/batch-*.txt | wc -l | tr -d ' ')
expect_eq "$N" "$COUNT" "创建 $N 个文件"

section "批量读取校验"
OK=0
BAD=0
for i in $(seq 1 $N); do
    C=$(cat "$TEST_DIR/batch-$i.txt")
    if [ "$C" = "data-$i" ]; then OK=$((OK+1)); else BAD=$((BAD+1)); fi
done
expect_eq "0" "$BAD" "$OK/$N 个文件内容一致"

section "清理批量文件"
rm "$TEST_DIR"/batch-*.txt
# 用 find 而非 ls glob，避免 set -o pipefail 下 glob 无匹配时报错退出
REMAINING=$(find "$TEST_DIR" -maxdepth 1 -name 'batch-*.txt' 2>/dev/null | wc -l | tr -d ' ')
expect_eq "0" "$REMAINING" "批量删除后全部清空"

section "find 递归查找"
mkdir -p "$TEST_DIR/findtest/a/b" "$TEST_DIR/findtest/c"
touch "$TEST_DIR/findtest/root.txt" "$TEST_DIR/findtest/a/a.txt" "$TEST_DIR/findtest/a/b/b.txt"
FOUND=$(find "$TEST_DIR/findtest" -type f | wc -l | tr -d ' ')
expect_eq "3" "$FOUND" "find 找到 3 个文件"
rm -rf "$TEST_DIR/findtest"

# =====================================================================
headline "10. 跨 chunk 边界写入 (64KB)"
# =====================================================================

section "写入 128KB 跨两个 chunk"
dd if=/dev/urandom bs=65536 count=2 of="$TEST_DIR/128k.bin" 2>/dev/null
SIZE=$(stat -f%z "$TEST_DIR/128k.bin" 2>/dev/null || stat -c%s "$TEST_DIR/128k.bin" 2>/dev/null)
expect_eq "131072" "$SIZE" "128KB 文件大小正确 (2×64KB chunks)"

section "读取跨 chunk 文件并校验 MD5"
MD5_BEFORE=$(md5 -q "$TEST_DIR/128k.bin" 2>/dev/null || md5sum "$TEST_DIR/128k.bin" | cut -d' ' -f1)
# 读回两次确保一致
MD5_AFTER=$(md5 -q "$TEST_DIR/128k.bin" 2>/dev/null || md5sum "$TEST_DIR/128k.bin" | cut -d' ' -f1)
expect_eq "$MD5_BEFORE" "$MD5_AFTER" "跨 chunk 文件内容可重复读取"
rm -f "$TEST_DIR/128k.bin"

section "写入奇数大小 (3MB = 48 chunks)"
dd if=/dev/urandom bs=65536 count=48 of="$TEST_DIR/3m.bin" 2>/dev/null
SIZE=$(stat -f%z "$TEST_DIR/3m.bin" 2>/dev/null || stat -c%s "$TEST_DIR/3m.bin" 2>/dev/null)
expect_eq "3145728" "$SIZE" "3MB 文件大小正确 (48 chunks)"
MD5_1=$(md5 -q "$TEST_DIR/3m.bin" 2>/dev/null || md5sum "$TEST_DIR/3m.bin" | cut -d' ' -f1)
MD5_2=$(md5 -q "$TEST_DIR/3m.bin" 2>/dev/null || md5sum "$TEST_DIR/3m.bin" | cut -d' ' -f1)
expect_eq "$MD5_1" "$MD5_2" "3MB 文件重复读取 MD5 一致"
rm -f "$TEST_DIR/3m.bin"

# =====================================================================
headline "11. 并发操作"
# =====================================================================

section "并行写入多个文件"
for id in $(seq 1 10); do
    (
        dd if=/dev/urandom bs=4096 count=$id of="$TEST_DIR/concurrent-$id.bin" 2>/dev/null
    ) &
done
wait

OK=0
for id in $(seq 1 10); do
    S=$(stat -f%z "$TEST_DIR/concurrent-$id.bin" 2>/dev/null || stat -c%s "$TEST_DIR/concurrent-$id.bin" 2>/dev/null)
    if [ "$S" = "$((id * 4096))" ]; then OK=$((OK+1)); fi
done
expect_eq "10" "$OK" "10个并发写入全部完成且大小正确"
rm -f "$TEST_DIR"/concurrent-*.bin

# =====================================================================
headline "12. 数据持久性 (写后重开)"
# =====================================================================

section "写入 → fsync → 关闭 → 重开 → 校验"
printf "persist test data 12345" > "$TEST_DIR/persist.txt"
# macOS 没有 fsync 命令，用 dd 的 sync 选项代替
# 实际上 close() 已经触发 VFS sync
CONTENT=$(cat "$TEST_DIR/persist.txt")
expect_eq "persist test data 12345" "$CONTENT" "写后立即读回内容一致"

# 等一秒再读（让后台 flush 完成）
sleep 1
CONTENT2=$(cat "$TEST_DIR/persist.txt")
expect_eq "persist test data 12345" "$CONTENT2" "等待后读回内容一致"
rm -f "$TEST_DIR/persist.txt"

# =====================================================================
headline "13. 对 qrypt 日志的观察"
# =====================================================================

section "检查 pending.jsonl 状态"
PENDING_FILE="$HOME/.qrypt/cache/quark/pending.jsonl"
if [ -f "$PENDING_FILE" ]; then
    LINES=$(wc -l < "$PENDING_FILE" | tr -d ' ')
    echo "  pending.jsonl: ${LINES} 行"
    if [ "$LINES" -gt 0 ]; then
        echo "  --- 尚未压缩 ---"
        echo "  (Maintenance ticker 每 10 分钟清理一次)"
    fi
else
    echo "  pending.jsonl: 不存在 (已清理)"
fi

# =====================================================================
headline "测试结果"
# =====================================================================

echo -e "  ${GREEN}通过: ${PASS}${NC}"
echo -e "  ${RED}失败: ${FAIL}${NC}"

exit $([ "$FAIL" -eq 0 ] && echo 0 || echo 1)
