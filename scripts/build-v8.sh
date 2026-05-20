#!/usr/bin/env bash
# Build the bundled V8 sources for one supported target and link them into a
# single shared libgv8 runtime. Platform-specific GN args live in args/*.gn.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ARGS_DIR="${ROOT}/args"
GV8_TARGET="${GV8_TARGET:-}"
SOURCE_ROOT="${ROOT}/.v8/src/v8"

if [ ! -f "${ROOT}/internal/v8/SOURCE_COMMIT" ]; then
  echo "error: run 'make fetch' first" >&2
  exit 1
fi

ensure_depot_tools() {
  if [ -d "${DEPOT_TOOLS}/.git" ]; then
    bootstrap_depot_tools
    return
  fi
  git clone --depth=1 \
    https://chromium.googlesource.com/chromium/tools/depot_tools.git \
    "${DEPOT_TOOLS}"
  bootstrap_depot_tools
}

bootstrap_depot_tools() {
  if [ -f "${DEPOT_TOOLS}/python3_bin_reldir.txt" ]; then
    return
  fi

  (
    cd "${DEPOT_TOOLS}"
    ./ensure_bootstrap
  )
}

ensure_gclient_config() {
  cat > "${WORKROOT}/.gclient" << 'EOF'
solutions = [
  {
    "name": "v8",
    "url": "https://chromium.googlesource.com/v8/v8.git",
    "deps_file": "DEPS",
    "managed": False,
    "custom_deps": {},
  }
]
EOF
}

recreate_worktree() {
  rm -rf "${WORKTREE}"
  git clone "${SOURCE_ROOT}" "${WORKTREE}" >/dev/null
}

ensure_worktree() {
  local commit
  local current_commit

  if [ ! -d "${SOURCE_ROOT}/.git" ]; then
    echo "error: V8 source not found - run 'make fetch' first" >&2
    exit 1
  fi

  mkdir -p "${WORKROOT}"

  if [ ! -d "${WORKTREE}/.git" ]; then
    recreate_worktree
  fi

  commit="$(cat "${ROOT}/internal/v8/SOURCE_COMMIT")"
  current_commit="$(git -C "${WORKTREE}" rev-parse HEAD 2>/dev/null || true)"

  if [ "${current_commit}" = "${commit}" ] && [ -f "${WORKTREE}/BUILD.gn" ]; then
    return
  fi

  # Only reset/recreate when the pinned source commit changes or the worktree
  # is otherwise invalid. Preserve out.gn so reruns can reuse prior Ninja
  # outputs for the same target.
  if ! git -C "${WORKTREE}" reset --hard >/dev/null 2>&1 ||
     ! git -C "${WORKTREE}" clean -fd -e out.gn >/dev/null 2>&1 ||
     ! git -C "${WORKTREE}" checkout --detach "${commit}" >/dev/null 2>&1 ||
     [ ! -f "${WORKTREE}/BUILD.gn" ]; then
    recreate_worktree
    git -C "${WORKTREE}" checkout --detach "${commit}" >/dev/null
  fi
}

HOST_OS="$(uname -s)"
HOST_ARCH="$(uname -m)"

if [ -z "${GV8_TARGET}" ]; then
  case "${HOST_OS}" in
    Darwin) PLATFORM="darwin" ;;
    Linux)  PLATFORM="linux"  ;;
    *) echo "unsupported OS: ${HOST_OS}" >&2; exit 1 ;;
  esac

  case "${HOST_ARCH}" in
    x86_64) PLATFORM_ARCH="x86_64" ;;
    arm64|aarch64) PLATFORM_ARCH="arm64" ;;
    *) echo "unsupported arch: ${HOST_ARCH}" >&2; exit 1 ;;
  esac
else
  PLATFORM="${GV8_TARGET%%/*}"
  PLATFORM_ARCH="${GV8_TARGET##*/}"
fi

WORKROOT="${ROOT}/.v8/workspaces/${PLATFORM}_${PLATFORM_ARCH}"
WORKTREE="${WORKROOT}/v8"
DEPOT_TOOLS="${WORKROOT}/depot_tools"

ensure_worktree
ensure_depot_tools

export PATH="${DEPOT_TOOLS}:${PATH}"
export DEPOT_TOOLS_UPDATE=0
export GCLIENT_SUPPRESS_GIT_VERSION_WARNING=1

OUT_DIR="${ROOT}/internal/v8/${PLATFORM}_${PLATFORM_ARCH}"
mkdir -p "${OUT_DIR}"
rm -f \
  "${OUT_DIR}/libv8.a" \
  "${OUT_DIR}/libgv8.dylib" \
  "${OUT_DIR}/libgv8.so" \
  "${OUT_DIR}/icudtl.dat"

case "${PLATFORM}/${PLATFORM_ARCH}" in
  darwin/arm64)
    BUILD_NAME="darwin.arm64.release"
    ARGS_FILE="${ARGS_DIR}/darwin.arm64.gn"
    ;;
  linux/x86_64)
    BUILD_NAME="linux.x64.release"
    ARGS_FILE="${ARGS_DIR}/linux.x64.gn"
    ;;
  linux/arm64)
    BUILD_NAME="linux.arm64.release"
    ARGS_FILE="${ARGS_DIR}/linux.arm64.gn"
    ;;
  *)
    echo "unsupported build target: ${PLATFORM}/${PLATFORM_ARCH}" >&2
    exit 1
    ;;
esac

if [ ! -f "${ARGS_FILE}" ]; then
  echo "error: missing GN args file: ${ARGS_FILE}" >&2
  exit 1
fi

ensure_deps() {
  local commit
  local sync_stamp

  cd "${WORKROOT}"
  ensure_gclient_config
  commit="$(cat "${ROOT}/internal/v8/SOURCE_COMMIT")"
  sync_stamp="${WORKROOT}/.gclient-sync-stamp"

  if [ -f "${sync_stamp}" ] && [ "$(cat "${sync_stamp}")" = "${commit}" ]; then
    return
  fi

  gclient sync -D
  printf '%s\n' "${commit}" > "${sync_stamp}"
}

ensure_target_sysroot() {
  if [ "${PLATFORM}/${PLATFORM_ARCH}" != "linux/arm64" ]; then
    return
  fi

  (
    cd "${WORKTREE}"
    python3 build/linux/sysroot_scripts/install-sysroot.py --arch=arm64
  )
}

monolith_implicit_archives() {
  local ninja_file="${BUILD_DIR}/obj/v8_monolith.ninja"
  python3 - "${ninja_file}" <<'PY'
import sys
from pathlib import Path

text = Path(sys.argv[1]).read_text()
for line in text.splitlines():
    if not line.startswith("build obj/libv8_monolith.a: alink "):
        continue
    if " | " not in line:
        break
    implicit = line.split(" | ", 1)[1].split(" || ", 1)[0]
    for token in implicit.split():
        if token.endswith(".a") or token.endswith(".rlib"):
            print(token)
    break
PY
}

alink_rule_inputs() {
  local ninja_file="$1"
  local output_name="$2"
  python3 - "${ninja_file}" "${output_name}" <<'PY'
import sys
from pathlib import Path

text = Path(sys.argv[1]).read_text()
prefix = f"build {sys.argv[2]}: alink "
for line in text.splitlines():
    if not line.startswith(prefix):
        continue
    main = line[len(prefix):]
    if " | " in main:
        main = main.split(" | ", 1)[0]
    for token in main.split():
        if token.endswith(".o"):
            print(token)
    break
PY
}

shared_output_name() {
  case "${PLATFORM}" in
    darwin) printf '%s\n' "libgv8.dylib" ;;
    linux) printf '%s\n' "libgv8.so" ;;
  esac
}

linux_arm64_runtime_flags() {
  local sysroot="${WORKTREE}/build/linux/debian_bullseye_arm64-sysroot"
  local gcc_dir

  gcc_dir="$(find "${sysroot}/usr/lib/gcc/aarch64-linux-gnu" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
  if [ -z "${gcc_dir}" ]; then
    echo "error: failed to locate arm64 gcc runtime dir in ${sysroot}" >&2
    exit 1
  fi

  printf '%s\n' \
    "-Wl,--sysroot=${sysroot}" \
    "-B${gcc_dir}" \
    "-B${sysroot}/usr/lib/aarch64-linux-gnu" \
    "-B${sysroot}/lib/aarch64-linux-gnu" \
    "-L${gcc_dir}" \
    "-L${sysroot}/usr/lib/aarch64-linux-gnu" \
    "-L${sysroot}/lib/aarch64-linux-gnu" \
    "-Wl,-rpath-link,${sysroot}/usr/lib/aarch64-linux-gnu" \
    "-Wl,-rpath-link,${sysroot}/lib/aarch64-linux-gnu"
}

linkable_monolith_inputs() {
  while IFS= read -r token; do
    [ -n "${token}" ] || continue
    case "${token}" in
      *libclang_rt.*) continue ;;
    esac
    printf '%s/%s\n' "${BUILD_DIR}" "${token}"
  done <<EOF
$(monolith_implicit_archives)
EOF
}

thin_archive_members() {
  local archive_path="$1"
  local archive_tool="${WORKTREE}/third_party/llvm-build/Release+Asserts/bin/llvm-ar"

  "${archive_tool}" t "${archive_path}" 2>/dev/null | while IFS= read -r member; do
    [ -n "${member}" ] || continue
    case "${member}" in
      *.o)
        printf '%s\n' "${member}"
        ;;
    esac
  done
}

toolchain_link_flags() {
  local ninja_file
  case "${PLATFORM}" in
    darwin)
      ninja_file="${BUILD_DIR}/obj/v8_libplatform.ninja"
      ;;
    linux)
      ninja_file="${BUILD_DIR}/obj/d8.ninja"
      ;;
  esac
  python3 - "${ninja_file}" <<'PY'
import shlex
import sys
from pathlib import Path

ninja_path = Path(sys.argv[1]).resolve()
ninja_dir = ninja_path.parent
text = ninja_path.read_text().splitlines()
line_value = ""
prefix = "cflags = "
if sys.argv[1].endswith("/obj/d8.ninja"):
    prefix = "  ldflags = "
for line in text:
    if line.startswith(prefix):
        line_value = line[len(prefix):]
        break

tokens = shlex.split(line_value)
result = []
i = 0

def normalize_path(value: str) -> str:
    path = Path(value)
    if path.is_absolute():
        return str(path)
    return str((ninja_dir / path).resolve())

while i < len(tokens):
    token = tokens[i]
    if prefix == "cflags = ":
        if token.startswith("--target="):
            result.append(token)
        elif token == "-isysroot" and i + 1 < len(tokens):
            if "darwin" in sys.argv[1]:
                result.extend([token, tokens[i + 1]])
            else:
                result.extend([token, normalize_path(tokens[i + 1])])
            i += 1
        elif token.startswith("-mmacos-version-min="):
            result.append(token)
        elif token.startswith("--sysroot="):
            result.append("--sysroot=" + normalize_path(token.split("=", 1)[1]))
        elif token == "-resource-dir" and i + 1 < len(tokens):
            if "darwin" in sys.argv[1]:
                result.extend([token, tokens[i + 1]])
            else:
                result.extend([token, normalize_path(tokens[i + 1])])
            i += 1
        elif token.startswith("-fuse-ld="):
            result.append(token)
        elif token == "-no-canonical-prefixes":
            result.append(token)
    else:
        if token in ("-pie", "-rdynamic"):
            pass
        elif token == "-Wl,-z,defs":
            pass
        elif token.startswith("--sysroot="):
            result.append("--sysroot=" + normalize_path(token.split("=", 1)[1]))
        else:
            result.append(token)
    i += 1

print(shlex.join(result))
PY
}

build_bridge_archive() {
  local clang_ninja="${BUILD_DIR}/obj/v8_libplatform.ninja"
  local bridge_src="${ROOT}/internal/gv8.cc"
  local bridge_obj_dir="${BUILD_DIR}/obj/gv8_bridge"
  local bridge_obj="${bridge_obj_dir}/gv8_bridge.o"
  local clangxx="${WORKTREE}/third_party/llvm-build/Release+Asserts/bin/clang++"
  local defines include_dirs cflags cflags_cc module_deps_no_self

  defines="$(sed -n 's/^defines = //p' "${clang_ninja}")"
  include_dirs="$(sed -n 's/^include_dirs = //p' "${clang_ninja}")"
  cflags="$(sed -n 's/^cflags = //p' "${clang_ninja}")"
  cflags_cc="$(sed -n 's/^cflags_cc = //p' "${clang_ninja}")"
  module_deps_no_self="$(sed -n 's/^module_deps_no_self = //p' "${clang_ninja}")"

  mkdir -p "${bridge_obj_dir}"

  (
    cd "${BUILD_DIR}"
    # Re-parse the generated Ninja flag lines as compiler arguments.
    # shellcheck disable=SC2086,SC2163
    eval "set -- ${defines} ${include_dirs} ${cflags} ${cflags_cc} ${module_deps_no_self}"
    "${clangxx}" "$@" -I"${ROOT}" -I"${ROOT}/internal/v8/include" -c "${bridge_src}" -o "${bridge_obj}"
  )
}

link_shared_library() {
  local clangxx="${WORKTREE}/third_party/llvm-build/Release+Asserts/bin/clang++"
  local bridge_obj="${BUILD_DIR}/obj/gv8_bridge/gv8_bridge.o"
  local output_file="${OUT_DIR}/$(shared_output_name)"
  local monolith_archive="${BUILD_DIR}/obj/libv8_monolith.a"
  local libcxx_archive="${BUILD_DIR}/obj/buildtools/third_party/libc++/libc++.a"
  local libcxxabi_archive="${BUILD_DIR}/obj/buildtools/third_party/libc++abi/libc++abi.a"
  local libcxx_ninja="${BUILD_DIR}/obj/buildtools/third_party/libc++/libc++.ninja"
  local libcxxabi_ninja="${BUILD_DIR}/obj/buildtools/third_party/libc++abi/libc++abi.ninja"
  local libcxx_objects=()
  local libcxxabi_objects=()
  local extra_inputs=()
  local darwin_thin_objects=()
  local link_args=()
  local toolchain_flags=()
  local runtime_flags=()
  local toolchain_flag_expr
  local input
  local obj

  toolchain_flag_expr="$(toolchain_link_flags)"
  # shellcheck disable=SC2086
  eval "toolchain_flags=( ${toolchain_flag_expr} )"

  if [ "${PLATFORM}/${PLATFORM_ARCH}" = "linux/arm64" ]; then
    while IFS= read -r input; do
      [ -n "${input}" ] || continue
      runtime_flags+=("${input}")
    done <<EOF
$(linux_arm64_runtime_flags)
EOF

    while IFS= read -r input; do
      [ -n "${input}" ] || continue
      libcxx_objects+=("${BUILD_DIR}/${input}")
    done <<EOF
$(alink_rule_inputs "${libcxx_ninja}" "obj/buildtools/third_party/libc++/libc++.a")
EOF

    while IFS= read -r input; do
      [ -n "${input}" ] || continue
      libcxxabi_objects+=("${BUILD_DIR}/${input}")
    done <<EOF
$(alink_rule_inputs "${libcxxabi_ninja}" "obj/buildtools/third_party/libc++abi/libc++abi.a")
EOF
  fi

  if [ "${PLATFORM}/${PLATFORM_ARCH}" = "linux/arm64" ]; then
    if [ "${#libcxx_objects[@]}" -eq 0 ] || [ "${#libcxxabi_objects[@]}" -eq 0 ]; then
      echo "error: failed to resolve target libc++ object lists in ${BUILD_DIR}" >&2
      exit 1
    fi
  elif [ ! -f "${libcxx_archive}" ] || [ ! -f "${libcxxabi_archive}" ]; then
    echo "error: target libc++ runtime archives are missing in ${BUILD_DIR}" >&2
    exit 1
  fi

  while IFS= read -r input; do
    [ -n "${input}" ] || continue
    extra_inputs+=("${input}")
  done <<EOF
$(linkable_monolith_inputs)
EOF

  rm -f "${output_file}"

  case "${PLATFORM}" in
    darwin)
      while IFS= read -r obj; do
        [ -n "${obj}" ] || continue
        darwin_thin_objects+=("${obj}")
      done <<EOF
$(thin_archive_members "${libcxx_archive}")
EOF
      while IFS= read -r obj; do
        [ -n "${obj}" ] || continue
        darwin_thin_objects+=("${obj}")
      done <<EOF
$(thin_archive_members "${libcxxabi_archive}")
EOF
      for input in "${extra_inputs[@]}"; do
        case "${input}" in
          *.a)
            while IFS= read -r obj; do
              [ -n "${obj}" ] || continue
              darwin_thin_objects+=("${obj}")
            done <<EOF
$(thin_archive_members "${input}")
EOF
            ;;
          *)
            darwin_thin_objects+=("${input}")
            ;;
        esac
      done
      link_args+=(
        -dynamiclib
        -nostdlib++
        -o "${output_file}"
        -Wl,-install_name,@rpath/$(basename "${output_file}")
        -Wl,-undefined,dynamic_lookup
        "${toolchain_flags[@]}"
        "${bridge_obj}"
        -Wl,-force_load
        -Wl,${monolith_archive}
        "${darwin_thin_objects[@]}"
      )
      link_args+=(-framework Security -framework CoreFoundation -framework Foundation -pthread)
      ;;
    linux)
      link_args+=(
        -shared
        -nostdlib++
        -o "${output_file}"
        -Wl,-soname,$(basename "${output_file}")
        -Wl,--allow-shlib-undefined
        "${toolchain_flags[@]}"
        "${runtime_flags[@]}"
        "${bridge_obj}"
        -Wl,--whole-archive
        "${monolith_archive}"
      )
      if [ "${PLATFORM_ARCH}" = "arm64" ]; then
        link_args+=("${libcxx_objects[@]}" "${libcxxabi_objects[@]}")
      else
        link_args+=("${libcxx_archive}" "${libcxxabi_archive}")
      fi
      for input in "${extra_inputs[@]}"; do
        link_args+=("${input}")
      done
      link_args+=(-Wl,--no-whole-archive -pthread -ldl -lrt)
      ;;
  esac

  if [ "${GV8_DEBUG_LINK:-0}" = "1" ]; then
    printf 'link command:\n'
    printf '  %q' "${clangxx}" "${link_args[@]}"
    printf '\n'
  fi

  (
    cd "${BUILD_DIR}"
    "${clangxx}" "${link_args[@]}"
  )

  if [ -f "${BUILD_DIR}/icudtl.dat" ]; then
    cp "${BUILD_DIR}/icudtl.dat" "${OUT_DIR}/icudtl.dat"
  fi
}

_h="${WORKTREE}/include/v8-version.h"
VERSION="$( \
  M="$(awk '/^#define V8_MAJOR_VERSION/{print $3}' "$_h")"; \
  m="$(awk '/^#define V8_MINOR_VERSION/{print $3}' "$_h")"; \
  b="$(awk '/^#define V8_BUILD_NUMBER/{print $3}' "$_h")"; \
  p="$(awk '/^#define V8_PATCH_LEVEL/{print $3}' "$_h")"; \
  echo "${M}.${m}.${b}.${p}" \
)"
echo "building bundled V8  platform=${PLATFORM}/${PLATFORM_ARCH}  version=${VERSION}"

if command -v getconf >/dev/null 2>&1 && getconf _NPROCESSORS_ONLN >/dev/null 2>&1; then
  CORES="$(getconf _NPROCESSORS_ONLN)"
else
  CORES=2
fi
NINJA_JOBS="${NINJA_JOBS:-${CORES}}"

BUILD_DIR="${WORKTREE}/out.gn/${BUILD_NAME}"
ensure_deps
ensure_target_sysroot
cd "${WORKTREE}"

GN_ARGS="$(sed '/^[[:space:]]*#/d; /^[[:space:]]*$/d' "${ARGS_FILE}" | tr '\n' ' ')"
gn gen "${BUILD_DIR}" --args="${GN_ARGS}"
ninja -C "${BUILD_DIR}" -j"${NINJA_JOBS}" v8_monolith v8_libplatform

if [ "${PLATFORM}" = "linux" ]; then
  ninja -C "${BUILD_DIR}" -j"${NINJA_JOBS}" libc++ libc++abi
fi

build_bridge_archive
link_shared_library
OUTPUT_FILE="${OUT_DIR}/$(shared_output_name)"
echo "done: ${OUTPUT_FILE} ($(du -sh "${OUTPUT_FILE}" | cut -f1))"
