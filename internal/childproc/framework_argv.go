package childproc

import (
	"path/filepath"
	"strconv"
	"strings"
)

// frameworkMeta describes dev servers that ignore PORT (and often HOST) and
// need explicit CLI flags instead.
type frameworkMeta struct {
	strictPort bool
}

// See vercel-labs/portless injectFrameworkFlags (cli-utils.ts).
var frameworksNeedingPort = map[string]frameworkMeta{
	"vite":         {strictPort: true},
	"vp":           {strictPort: true}, // VitePress CLI binary
	"react-router": {strictPort: true},
	"astro":        {strictPort: false},
	"ng":           {strictPort: false},
	"react-native": {strictPort: false},
	"expo":         {strictPort: false},
}

// packageRunners maps a runner binary to subcommands that must appear before
// the framework binary (empty slice: runner invokes the framework directly,
// e.g. npx vite). We intentionally do not treat "run" as a runner subcommand:
// yarn run vite / pnpm run vite execute package.json scripts and are not
// expanded here (the child process is yarn/pnpm, not vite).
var packageRunners = map[string][]string{
	"npx":  {},
	"bunx": {},
	"pnpx": {},
	"yarn": {"dlx", "exec"},
	"pnpm": {"dlx", "exec"},
}

// InjectFrameworkArgs returns a copy of argv with --port / --strictPort / --host
// appended when the command targets a dev server that ignores PORT (Vite,
// Astro, Expo, etc.). bindHost is the listen address (e.g. 127.0.0.1); expo
// uses --host localhost to match common Metro expectations.
func InjectFrameworkArgs(argv []string, port int, bindHost string) []string {
	fw := findFrameworkBasename(argv)
	if fw == "" {
		return argv
	}
	cfg := frameworksNeedingPort[fw]

	out := append([]string(nil), argv...)

	if !argvHasFlag(out, "--port") {
		out = append(out, "--port", strconv.Itoa(port))
	}
	// strictPort is independent: user-supplied --port still needs --strictPort
	// so Vite cannot silently hop to another port and desync from the proxy.
	if cfg.strictPort && !argvHasFlag(out, "--strictPort") {
		out = append(out, "--strictPort")
	}

	if !argvHasFlag(out, "--host") {
		host := bindHost
		if host == "" {
			host = "127.0.0.1"
		}
		if fw == "expo" {
			host = "localhost"
		}
		out = append(out, "--host", host)
	}

	return out
}

func argvHasFlag(argv []string, name string) bool {
	for _, a := range argv {
		if a == name || strings.HasPrefix(a, name+"=") {
			return true
		}
	}
	return false
}

func findFrameworkBasename(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	first := filepath.Base(argv[0])
	if _, ok := frameworksNeedingPort[first]; ok {
		return first
	}
	subcommands, ok := packageRunners[first]
	if !ok {
		return ""
	}
	i := 1
	if len(subcommands) > 0 {
		for i < len(argv) && strings.HasPrefix(argv[i], "-") {
			i++
		}
		if i >= len(argv) {
			return ""
		}
		if !containsString(subcommands, argv[i]) {
			name := filepath.Base(argv[i])
			if _, ok := frameworksNeedingPort[name]; ok {
				return name
			}
			return ""
		}
		i++
	}
	for i < len(argv) && strings.HasPrefix(argv[i], "-") {
		i++
	}
	if i >= len(argv) {
		return ""
	}
	name := filepath.Base(argv[i])
	if _, ok := frameworksNeedingPort[name]; ok {
		return name
	}
	return ""
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
