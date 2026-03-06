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
	"time"
)

var appVersion = "0.1.1-alpha"

const repoSlug = "laurovitor/phpvm"

type releaseInfo struct {
	Version string `json:"version"`
}

type releaseIndex map[string]releaseInfo

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}
var verbose bool

func parseGlobalFlags(args []string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		switch a {
		case "--verbose", "-V", "--log":
			verbose = true
		default:
			out = append(out, a)
		}
	}
	return out
}

func logf(format string, a ...any) {
	if verbose {
		fmt.Printf("[verbose] "+format+"\n", a...)
	}
}

func main() {
	args := parseGlobalFlags(os.Args[1:])
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
	case "list", "ls", "l":
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
	case "doctor", "d":
		must(runDoctor())
	case "selfupdate", "su":
		must(runSelfUpdate())
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
	fmt.Println("  phpvm remove <version> [--force] (alias: rm)")
	fmt.Println("  phpvm version                (alias: v)")
	fmt.Println("  phpvm doctor                 (alias: d)")
	fmt.Println("  phpvm selfupdate             (alias: su)")
	fmt.Println("Global flags: --verbose | -V | --log (can be placed anywhere)")
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

func versionsDir() string        { return filepath.Join(rootDir(), "versions") }
func currentLink() string        { return filepath.Join(rootDir(), "current") }
func currentVersionFile() string { return filepath.Join(rootDir(), "current.version") }

func ensureDirs() error {
	if err := os.MkdirAll(versionsDir(), 0o755); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(rootDir(), "bin"), 0o755)
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
		if !e.IsDir() {
			continue
		}
		v := e.Name()
		if _, err := os.Stat(phpBinaryPath(filepath.Join(versionsDir(), v))); err != nil {
			continue
		}
		versions = append(versions, v)
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
	if b, err := os.ReadFile(currentVersionFile()); err == nil {
		v := strings.TrimSpace(string(b))
		if v != "" {
			return v, nil
		}
	}
	target, err := os.Readlink(currentLink())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
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

	fmt.Println("[1/5] Resolving version...")
	resolved, err := resolveVersion(args[0])
	if err != nil {
		return err
	}
	dst := filepath.Join(versionsDir(), resolved)
	if _, err := os.Stat(dst); err == nil {
		if _, err2 := os.Stat(phpBinaryPath(dst)); err2 == nil {
			fmt.Println("Already installed:", resolved)
			return nil
		}
		_ = os.RemoveAll(dst)
	}

	tmpRoot := filepath.Join(rootDir(), "tmp")
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return err
	}
	stage := filepath.Join(tmpRoot, "install-"+resolved+"-"+fmt.Sprint(time.Now().UnixNano()))
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(stage)

	fmt.Println("[2/5] Downloading package...")
	archive, kind, err := downloadPHPArchive(resolved, stage)
	if err != nil {
		return err
	}

	extractDir := filepath.Join(stage, "_extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	fmt.Println("[3/5] Extracting package...")
	switch kind {
	case "zip":
		if err := extractZip(archive, extractDir); err != nil {
			return err
		}
	case "tar.gz":
		if err := extractTarGz(archive, extractDir); err != nil {
			return err
		}
	}

	fmt.Println("[4/5] Validating PHP binary...")
	phpDir, err := findPHPDir(extractDir)
	if err != nil {
		return fmt.Errorf("installed archive does not contain runnable PHP binary. On Linux, php.net tarballs are source builds; prebuilt Linux binaries are not wired yet")
	}
	finalTmp := filepath.Join(stage, "final")
	if err := os.MkdirAll(finalTmp, 0o755); err != nil {
		return err
	}
	if err := moveDirContents(phpDir, finalTmp); err != nil {
		return err
	}
	if _, err := os.Stat(phpBinaryPath(finalTmp)); err != nil {
		return fmt.Errorf("installed archive does not contain runnable PHP binary at %s", phpBinaryPath(finalTmp))
	}

	if err := os.RemoveAll(dst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(finalTmp, dst); err != nil {
		if err := copyDir(finalTmp, dst); err != nil {
			return err
		}
	}

	if _, err := os.Stat(phpBinaryPath(dst)); err != nil {
		_ = os.RemoveAll(dst)
		return fmt.Errorf("install aborted: target does not contain php binary")
	}
	fmt.Println("[5/5] Finalizing install...")
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

	if err := setCurrentTarget(target); err != nil {
		return err
	}
	_ = os.WriteFile(currentVersionFile(), []byte(resolved+"\n"), 0o644)

	bin := phpBinaryPath(target)
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("switched but php binary not found at %s", bin)
	}

	fmt.Println("Switched to PHP", resolved)
	fmt.Println("Ensure your PATH contains:")
	if runtime.GOOS == "windows" {
		fmt.Println("  USERPROFILE\\.phpvm\\current")
	} else {
		fmt.Println("  ~/.phpvm/current/bin")
	}
	return nil
}

func setCurrentTarget(target string) error {
	if err := os.RemoveAll(currentLink()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Symlink(target, currentLink()); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}
	// Windows fallback without symlink privilege: copy target to current
	if err := copyDir(target, currentLink()); err != nil {
		return fmt.Errorf("failed to activate version on windows (symlink denied and copy fallback failed): %w", err)
	}
	return nil
}

func runDoctor() error {
	if err := ensureDirs(); err != nil {
		return err
	}
	fmt.Println("phpvm doctor")
	fmt.Println("-----------")
	fmt.Println("Root:", rootDir())
	fmt.Println("Versions:", versionsDir())

	curr, _ := activeVersion()
	if curr == "" {
		fmt.Println("Active version: none")
	} else {
		fmt.Println("Active version:", curr)
	}

	path := os.Getenv("PATH")
	if runtime.GOOS == "windows" {
		want := filepath.Clean(currentLink())
		ok := strings.Contains(strings.ToLower(path), strings.ToLower(want))
		fmt.Println("PATH has phpvm current:", ok)
		if !ok {
			fmt.Println("Add this to PATH:")
			fmt.Println(" ", want)
			fmt.Println("PowerShell (current user):")
			fmt.Println("  [Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\",\"User\") + \";\" + \"$HOME\\.phpvm\\current\", \"User\")")
		}
	} else {
		want := filepath.Join(currentLink(), "bin")
		ok := strings.Contains(path, want)
		fmt.Println("PATH has phpvm current/bin:", ok)
		if !ok {
			fmt.Println("Add one of the snippets below to your shell profile:")
			fmt.Println(" bash/zsh: export PATH=\"$HOME/.phpvm/current/bin:$PATH\"")
		}
	}

	fmt.Println("Composer note: composer follows the active php when `php` resolves to phpvm current first in PATH")
	return nil
}

func runSelfUpdate() error {
	rel, err := fetchLatestRelease()
	if err != nil {
		return err
	}
	asset := "phpvm-windows-amd64.zip"
	if runtime.GOOS != "windows" {
		asset = "phpvm-linux-arm64"
	}
	var url string
	for _, a := range rel.Assets {
		if a.Name == asset || (runtime.GOOS == "windows" && a.Name == "phpvm.exe") {
			url = a.URL
			break
		}
	}
	if url == "" {
		return fmt.Errorf("no compatible asset found in latest release %s", rel.TagName)
	}
	if runtime.GOOS == "windows" {
		tmp := filepath.Join(os.TempDir(), "phpvm-update.zip")
		if strings.HasSuffix(url, ".exe") {
			tmp = filepath.Join(os.TempDir(), "phpvm-new.exe")
		}
		if err := downloadFile(url, tmp); err != nil {
			return err
		}
		fmt.Println("Downloaded update to:", tmp)
		fmt.Println("On Windows, replace phpvm.exe manually (cannot overwrite running executable).")
		return nil
	}
	target, _ := os.Executable()
	tmp := target + ".new"
	if err := downloadFile(url, tmp); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	if err := os.Rename(tmp, target); err != nil {
		return err
	}
	fmt.Println("Updated to", rel.TagName)
	return nil
}

func fetchLatestRelease() (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repoSlug)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("no public release found yet (latest release endpoint returned 404)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases endpoint returned %d", resp.StatusCode)
	}
	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func runRemove(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: phpvm remove <version> [--force]")
	}
	force := false
	for _, a := range args[1:] {
		if a == "--force" || a == "-f" {
			force = true
		}
	}
	resolved, err := resolveLocalVersion(args[0])
	if err != nil {
		return err
	}
	curr, _ := activeVersion()
	if curr == resolved && !force {
		return errors.New("cannot remove active version; switch first or use --force")
	}
	path := filepath.Join(versionsDir(), resolved)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("version not installed: %s", resolved)
	}
	if err := os.RemoveAll(path); err != nil {
		return err
	}
	if curr == resolved {
		_ = os.RemoveAll(currentLink())
		_ = os.Remove(currentVersionFile())
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
	filter = strings.TrimSpace(filter)
	if filter != "" {
		if regexp.MustCompile(`^\d+$`).MatchString(filter) || regexp.MustCompile(`^\d+\.\d+$`).MatchString(filter) {
			v, err := fetchLatestForTrack(filter)
			if err != nil {
				return nil, err
			}
			if v == "" {
				return []string{}, nil
			}
			return []string{v}, nil
		}
	}

	resp, err := httpClient.Get("https://www.php.net/releases/?json")
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
	for k, ri := range idx {
		v := k
		if ri.Version != "" {
			v = ri.Version
		}
		if !isStableVersion(v) {
			continue
		}
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool { return semverLess(versions[j], versions[i]) })
	return dedupe(versions), nil
}

func fetchLatestForTrack(track string) (string, error) {
	url := "https://www.php.net/releases/index.php?json&version=" + track
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("php releases endpoint returned %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	if v, ok := raw["version"].(string); ok && isStableVersion(v) {
		return v, nil
	}
	for _, val := range raw {
		if m, ok := val.(map[string]any); ok {
			if v, ok := m["version"].(string); ok && isStableVersion(v) {
				return v, nil
			}
		}
	}
	return "", nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
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
		fname, err := resolveWindowsZipFromIndex(version)
		if err != nil {
			return "", "", err
		}
		bases := []string{
			"https://windows.php.net/downloads/releases/",
			"https://windows.php.net/downloads/releases/archives/",
		}
		var lastErr error
		for _, b := range bases {
			url := b + fname
			out := filepath.Join(dst, fname)
			if err := downloadFile(url, out); err == nil {
				return out, "zip", nil
			}
			lastErr = err
		}
		return "", "", fmt.Errorf("windows package download failed for %s: %w", fname, lastErr)
	}

	fname := fmt.Sprintf("php-%s.tar.gz", version)
	url := "https://www.php.net/distributions/" + fname
	out := filepath.Join(dst, fname)
	if err := downloadFile(url, out); err != nil {
		return "", "", fmt.Errorf("linux package download failed (%s): %w", url, err)
	}
	return out, "tar.gz", nil
}

func resolveWindowsZipFromIndex(version string) (string, error) {
	resp, err := httpClient.Get("https://windows.php.net/downloads/releases/releases.json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("windows releases index returned %d", resp.StatusCode)
	}
	var idx map[string]map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return "", err
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid version %s", version)
	}
	track := parts[0] + "." + parts[1]
	entry, ok := idx[track]
	if !ok {
		return "", fmt.Errorf("no windows package track for %s", track)
	}
	if v, ok := entry["version"].(string); ok && v != version {
		logf("requested %s but windows index latest for %s is %s", version, track, v)
	}
	pref := []string{"nts-vs17-x64", "nts-vs16-x64", "ts-vs17-x64", "ts-vs16-x64"}
	for _, k := range pref {
		if raw, ok := entry[k].(map[string]any); ok {
			if zipv, ok := raw["zip"].(map[string]any); ok {
				if p, ok := zipv["path"].(string); ok && strings.TrimSpace(p) != "" {
					return p, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no supported x64 zip package found for %s", version)
}

func downloadFile(url, out string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "phpvm/0.1")
	resp, err := httpClient.Do(req)
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
	if cl := resp.ContentLength; cl > 0 {
		var read int64
		buf := make([]byte, 32*1024)
		lastPct := int64(-1)
		for {
			n, er := resp.Body.Read(buf)
			if n > 0 {
				if _, ew := f.Write(buf[:n]); ew != nil {
					return ew
				}
				read += int64(n)
				pct := (read * 100) / cl
				if pct != lastPct && (pct%10 == 0 || pct == 100) {
					fmt.Printf("\rDownloading... %d%%", pct)
					lastPct = pct
				}
			}
			if er == io.EOF {
				break
			}
			if er != nil {
				return er
			}
		}
		fmt.Println()
		return nil
	}
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

func isStableVersion(v string) bool {
	return regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v)
}

func findPHPDir(root string) (string, error) {
	want := "php"
	if runtime.GOOS == "windows" {
		want = "php.exe"
	}
	var found string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Base(path) == want {
			found = filepath.Dir(path)
			return io.EOF
		}
		return nil
	})
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if found == "" {
		return "", errors.New("php binary not found")
	}
	return found, nil
}

func moveDirContents(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if _, err := os.Stat(to); err == nil {
			continue
		}
		if err := os.Rename(from, to); err != nil {
			if e.IsDir() {
				if err := copyDir(from, to); err != nil {
					return err
				}
				_ = os.RemoveAll(from)
			} else {
				if err := copyFile(from, to); err != nil {
					return err
				}
				_ = os.Remove(from)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func phpBinaryPath(versionDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(versionDir, "php.exe")
	}
	bin := filepath.Join(versionDir, "bin", "php")
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	return filepath.Join(versionDir, "php")
}
