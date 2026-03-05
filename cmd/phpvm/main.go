package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

var appVersion = "0.1.0-alpha"

type releaseInfo struct {
	Version string `json:"version"`
}

type releaseIndex map[string]releaseInfo

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printHelp()
		return
	}

	cmd := strings.ToLower(args[0])
	rest := args[1:]

	switch cmd {
	case "help", "h", "--help", "-h":
		printHelp()
	case "version", "v", "--version", "-v":
		fmt.Println("phpvm", appVersion)
	case "list", "ls":
		must(runList())
	case "current", "c":
		must(runCurrent())
	case "available", "a":
		must(runAvailable(rest))
	case "install", "i":
		must(runInstall(rest))
	case "use", "u":
		must(runUse(rest))
	case "remove", "rm":
		must(runRemove(rest))
	default:
		must(fmt.Errorf("unknown command: %s", cmd))
	}
}

func printHelp() {
	fmt.Println("phpvm - cross-platform PHP version manager (Windows/Linux first)")
	fmt.Println("")
	fmt.Println("Usage:")
	fmt.Println("  phpvm install <version>      (alias: i)")
	fmt.Println("  phpvm use <version>          (alias: u)")
	fmt.Println("  phpvm list                   (alias: ls)")
	fmt.Println("  phpvm current                (alias: c)")
	fmt.Println("  phpvm available [major|x.y]  (alias: a)")
	fmt.Println("  phpvm remove <version>       (alias: rm)")
	fmt.Println("  phpvm version                (alias: v)")
	fmt.Println("")
	fmt.Println("Version inputs:")
	fmt.Println("  - exact: 8.2.30")
	fmt.Println("  - major: 8      -> latest stable 8.x")
	fmt.Println("  - major.minor: 8.2 -> latest stable 8.2.x")
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func rootDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = "."
	}
	return filepath.Join(home, ".phpvm")
}

func versionsDir() string { return filepath.Join(rootDir(), "versions") }
func currentLink() string { return filepath.Join(rootDir(), "current") }

func ensureDirs() error {
	return os.MkdirAll(versionsDir(), 0o755)
}

func runList() error {
	if err := ensureDirs(); err != nil {
		return err
	}
	entries, err := os.ReadDir(versionsDir())
	if err != nil {
		return err
	}
	curr, _ := activeVersion()
	versions := make([]string, 0)
	for _, e := range entries {
		if e.IsDir() {
			versions = append(versions, e.Name())
		}
	}
	sort.Slice(versions, func(i, j int) bool { return semverLess(versions[j], versions[i]) })
	if len(versions) == 0 {
		fmt.Println("No PHP versions installed yet.")
		return nil
	}
	for _, v := range versions {
		mark := " "
		if v == curr {
			mark = "*"
		}
		fmt.Printf("%s %s\n", mark, v)
	}
	return nil
}

func runCurrent() error {
	curr, err := activeVersion()
	if err != nil {
		return err
	}
	if curr == "" {
		fmt.Println("No active version")
		return nil
	}
	fmt.Println(curr)
	return nil
}

func activeVersion() (string, error) {
	target, err := os.Readlink(currentLink())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		// Windows junction fallback
		if runtime.GOOS == "windows" {
			if st, statErr := os.Stat(currentLink()); statErr == nil && st.IsDir() {
				return filepath.Base(currentLinkResolved()), nil
			}
		}
		return "", err
	}
	return filepath.Base(target), nil
}

func currentLinkResolved() string {
	p, err := filepath.EvalSymlinks(currentLink())
	if err != nil {
		return currentLink()
	}
	return p
}

func runAvailable(args []string) error {
	filter := ""
	if len(args) > 0 {
		filter = args[0]
	}
	versions, err := fetchAvailable(filter)
	if err != nil {
		return err
	}
	for _, v := range versions {
		fmt.Println(v)
	}
	return nil
}

func runInstall(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: phpvm install <version>")
	}
	if err := ensureDirs(); err != nil {
		return err
	}

	resolved, err := resolveVersion(args[0])
	if err != nil {
		return err
	}
	dst := filepath.Join(versionsDir(), resolved)
	if _, err := os.Stat(dst); err == nil {
		fmt.Println("Already installed:", resolved)
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}

	archive, kind, err := downloadPHPArchive(resolved, dst)
	if err != nil {
		return err
	}

	switch kind {
	case "zip":
		if err := extractZip(archive, dst); err != nil {
			return err
		}
	case "tar.gz":
		if err := extractTarGz(archive, dst); err != nil {
			return err
		}
	}

	bin := phpBinaryPath(dst)
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("installed archive does not contain runnable PHP binary at %s (OS package may differ)", bin)
	}
	fmt.Println("Installed", resolved)
	return nil
}

func runUse(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: phpvm use <version>")
	}
	resolved, err := resolveLocalVersion(args[0])
	if err != nil {
		return err
	}
	target := filepath.Join(versionsDir(), resolved)
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("version not installed: %s", resolved)
	}

	if err := os.RemoveAll(currentLink()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Symlink(target, currentLink()); err != nil {
		if runtime.GOOS == "windows" {
			return fmt.Errorf("failed to create link on windows (run terminal as admin or enable developer mode): %w", err)
		}
		return err
	}

	fmt.Println("Switched to PHP", resolved)
	fmt.Println("Ensure your PATH contains:")
	if runtime.GOOS == "windows" {
		fmt.Println("  %USERPROFILE%\\.phpvm\\current")
	} else {
		fmt.Println("  ~/.phpvm/current/bin")
	}
	return nil
}

func runRemove(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: phpvm remove <version>")
	}
	resolved, err := resolveLocalVersion(args[0])
	if err != nil {
		return err
	}
	curr, _ := activeVersion()
	if curr == resolved {
		return errors.New("cannot remove active version; switch first")
	}
	path := filepath.Join(versionsDir(), resolved)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("version not installed: %s", resolved)
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	fmt.Println("Removed", resolved)
	return nil
}

func resolveVersion(input string) (string, error) {
	input = strings.TrimSpace(input)
	if matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+$`, input); matched {
		return input, nil
	}
	if matched, _ := regexp.MatchString(`^\d+$`, input); matched {
		list, err := fetchAvailable(input)
		if err != nil || len(list) == 0 {
			return "", fmt.Errorf("no stable versions found for major %s", input)
		}
		return list[0], nil
	}
	if matched, _ := regexp.MatchString(`^\d+\.\d+$`, input); matched {
		list, err := fetchAvailable(input)
		if err != nil || len(list) == 0 {
			return "", fmt.Errorf("no stable versions found for %s.x", input)
		}
		return list[0], nil
	}
	return "", fmt.Errorf("invalid version selector: %s", input)
}

func resolveLocalVersion(input string) (string, error) {
	if matched, _ := regexp.MatchString(`^\d+\.\d+\.\d+$`, input); matched {
		return input, nil
	}
	entries, err := os.ReadDir(versionsDir())
	if err != nil {
		return "", err
	}
	candidates := make([]string, 0)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		v := e.Name()
		if input == "" || strings.HasPrefix(v, input+".") || v == input {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no installed versions match: %s", input)
	}
	sort.Slice(candidates, func(i, j int) bool { return semverLess(candidates[j], candidates[i]) })
	return candidates[0], nil
}

func fetchAvailable(filter string) ([]string, error) {
	resp, err := http.Get("https://www.php.net/releases/?json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("php releases endpoint returned %d", resp.StatusCode)
	}
	var idx releaseIndex
	if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(idx))
	for k := range idx {
		if filter == "" || strings.HasPrefix(k, filter+".") || k == filter {
			versions = append(versions, k)
		}
	}
	sort.Slice(versions, func(i, j int) bool { return semverLess(versions[j], versions[i]) })
	return versions, nil
}

func semverLess(a, b string) bool {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func parseSemver(v string) [3]int {
	parts := strings.Split(v, ".")
	out := [3]int{}
	for i := 0; i < len(parts) && i < 3; i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}

func downloadPHPArchive(version, dst string) (path string, kind string, err error) {
	if runtime.GOOS == "windows" {
		fname := fmt.Sprintf("php-%s-nts-Win32-vs17-x64.zip", version)
		url := "https://windows.php.net/downloads/releases/" + fname
		out := filepath.Join(dst, fname)
		if err := downloadFile(url, out); err != nil {
			return "", "", fmt.Errorf("windows package download failed (%s): %w", url, err)
		}
		return out, "zip", nil
	}

	fname := fmt.Sprintf("php-%s.tar.gz", version)
	url := "https://www.php.net/distributions/" + fname
	out := filepath.Join(dst, fname)
	if err := downloadFile(url, out); err != nil {
		return "", "", fmt.Errorf("linux package download failed (%s): %w", url, err)
	}
	return out, "tar.gz", nil
}

func downloadFile(url, out string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func extractZip(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		path := filepath.Join(dst, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(path)
		if err != nil {
			in.Close()
			return err
		}
		_, err = io.Copy(out, in)
		in.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dst, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}

func phpBinaryPath(versionDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(versionDir, "php.exe")
	}
	return filepath.Join(versionDir, "bin", "php")
}
