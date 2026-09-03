package cli

import (
	"fmt"
	"strings"
)

// The command tree, used to generate completions. Keep in sync with Main.
var commandTree = map[string][]string{
	"":           {"auth", "leads", "vocabulary", "health", "completion", "version", "help"},
	"auth":       {"check"},
	"leads":      {"submit"},
	"completion": {"bash", "zsh", "fish"},
	"help":       {"auth", "leads", "vocabulary", "health", "completion", "exit-codes"},
}

var globalFlagNames = []string{
	"--base-url", "--json", "--timeout", "--max-attempts", "--retry-backoff",
	"--api-key-file", "--user-agent", "--request-id", "--quiet",
}

var submitFlagNames = []string{
	"--json-file", "--external-id", "--first-name", "--last-name", "--phone",
	"--email", "--street-address", "--city", "--state", "--zip", "--country",
	"--project-area", "--project-service", "--project-material",
	"--project-details", "--cost", "--extra", "--idempotency-key",
	"--no-idempotency-key", "--dry-run",
}

func (a *app) completion(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		a.printCompletionHelp()
		if len(args) == 0 {
			return ExitUsage
		}
		return ExitOK
	}
	switch args[0] {
	case "bash":
		fmt.Fprint(a.stdout, bashCompletion())
	case "zsh":
		fmt.Fprint(a.stdout, zshCompletion())
	case "fish":
		fmt.Fprint(a.stdout, fishCompletion())
	default:
		return a.usage(fmt.Sprintf("completion: unsupported shell %q (bash, zsh, fish)", args[0]))
	}
	return ExitOK
}

func bashCompletion() string {
	return fmt.Sprintf(`# bash completion for hellojade — source <(hellojade completion bash)
_hellojade() {
  local cur prev words cword
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  local globals="%s"
  local cmd="" sub=""
  local i
  for ((i=1; i < COMP_CWORD; i++)); do
    case "${COMP_WORDS[i]}" in
      --*) continue ;;
      *) if [ -z "$cmd" ]; then cmd="${COMP_WORDS[i]}"; elif [ -z "$sub" ]; then sub="${COMP_WORDS[i]}"; fi ;;
    esac
  done
  case "$prev" in
    --api-key-file|--json-file) COMPREPLY=( $(compgen -f -- "$cur") ); return ;;
    --project-service) COMPREPLY=( $(compgen -W "replacement repair remodel maintain" -- "$cur") ); return ;;
  esac
  if [ -z "$cmd" ]; then
    COMPREPLY=( $(compgen -W "%s $globals" -- "$cur") ); return
  fi
  case "$cmd" in
    auth)       [ -z "$sub" ] && COMPREPLY=( $(compgen -W "check $globals" -- "$cur") ) || COMPREPLY=( $(compgen -W "$globals" -- "$cur") ) ;;
    leads)      if [ -z "$sub" ]; then COMPREPLY=( $(compgen -W "submit $globals" -- "$cur") ); else COMPREPLY=( $(compgen -W "%s $globals" -- "$cur") ); fi ;;
    completion) COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") ) ;;
    help)       COMPREPLY=( $(compgen -W "%s" -- "$cur") ) ;;
    *)          COMPREPLY=( $(compgen -W "$globals" -- "$cur") ) ;;
  esac
}
complete -F _hellojade hellojade
`,
		strings.Join(globalFlagNames, " "),
		strings.Join(commandTree[""], " "),
		strings.Join(submitFlagNames, " "),
		strings.Join(commandTree["help"], " "),
	)
}

func zshCompletion() string {
	var b strings.Builder
	b.WriteString("#compdef hellojade\n# zsh completion for hellojade — hellojade completion zsh > \"${fpath[1]}/_hellojade\"\n\n")
	b.WriteString("_hellojade() {\n  local -a globals\n  globals=(\n")
	for _, f := range globalFlagNames {
		fmt.Fprintf(&b, "    '%s[global option]'\n", f)
	}
	b.WriteString("  )\n  local -a submit_flags\n  submit_flags=(\n")
	for _, f := range submitFlagNames {
		switch f {
		case "--json-file", "--api-key-file":
			fmt.Fprintf(&b, "    '%s[file]:file:_files'\n", f)
		case "--project-service":
			fmt.Fprintf(&b, "    '%s[service]:service:(replacement repair remodel maintain)'\n", f)
		case "--extra":
			fmt.Fprintf(&b, "    '*%s[key=value]:pair:'\n", f)
		default:
			fmt.Fprintf(&b, "    '%s[lead field]:value:'\n", f)
		}
	}
	b.WriteString("  )\n")
	b.WriteString(`  _arguments -C $globals '1:command:->cmd' '*::arg:->args'
  case $state in
    cmd)
      _values 'command' auth leads vocabulary health completion version help ;;
    args)
      case $words[1] in
        auth)       _values 'auth command' check ;;
        leads)
          if (( CURRENT == 2 )); then _values 'leads command' submit
          else _arguments $globals $submit_flags; fi ;;
        completion) _values 'shell' bash zsh fish ;;
        help)       _values 'topic' auth leads vocabulary health completion exit-codes ;;
        *)          _arguments $globals ;;
      esac ;;
  esac
}
_hellojade "$@"
`)
	return b.String()
}

func fishCompletion() string {
	var b strings.Builder
	b.WriteString("# fish completion for hellojade — hellojade completion fish > ~/.config/fish/completions/hellojade.fish\n\n")
	b.WriteString("function __hj_no_cmd\n  not __fish_seen_subcommand_from auth leads vocabulary health completion version help\nend\n\n")
	desc := map[string]string{
		"auth": "prove the API key", "leads": "submit leads", "vocabulary": "accepted project_area terms",
		"health": "liveness of the intake edge", "completion": "shell completion script", "version": "print version", "help": "help",
	}
	for _, c := range commandTree[""] {
		fmt.Fprintf(&b, "complete -c hellojade -n __hj_no_cmd -f -a %s -d '%s'\n", c, desc[c])
	}
	b.WriteString("complete -c hellojade -n '__fish_seen_subcommand_from auth' -f -a check -d 'key check; stores nothing'\n")
	b.WriteString("complete -c hellojade -n '__fish_seen_subcommand_from leads' -f -a submit -d 'post one lead'\n")
	b.WriteString("complete -c hellojade -n '__fish_seen_subcommand_from completion' -f -a 'bash zsh fish'\n")
	b.WriteString("complete -c hellojade -n '__fish_seen_subcommand_from help' -f -a 'auth leads vocabulary health completion exit-codes'\n")
	for _, f := range globalFlagNames {
		name := strings.TrimPrefix(f, "--")
		switch name {
		case "json", "quiet":
			fmt.Fprintf(&b, "complete -c hellojade -l %s -d 'global option'\n", name)
		case "api-key-file":
			fmt.Fprintf(&b, "complete -c hellojade -l %s -r -F -d 'read the API key from a file'\n", name)
		default:
			fmt.Fprintf(&b, "complete -c hellojade -l %s -r -d 'global option'\n", name)
		}
	}
	for _, f := range submitFlagNames {
		name := strings.TrimPrefix(f, "--")
		cond := "-n '__fish_seen_subcommand_from submit'"
		switch name {
		case "dry-run", "no-idempotency-key":
			fmt.Fprintf(&b, "complete -c hellojade %s -l %s -d 'submit option'\n", cond, name)
		case "json-file":
			fmt.Fprintf(&b, "complete -c hellojade %s -l %s -r -F -d 'lead JSON file or -'\n", cond, name)
		case "project-service":
			fmt.Fprintf(&b, "complete -c hellojade %s -l %s -r -f -a 'replacement repair remodel maintain'\n", cond, name)
		default:
			fmt.Fprintf(&b, "complete -c hellojade %s -l %s -r -d 'lead field'\n", cond, name)
		}
	}
	return b.String()
}
