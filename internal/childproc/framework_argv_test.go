package childproc

import (
	"reflect"
	"testing"
)

func TestInjectFrameworkArgsLeavesUnknownCommands(t *testing.T) {
	argv := []string{"node", "server.js"}
	got := InjectFrameworkArgs(argv, 5555, "127.0.0.1")
	if !reflect.DeepEqual(got, argv) {
		t.Fatalf("got %v, want unchanged", got)
	}
}

func TestInjectFrameworkArgsVite(t *testing.T) {
	got := InjectFrameworkArgs([]string{"vite"}, 5555, "127.0.0.1")
	want := []string{"vite", "--port", "5555", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsNPXVite(t *testing.T) {
	got := InjectFrameworkArgs([]string{"npx", "--yes", "vite"}, 4000, "127.0.0.1")
	want := []string{"npx", "--yes", "vite", "--port", "4000", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsYarnImplicitVite(t *testing.T) {
	got := InjectFrameworkArgs([]string{"yarn", "vite"}, 4001, "127.0.0.1")
	want := []string{"yarn", "vite", "--port", "4001", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsYarnDlxVite(t *testing.T) {
	got := InjectFrameworkArgs([]string{"yarn", "dlx", "vite"}, 4002, "127.0.0.1")
	want := []string{"yarn", "dlx", "vite", "--port", "4002", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsPnpmExecVite(t *testing.T) {
	got := InjectFrameworkArgs([]string{"pnpm", "exec", "vite"}, 4010, "127.0.0.1")
	want := []string{"pnpm", "exec", "vite", "--port", "4010", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsBunxVite(t *testing.T) {
	got := InjectFrameworkArgs([]string{"bunx", "vite"}, 4011, "127.0.0.1")
	want := []string{"bunx", "vite", "--port", "4011", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsNoDupWhenPortPresentStillAddsStrictPort(t *testing.T) {
	got := InjectFrameworkArgs([]string{"vite", "--port", "9999"}, 5555, "127.0.0.1")
	want := []string{"vite", "--port", "9999", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsNoDupWhenHostPresent(t *testing.T) {
	got := InjectFrameworkArgs([]string{"vite", "--host", "0.0.0.0"}, 5555, "127.0.0.1")
	want := []string{"vite", "--host", "0.0.0.0", "--port", "5555", "--strictPort"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsPortEqualsFormStillAddsStrictPort(t *testing.T) {
	got := InjectFrameworkArgs([]string{"vite", "--port=3000"}, 5555, "127.0.0.1")
	want := []string{"vite", "--port=3000", "--strictPort", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsExpoHost(t *testing.T) {
	got := InjectFrameworkArgs([]string{"expo", "start"}, 8081, "127.0.0.1")
	want := []string{"expo", "start", "--port", "8081", "--host", "localhost"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestInjectFrameworkArgsAstroNoStrictPort(t *testing.T) {
	got := InjectFrameworkArgs([]string{"astro", "dev"}, 4321, "127.0.0.1")
	want := []string{"astro", "dev", "--port", "4321", "--host", "127.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
