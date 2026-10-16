package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type FileMeta struct {
	RelativePath string    `json:"relative_path"`
	SizeBytes    int64     `json:"size_bytes"`
	SHA256       string    `json:"sha256"`
	ModifiedAt   time.Time `json:"modified_at"`
}

type BackupManifest struct {
	CreatedAt   time.Time  `json:"created_at"`
	SourceDir   string     `json:"source_dir"`
	TotalFiles  int        `json:"total_files"`
	TotalBytes  int64      `json:"total_bytes"`
	Files       []FileMeta `json:"files"`
}

func computeHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	os.MkdirAll(filepath.Dir(dst), 0755)
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

func performBackup(srcDir, destDir string) {
	srcAbs, _ := filepath.Abs(srcDir)
	destAbs, _ := filepath.Abs(destDir)

	timestamp := time.Now().Format("2006-01-02_15-04-05")
	snapshotDir := filepath.Join(destAbs, "snapshot_"+timestamp)
	os.MkdirAll(snapshotDir, 0755)

	fmt.Printf("[*] Creating backup of '%s' into '%s'\n", srcAbs, snapshotDir)

	var manifest BackupManifest
	manifest.CreatedAt = time.Now()
	manifest.SourceDir = srcAbs

	err := filepath.Walk(srcAbs, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(srcAbs, path)
		if err != nil {
			return nil
		}

		hash, err := computeHash(path)
		if err != nil {
			return nil
		}

		targetFile := filepath.Join(snapshotDir, relPath)
		if err := copyFile(path, targetFile); err != nil {
			fmt.Printf("[-] Failed to copy %s: %v\n", relPath, err)
			return nil
		}

		manifest.Files = append(manifest.Files, FileMeta{
			RelativePath: relPath,
			SizeBytes:    info.Size(),
			SHA256:       hash,
			ModifiedAt:   info.ModTime(),
		})
		manifest.TotalFiles++
		manifest.TotalBytes += info.Size()
		return nil
	})

	if err != nil {
		fmt.Printf("[-] Walk error: %v\n", err)
		return
	}

	// Write manifest JSON
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(snapshotDir, "manifest.json"), manifestData, 0644)

	fmt.Println("[+] Backup completed successfully!")
	fmt.Printf("    Files Copied : %d\n", manifest.TotalFiles)
	fmt.Printf("    Total Volume : %.2f MB\n", float64(manifest.TotalBytes)/(1024*1024))
	fmt.Printf("    Manifest Path: %s\n", filepath.Join(snapshotDir, "manifest.json"))
}

func verifyBackup(manifestPath string) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		fmt.Printf("[-] Failed to read manifest: %v\n", err)
		return
	}

	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		fmt.Printf("[-] Corrupted manifest JSON: %v\n", err)
		return
	}

	baseDir := filepath.Dir(manifestPath)
	fmt.Printf("[*] Verifying integrity for %d files in '%s'...\n", manifest.TotalFiles, baseDir)

	passed := 0
	failed := 0

	for _, file := range manifest.Files {
		fullPath := filepath.Join(baseDir, file.RelativePath)
		currentHash, err := computeHash(fullPath)
		if err != nil {
			fmt.Printf("[-] MISSING FILE: %s\n", file.RelativePath)
			failed++
			continue
		}

		if currentHash == file.SHA256 {
			passed++
		} else {
			fmt.Printf("[-] HASH MISMATCH: %s (Expected: %s, Found: %s)\n", file.RelativePath, file.SHA256[:8], currentHash[:8])
			failed++
		}
	}

	fmt.Println("----------------------------------------------------------------")
	if failed == 0 {
		fmt.Printf("[+] INTEGRITY CHECK PASSED: All %d files verified perfectly!\n", passed)
	} else {
		fmt.Printf("[-] VERIFICATION FAILED: %d passed, %d corrupted/missing!\n", passed, failed)
	}
}

func main() {
	modeFlag := flag.String("mode", "backup", "Mode: 'backup' or 'verify'")
	srcFlag := flag.String("src", ".", "Source directory to backup")
	destFlag := flag.String("dest", "./backups", "Destination backup directory")
	manifestFlag := flag.String("manifest", "", "Path to manifest.json for verification")
	flag.Parse()

	fmt.Println("================================================================")
	fmt.Println("             GO LOCAL BACKUP & INTEGRITY TOOL                   ")
	fmt.Println("================================================================")

	if *modeFlag == "verify" {
		if *manifestFlag == "" {
			fmt.Println("[-] Please provide -manifest <path_to_manifest.json>")
			return
		}
		verifyBackup(*manifestFlag)
	} else {
		performBackup(*srcFlag, *destFlag)
	}
	fmt.Println("================================================================")
}
