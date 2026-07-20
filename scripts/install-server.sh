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
profile_set=false
engines_set=false
listen_set=false
service_set=false
interactive_mode="auto"
start_server=true
start_set=false
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
  --interactive          Prompt for profile, engines, listener, port, and service
  --non-interactive      Use flags, existing settings, or safe defaults
  --no-start              Install without starting the server
  --dry-run               Print planned actions without changing files
  -h, --help              Show this help

The installer does not use sudo and does not edit shell startup files.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) [[ $# -ge 2 ]] || die "--version needs a value"; version="$2"; shift 2 ;;
    --profile) [[ $# -ge 2 ]] || die "--profile needs a value"; profile="$2"; profile_set=true; shift 2 ;;
    --engines) [[ $# -ge 2 ]] || die "--engines needs a value"; engines="$2"; engines_set=true; shift 2 ;;
    --listen) [[ $# -ge 2 ]] || die "--listen needs a value"; listen="$2"; listen_set=true; shift 2 ;;
    --install-dir) [[ $# -ge 2 ]] || die "--install-dir needs a value"; install_root="$2"; shift 2 ;;
    --service) [[ $# -ge 2 ]] || die "--service needs a value"; service_mode="$2"; service_set=true; shift 2 ;;
    --interactive) interactive_mode=true; shift ;;
    --non-interactive) interactive_mode=false; shift ;;
    --no-start) start_server=false; start_set=true; shift ;;
    --dry-run) dry_run=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown option: $1" ;;
  esac
done

config_file="${install_root}/config/server.env"
override_file="${install_root}/config/server.override.env"
install_state="${install_root}/config/install.env"
unit_file="${HOME}/.config/systemd/user/remote-latexmk.service"
existing_installation=false
existing_service_mode=""
service_was_running=false
systemd_was_enabled=false

read_existing_config() {
  [[ -f "${config_file}" ]] || return 0
  local existing_values
  existing_values="$({
    set +u
    # This file is created with mode 0600 by this installer.
    # shellcheck disable=SC1090
    source "${config_file}"
    printf '%s\034%s\034%s\034%s' \
      "${REMOTE_LATEXMK_PROFILE:-}" \
      "${LATEXMK_ENGINES:-}" \
      "${LATEXMK_ADDR:-}" \
      "${REMOTE_LATEXMK_SERVICE_MODE:-}"
  })"
  local existing_profile existing_engines existing_listen existing_service
  IFS=$'\034' read -r existing_profile existing_engines existing_listen existing_service <<<"${existing_values}"
  existing_service_mode="${existing_service}"
  if [[ "${profile_set}" != true && -n "${existing_profile}" ]]; then profile="${existing_profile}"; fi
  if [[ "${engines_set}" != true && -n "${existing_engines}" ]]; then engines="${existing_engines}"; fi
  if [[ "${listen_set}" != true && -n "${existing_listen}" ]]; then listen="${existing_listen}"; fi
  if [[ "${service_set}" != true ]]; then
    case "${existing_service}" in
      systemd) service_mode="systemd" ;;
      fallback) service_mode="none" ;;
      stopped) service_mode="auto" ;;
    esac
  fi
}

snapshot_runtime_state() {
  if [[ -L "${install_root}/current" || -f "${config_file}" ]]; then
    existing_installation=true
  fi
  if [[ -f "${unit_file}" ]] && command -v systemctl >/dev/null 2>&1; then
    if systemctl --user is-active --quiet remote-latexmk.service 2>/dev/null; then
      service_was_running=true
    fi
    if systemctl --user is-enabled --quiet remote-latexmk.service 2>/dev/null; then
      systemd_was_enabled=true
    fi
  fi
  if [[ "${service_was_running}" != true && -f "${install_root}/run/server.pid" ]]; then
    local previous_pid previous_start current_start
    read -r previous_pid previous_start <"${install_root}/run/server.pid" || true
    if [[ "${previous_pid:-}" =~ ^[0-9]+$ && "${previous_start:-}" =~ ^[0-9]+$ ]] &&
      kill -0 "${previous_pid}" 2>/dev/null && [[ -r "/proc/${previous_pid}/stat" ]]; then
      current_start="$(awk '{print $22}' "/proc/${previous_pid}/stat" 2>/dev/null || true)"
      [[ "${current_start}" == "${previous_start}" ]] && service_was_running=true
    fi
  fi
  if [[ "${existing_installation}" == true && "${start_set}" != true ]]; then
    start_server="${service_was_running}"
  fi
}

listen_host=""
listen_port=""
parse_listen() {
  local value="$1"
  if [[ "${value}" =~ ^\[([^]]+)\]:([0-9]+)$ ]]; then
    listen_host="${BASH_REMATCH[1]}"
    listen_port="${BASH_REMATCH[2]}"
  elif [[ "${value}" =~ ^([^][:space:]/]+):([0-9]+)$ ]]; then
    listen_host="${BASH_REMATCH[1]}"
    listen_port="${BASH_REMATCH[2]}"
  else
    die "--listen must be HOST:PORT; wrap IPv6 addresses in brackets"
  fi
  (( 10#${listen_port} >= 1 && 10#${listen_port} <= 65535 )) || die "listen port must be between 1 and 65535"
  [[ "${listen_host}" != *'$'* && "${listen_host}" != *'`'* && "${listen_host}" != *'"'* && "${listen_host}" != *'\\'* ]] || die "listen host contains unsafe characters"
}

format_listen() {
  local host="$1" port="$2"
  if [[ "${host}" == *:* ]]; then
    printf '[%s]:%s' "${host}" "${port}"
  else
    printf '%s:%s' "${host}" "${port}"
  fi
}

discover_interface_addresses() {
  if [[ -n "${REMOTE_LATEXMK_TEST_INTERFACES:-}" ]]; then
    printf '%s\n' "${REMOTE_LATEXMK_TEST_INTERFACES}"
    return
  fi
  if command -v ip >/dev/null 2>&1; then
    ip -o addr show up 2>/dev/null | awk '
      $3 == "inet" || $3 == "inet6" {
        split($4, value, "/")
        if (value[1] !~ /^fe80:/) print $2 "|" value[1]
      }
    '
    return
  fi
  if command -v hostname >/dev/null 2>&1; then
    local value
    for value in $(hostname -I 2>/dev/null || true); do
      [[ "${value}" != fe80:* ]] && printf 'network|%s\n' "${value}"
    done
  fi
}

interface_label() {
  local interface="$1" address="$2"
  case "${interface}" in
    tailscale*|wg*|tun*) printf '%s (VPN interface)' "${interface}" ;;
    *) printf '%s' "${interface}" ;;
  esac
  printf ' — %s' "${address}"
  if address_may_be_public "${address}"; then
    printf ' (may be public)'
  fi
}

address_may_be_public() {
  local address="$1" second
  case "${address}" in
    10.*|127.*|169.254.*|192.168.*|::1|[fF][cCdD]*) return 1 ;;
  esac
  if [[ "${address}" =~ ^172\.([0-9]+)\. ]]; then
    second="${BASH_REMATCH[1]}"
    (( 10#${second} >= 16 && 10#${second} <= 31 )) && return 1
  fi
  if [[ "${address}" =~ ^100\.([0-9]+)\. ]]; then
    second="${BASH_REMATCH[1]}"
    (( 10#${second} >= 64 && 10#${second} <= 127 )) && return 1
  fi
  [[ "${address}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ || "${address}" == *:* ]]
}

prompt_read() {
  local prompt="$1" default_value="$2" answer
  printf '%s' "${prompt}"
  IFS= read -r -u 3 answer || die "interactive input ended before installation was confirmed"
  if [[ -z "${answer}" ]]; then answer="${default_value}"; fi
  printf -v REPLY '%s' "${answer}"
}

prompt_index() {
  local title="$1" default_index="$2"
  shift 2
  local options=("$@") index
  printf '\n%s\n' "${title}"
  for index in "${!options[@]}"; do
    printf '  %d) %s\n' "$((index + 1))" "${options[index]}"
  done
  while true; do
    prompt_read "Choice [${default_index}]: " "${default_index}"
    if [[ "${REPLY}" =~ ^[0-9]+$ ]] && (( 10#${REPLY} >= 1 && 10#${REPLY} <= ${#options[@]} )); then
      PROMPT_INDEX="$((10#${REPLY}))"
      return
    fi
    echo "Enter a number from 1 to ${#options[@]}."
  done
}

run_interactive_wizard() {
  local prompt_file="${REMOTE_LATEXMK_TEST_INPUT_FILE:-}"
  local index summary_listen tex_action
  if [[ -n "${prompt_file}" ]]; then
    exec 3<"${prompt_file}"
  elif ! exec 3<>/dev/tty 2>/dev/null; then
    die "--interactive requires a terminal; use --non-interactive with explicit flags"
  fi

  echo
  echo "remote-latexmk server setup"
  echo "Press Enter to keep the value shown in brackets."

  if [[ "${profile_set}" != true ]]; then
    local profile_default=1
    [[ "${profile}" == slim ]] && profile_default=2
    prompt_index "TeX Live profile" "${profile_default}" "full — broad package set (recommended)" "slim — smaller package set"
    [[ "${PROMPT_INDEX}" == 1 ]] && profile=full || profile=slim
  fi

  if [[ "${engines_set}" != true ]]; then
    local engine_options=("XeLaTeX + PDFLaTeX (recommended)" "XeLaTeX only" "PDFLaTeX only")
    local engine_values=("xelatex,pdflatex" "xelatex" "pdflatex")
    if [[ "${profile}" == full ]]; then
      engine_options+=("XeLaTeX + PDFLaTeX + LuaLaTeX (opt in for trusted papers)")
      engine_values+=("xelatex,pdflatex,lualatex")
    fi
    engine_options+=("Custom comma-separated list")
    engine_values+=("__custom__")
    local engine_default=1
    for index in "${!engine_values[@]}"; do
      [[ "${engine_values[index]}" == "${engines}" ]] && engine_default=$((index + 1))
    done
    prompt_index "Enabled engines" "${engine_default}" "${engine_options[@]}"
    engines="${engine_values[PROMPT_INDEX - 1]}"
    if [[ "${engines}" == __custom__ ]]; then
      prompt_read "Engines [xelatex,pdflatex]: " "xelatex,pdflatex"
      engines="${REPLY}"
    fi
  fi

  parse_listen "${listen}"
  local current_host="${listen_host}" current_port="${listen_port}"
  if [[ "${listen_set}" != true ]]; then
    local address_values=("127.0.0.1")
    local address_options=("127.0.0.1 — local machine or SSH tunnel (recommended)")
    local interface address seen current_label wildcard_ipv4_label wildcard_ipv6_label
    seen='|127.0.0.1|'
    while IFS='|' read -r interface address; do
      [[ -n "${interface}" && -n "${address}" && "${address}" != 127.* && "${address}" != 0.0.0.0 ]] || continue
      [[ "${seen}" != *"|${address}|"* ]] || continue
      seen+="${address}|"
      address_values+=("${address}")
      address_options+=("$(interface_label "${interface}" "${address}")")
    done < <(discover_interface_addresses)
    if [[ "${current_host}" != 127.0.0.1 && "${current_host}" != 0.0.0.0 && "${current_host}" != :: && "${seen}" != *"|${current_host}|"* ]]; then
      address_values+=("${current_host}")
      current_label="${current_host} — current configuration"
      if address_may_be_public "${current_host}"; then current_label+=" (may be public)"; fi
      address_options+=("${current_label}")
    fi
    wildcard_ipv4_label="0.0.0.0 — all IPv4 interfaces (may include public networks)"
    wildcard_ipv6_label=":: — all IPv6 interfaces (may include public networks)"
    if [[ "${current_host}" == 0.0.0.0 ]]; then wildcard_ipv4_label+="; current configuration"; fi
    if [[ "${current_host}" == :: ]]; then wildcard_ipv6_label+="; current configuration"; fi
    address_values+=("__custom__" "0.0.0.0" "::")
    address_options+=("Enter another host or IP" "${wildcard_ipv4_label}" "${wildcard_ipv6_label}")
    local address_default=1
    if [[ "${current_host}" != 0.0.0.0 && "${current_host}" != :: ]]; then
      for index in "${!address_values[@]}"; do
        [[ "${address_values[index]}" == "${current_host}" ]] && address_default=$((index + 1))
      done
    fi
    prompt_index "Listen address" "${address_default}" "${address_options[@]}"
    current_host="${address_values[PROMPT_INDEX - 1]}"
    if [[ "${current_host}" == __custom__ ]]; then
      prompt_read "Host or IP [${listen_host}]: " "${listen_host}"
      current_host="${REPLY}"
      if [[ "${current_host}" =~ ^\[([^]]+)\]$ ]]; then current_host="${BASH_REMATCH[1]}"; fi
      [[ -n "${current_host}" ]] || die "listen host must not be empty"
    fi
    if [[ "${current_host}" == 0.0.0.0 || "${current_host}" == :: ]]; then
      if [[ "${current_host}" == 0.0.0.0 ]]; then
        echo "Warning: 0.0.0.0 listens on every IPv4 interface, including any public interface."
      else
        echo "Warning: :: listens on every IPv6 interface, including any public interface."
      fi
      prompt_read "Use ${current_host} anyway? [y/N]: " "N"
      [[ "${REPLY}" == y || "${REPLY}" == Y ]] || die "installation cancelled"
    fi
    prompt_read "Listen port [${current_port}]: " "${current_port}"
    current_port="${REPLY}"
    listen="$(format_listen "${current_host}" "${current_port}")"
  fi

  if [[ "${service_set}" != true ]]; then
    local service_default=1
    case "${service_mode}" in systemd) service_default=2 ;; none) service_default=3 ;; esac
    prompt_index "Service mode" "${service_default}" "auto (systemd when available)" "systemd user service" "PID-file fallback (weaker isolation)"
    case "${PROMPT_INDEX}" in 1) service_mode=auto ;; 2) service_mode=systemd ;; 3) service_mode=none ;; esac
  fi

  parse_listen "${listen}"
  if [[ "${listen_host}" == 0.0.0.0 || "${listen_host}" == :: ]]; then
    summary_listen="${listen} (bind address only; not a client URL)"
  else
    summary_listen="http://${listen}"
  fi
  if [[ -d "${install_root}/texlive/current" ]]; then
    tex_action="reuse the private TeX Live installation and add required packages if needed"
  else
    tex_action="install a private TeX Live under ${install_root}/texlive"
  fi
  printf '\nInstall plan\n'
  printf '  release:  %s (verified server archive will be downloaded)\n' "${version}"
  printf '  install:  %s\n' "${install_root}"
  printf '  profile:  %s\n' "${profile}"
  printf '  engines:  %s\n' "${engines}"
  printf '  listen:   %s\n' "${summary_listen}"
  printf '  service:  %s\n' "${service_mode}"
  printf '  TeX Live: %s\n' "${tex_action}"
  prompt_read "Continue? [Y/n]: " "Y"
  [[ "${REPLY}" != n && "${REPLY}" != N ]] || die "installation cancelled"
  exec 3<&-
}

read_existing_config
snapshot_runtime_state

if [[ "${dry_run}" != true ]]; then
  if [[ "${interactive_mode}" == true ]]; then
    run_interactive_wizard
  elif [[ "${interactive_mode}" == auto && "${profile_set}" != true && "${engines_set}" != true && "${listen_set}" != true && "${service_set}" != true ]]; then
    if [[ -n "${REMOTE_LATEXMK_TEST_INPUT_FILE:-}" || -t 0 || -t 1 ]]; then
      run_interactive_wizard
    fi
  fi
fi

[[ "${version}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$ ]] || die "--version must be an immutable tag such as v0.3.0"
[[ "${profile}" == full || "${profile}" == slim ]] || die "--profile must be full or slim"
[[ "${service_mode}" == auto || "${service_mode}" == systemd || "${service_mode}" == none ]] || die "--service must be auto, systemd, or none"
[[ "${listen}" != *$'\n'* && "${install_root}" != *$'\n'* && "${engines}" != *$'\n'* ]] || die "paths and values must not contain newlines"
parse_listen "${listen}"
listen="$(format_listen "${listen_host}" "${listen_port}")"

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
  download() { curl --fail --location --silent --show-error --retry 3 --connect-timeout 10 --output "$2" "$1"; }
  probe_download() {
    local status
    status="$(curl --fail --silent --show-error --noproxy '*' --connect-timeout 1 --max-time 3 --output "$2" --write-out '%{http_code}' "$1")" || return
    [[ "${status}" =~ ^2[0-9][0-9]$ ]]
  }
elif command -v wget >/dev/null 2>&1; then
  download() { wget -q --tries=3 --timeout=30 -O "$2" "$1"; }
  probe_download() { wget -q --no-proxy --max-redirect=0 --tries=1 --timeout=3 -O "$2" "$1"; }
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
release_dir="${install_root}/releases/${version#v}/linux-${arch}"

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
activation_started=false
release_replaced=false
rollback_preserve_backup=false
had_current=false
had_config=false
had_override=false
had_install_state=false
had_unit=false
had_target_release=false
previous_current=""
target_release_backup="${release_dir}.rollback.$$.${RANDOM}"

if [[ -L "${install_root}/current" ]]; then
  had_current=true
  previous_current="$(readlink "${install_root}/current")"
fi
if [[ -f "${config_file}" ]]; then
  had_config=true
  cp "${config_file}" "${temp_dir}/server.env.previous"
fi
if [[ -f "${override_file}" ]]; then
  had_override=true
  cp "${override_file}" "${temp_dir}/server.override.env.previous"
fi
if [[ -f "${install_state}" ]]; then
  had_install_state=true
  cp "${install_state}" "${temp_dir}/install.env.previous"
fi
if [[ -f "${unit_file}" ]]; then
  had_unit=true
  cp "${unit_file}" "${temp_dir}/remote-latexmk.service.previous"
fi
if [[ -e "${release_dir}" ]]; then
  had_target_release=true
fi

rollback_activation() {
  [[ "${activation_started}" == true ]] || return 0
  set +e
  echo "Installation failed after activation; restoring the previous server configuration." >&2
  local rollback_failed=false restart_failed=false
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user stop remote-latexmk.service >/dev/null 2>&1 || true
    if [[ "${had_unit}" != true && "${install_systemd:-false}" == true ]]; then
      systemctl --user disable --now remote-latexmk.service >/dev/null 2>&1 || rollback_failed=true
    fi
  fi
  if [[ -f "${install_root}/run/server.pid" && -x "${install_root}/bin/remote-latexmkctl" ]]; then
    "${install_root}/bin/remote-latexmkctl" stop >/dev/null 2>&1 || true
  fi
  if [[ "${had_current}" == true ]]; then
    ln -sfn "${previous_current}" "${install_root}/current" || rollback_failed=true
  else
    rm -f "${install_root}/current" || rollback_failed=true
  fi
  if [[ "${release_replaced}" == true ]]; then
    rm -rf -- "${release_dir}" || rollback_failed=true
    if [[ "${had_target_release}" == true ]]; then
      mv "${target_release_backup}" "${release_dir}" || rollback_failed=true
    fi
  fi
  if [[ "${had_config}" == true ]]; then
    cp "${temp_dir}/server.env.previous" "${config_file}" || rollback_failed=true
    chmod 600 "${config_file}" || rollback_failed=true
  else
    rm -f "${config_file}" || rollback_failed=true
  fi
  if [[ "${had_override}" == true ]]; then
    cp "${temp_dir}/server.override.env.previous" "${override_file}" || rollback_failed=true
    chmod 600 "${override_file}" || rollback_failed=true
  else
    rm -f "${override_file}" || rollback_failed=true
  fi
  if [[ "${had_install_state}" == true ]]; then
    cp "${temp_dir}/install.env.previous" "${install_state}" || rollback_failed=true
    chmod 600 "${install_state}" || rollback_failed=true
  else
    rm -f "${install_state}" || rollback_failed=true
  fi
  if [[ "${had_unit}" == true ]]; then
    cp "${temp_dir}/remote-latexmk.service.previous" "${unit_file}" || rollback_failed=true
  else
    rm -f "${unit_file}" || rollback_failed=true
  fi
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload >/dev/null 2>&1 || rollback_failed=true
    if [[ "${had_unit}" == true ]]; then
      if [[ "${systemd_was_enabled}" == true ]]; then
        systemctl --user enable remote-latexmk.service >/dev/null 2>&1 || rollback_failed=true
      else
        systemctl --user disable remote-latexmk.service >/dev/null 2>&1 || rollback_failed=true
      fi
    fi
  fi
  if [[ "${service_was_running}" == true && -x "${install_root}/bin/remote-latexmkctl" ]]; then
    "${install_root}/bin/remote-latexmkctl" start >/dev/null 2>&1 || restart_failed=true
    if [[ "${had_unit}" == true && "${systemd_was_enabled}" != true ]] && command -v systemctl >/dev/null 2>&1; then
      systemctl --user disable remote-latexmk.service >/dev/null 2>&1 || rollback_failed=true
    fi
  fi
  if [[ "${restart_failed}" == true ]]; then
    echo "ERROR: rollback restored the old files, but the previous server FAILED TO RESTART." >&2
    echo "Run ${install_root}/bin/remote-latexmkctl status and inspect its logs before retrying." >&2
  fi
  if [[ "${rollback_failed}" == true ]]; then
    rollback_preserve_backup=true
    echo "ERROR: rollback could not restore every previous installation file or systemd setting." >&2
    echo "Do not retry the update until ${install_root} and the user service have been inspected." >&2
    echo "Rollback backups were preserved at ${temp_dir} and ${target_release_backup}." >&2
  fi
  set -e
}

cleanup() {
  local status=$?
  trap - EXIT
  if (( status != 0 )); then rollback_activation; fi
  rm -f "${config_file}.new" "${override_file}.new" "${install_state}.new" "${unit_file}.new"
  rm -rf -- "${release_dir}.new"
  if [[ "${rollback_preserve_backup}" == true ]]; then
    echo "Preserving rollback backup directory: ${temp_dir}" >&2
  else
    rm -rf -- "${temp_dir}"
    rm -rf -- "${target_release_backup}"
  fi
  exit "${status}"
}
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
rm -rf -- "${release_dir}.new"
mkdir -p "${release_dir}.new"
cp "${archive_dir}/remote-latexmk-server" "${archive_dir}/remote-latexmkctl" "${archive_dir}/install-server.sh" "${release_dir}.new/"
chmod 755 "${release_dir}.new/remote-latexmk-server" "${release_dir}.new/remote-latexmkctl" "${release_dir}.new/install-server.sh"

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

[[ -x "${tex_bin}/latexmk" ]] || die "TeX Live installation is missing latexmk"
for engine in "${requested_engines[@]}"; do
  [[ -x "${tex_bin}/${engine}" ]] || die "TeX Live installation is missing ${engine}"
done

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

escape_env() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//\"/\\\"}"
  value="${value//\$/\\\$}"
  value="${value//\`/\\\`}"
  printf '%s' "${value}"
}

is_persistent_tuning_key() {
  case "$1" in
    DATABASE_URL|LATEXMK_DATABASE_MODE|LATEXMK_CORS_ORIGINS|\
    LATEXMK_COMPILE_TIMEOUT|LATEXMK_SHUTDOWN_TIMEOUT|\
    LATEXMK_MAX_UPLOAD_BYTES|LATEXMK_MAX_EXPANDED_BYTES|\
    LATEXMK_MAX_ARTIFACT_BYTES|LATEXMK_MAX_FILES|\
    LATEXMK_MAX_CONCURRENT_COMPILES|LATEXMK_MAX_QUEUED_JOBS|\
    LATEXMK_MAX_LOG_BYTES|LATEXMK_MAX_STATE_BYTES|\
    LATEXMK_MAX_UPLOAD_SESSIONS|LATEXMK_RESULT_RETENTION|\
    LATEXMK_SNAPSHOT_RETENTION|LATEXMK_BLOB_RETENTION|\
    LATEXMK_STATE_SWEEP_INTERVAL) return 0 ;;
    *) return 1 ;;
  esac
}

valid_tuning_assignment() {
  local line="$1" key value inner
  [[ "${line}" == *=* ]] || return 1
  key="${line%%=*}"
  value="${line#*=}"
  is_persistent_tuning_key "${key}" || return 1
  if [[ "${value}" == \"*\" && "${value}" == *\" && ${#value} -ge 2 ]]; then
    inner="${value:1:${#value}-2}"
    [[ "${inner}" != *'"'* && "${inner}" != *'$'* && "${inner}" != *'`'* && "${inner}" != *'\'* && "${inner}" != *$'\n'* ]]
    return
  fi
  if [[ "${value}" == \'*\' && "${value}" == *\' && ${#value} -ge 2 ]]; then
    inner="${value:1:${#value}-2}"
    [[ "${inner}" != *"'"* && "${inner}" != *$'\n'* ]]
    return
  fi
  [[ "${value}" =~ ^[-A-Za-z0-9_./,:+@%]*$ ]]
}

stage_persistent_tuning() {
  local source_file line line_number=0 key
  if [[ -f "${override_file}" ]]; then
    source_file="${override_file}"
  elif [[ -f "${config_file}" ]]; then
    source_file="${config_file}"
  else
    : >"${override_file}.new"
    chmod 600 "${override_file}.new"
    return
  fi
  : >"${override_file}.new"
  while IFS= read -r line || [[ -n "${line}" ]]; do
    line_number=$((line_number + 1))
    if [[ -z "${line}" || "${line}" == \#* ]]; then
      [[ "${source_file}" == "${override_file}" ]] && printf '%s\n' "${line}" >>"${override_file}.new"
      continue
    fi
    if valid_tuning_assignment "${line}"; then
      printf '%s\n' "${line}" >>"${override_file}.new"
    else
      if [[ "${line}" == *=* ]]; then
        key="${line%%=*}"
      else
        key=""
      fi
      if [[ "${source_file}" == "${override_file}" ]] ||
        { [[ "${key}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] && is_persistent_tuning_key "${key}"; }; then
        if [[ "${key}" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]]; then
          die "invalid persistent tuning in ${source_file} at line ${line_number} (key: ${key})"
        fi
        die "invalid persistent tuning in ${source_file} at line ${line_number}"
      fi
    fi
  done <"${source_file}"
  chmod 600 "${override_file}.new"
}

stage_persistent_tuning
cat >"${config_file}.new" <<EOF
PATH="$(escape_env "${tex_bin}:/usr/local/bin:/usr/bin:/bin")"
LATEXMK_TOOLCHAIN_PATH="$(escape_env "${tex_bin}:/usr/local/bin:/usr/bin:/bin")"
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
printf 'REMOTE_LATEXMK_SERVICE_MODE="%s"\n' "${actual_service_mode}" >>"${config_file}.new"

if [[ "${install_systemd}" == true ]]; then
  [[ "${install_root}" != *[[:space:]]* ]] || die "systemd service paths cannot contain whitespace; choose another --install-dir"
  mkdir -p "${HOME}/.config/systemd/user"
  unit_file="${HOME}/.config/systemd/user/remote-latexmk.service"
  unit_stage="${temp_dir}/remote-latexmk.service"
  cat >"${unit_stage}" <<EOF
[Unit]
Description=remote-latexmk private LaTeX compilation server
After=network.target

[Service]
Type=simple
EnvironmentFile=${config_file}
EnvironmentFile=-${override_file}
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

cat >"${install_state}.new" <<EOF
REMOTE_LATEXMK_INSTALL_SCHEMA="1"
REMOTE_LATEXMK_ACTIVE_VERSION="${version}"
REMOTE_LATEXMK_PROFILE="${profile}"
REMOTE_LATEXMK_ENGINES="${engines}"
REMOTE_LATEXMK_LISTEN="$(escape_env "${listen}")"
REMOTE_LATEXMK_SERVICE="${actual_service_mode}"
EOF
chmod 600 "${install_state}.new"

activation_started=true
if [[ "${service_was_running}" == true && -x "${install_root}/bin/remote-latexmkctl" ]]; then
  echo "Stopping the previous server before activation."
  "${install_root}/bin/remote-latexmkctl" stop || die "could not stop the previous server safely"
fi
if [[ "${had_target_release}" == true ]]; then
  [[ ! -e "${target_release_backup}" ]] || die "rollback path already exists: ${target_release_backup}"
  mv "${release_dir}" "${target_release_backup}"
fi
release_replaced=true
mv "${release_dir}.new" "${release_dir}"
ln -sfn "${release_dir}" "${install_root}/current"
ln -sfn "${install_root}/current/remote-latexmk-server" "${install_root}/bin/remote-latexmk-server"
ln -sfn "${install_root}/current/remote-latexmkctl" "${install_root}/bin/remote-latexmkctl"
ln -sfn "${install_root}/current/install-server.sh" "${install_root}/bin/install-server.sh"
mv "${config_file}.new" "${config_file}"
mv "${override_file}.new" "${override_file}"
mv "${install_state}.new" "${install_state}"

if [[ "${install_systemd}" == true ]]; then
  cp "${unit_stage}" "${unit_file}.new"
  chmod 600 "${unit_file}.new"
  mv "${unit_file}.new" "${unit_file}"
  systemctl --user daemon-reload
  if [[ "${existing_installation}" == true && "${systemd_was_enabled}" == true ]]; then
    systemctl --user enable remote-latexmk.service >/dev/null
  elif [[ "${existing_installation}" == true ]]; then
    systemctl --user disable remote-latexmk.service >/dev/null 2>&1 || true
  fi
  if command -v loginctl >/dev/null 2>&1; then
    linger="$(loginctl show-user "${USER:-$(id -un)}" -p Linger --value 2>/dev/null || true)"
    if [[ "${linger}" == no ]]; then
      echo "Note: systemd user lingering is disabled; the service may stop after logout and will not start at boot until an administrator enables lingering." >&2
    fi
  fi
elif [[ -f "${unit_file}" ]]; then
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user disable --now remote-latexmk.service >/dev/null 2>&1 || true
  fi
  rm -f "${unit_file}"
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user daemon-reload >/dev/null 2>&1 || true
  fi
fi

server_started=false
if [[ "${start_server}" == true ]]; then
  "${install_root}/bin/remote-latexmkctl" restart
  parse_listen "${listen}"
  probe_host="${listen_host}"
  case "${probe_host}" in 0.0.0.0) probe_host=127.0.0.1 ;; ::) probe_host=::1 ;; esac
  probe_address="$(format_listen "${probe_host}" "${listen_port}")"
  probe_url="http://${probe_address}"
  probe_ok=false
  for _ in {1..20}; do
    if probe_download "${probe_url}/healthz" "${temp_dir}/healthz" >/dev/null 2>&1; then
      probe_ok=true
      break
    fi
    sleep 0.25
  done
  [[ "${probe_ok}" == true ]] || die "the new server did not pass its health check at ${probe_url}"
  probe_download "${probe_url}/v1/meta" "${temp_dir}/meta.json" >/dev/null 2>&1 || die "the new server metadata endpoint is unavailable at ${probe_url}"
  grep -Eq '"service"[[:space:]]*:[[:space:]]*"remote-latexmk"' "${temp_dir}/meta.json" || die "the new endpoint does not identify as remote-latexmk"
  if [[ "${install_systemd}" == true && "${existing_installation}" == true && "${systemd_was_enabled}" != true ]]; then
    systemctl --user disable remote-latexmk.service >/dev/null
  fi
  server_started=true
fi

activation_started=false

if [[ "${server_started}" == true ]]; then
  install_status="The server is running and passed its health and identity checks."
else
  install_status="The server is installed but not running. Start it after reviewing the service warning above."
fi
parse_listen "${listen}"
if [[ "${listen_host}" == 0.0.0.0 || "${listen_host}" == :: ]]; then
  client_server="SERVER_URL"
  client_url_note="The server listens on ${listen}. Use one real reachable private address from the client; the wildcard bind address is not a client URL."
else
  client_server="http://${listen}"
  client_url_note="Server listen address: ${listen}"
fi

cat <<EOF

========================================================================
 remote-latexmk ${version} is installed
========================================================================
${install_status}
The remote-latexmk API token is also stored on this server at:
  ${token_file}

Show the token again on this server:
  ${install_root}/bin/remote-latexmkctl token

${client_url_note}

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
  npx --yes --ignore-scripts remote-latexmk@${version#v} auth login --server "${client_server}"

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
