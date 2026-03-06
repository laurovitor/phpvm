package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
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

type windowsZipCandidate struct {
	Path string
	SHA  string
}

type ghRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

var httpClient = &http.Client{Timeout: 30 * time.Second}
var downloadClient = &http.Client{Timeout: 5 * time.Minute}
var verbose bool
var appLang string

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

func langFile() string { return filepath.Join(rootDir(), "lang") }

func normalizeLang(v string) string {
	s := strings.ToLower(strings.TrimSpace(v))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, ".utf-8", "")
	s = strings.ReplaceAll(s, ".utf8", "")
	s = strings.TrimSpace(s)
	switch {
	case s == "pt" || strings.HasPrefix(s, "pt-br"):
		return "pt-BR"
	case s == "es" || strings.HasPrefix(s, "es-"):
		return "es"
	default:
		return "en"
	}
}

func detectOSLang() string {
	for _, k := range []string{"PHPVM_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return normalizeLang(v)
		}
	}
	return "en"
}

func loadLang() string {
	if appLang != "" {
		return appLang
	}
	if b, err := os.ReadFile(langFile()); err == nil {
		appLang = normalizeLang(string(b))
		return appLang
	}
	appLang = detectOSLang()
	return appLang
}

func setLang(v string) error {
	n := normalizeLang(v)
	if err := os.MkdirAll(rootDir(), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(langFile(), []byte(n+"\n"), 0o644); err != nil {
		return err
	}
	appLang = n
	return nil
}

func tr(en, pt, es string) string {
	switch loadLang() {
	case "pt-BR":
		if pt != "" {
			return pt
		}
	case "es":
		if es != "" {
			return es
		}
	}
	return en
}

func main() {
	_ = loadLang()
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
	case "lang", "language":
		must(runLang(rest))
	default:
		must(fmt.Errorf("unknown command: %s", cmd))
	}
}

func printHelp() {
	fmt.Println(tr("phpvm - cross-platform PHP version manager (Windows/Linux first)", "phpvm - gerenciador de versões PHP multiplataforma (Windows/Linux primeiro)", "phpvm - gestor de versiones PHP multiplataforma (Windows/Linux primero)"))
	fmt.Println("")
	fmt.Println(tr("Usage:", "Uso:", "Uso:"))
	fmt.Println("  phpvm install <version>      (alias: i)")
	fmt.Println("  phpvm use <version>          (alias: u)")
	fmt.Println("  phpvm list                   (alias: ls)")
	fmt.Println("  phpvm current                (alias: c)")
	fmt.Println("  phpvm available [major|x.y]  (alias: a)")
	fmt.Println("  phpvm remove <version> [--force] (alias: rm)")
	fmt.Println("  phpvm version                (alias: v)")
	fmt.Println("  phpvm doctor                 (alias: d)")
	fmt.Println("  phpvm selfupdate             (alias: su)")
	fmt.Println("  phpvm lang list|get|set <lang>")
	fmt.Println(tr("Global flags: --verbose | -V | --log (can be placed anywhere)", "Flags globais: --verbose | -V | --log (podem ser usadas em qualquer posição)", "Banderas globales: --verbose | -V | --log (pueden usarse en cualquier posición)"))
	fmt.Println("")
	fmt.Println(tr("Version inputs:", "Entradas de versão:", "Entradas de versión:"))
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

	fmt.Println(tr("[1/5] Resolving version...", "[1/5] Resolvendo versão...", "[1/5] Resolviendo versión..."))
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

	fmt.Println(tr("[2/5] Downloading package...", "[2/5] Baixando pacote...", "[2/5] Descargando paquete..."))
	archive, kind, err := downloadPHPArchive(resolved, stage)
	if err != nil {
		return err
	}

	extractDir := filepath.Join(stage, "_extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return err
	}
	if err := runWithDots(tr("[3/5] Extracting package", "[3/5] Extraindo pacote", "[3/5] Extrayendo paquete"), func() error {
		switch kind {
		case "zip":
			return extractZip(archive, extractDir)
		case "tar.gz":
			return extractTarGz(archive, extractDir)
		default:
			return nil
		}
	}); err != nil {
		return err
	}

	var phpDir string
	if err := runWithDots(tr("[4/5] Validating PHP binary", "[4/5] Validando binário do PHP", "[4/5] Validando binario de PHP"), func() error {
		var ferr error
		phpDir, ferr = findPHPDir(extractDir)
		if ferr != nil {
			return fmt.Errorf("installed archive does not contain runnable PHP binary. On Linux, php.net tarballs are source builds; prebuilt Linux binaries are not wired yet")
		}
		return nil
	}); err != nil {
		return err
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
	if err := runWithDots(tr("[5/5] Finalizing install", "[5/5] Finalizando instalação", "[5/5] Finalizando instalación"), func() error { return nil }); err != nil {
		return err
	}
	fmt.Println(tr("Installed", "Instalado", "Instalado"), resolved)
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
	if runtime.GOOS == "windows" {
		changed, err := ensureWindowsUserPathContainsCurrent()
		if err != nil {
			logf("failed to update user PATH automatically: %v", err)
		}
		if changed {
			fmt.Printf("Updated user PATH with %%USERPROFILE%%\\.phpvm\\current\n")
		}
	}
	fmt.Println("Ensure your PATH contains:")
	if runtime.GOOS == "windows" {
		fmt.Printf("  %%LOCALAPPDATA%%\\phpvm\\bin\n")
		fmt.Printf("  %%USERPROFILE%%\\.phpvm\\current\n")
		fmt.Println("Tip: open a new terminal, then run: where phpvm && where php && php -v")
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

func ensureWindowsUserPathContainsCurrent() (bool, error) {
	if runtime.GOOS != "windows" {
		return false, nil
	}
	ps := `$currentTarget = [Environment]::ExpandEnvironmentVariables('%USERPROFILE%\\.phpvm\\current')
$phpvmBin = [Environment]::ExpandEnvironmentVariables('%LOCALAPPDATA%\\phpvm\\bin')
$userPath = [Environment]::GetEnvironmentVariable('Path','User')
$parts = @()
if (-not [string]::IsNullOrWhiteSpace($userPath)) {
  $parts = $userPath.Split(';') | ForEach-Object { $_.Trim() } | Where-Object { $_ -ne '' }
}
function HasPath([string[]]$arr, [string]$p) {
  $n = $p.TrimEnd('\\').ToLowerInvariant()
  foreach ($x in $arr) {
    if ($x.TrimEnd('\\').ToLowerInvariant() -eq $n) { return $true }
  }
  return $false
}
$changed = $false
if (-not (HasPath $parts $phpvmBin)) { $parts += $phpvmBin; $changed = $true }
if (-not (HasPath $parts $currentTarget)) { $parts += $currentTarget; $changed = $true }
if ($changed) {
  [Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')
  Write-Output 'ADDED'
} else {
  Write-Output 'UNCHANGED'
}`
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("powershell path update failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.Contains(string(out), "ADDED"), nil
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
		wantCurrent := filepath.Clean(currentLink())
		wantBin := filepath.Clean(filepath.Join(os.Getenv("LOCALAPPDATA"), "phpvm", "bin"))
		okCurrent := strings.Contains(strings.ToLower(path), strings.ToLower(wantCurrent))
		okBin := strings.Contains(strings.ToLower(path), strings.ToLower(wantBin))
		fmt.Println("PATH has phpvm bin:", okBin)
		fmt.Println("PATH has phpvm current:", okCurrent)
		if !okBin || !okCurrent {
			fmt.Println("Add this to PATH:")
			fmt.Println(" ", wantBin)
			fmt.Println(" ", wantCurrent)
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

func runLang(args []string) error {
	if len(args) == 0 || args[0] == "get" || args[0] == "current" {
		fmt.Println(loadLang())
		return nil
	}
	sub := strings.ToLower(args[0])
	switch sub {
	case "list", "ls":
		fmt.Println("pt-BR")
		fmt.Println("es")
		fmt.Println("en")
		return nil
	case "set":
		if len(args) < 2 {
			return errors.New("usage: phpvm lang set <pt-BR|es|en>")
		}
		if err := setLang(args[1]); err != nil {
			return err
		}
		fmt.Println("language set to", loadLang())
		return nil
	default:
		if err := setLang(sub); err == nil {
			fmt.Println("language set to", loadLang())
			return nil
		}
		return errors.New("usage: phpvm lang list|get|set <pt-BR|es|en>")
	}
}

func runSelfUpdate() error {
	rel, err := fetchLatestRelease()
	if err != nil {
		return err
	}
	current := appVersion
	if current == "" {
		current = "unknown"
	}
	currScore, currOK := releaseVersionScore(current)
	latestScore, latestOK := releaseVersionScore(rel.TagName)
	if currOK && latestOK && currScore >= latestScore {
		fmt.Println(tr("Already on latest release:", "Você já está na última release:", "Ya estás en la última release:"), current)
		return nil
	}
	fmt.Println(tr("Current version:", "Versão atual:", "Versión actual:"), current)
	fmt.Println(tr("Latest release:", "Última release:", "Última release:"), rel.TagName)
	fmt.Print(tr("Update now? [y/N]: ", "Atualizar agora? [s/N]: ", "¿Actualizar ahora? [s/N]: "))
	var answer string
	_, _ = fmt.Scanln(&answer)
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer == "" || (answer != "y" && answer != "yes" && answer != "s" && answer != "sim" && answer != "si") {
		fmt.Println(tr("Update canceled.", "Atualização cancelada.", "Actualización cancelada."))
		return nil
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
		fmt.Println(tr("Downloaded update to:", "Atualização baixada em:", "Actualización descargada en:"), tmp)
		fmt.Println(tr("On Windows, replace phpvm.exe manually (cannot overwrite running executable).", "No Windows, substitua o phpvm.exe manualmente (não dá para sobrescrever em execução).", "En Windows, reemplaza phpvm.exe manualmente (no se puede sobrescribir en ejecución)."))
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
	fmt.Println(tr("Updated to", "Atualizado para", "Actualizado a"), rel.TagName)
	return nil
}

func fetchLatestRelease() (*ghRelease, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=50", repoSlug)
	logf("selfupdate checking releases: %s", url)
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases endpoint returned %d", resp.StatusCode)
	}
	var rels []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rels); err != nil {
		return nil, err
	}
	var best *ghRelease
	bestScore := ""
	for i := range rels {
		r := &rels[i]
		if r.Draft {
			continue
		}
		score, ok := releaseVersionScore(r.TagName)
		if !ok {
			continue
		}
		if best == nil || score > bestScore {
			best = r
			bestScore = score
		}
	}
	if best == nil {
		return nil, errors.New("no published release found yet")
	}
	return best, nil
}

func releaseVersionScore(tag string) (string, bool) {
	t := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(tag)), "v")
	re := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)(?:-([a-z]+)\.(\d+))?$`)
	m := re.FindStringSubmatch(t)
	if m == nil {
		return "", false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	preKind := "zzzz"
	preNum := 999999
	if m[4] != "" {
		preKind = m[4]
		preNum, _ = strconv.Atoi(m[5])
	}
	return fmt.Sprintf("%06d%06d%06d-%s-%06d", major, minor, patch, preKind, preNum), true
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
		cands, err := resolveWindowsZipCandidates(version)
		if err != nil {
			return "", "", err
		}
		bases := []string{
			"https://windows.php.net/downloads/releases/",
			"https://windows.php.net/downloads/releases/archives/",
		}
		var lastErr error
		lastURL := ""
		for _, c := range cands {
			for _, b := range bases {
				url := b + c.Path
				out := filepath.Join(dst, c.Path)
				logf("trying windows package url: %s", url)
				if err := downloadFileWithFallback(url, out, c.SHA); err == nil {
					return out, "zip", nil
				} else {
					lastErr = err
					lastURL = url
					logf("download failed for %s: %v", url, err)
				}
			}
		}
		if lastErr == nil {
			return "", "", fmt.Errorf("windows package download failed for %s (no URL succeeded)", version)
		}
		return "", "", fmt.Errorf("windows package download failed (last tried %s): %v", lastURL, lastErr)
	}

	fname := fmt.Sprintf("php-%s.tar.gz", version)
	url := "https://www.php.net/distributions/" + fname
	out := filepath.Join(dst, fname)
	if err := downloadFile(url, out); err != nil {
		return "", "", fmt.Errorf("linux package download failed (%s): %w", url, err)
	}
	return out, "tar.gz", nil
}

func resolveWindowsZipCandidates(version string) ([]windowsZipCandidate, error) {
	resp, err := httpClient.Get("https://windows.php.net/downloads/releases/releases.json")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("windows releases index returned %d", resp.StatusCode)
	}
	var idx map[string]map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&idx); err != nil {
		return nil, err
	}
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version %s", version)
	}
	track := parts[0] + "." + parts[1]
	entry, ok := idx[track]
	if !ok {
		return nil, fmt.Errorf("no windows package track for %s", track)
	}
	if v, ok := entry["version"].(string); ok && v != version {
		logf("requested %s but windows index latest for %s is %s", version, track, v)
	}
	pref := []string{"nts-vs17-x64", "nts-vs16-x64", "ts-vs17-x64", "ts-vs16-x64"}
	out := make([]windowsZipCandidate, 0, 4)
	for _, k := range pref {
		if raw, ok := entry[k].(map[string]any); ok {
			if zipv, ok := raw["zip"].(map[string]any); ok {
				p, _ := zipv["path"].(string)
				h, _ := zipv["sha256"].(string)
				if strings.TrimSpace(p) != "" {
					out = append(out, windowsZipCandidate{Path: p, SHA: strings.TrimSpace(h)})
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no supported x64 zip package found for %s", version)
	}
	return out, nil
}

func runWithDots(label string, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	frames := []string{"■■□□□", "□■■□□", "□□■■□", "□□□■■", "■□□□■"}
	i := 0
	for {
		select {
		case err := <-done:
			fmt.Printf("\r%s ■■■■■\n", label)
			return err
		case <-time.After(250 * time.Millisecond):
			fmt.Printf("\r%s %s", label, frames[i])
			i = (i + 1) % len(frames)
		}
	}
}

func downloadFileWithFallback(url, out, sha string) error {
	err := downloadFile(url, out)
	if err != nil {
		logf("native http downloader failed, trying curl fallback for %s", url)
		if cerr := curlDownload(url, out); cerr == nil {
			err = nil
		} else {
			logf("curl fallback failed: %v", cerr)
		}
	}
	if err != nil && runtime.GOOS == "windows" {
		logf("native fallback powershell download for %s", url)
		if perr := powershellDownload(url, out); perr == nil {
			err = nil
		} else {
			logf("powershell fallback failed: %v", perr)
		}
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(sha) != "" {
		if err := verifySHA256(out, sha); err != nil {
			_ = os.Remove(out)
			return fmt.Errorf("checksum mismatch: %w", err)
		}
	}
	return nil
}

func downloadFile(url, out string) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
		logf("download attempt %d/%d: %s", attempt, maxAttempts, url)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("User-Agent", "phpvm/0.1")
		resp, err := downloadClient.Do(req)
		if err != nil {
			lastErr = err
			if isRetryableErr(err) {
				continue
			}
			return err
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			if resp.StatusCode >= 500 && attempt < maxAttempts {
				lastErr = fmt.Errorf("http %d", resp.StatusCode)
				continue
			}
			return fmt.Errorf("http %d", resp.StatusCode)
		}
		if err := streamToFile(resp.Body, out, resp.ContentLength); err != nil {
			resp.Body.Close()
			lastErr = err
			if isRetryableErr(err) {
				continue
			}
			return err
		}
		resp.Body.Close()
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("download failed")
}

func streamToFile(body io.ReadCloser, out string, contentLen int64) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()

	if contentLen > 0 {
		fmt.Printf("%s\n", tr(
			fmt.Sprintf("Downloading file (%s). This may take a while depending on your connection...", formatBytes(contentLen)),
			fmt.Sprintf("Baixando arquivo (%s). Isso pode demorar dependendo da sua conexão...", formatBytes(contentLen)),
			fmt.Sprintf("Descargando archivo (%s). Esto puede tardar según tu conexión...", formatBytes(contentLen)),
		))
		var read int64
		buf := make([]byte, 32*1024)
		barWidth := 18
		start := time.Now()
		lastPaint := time.Time{}
		painted := false
		lastLineLen := 0
		defer func() {
			if painted {
				fmt.Print("\n")
			}
		}()
		for {
			n, er := body.Read(buf)
			if n > 0 {
				if _, ew := f.Write(buf[:n]); ew != nil {
					return ew
				}
				read += int64(n)
				now := time.Now()
				if !lastPaint.IsZero() && now.Sub(lastPaint) < 200*time.Millisecond && read < contentLen {
					// keep output smooth without flooding console
				} else {
					pct := float64(read) / float64(contentLen)
					if pct > 1 {
						pct = 1
					}
					filled := int(pct * float64(barWidth))
					if filled > barWidth {
						filled = barWidth
					}
					bar := strings.Repeat("■", filled) + strings.Repeat("□", barWidth-filled)
					elapsed := time.Since(start).Seconds()
					if elapsed < 0.001 {
						elapsed = 0.001
					}
					rate := float64(read) / elapsed
					remain := contentLen - read
					eta := "--"
					if rate > 1 {
						eta = formatETA(time.Duration(float64(remain)/rate) * time.Second)
					}
					line := fmt.Sprintf("%s %3d%% - %s/s - %s %s %s - %s %s", bar, int(pct*100), formatBytes(int64(rate)), formatBytes(read), tr("of", "de", "de"), formatBytes(contentLen), eta, tr("remaining", "restante", "restante"))
					pad := ""
					if len(line) < lastLineLen {
						pad = strings.Repeat(" ", lastLineLen-len(line))
					}
					fmt.Printf("\r%s%s", line, pad)
					lastLineLen = len(line)
					lastPaint = now
					painted = true
				}
			}
			if er == io.EOF {
				break
			}
			if er != nil {
				return er
			}
		}
		return nil
	}

	_, err = io.Copy(f, body)
	return err
}

func formatBytes(n int64) string {
	units := []string{"B", "KB", "MB", "GB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", int64(v), units[i])
	}
	s := fmt.Sprintf("%.1f", v)
	s = strings.Replace(s, ".", ",", 1)
	return fmt.Sprintf("%s %s", s, units[i])
}

func formatETA(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	totalSec := int(d.Seconds())
	if totalSec < 60 {
		return fmt.Sprintf("%ds", totalSec)
	}
	min := totalSec / 60
	if min < 60 {
		return fmt.Sprintf("%d min", min)
	}
	h := min / 60
	m := min % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dmin", h, m)
}

func isRetryableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "tempor") || strings.Contains(msg, "connection reset") || strings.Contains(msg, "context deadline") {
		return true
	}
	return false
}

func curlDownload(url, out string) error {
	if _, err := exec.LookPath("curl"); err != nil {
		return errors.New("curl not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "curl", "-fL", "--retry", "2", "--connect-timeout", "20", "--max-time", "720", "-o", out, url)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("curl download failed: %v (%s)", err, strings.TrimSpace(string(b)))
	}
	return nil
}

func powershellDownload(url, out string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	script := fmt.Sprintf("$ProgressPreference='SilentlyContinue'; Invoke-WebRequest -UseBasicParsing -Uri '%s' -OutFile '%s'", strings.ReplaceAll(url, "'", "''"), strings.ReplaceAll(out, "'", "''"))
	cmd := exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", script)
	b, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell download failed: %v (%s)", err, strings.TrimSpace(string(b)))
	}
	return nil
}

func verifySHA256(path, expected string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(strings.TrimSpace(expected), strings.TrimSpace(actual)) {
		return fmt.Errorf("expected %s got %s", expected, actual)
	}
	return nil
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
