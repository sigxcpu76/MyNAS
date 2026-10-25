//go:build ignore

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorCyan   = "\033[36m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
)

func main() {
	skipBuf := flag.Bool("skip-buf", false, "Skip protobuf generation with buf")
	skipWeb := flag.Bool("skip-web", false, "Skip Vue.js frontend build with npm")
	skipGo := flag.Bool("skip-go", false, "Skip Go binary compilation")
	outputBin := flag.String("output", "bin/nasctl", "Output path for the compiled Go binary")
	verbose := flag.Bool("v", false, "Enable verbose output from underlying tools")
	flag.Parse()

	rootDir, err := findRepoRoot()
	if err != nil {
		log.Fatalf("%sError:%s failed to determine repository root: %v", colorRed, colorReset, err)
	}

	setupEnvironment(rootDir)

	startTime := time.Now()
	fmt.Printf("\n%s%s=== NAS Platform Build & Code Generation Pipeline ===%s\n", colorBold, colorCyan, colorReset)
	fmt.Printf("Repository root: %s\n\n", rootDir)

	totalSteps := 3
	currentStep := 1

	// 1. Buf Operations
	if !*skipBuf {
		fmt.Printf("%s[%d/%d]%s %sRunning Buf Operations (protobuf code generation)...%s\n",
			colorBold, currentStep, totalSteps, colorReset, colorCyan, colorReset)
		t0 := time.Now()
		if err := runBufGenerate(rootDir, *verbose); err != nil {
			log.Fatalf("\n%s✖ Buf operations failed:%s %v", colorRed, colorReset, err)
		}
		fmt.Printf("%s✓ Buf generation completed successfully%s (elapsed: %v)\n\n", colorGreen, colorReset, time.Since(t0).Round(time.Millisecond))
	} else {
		fmt.Printf("%s[%d/%d]%s %sSkipping Buf operations%s\n\n", colorBold, currentStep, totalSteps, colorReset, colorYellow, colorReset)
	}
	currentStep++

	// 2. NPM Build Operations
	if !*skipWeb {
		fmt.Printf("%s[%d/%d]%s %sRunning Frontend Build Operations (npm run build)...%s\n",
			colorBold, currentStep, totalSteps, colorReset, colorCyan, colorReset)
		t0 := time.Now()
		if err := runNpmBuild(rootDir, *verbose); err != nil {
			log.Fatalf("\n%s✖ NPM build operations failed:%s %v", colorRed, colorReset, err)
		}
		fmt.Printf("%s✓ Frontend production build completed successfully%s (elapsed: %v)\n\n", colorGreen, colorReset, time.Since(t0).Round(time.Millisecond))
	} else {
		fmt.Printf("%s[%d/%d]%s %sSkipping Frontend build operations%s\n\n", colorBold, currentStep, totalSteps, colorReset, colorYellow, colorReset)
	}
	currentStep++

	// 3. Golang Build Operations
	if !*skipGo {
		fmt.Printf("%s[%d/%d]%s %sRunning Golang Build Operations (go build -o %s ./cmd/nasctl)...%s\n",
			colorBold, currentStep, totalSteps, colorReset, colorCyan, *outputBin, colorReset)
		t0 := time.Now()
		if err := runGoBuild(rootDir, *outputBin, *verbose); err != nil {
			log.Fatalf("\n%s✖ Golang build operations failed:%s %v", colorRed, colorReset, err)
		}
		fmt.Printf("%s✓ Single binary distribution built at %s%s (elapsed: %v)\n\n", colorGreen, *outputBin, colorReset, time.Since(t0).Round(time.Millisecond))
	} else {
		fmt.Printf("%s[%d/%d]%s %sSkipping Golang build operations%s\n\n", colorBold, currentStep, totalSteps, colorReset, colorYellow, colorReset)
	}

	fmt.Printf("%s%s✓ Pipeline completed successfully in %v%s\n\n",
		colorBold, colorGreen, time.Since(startTime).Round(time.Millisecond), colorReset)
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return os.Getwd()
}

func setupEnvironment(rootDir string) {
	pathEnv := os.Getenv("PATH")
	var extraPaths []string

	homeDir := os.Getenv("HOME")
	if homeDir != "" {
		extraPaths = append(extraPaths, filepath.Join(homeDir, "go", "bin"))
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		extraPaths = append(extraPaths, filepath.Join(gopath, "bin"))
	}
	extraPaths = append(extraPaths, "/usr/local/bin", "/usr/bin", "/bin")

	for _, p := range extraPaths {
		if !strings.Contains(pathEnv, p) {
			pathEnv = p + string(os.PathListSeparator) + pathEnv
		}
	}
	_ = os.Setenv("PATH", pathEnv)
}

func runBufGenerate(rootDir string, verbose bool) error {
	bufBin, err := exec.LookPath("buf")
	if err != nil {
		return fmt.Errorf("buf binary not found in PATH: please install buf (https://buf.build/docs/installation)")
	}

	cmd := exec.Command(bufBin, "generate")
	cmd.Dir = rootDir
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func runNpmBuild(rootDir string, verbose bool) error {
	webDir := filepath.Join(rootDir, "web")
	if _, err := os.Stat(webDir); err != nil {
		return fmt.Errorf("web directory not found at %s: %w", webDir, err)
	}

	// Verify or install node_modules
	nodeModulesDir := filepath.Join(webDir, "node_modules")
	if _, err := os.Stat(nodeModulesDir); os.IsNotExist(err) {
		fmt.Printf("  %s→ node_modules not found, executing 'npm install'...%s\n", colorYellow, colorReset)
		npmInstall := exec.Command("npm", "install")
		npmInstall.Dir = webDir
		npmInstall.Stdout = os.Stdout
		npmInstall.Stderr = os.Stderr
		if err := npmInstall.Run(); err != nil {
			return fmt.Errorf("npm install failed: %w", err)
		}
	}

	cmd := exec.Command("npm", "run", "build")
	cmd.Dir = webDir
	if verbose {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("npm run build failed: %w", err)
	}

	// Verify build artifact
	indexFile := filepath.Join(webDir, "dist", "index.html")
	if _, err := os.Stat(indexFile); err != nil {
		return fmt.Errorf("web build verification failed: %s does not exist", indexFile)
	}
	return nil
}

func runGoBuild(rootDir, outputBin string, verbose bool) error {
	outPath := outputBin
	if !filepath.IsAbs(outPath) {
		outPath = filepath.Join(rootDir, outPath)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go binary not found in PATH")
	}

	cmd := exec.Command(goBin, "build", "-o", outPath, "./cmd/nasctl")
	cmd.Dir = rootDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	// Verify binary was generated
	if fi, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("binary verification failed: %s was not created: %w", outPath, err)
	} else {
		_ = os.Chmod(outPath, 0755)
		fmt.Printf("  %s→ Binary size: %s%s\n", colorCyan, formatBytes(fi.Size()), colorReset)
	}
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
