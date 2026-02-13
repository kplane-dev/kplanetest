package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	defaultVersion = "v0.0.4"
	modulePath     = "github.com/kplane-dev/apiserver"
	binaryName     = "kplane-apiserver"
)

func main() {
	if len(os.Args) < 2 {
		failf("usage: setup-kplanetest use [-p path] [version]")
	}

	switch os.Args[1] {
	case "use":
		runUse(os.Args[2:])
	default:
		failf("unknown command %q (expected: use)", os.Args[1])
	}
}

func runUse(args []string) {
	fs := flag.NewFlagSet("use", flag.ExitOnError)
	printMode := fs.String("p", "", "print mode (supported: path)")
	if err := fs.Parse(args); err != nil {
		failf("parse args: %v", err)
	}
	if *printMode != "" && *printMode != "path" {
		failf("unsupported -p value %q (supported: path)", *printMode)
	}

	version := defaultVersion
	if fs.NArg() > 1 {
		failf("too many arguments")
	}
	if fs.NArg() == 1 {
		version = fs.Arg(0)
	}
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	assetsDir, err := ensureAssets(version)
	if err != nil {
		failf("%v", err)
	}

	if *printMode == "path" {
		fmt.Println(assetsDir)
		return
	}
	fmt.Printf("kplanetest assets ready at %s\n", assetsDir)
	fmt.Printf("export KPLANETEST_ASSETS=%q\n", assetsDir)
}

func ensureAssets(version string) (string, error) {
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve user cache dir: %w", err)
	}
	assetsDir := filepath.Join(cacheRoot, "kplanetest", "apiserver", version, runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(assetsDir, 0o755); err != nil {
		return "", fmt.Errorf("create assets dir: %w", err)
	}

	out := filepath.Join(assetsDir, binaryName)
	if info, err := os.Stat(out); err == nil && !info.IsDir() {
		return assetsDir, nil
	}

	moduleDir, err := resolveModuleDir(version)
	if err != nil {
		return "", err
	}

	build := exec.Command("go", "build", "-o", out, "./cmd/apiserver")
	build.Dir = moduleDir
	rawBuild, err := build.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build %s@%s: %w (%s)", modulePath, version, err, strings.TrimSpace(string(rawBuild)))
	}
	if err := os.Chmod(out, 0o755); err != nil {
		return "", fmt.Errorf("chmod output binary: %w", err)
	}
	return assetsDir, nil
}

func resolveModuleDir(version string) (string, error) {
	cmd := exec.Command("go", "mod", "download", "-json", modulePath+"@"+version)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve module dir for %s@%s: %w (%s)", modulePath, version, err, strings.TrimSpace(string(raw)))
	}
	var info struct {
		Dir string
	}
	if err := json.Unmarshal(raw, &info); err != nil {
		return "", fmt.Errorf("parse module metadata: %w", err)
	}
	info.Dir = strings.TrimSpace(info.Dir)
	if info.Dir == "" {
		return "", fmt.Errorf("module metadata did not include a dir")
	}
	return info.Dir, nil
}

func failf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
