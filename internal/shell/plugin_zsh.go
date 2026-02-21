package shell

// ZshPlugin is the zsh plugin source. It installs a preexec hook that logs
// every command with an epoch timestamp to the handoff commands log, but only
// when a handoff session is active.
const ZshPlugin = `# handoff shell plugin — auto-generated, do not edit manually
# Source this file from your ~/.zshrc:
#   source ~/.config/handoff/handoff.plugin.zsh

_handoff_log_file="${XDG_DATA_HOME:-$HOME/.local/share}/handoff/commands.log"
_handoff_session_file="${XDG_DATA_HOME:-$HOME/.local/share}/handoff/session.json"

_handoff_preexec() {
  # Only log when a session is active.
  [[ -f "$_handoff_session_file" ]] || return
  local cmd="$1"
  # Skip handoff start/stop noise.
  [[ "$cmd" =~ ^[[:space:]]*(.*\/)?handoff[[:space:]]+(start|stop) ]] && return
  printf '%s\t%s\n' "$(date +%s)" "$cmd" >> "$_handoff_log_file"
}

autoload -Uz add-zsh-hook
add-zsh-hook preexec _handoff_preexec
`
