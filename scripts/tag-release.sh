#!/usr/bin/env bash
# Zeus 多 module 仓库一键打 tag 脚本
#
# 用法：
#   ./scripts/tag-release.sh v0.1.0           # 打 v0.1.0 tag（主仓 + 21 plugins，默认）
#   ./scripts/tag-release.sh v0.1.0 --dry-run # 预演（只打印，不实际打 tag）
#   ./scripts/tag-release.sh v0.1.0 --main    # 只打主仓 tag
#   ./scripts/tag-release.sh v0.1.0 --plugins # 主仓 + plugins（同默认行为，显式写法）
#   ./scripts/tag-release.sh v0.1.0 --all     # 主仓 + plugins + examples（examples 通常无需打 tag）
#   ./scripts/tag-release.sh v0.1.0 --delete  # 删除指定版本的所有 tag（含未推送）
#
# Go module 命名约定：
#   主仓          github.com/go-zeus/zeus                        → tag: v0.1.0
#   子 module     github.com/go-zeus/zeus/plugins/registry/etcd → tag: plugins/registry/etcd/v0.1.0
#
# 参考：kratos / grpc-go / opentelemetry-go 等多 module 仓库的标准做法

set -euo pipefail

VERSION=""
DRY_RUN=false
SCOPE="main-plugins"  # main-plugins（默认）/ main / all
DELETE=false

usage() {
    cat <<EOF
用法: $0 <version> [选项]

参数:
  <version>      版本号，必须以 v 开头（如 v0.1.0）

选项:
  --dry-run      预演模式（只打印，不实际打 tag）
  --main         只打主仓 tag
  --plugins      打主仓 + plugins tag（同默认，显式写法）
  --all          打主仓 + plugins + examples（examples 通常无需打 tag）
  --delete       删除指定版本的所有本地 + 远程 tag（用于回滚）
  -h, --help     显示帮助

示例:
  $0 v0.1.0                    # 默认：主仓 + plugins
  $0 v0.1.0 --dry-run          # 预演
  $0 v0.1.0 --main             # 只打主仓
  $0 v0.1.0 --all              # 包含 examples
  $0 v0.1.0 --delete           # 回滚 v0.1.0 的所有 tag
EOF
    exit 0
}

# 解析参数
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dry-run)    DRY_RUN=true; shift ;;
        --main)       SCOPE="main"; shift ;;
        --plugins)    SCOPE="main-plugins"; shift ;;
        --all)        SCOPE="all"; shift ;;
        --delete)     DELETE=true; shift ;;
        -h|--help)    usage ;;
        v*)           VERSION="$1"; shift ;;
        *)            echo "错误：未知参数 $1"; usage ;;
    esac
done

if [[ -z "$VERSION" ]]; then
    echo "错误：缺少版本号"
    usage
fi

if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
    echo "错误：版本号必须符合 SemVer 格式（如 v0.1.0）"
    exit 1
fi

# 在 git 仓库根目录运行
cd "$(git rev-parse --show-toplevel)"

# 收集所有 module 路径（相对仓库根）
collect_modules() {
    local result=()
    # 主仓
    [[ "$SCOPE" == "all" || "$SCOPE" == "main" || "$SCOPE" == "main-plugins" ]] && result+=("")
    # plugins
    if [[ "$SCOPE" == "all" || "$SCOPE" == "main-plugins" ]]; then
        while IFS= read -r mod; do
            result+=("$(dirname "$mod")")
        done < <(find plugins -name go.mod -type f | sort)
    fi
    # examples
    if [[ "$SCOPE" == "all" ]]; then
        while IFS= read -r mod; do
            result+=("$(dirname "$mod")")
        done < <(find examples -maxdepth 2 -name go.mod -type f | sort)
    fi
    printf '%s\n' "${result[@]}"
}

# === Delete 模式 ===
if $DELETE; then
    echo "[del]  删除 $VERSION 的所有 tag..."
    modules=()
    while IFS= read -r line; do
        [[ -n "$line" ]] && modules+=("$line")
    done < <(collect_modules)

    for mod in "${modules[@]}"; do
        if [[ -z "$mod" ]]; then
            tag="$VERSION"
        else
            tag="$mod/$VERSION"
        fi
        # 本地删除
        if git rev-parse "$tag" >/dev/null 2>&1; then
            git tag -d "$tag"
            echo "  ✓ 删除本地 tag: $tag"
        fi
        # 远程删除
        if git ls-remote --tags origin "refs/tags/$tag" 2>/dev/null | grep -q "$tag"; then
            git push origin ":refs/tags/$tag"
            echo "  ✓ 删除远程 tag: $tag"
        fi
    done
    exit 0
fi

# === 前置检查 ===
echo "[check] 检查工作区状态..."
if [[ -n "$(git status --porcelain)" ]] && ! $DRY_RUN; then
    echo "错误：工作区有未提交的变更，请先 commit 或 stash"
    git status --short | head -20
    echo "... ($(git status --porcelain | wc -l | tr -d ' ') 个文件)"
    exit 1
fi

current_branch=$(git branch --show-current)
echo "Current branch: $current_branch"

if [[ "$current_branch" != "main" ]]; then
    echo "Warning: not on main branch (industry convention: tag on main)"
    if ! $DRY_RUN; then
        read -p "Continue? [y/N] " confirm
        [[ "$confirm" =~ ^[Yy]$ ]] || exit 1
    fi
fi

# 检查 tag 是否已存在
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "Error: tag $VERSION already exists"
    exit 1
fi

# === 主流程：批量打 tag ===
echo ""
echo "=== Prepare to tag $VERSION (scope: $SCOPE) ==="
echo ""

modules=()
while IFS= read -r line; do
    [[ -n "$line" ]] && modules+=("$line")
done < <(collect_modules)

total=${#modules[@]}
idx=0
for mod in "${modules[@]}"; do
    idx=$((idx + 1))
    if [[ -z "$mod" ]]; then
        tag="$VERSION"
        display="(主仓)"
    else
        tag="$mod/$VERSION"
        display="$mod"
    fi
    printf "  [%d/%d] %s → tag: %s\n" "$idx" "$total" "$display" "$tag"

    if $DRY_RUN; then
        echo "      (dry-run) skip"
        continue
    fi

    # 校验 module path
    if [[ -n "$mod" ]]; then
        declared=$(head -1 "$mod/go.mod" | awk '{print $2}')
        expected="github.com/go-zeus/zeus/$mod"
        if [[ "$declared" != "$expected" ]]; then
            echo "      [warn]  警告：go.mod 声明 '$declared' 与期望 '$expected' 不一致"
        fi
    fi

    git tag -a "$tag" -m "Release $tag"
done

echo ""
if $DRY_RUN; then
    echo "[ok] Dry-run 完成（未实际打 tag）"
    echo "   实际执行请去掉 --dry-run 参数"
else
    echo "[ok] 共打 $total 个 tag"
    echo ""
    echo "[push] 推送 tag 到远程："
    echo "   git push origin --tags"
    echo ""
    echo "[pkg] 用户使用方式："
    echo "   go get github.com/go-zeus/zeus@$VERSION"
    echo "   go get github.com/go-zeus/zeus/plugins/registry/etcd@$VERSION"
    echo ""
    echo "[web] pkg.go.dev 会在 10 分钟内自动索引"
fi
