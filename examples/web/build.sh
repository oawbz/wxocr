#!/usr/bin/env bash
set -euo pipefail

WEB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$WEB_DIR/../.." && pwd)"
command -v go >/dev/null 2>&1 || { echo '错误：请先安装 Go 1.25 或以上版本。' >&2; exit 1; }
HOST_OS="$(go env GOHOSTOS)"
HOST_ARCH="$(go env GOHOSTARCH)"
case "$HOST_OS" in darwin) DEFAULT_OS=1;;linux) DEFAULT_OS=2;;*) echo '错误：仅支持 macOS / Linux。' >&2;exit 1;;esac
case "$HOST_ARCH" in arm64) DEFAULT_ARCH=1;;amd64) DEFAULT_ARCH=2;;*) echo '错误：仅支持 arm64 / amd64。' >&2;exit 1;;esac
printf '选择平台：1) macOS  2) Linux\n'
read -r -p "平台 [$DEFAULT_OS]：" choice || choice=''
choice="${choice:-$DEFAULT_OS}"
case "$choice" in 1|mac|macos|darwin) TARGET_OS=darwin;;2|linux) TARGET_OS=linux;;*) echo '错误：无效平台。' >&2;exit 1;;esac
printf '选择架构：1) arm64  2) amd64\n'
read -r -p "架构 [$DEFAULT_ARCH]：" choice || choice=''
choice="${choice:-$DEFAULT_ARCH}"
case "$choice" in 1|arm64|aarch64) TARGET_ARCH=arm64;;2|amd64|x86_64) TARGET_ARCH=amd64;;*) echo '错误：无效架构。' >&2;exit 1;;esac
LIB_NAME=libonnxruntime.so
[[ "$TARGET_OS" != darwin ]] || LIB_NAME=libonnxruntime.dylib
# Supply target-specific libraries without accidentally packaging the host library.
LIB_SOURCE="${ORT_LIBRARY:-$ROOT/runtime/$TARGET_OS-$TARGET_ARCH/$LIB_NAME}"
if [[ ! -f "$LIB_SOURCE" && "$TARGET_OS" == "$HOST_OS" && "$TARGET_ARCH" == "$HOST_ARCH" && -z "${ORT_LIBRARY:-}" ]];then LIB_SOURCE="$ROOT/runtime/$LIB_NAME";fi
if [[ ! -f "$LIB_SOURCE" ]];then
 printf '错误：缺少 %s/%s 的 ONNX Runtime 动态库。\n放到 runtime/%s-%s/%s，或设置 ORT_LIBRARY=/绝对路径/%s 后重试。\n' "$TARGET_OS" "$TARGET_ARCH" "$TARGET_OS" "$TARGET_ARCH" "$LIB_NAME" "$LIB_NAME" >&2
 exit 1
fi
if command -v file >/dev/null 2>&1;then
 description="$(file -Lb "$LIB_SOURCE")"
 case "$TARGET_OS" in darwin) [[ "$description" == *Mach-O* ]] || { echo '错误：运行库不是 macOS 格式。' >&2;exit 1; };;linux) [[ "$description" == *ELF* ]] || { echo '错误：运行库不是 Linux ELF 格式。' >&2;exit 1; };;esac
 case "$TARGET_ARCH" in amd64) pattern='x86[-_]64';;arm64) pattern='arm64|aarch64';;esac
 if ! printf '%s' "$description" | grep -Eq "$pattern";then echo "错误：运行库架构不匹配：$description" >&2;exit 1;fi
fi
if [[ "$TARGET_OS" != "$HOST_OS" || "$TARGET_ARCH" != "$HOST_ARCH" ]];then
 if [[ "$TARGET_OS" == linux ]];then
  case "$TARGET_ARCH" in amd64) prefix=x86_64-linux-gnu;;arm64) prefix=aarch64-linux-gnu;;esac
  export CC="${CC:-$prefix-gcc}" CXX="${CXX:-$prefix-g++}"
 else
  if [[ "$HOST_OS" != darwin && ( -z "${CC:-}" || -z "${CXX:-}" ) ]];then echo '错误：Linux 到 macOS 需要 macOS SDK 和交叉工具链，请设置 CC、CXX。' >&2;exit 1;fi
  export CC="${CC:-clang}" CXX="${CXX:-clang++}"
 fi
else
 export CC="${CC:-$(go env CC)}" CXX="${CXX:-$(go env CXX)}"
fi
for compiler in "$CC" "$CXX";do
 command -v "${compiler%% *}" >/dev/null 2>&1 || { echo "错误：缺少编译器 $compiler。当前项目依赖 CGO；请安装目标工具链或在目标系统编译。" >&2;exit 1; }
done
mkdir -p "$WEB_DIR/dist"
STAGE="$(mktemp -d "$WEB_DIR/dist/.build-XXXXXXXX")"
trap 'rm -rf "$STAGE"' EXIT
printf '\n正在编译 %s/%s（CGO_ENABLED=1）…\n' "$TARGET_OS" "$TARGET_ARCH"
(cd "$WEB_DIR"; CGO_ENABLED=1 GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" go build -trimpath -o "$STAGE/ocr-web" .)
mkdir -p "$STAGE/models" "$STAGE/runtime" "$STAGE/licenses"
cp "$ROOT"/models/*.onnx "$ROOT/models/charset_zh13562.txt" "$ROOT/models/manifest.json" "$STAGE/models/"
cp "$LIB_SOURCE" "$STAGE/runtime/$LIB_NAME"
sed -e 's|^models_dir:.*|models_dir: ./models|' -e "s|^runtime_library:.*|runtime_library: ./runtime/$LIB_NAME|" "$ROOT/examples/web/config.yaml" > "$STAGE/config.yaml"
chmod 600 "$STAGE/config.yaml"
cp "$ROOT/runtime/LICENSE" "$STAGE/licenses/onnxruntime.txt"
cp "$ROOT/LICENSE.opencv" "$ROOT/LICENSE.clipper" "$STAGE/licenses/"
cp "$ROOT/internal/jpegcore/LICENSE.md" "$STAGE/licenses/libjpeg-turbo.md"
cp "$ROOT/internal/jpegcore/README.ijg" "$STAGE/licenses/README.ijg"
cp "$WEB_DIR/start.sh" "$STAGE/start.sh"
chmod +x "$STAGE/start.sh"
cat > "$STAGE/README.txt" <<'INFO'
运行 ./start.sh；默认 http://127.0.0.1:7676。
Token、监听地址、端口和限流在 config.yaml 配置，修改后重启。
HTML/CSS/JS 已嵌入程序，无需静态文件。
请保持 models/、runtime/、config.yaml 随程序一起部署。
Linux 需要与编译工具链及 ONNX Runtime 兼容的 libc / C++ 运行库。
模型授权仍需单独确认；打包不代表获得商业使用许可。
INFO
OUTPUT="$WEB_DIR/dist/$TARGET_OS-$TARGET_ARCH-$(date +%Y%m%d-%H%M%S)-$$"
mv "$STAGE" "$OUTPUT"
trap - EXIT
printf '\n编译完成：%s\n运行：%s/start.sh\n' "$OUTPUT" "$OUTPUT"
