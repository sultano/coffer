#!/bin/sh
set -eu

REPO="sultano/coffer"
BINARY="coffer"

main() {
    os="$(detect_os)"
    arch="$(detect_arch)"

    if [ -z "$os" ] || [ -z "$arch" ]; then
        exit 1
    fi

    version="$(fetch_latest_version)"
    if [ -z "$version" ]; then
        echo "Error: could not determine latest version" >&2
        exit 1
    fi

    install_dir="$(resolve_install_dir)"
    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT

    archive="${BINARY}_${version#v}_${os}_${arch}.tar.gz"
    base_url="https://github.com/${REPO}/releases/download/${version}"

    echo "Installing ${BINARY} ${version} (${os}/${arch})..."

    echo "Downloading ${archive}..."
    download "${base_url}/${archive}" "${tmpdir}/${archive}"
    download "${base_url}/checksums.txt" "${tmpdir}/checksums.txt"

    echo "Verifying checksum..."
    verify_checksum "${tmpdir}" "${archive}"

    tar -xzf "${tmpdir}/${archive}" -C "${tmpdir}"

    mkdir -p "${install_dir}"
    cp "${tmpdir}/${BINARY}" "${install_dir}/${BINARY}"
    chmod +x "${install_dir}/${BINARY}"

    echo "Installed ${BINARY} to ${install_dir}/${BINARY}"

    check_path "${install_dir}"
}

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux" ;;
        Darwin*) echo "darwin" ;;
        *)
            echo "Error: unsupported OS: $(uname -s)" >&2
            echo ""
            ;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64|amd64)  echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            echo "Error: unsupported architecture: $(uname -m)" >&2
            echo ""
            ;;
    esac
}

fetch_latest_version() {
    url="https://api.github.com/repos/${REPO}/releases/latest"
    if command -v curl >/dev/null 2>&1; then
        curl -sfL "$url" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
    elif command -v wget >/dev/null 2>&1; then
        wget -qO- "$url" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/'
    else
        echo "Error: curl or wget is required" >&2
        echo ""
    fi
}

download() {
    url="$1"
    dest="$2"
    if command -v curl >/dev/null 2>&1; then
        curl -sfL -o "$dest" "$url"
    elif command -v wget >/dev/null 2>&1; then
        wget -qO "$dest" "$url"
    else
        echo "Error: curl or wget is required" >&2
        exit 1
    fi
}

verify_checksum() {
    dir="$1"
    file="$2"
    expected="$(grep "${file}" "${dir}/checksums.txt" | awk '{print $1}')"

    if [ -z "$expected" ]; then
        echo "Error: checksum not found for ${file}" >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        actual="$(sha256sum "${dir}/${file}" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
        actual="$(shasum -a 256 "${dir}/${file}" | awk '{print $1}')"
    else
        echo "Warning: no SHA-256 tool found, skipping checksum verification" >&2
        return 0
    fi

    if [ "$expected" != "$actual" ]; then
        echo "Error: checksum mismatch" >&2
        echo "  expected: ${expected}" >&2
        echo "  actual:   ${actual}" >&2
        exit 1
    fi
}

resolve_install_dir() {
    if [ -n "${INSTALL_DIR:-}" ]; then
        echo "$INSTALL_DIR"
    elif [ -d "${HOME}/.local/bin" ] || mkdir -p "${HOME}/.local/bin" 2>/dev/null; then
        echo "${HOME}/.local/bin"
    else
        echo "/usr/local/bin"
    fi
}

check_path() {
    dir="$1"
    case ":${PATH}:" in
        *":${dir}:"*) ;;
        *)
            echo ""
            echo "Warning: ${dir} is not in your PATH"
            echo "Add it with: export PATH=\"${dir}:\$PATH\""
            ;;
    esac
}

main
