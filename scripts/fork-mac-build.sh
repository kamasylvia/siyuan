#!/bin/bash
# Fork mac 客户端本地构建:kernel(fts5+sqlcipher) + 前端 + electron-builder arm64 DMG。
# GH macOS runner 池长期排队不可用,客户端构建以本地为准;产物随 tag 推 GitHub release。
# 用法: scripts/fork-mac-build.sh <fork-build-tag>   例: scripts/fork-mac-build.sh v3.8.8.23
# 依赖: go(brew), node, npm(取 pnpm 12.6.0), python3;网络走国内镜像,OCR 资源经 hf-mirror/gh-proxy。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-}"
if [[ -z "$TAG" ]]; then
    echo "usage: $0 <fork-build-tag>" >&2
    exit 1
fi

echo "== stamp forkBuild=$TAG"
cd "$ROOT/app"
node -e "const fs=require('fs');const p=JSON.parse(fs.readFileSync('package.json'));p.forkBuild='$TAG';fs.writeFileSync('package.json',JSON.stringify(p,null,2)+'\n')"

echo "== build kernel (darwin/arm64)"
cd "$ROOT/kernel"
CGO_ENABLED=1 GOOS=darwin GOARCH=arm64 \
    GOPROXY="https://mirrors.aliyun.com/goproxy/,https://goproxy.cn,direct" \
    go build -tags "fts5 sqlcipher" -o ../app/kernel-darwin-arm64/SiYuan-Kernel -ldflags "-s -w" .

echo "== install + build frontend"
cd "$ROOT/app"
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
npx -y pnpm@12.6.0 install --frozen-lockfile
npx -y pnpm@12.6.0 run build

echo "== prepare OCR assets (missing entries download via mirrors, sha256 verified)"
cd "$ROOT"
python3 - <<'PY'
import hashlib, json, urllib.request
from pathlib import Path

UA = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"}
root = Path.cwd()
manifest = json.loads((root / "scripts/ocr-assets.json").read_text(encoding="utf-8"))
stage = root / "app/stage/ocr"
cache = root / ".cache/ocr"

def fetch(url):
    url = url.replace("https://huggingface.co/", "https://hf-mirror.com/")
    url = url.replace("https://github.com/", "https://gh-proxy.com/https://github.com/")
    request = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(request, timeout=300) as source:
        return source.read()

for entry in manifest["models"]:
    dest = stage / "models" / entry["path"]
    if dest.is_file() and hashlib.sha256(dest.read_bytes()).hexdigest() == entry["sha256"]:
        continue
    dest.parent.mkdir(parents=True, exist_ok=True)
    data = fetch(entry["url"])
    got = hashlib.sha256(data).hexdigest()
    assert got == entry["sha256"], f"checksum mismatch {entry['path']}: {got}"
    dest.write_bytes(data)
    print("model:", entry["path"])

runtime = manifest["runtime"]["darwin-arm64"]
cache_dest = cache / runtime["url"].rsplit("/", 1)[1]
cache.mkdir(parents=True, exist_ok=True)
if not (cache_dest.is_file() and hashlib.sha256(cache_dest.read_bytes()).hexdigest() == runtime["sha256"]):
    data = fetch(runtime["url"])
    got = hashlib.sha256(data).hexdigest()
    assert got == runtime["sha256"], f"checksum mismatch runtime: {got}"
    cache_dest.write_bytes(data)
print("runtime cache ok")
PY
python3 scripts/prepare-ocr.py --runtime darwin-arm64

echo "== package arm64 dmg (electron-builder, pnpm 12 on PATH)"
cd "$ROOT/app"
PNPM12_DIR="$(mktemp -d)"
npm install --prefix "$PNPM12_DIR" pnpm@12.6.0 --no-save --silent
export PATH="$PNPM12_DIR/node_modules/.bin:$PATH"
npx electron-builder --arm64 --mac --config electron-builder-fork-darwin-arm64.yml --publish=never
rm -rf "$PNPM12_DIR"

ls -lh "$ROOT/app/build/"*.dmg
echo "== done; forkBuild=$TAG stamped in app/package.json (提交与否由发布流程决定)"
