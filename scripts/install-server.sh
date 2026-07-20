#!/usr/bin/env bash
set -euo pipefail

repository="${REMOTE_LATEXMK_REPOSITORY:-InvisCat/remote-latexmk}"
release_base="${REMOTE_LATEXMK_RELEASE_BASE_URL:-https://github.com/${repository}/releases/download}"
texlive_url="${REMOTE_LATEXMK_TEXLIVE_URL:-https://mirror.ctan.org/systems/texlive/tlnet/install-tl-unx.tar.gz}"
install_root="${REMOTE_LATEXMK_HOME:-${HOME}/.remote-latexmk}"
version=""
profile="full"
engines="xelatex,pdflatex"
listen="127.0.0.1:8080"
service_mode="auto"
start_server=true
dry_run=false

die() {
  echo "install-server.sh: $*" >&2
  exit 2
}

usage() {
  cat <<'EOF'
Install a tagged remote-latexmk server and a private TeX Live under ~/.remote-latexmk.

Usage:
  install-server.sh --version vX.Y.Z [options]

Options:
  --version VERSION       Required immutable GitHub release tag
  --profile full|slim     TeX Live profile (default: full)
  --engines LIST          Enabled engines (default: xelatex,pdflatex)
  --listen HOST:PORT      Listen address (default: 127.0.0.1:8080)
  --install-dir PATH      Installation root (default: ~/.remote-latexmk)
  --service auto|systemd|none
                          Prefer a systemd user service, or use a PID-file fallback
  --no-start              Install without starting the server
  --dry-run               Print planned actions without changing files
  -h, --help              Show this help

The installer does not use sudo and does not edit shell startup files.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) [[ $# -ge 2 ]] || die "--version needs a value"; version="$2"; shift 2 ;;
    --profile) [[ $# -ge 2 ]] || die "--profile needs a value"; profile="$2"; shift 2 ;;
    --engines) [[ $# -ge 2 ]] || die "--engines needs a value"; engines="$2"; shift 2 ;;
    --listen) [[ $# -ge 2 ]] || die "--listen needs a value"; listen="$2"; shift 2 ;;
    --install-dir) [[ $# -ge 2 ]] || die "--install-dir needs a value"; install_root="$2"; shift 2 ;;
    --service) [[ $# -ge 2 ]] || die "--service needs a value"; service_mode="$2"; shift 2 ;;
    --no-start) start_server=false; shift ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

[[ "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || die "--version must be an immutable tag such as v0.3.0"
[[ "${profile}" == full || "${profile}" == slim ]] || die "--profile must be full or slim"
[[ "${service_mode}" == auto || "${service_mode}" == systemd || "${service_mode}" == none ]] || die "--service must be auto, systemd, or none"
[[ "${listen}" != *$'\n'* && "${install_root}" != *$'\n'* && "${engines}" != *$'\n'* ]] || die "paths and values must not contain newlines"

normalized_engines=""
IFS=',' read -r -a requested_engines <<<"${engines}"
[[ ${#requested_engines[@]} -gt 0 ]] || die "--engines must not be empty"
for engine in "${requested_engines[@]}"; do
  case "${engine}" in
    xelatex|pdflatex) ;;
    lualatex)
      [[ "${profile}" == full ]] || die "lualatex requires --profile full"
      ;;
    *) die "unsupported engine in --engines: ${engine}" ;;
  esac
  [[ ",${normalized_engines}," != *",${engine},"* ]] || die "duplicate engine in --engines: ${engine}"
  if [[ -n "${normalized_engines}" ]]; then normalized_engines+=","; fi
  normalized_engines+="${engine}"
done
engines="${normalized_engines}"
os_name="${REMOTE_LATEXMK_TEST_OS:-$(uname -s)}"
machine="${REMOTE_LATEXMK_TEST_ARCH:-$(uname -m)}"
[[ "${os_name}" == Linux ]] || die "the native server installer currently supports Linux only; use Docker Compose on other systems"

case "${machine}" in
  x86_64|amd64) arch="amd64"; tex_arch="x86_64-linux" ;;
  aarch64|arm64) arch="arm64"; tex_arch="aarch64-linux" ;;
  *) die "unsupported architecture: ${machine}" ;;
esac

for tool in tar gzip perl; do
  command -v "${tool}" >/dev/null 2>&1 || die "required command is missing: ${tool}"
done
if command -v curl >/dev/null 2>&1; then
  download() { curl --fail --location --silent --show-error --retry 3 --output "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  download() { wget -q --tries=3 -O "$2" "$1"; }
else
  die "curl or wget is required"
fi
if command -v sha256sum >/dev/null 2>&1; then
  sha256_file() { sha256sum "$1" | awk '{print $1}'; }
elif command -v shasum >/dev/null 2>&1; then
  sha256_file() { shasum -a 256 "$1" | awk '{print $1}'; }
else
  die "sha256sum or shasum is required"
fi

archive="remote-latexmk-server_${version#v}_linux_${arch}.tar.gz"
archive_url="${release_base}/${version}/${archive}"
checksums_url="${release_base}/${version}/SHA256SUMS"

if [[ "${dry_run}" == true ]]; then
  cat <<EOF
Would install remote-latexmk ${version} for linux/${arch}
  root:      ${install_root}
  profile:   ${profile}
  engines:   ${engines}
  listen:    ${listen}
  server:    ${archive_url}
  checksums: ${checksums_url}
  TeX Live:  ${texlive_url}
  service:   ${service_mode}
EOF
  exit 0
fi

umask 077
mkdir -p "${install_root}" "${install_root}/bin" "${install_root}/config" "${install_root}/logs" "${install_root}/run" "${install_root}/state" "${install_root}/releases"
chmod 700 "${install_root}" "${install_root}/config" "${install_root}/logs" "${install_root}/run" "${install_root}/state"
printf 'remote-latexmk native installation\n' >"${install_root}/.remote-latexmk-install"

temp_dir="$(mktemp -d "${TMPDIR:-/tmp}/remote-latexmk-install.XXXXXX")"
cleanup() { rm -rf -- "${temp_dir}"; }
trap cleanup EXIT

echo "Downloading ${archive}"
download "${archive_url}" "${temp_dir}/${archive}"
download "${checksums_url}" "${temp_dir}/SHA256SUMS"
expected="$(awk -v name="${archive}" '$2 == name || $2 == "*" name { print $1; exit }' "${temp_dir}/SHA256SUMS")"
[[ "${expected}" =~ ^[0-9a-fA-F]{64}$ ]] || die "SHA256SUMS has no entry for ${archive}"
actual="$(sha256_file "${temp_dir}/${archive}")"
actual="$(printf '%s' "${actual}" | tr 'A-F' 'a-f')"
expected="$(printf '%s' "${expected}" | tr 'A-F' 'a-f')"
[[ "${actual}" == "${expected}" ]] || die "checksum mismatch for ${archive}"

tar -xzf "${temp_dir}/${archive}" -C "${temp_dir}"
archive_dir="${temp_dir}/remote-latexmk-server_${version#v}_linux_${arch}"
[[ -x "${archive_dir}/remote-latexmk-server" && -x "${archive_dir}/remote-latexmkctl" ]] || die "release archive is incomplete"
release_dir="${install_root}/releases/${version#v}/linux-${arch}"
rm -rf -- "${release_dir}.new"
mkdir -p "${release_dir}.new"
cp "${archive_dir}/remote-latexmk-server" "${archive_dir}/remote-latexmkctl" "${archive_dir}/install-server.sh" "${release_dir}.new/"
chmod 755 "${release_dir}.new/remote-latexmk-server" "${release_dir}.new/remote-latexmkctl" "${release_dir}.new/install-server.sh"
rm -rf -- "${release_dir}"
mv "${release_dir}.new" "${release_dir}"
ln -sfn "${release_dir}" "${install_root}/current"
ln -sfn "${install_root}/current/remote-latexmk-server" "${install_root}/bin/remote-latexmk-server"
ln -sfn "${install_root}/current/remote-latexmkctl" "${install_root}/bin/remote-latexmkctl"
ln -sfn "${install_root}/current/install-server.sh" "${install_root}/bin/install-server.sh"

tex_root="${install_root}/texlive/current"
tex_profile_file="${install_root}/texlive/.profile"
if [[ "${REMOTE_LATEXMK_TEST_SKIP_TEXLIVE:-0}" == 1 ]]; then
  tex_bin="${REMOTE_LATEXMK_TEST_TEX_BIN:?REMOTE_LATEXMK_TEST_TEX_BIN is required with the test hook}"
elif [[ -x "${tex_root}/bin/${tex_arch}/latexmk" ]]; then
  tex_bin="${tex_root}/bin/${tex_arch}"
  echo "Using existing TeX Live at ${tex_root}"
  existing_profile="$(cat "${tex_profile_file}" 2>/dev/null || true)"
  if [[ "${profile}" == full && "${existing_profile}" != full ]]; then
    echo "Expanding the existing TeX Live installation to the full profile."
    "${tex_bin}/tlmgr" install scheme-full
  elif [[ "${profile}" == slim && "${existing_profile}" == full ]]; then
    echo "Keeping existing full TeX Live packages; the server engine policy will use the slim profile."
  fi
else
  echo "Installing TeX Live profile '${profile}'. This is the large part of the installation."
  download "${texlive_url}" "${temp_dir}/install-tl-unx.tar.gz"
  mkdir -p "${temp_dir}/install-tl"
  tar -xzf "${temp_dir}/install-tl-unx.tar.gz" -C "${temp_dir}/install-tl" --strip-components=1
  mkdir -p "${install_root}/texlive"
  if [[ "${profile}" == full ]]; then
    scheme="scheme-full"
  else
    scheme="scheme-small"
  fi
  cat >"${temp_dir}/texlive.profile" <<EOF
selected_scheme ${scheme}
TEXDIR ${tex_root}
TEXMFCONFIG ${install_root}/texlive/texmf-config
TEXMFHOME ${install_root}/texlive/texmf-home
TEXMFLOCAL ${install_root}/texlive/texmf-local
TEXMFSYSCONFIG ${install_root}/texlive/texmf-sysconfig
TEXMFSYSVAR ${install_root}/texlive/texmf-sysvar
TEXMFVAR ${install_root}/texlive/texmf-var
option_doc 0
option_src 0
EOF
  perl "${temp_dir}/install-tl/install-tl" -profile "${temp_dir}/texlive.profile" -repository "${REMOTE_LATEXMK_TEXLIVE_REPOSITORY:-https://mirror.ctan.org/systems/texlive/tlnet}"
  tex_bin="${tex_root}/bin/${tex_arch}"
  if [[ "${profile}" == slim ]]; then
    "${tex_bin}/tlmgr" install latexmk biber biblatex collection-xetex collection-latexrecommended collection-latexextra collection-fontsrecommended collection-langchinese collection-langjapanese collection-langkorean
  fi
fi
if [[ "${REMOTE_LATEXMK_TEST_SKIP_TEXLIVE:-0}" != 1 ]]; then
  printf '%s\n' "${profile}" >"${tex_profile_file}"
  chmod 600 "${tex_profile_file}"
fi

for tool in latexmk xelatex pdflatex; do
  [[ -x "${tex_bin}/${tool}" ]] || die "TeX Live installation is missing ${tool}"
done
if [[ ",${engines}," == *,lualatex,* ]]; then
  [[ -x "${tex_bin}/lualatex" ]] || die "full TeX Live installation is missing lualatex"
fi

config_file="${install_root}/config/server.env"
token_file="${install_root}/config/token"
token=""
if [[ -f "${token_file}" ]]; then
  token="$(<"${token_file}")"
  [[ -n "${token}" ]] || die "existing token file is empty: ${token_file}"
elif [[ -f "${config_file}" ]]; then
  # Migrate native installations that stored the token in server.env.
  # shellcheck disable=SC1090
  source "${config_file}"
  if [[ -n "${LATEXMK_API_TOKEN:-}" ]]; then
    token="${LATEXMK_API_TOKEN}"
  elif [[ -n "${LATEXMK_API_TOKEN_FILE:-}" ]]; then
    [[ -f "${LATEXMK_API_TOKEN_FILE}" ]] || die "configured token file is missing: ${LATEXMK_API_TOKEN_FILE}"
    token="$(<"${LATEXMK_API_TOKEN_FILE}")"
  fi
fi
if [[ -z "${token}" ]]; then
  if command -v openssl >/dev/null 2>&1; then
    token="$(openssl rand -hex 32)"
  else
    token="$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')"
  fi
fi
[[ ${#token} -ge 24 && "${token}" != *$'\n'* && "${token}" != *$'\r'* ]] || die "existing API token is invalid"

printf '%s\n' "${token}" >"${token_file}.new"
chmod 600 "${token_file}.new"
mv "${token_file}.new" "${token_file}"

escape_env() { printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'; }
cat >"${config_file}.new" <<EOF
PATH="$(escape_env "${tex_bin}:/usr/local/bin:/usr/bin:/bin")"
REMOTE_LATEXMK_SERVER_BIN="$(escape_env "${install_root}/bin/remote-latexmk-server")"
REMOTE_LATEXMK_PROFILE="${profile}"
LATEXMK_ADDR="$(escape_env "${listen}")"
LATEXMK_AUTH_MODE="token"
LATEXMK_API_TOKEN=""
LATEXMK_API_TOKEN_FILE="$(escape_env "${token_file}")"
LATEXMK_STATE_DIR="$(escape_env "${install_root}/state")"
LATEXMK_TEMP_DIR="$(escape_env "${install_root}/run")"
LATEXMK_IMAGE_PROFILE="native-${profile}"
LATEXMK_ENGINES="${engines}"
LATEXMK_ALLOW_SHELL_ESCAPE="false"
EOF
chmod 600 "${config_file}.new"
mv "${config_file}.new" "${config_file}"

install_systemd=false
if [[ "${service_mode}" == systemd ]]; then
  command -v systemctl >/dev/null 2>&1 || die "--service systemd requested but systemctl is missing"
  systemctl --user show-environment >/dev/null 2>&1 || die "the systemd user manager is not available; use --service none"
  install_systemd=true
elif [[ "${service_mode}" == auto ]] && command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
  install_systemd=true
fi

if [[ "${install_systemd}" == true ]]; then
  actual_service_mode="systemd"
elif [[ "${service_mode}" == none ]]; then
  actual_service_mode="fallback"
else
  actual_service_mode="stopped"
fi
printf 'REMOTE_LATEXMK_SERVICE_MODE="%s"\n' "${actual_service_mode}" >>"${config_file}"

if [[ "${install_systemd}" == true ]]; then
  [[ "${install_root}" != *[[:space:]]* ]] || die "systemd service paths cannot contain whitespace; choose another --install-dir"
  mkdir -p "${HOME}/.config/systemd/user"
  cat >"${HOME}/.config/systemd/user/remote-latexmk.service" <<EOF
[Unit]
Description=remote-latexmk private LaTeX compilation server
After=network.target

[Service]
Type=simple
EnvironmentFile=${config_file}
ExecStart=${release_dir}/remote-latexmk-server
Restart=on-failure
RestartSec=3
UMask=0077
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=tmpfs
BindReadOnlyPaths=${release_dir}
BindReadOnlyPaths=${tex_root}
BindReadOnlyPaths=${install_root}/config
BindPaths=${install_root}/state
BindPaths=${install_root}/run
ReadWritePaths=${install_root}/state ${install_root}/run
CapabilityBoundingSet=
LockPersonality=true
ProtectClock=true
ProtectControlGroups=true
ProtectHostname=true
ProtectKernelLogs=true
ProtectKernelModules=true
ProtectKernelTunables=true
RemoveIPC=true
RestrictNamespaces=true
RestrictRealtime=true
RestrictSUIDSGID=true

[Install]
WantedBy=default.target
EOF
  systemctl --user daemon-reload
  if command -v loginctl >/dev/null 2>&1; then
    linger="$(loginctl show-user "${USER:-$(id -un)}" -p Linger --value 2>/dev/null || true)"
    if [[ "${linger}" == no ]]; then
      echo "Note: systemd user lingering is disabled; the service may stop after logout and will not start at boot until an administrator enables lingering." >&2
    fi
  fi
elif [[ "${service_mode}" == none && -f "${HOME}/.config/systemd/user/remote-latexmk.service" ]]; then
  systemctl --user disable --now remote-latexmk.service >/dev/null 2>&1 || true
  rm -f "${HOME}/.config/systemd/user/remote-latexmk.service"
  systemctl --user daemon-reload >/dev/null 2>&1 || true
fi

if [[ "${service_mode}" == auto && "${install_systemd}" != true ]]; then
  start_server=false
  cat >&2 <<'EOF'
Warning: no systemd user manager is available. The server was installed but was
not started because the PID-file fallback cannot hide the rest of your home
directory from TeX. Use a dedicated Unix account, enable a systemd user
manager, or rerun with --service none to choose the weaker fallback explicitly.
EOF
fi

if [[ "${start_server}" == true ]]; then
  "${install_root}/bin/remote-latexmkctl" restart
fi

cat <<EOF

========================================================================
 remote-latexmk ${version} is ready
========================================================================
The remote-latexmk API token is also stored on this server at:
  ${token_file}

Show the token again on this server:
  ${install_root}/bin/remote-latexmkctl token

Server listen URL: http://${listen}

Install the Plugin on the client:
  Codex Desktop:
    npx --yes --ignore-scripts remote-latexmk@${version#v} plugin install codex

  Codex CLI:
    codex plugin marketplace add InvisCat/remote-latexmk
    codex plugin add remote-latexmk@remote-latexmk

  Claude Code:
    claude plugin marketplace add InvisCat/remote-latexmk
    claude plugin install remote-latexmk@remote-latexmk

Save the connection on the client. Replace the URL with the VPN or tunnel
endpoint when needed, then paste the remote-latexmk API token from the final
box when prompted; terminal echo is disabled:
  npx --yes --ignore-scripts remote-latexmk@${version#v} auth login --server "http://${listen}"

Then start the Agent in the paper directory and ask it to compile the paper.

The installer did not use sudo or edit shell startup files.
Control command: ${install_root}/bin/remote-latexmkctl
EOF

box_width=64
if (( ${#token} > box_width )); then box_width=${#token}; fi
printf -v box_border '%*s' "$((box_width + 2))" ''
box_border="${box_border// /-}"
accent=''
reset=''
if [[ -t 1 && "${TERM:-dumb}" != dumb && -z "${NO_COLOR+x}" ]]; then
  if [[ "${COLORTERM:-}" == truecolor || "${COLORTERM:-}" == 24bit ]]; then
    accent=$'\033[1;38;2;103;232;197m'
  else
    accent=$'\033[1;36m'
  fi
  reset=$'\033[0m'
fi
printf '\n%b+%s+%b\n' "${accent}" "${box_border}" "${reset}"
printf '%b| %-*s |%b\n' "${accent}" "${box_width}" 'REMOTE-LATEXMK API TOKEN' "${reset}"
printf '%b| %-*s |%b\n' "${accent}" "${box_width}" "${token}" "${reset}"
printf '%b+%s+%b\n' "${accent}" "${box_border}" "${reset}"
