package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	if runtime.GOOS != "windows" {
		fmt.Println("phpvm-setup is intended for Windows")
		os.Exit(1)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	targetDir := filepath.Join(home, ".phpvm", "bin")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		panic(err)
	}

	exePath, err := os.Executable()
	if err != nil {
		panic(err)
	}
	baseDir := filepath.Dir(exePath)
	source := filepath.Join(baseDir, "phpvm.exe")
	if _, err := os.Stat(source); err != nil {
		fmt.Println("Could not find phpvm.exe next to installer.")
		fmt.Println("Place phpvm.exe in the same folder as phpvm-setup.exe and run again.")
		os.Exit(1)
	}
	dest := filepath.Join(targetDir, "phpvm.exe")
	if err := copyFile(source, dest); err != nil {
		panic(err)
	}

	addPath(filepath.Join(home, ".phpvm", "bin"))

	fmt.Println("phpvm installed to", dest)
	fmt.Println("Restart terminal and run: phpvm v")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func addPath(bin string) {
	current := os.Getenv("Path")
	if strings.Contains(strings.ToLower(current), strings.ToLower(bin)) {
		return
	}
	_ = os.Setenv("Path", current+";"+bin)
	// Persistent user PATH set via PowerShell one-liner as fallback instruction.
	fmt.Println("If phpvm command is not found after reopening terminal, run:")
	fmt.Printf("[Environment]::SetEnvironmentVariable(\"Path\", [Environment]::GetEnvironmentVariable(\"Path\",\"User\") + \";%s\", \"User\")\n", strings.ReplaceAll(bin, "\\", "\\\\"))
}
