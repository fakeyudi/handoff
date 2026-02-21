package shell

// BashPlugin is the bash plugin source. It prepends a DEBUG trap that logs
// commands with epoch timestamps when a handoff session is active.
const BashPlugin = `# handoff shell plugin — auto-generated, do not edit manually
# Source this file from your ~/.bashrc:
#   source ~/.config/handoff/handoff.plugin.bash

_handoff_log_file="${XDG_DATA_HOME:-$HOME/.local/share}/handoff/commands.log"
_handoff_session_file="${XDG_DATA_HOME:-$HOME/.local/share}/handoff/session.json"

_handoff_preexec() {
  [[ -f "$_handoff_session_file" ]] || return
  local cmd="$BASH_COMMAND"
  [[ "$cmd" =~ ^[[:space:]]*(.*\/)?handoff[[:space:]]+(start|stop) ]] && return
  printf '%s\t%s\n' "$(date +%s)" "$cmd" >> "$_handoff_log_file"
}

trap '_handoff_preexec' DEBUG
`
