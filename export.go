package main

import (
	"archive/zip"
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DictEntry represents a code-word pair [code, word]
type DictEntry [2]string

// ExportConfig contains configuration for export operations
type ExportConfig struct {
	MethodName string
	Suffix     string
	YuhaoPath  string
	TargetPath string
	Unique     bool
}

func export(src, tar string, unique bool) error {
	// Validate src is a zip file
	if !strings.HasSuffix(strings.ToLower(src), ".zip") {
		return fmt.Errorf("source must be a zip file, got: %s", src)
	}

	// Extract zip to temporary directory
	tempDir, err := os.MkdirTemp("", "yu_tool_")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	if err := extractZipToDir(src, tempDir); err != nil {
		return fmt.Errorf("failed to extract zip file: %w", err)
	}

	// Read schema name from default.custom.yaml
	customPath := filepath.Join(tempDir, "schema/default.custom.yaml")
	methodName, err := readSchemaName(customPath)
	if err != nil {
		return fmt.Errorf("failed to read schema name: %w", err)
	}

	baseMethodName, suffix := parseMethodName(methodName)

	config := ExportConfig{
		MethodName: baseMethodName,
		Suffix:     suffix,
		YuhaoPath:  filepath.Join(tempDir, "schema/yuhao"),
		TargetPath: tar,
		Unique:     unique,
	}

	// Ensure target directory exists
	if _, err := os.Stat(tar); os.IsNotExist(err) {
		if err := os.MkdirAll(tar, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", tar, err)
		}
	}

	// Export root
	if err := exportRoot(config); err != nil {
		return fmt.Errorf("failed to export root: %w", err)
	}

	// Export quick words
	if err := exportQuickWords(config); err != nil {
		return fmt.Errorf("failed to export quick words: %w", err)
	}

	// Export pop words (ignore if file doesn't exist)
	if err := exportPopWords(config); err != nil {
		if !strings.Contains(err.Error(), "no such file or directory") &&
			!strings.Contains(err.Error(), "cannot find the file") {
			return fmt.Errorf("failed to export pop words: %w", err)
		}
		fmt.Printf("Warning: Pop words file not found, skipping pop words export\n")
	}

	return nil
}

func parseMethodName(methodName string) (base, suffix string) {
	parts := strings.Split(methodName, ",")
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// readSchemaName reads the shortest schema name from default.custom.yaml
// Looks for lines containing "- schema:" and extracts the schema name
func readSchemaName(configPath string) (string, error) {
	file, err := os.Open(configPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var schemaNames []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Find lines containing "- schema:"
		if idx := strings.Index(line, "- schema:"); idx != -1 {
			// Get the part after "- schema:"
			schemaPart := strings.TrimSpace(line[idx+len("- schema:"):])
			// Extract the first word (schema name)
			fields := strings.Fields(schemaPart)
			if len(fields) > 0 {
				schemaNames = append(schemaNames, fields[0])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	if len(schemaNames) == 0 {
		return "", errors.New("no schema name found in default.custom.yaml")
	}

	// Return the shortest schema name
	shortest := schemaNames[0]
	for _, name := range schemaNames[1:] {
		if len(name) < len(shortest) {
			shortest = name
		}
	}
	return shortest, nil
}

func findDictFile(yuhaoPath, methodName, suffix, fileType string) string {
	if suffix != "" {
		suffixedName := methodName + "_" + suffix + "." + fileType + ".dict.yaml"
		suffixedPath := filepath.Join(yuhaoPath, suffixedName)
		if _, err := os.Stat(suffixedPath); err == nil {
			return suffixedPath
		}
	}
	return filepath.Join(yuhaoPath, methodName+"."+fileType+".dict.yaml")
}

func sortByCode(entries []DictEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		codeI := entries[i][0]
		codeJ := entries[j][0]
		if len(codeI) != len(codeJ) {
			return len(codeI) < len(codeJ)
		}
		return codeI < codeJ
	})
}

func writeCodeWordPairs(path string, entries []DictEntry, unique bool) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create '%s': %w", path, err)
	}
	defer file.Close()

	sortByCode(entries)

	seenCodes := make(map[string]bool)
	for _, entry := range entries {
		if unique && seenCodes[entry[0]] {
			continue
		}
		seenCodes[entry[0]] = true
		if _, err := file.WriteString(entry[0] + "\t" + entry[1] + "\n"); err != nil {
			return fmt.Errorf("failed to write to '%s': %w", path, err)
		}
	}
	return nil
}

func exportRoot(config ExportConfig) error {
	dictPath := findDictFile(config.YuhaoPath, config.MethodName, config.Suffix, "roots")

	file, err := os.Open(dictPath)
	if err != nil {
		return fmt.Errorf("failed to open '%s': %w", dictPath, err)
	}
	defer file.Close()

	outputPath := filepath.Join(config.TargetPath, "roots.txt")
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create '%s': %w", outputPath, err)
	}
	defer outputFile.Close()

	// Build keyCode -> words map
	wordMap := make(map[string][]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "+") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		code := fields[1]
		words := fields[3]
		key := fields[len(fields)-1][3:]
		keyCode := key + code

		for _, word := range []rune(words) {
			wordMap[keyCode] = append(wordMap[keyCode], string(word))
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary: %w", err)
	}

	// Write sorted entries
	var keyCodes []string
	for k := range wordMap {
		keyCodes = append(keyCodes, k)
	}
	sort.SliceStable(keyCodes, func(i, j int) bool {
		if len(keyCodes[i]) != len(keyCodes[j]) {
			return len(keyCodes[i]) < len(keyCodes[j])
		}
		return keyCodes[i] < keyCodes[j]
	})

	for _, keyCode := range keyCodes {
		for _, word := range wordMap[keyCode] {
			if _, err := outputFile.WriteString(word + "\t" + keyCode + "\n"); err != nil {
				return fmt.Errorf("failed to write to '%s': %w", outputPath, err)
			}
		}
	}
	return nil
}

func exportQuickWords(config ExportConfig) error {
	dictPath := findDictFile(config.YuhaoPath, config.MethodName, config.Suffix, "quick")

	file, err := os.Open(dictPath)
	if err != nil {
		return fmt.Errorf("failed to open '%s': %w", dictPath, err)
	}
	defer file.Close()

	var words, chars []DictEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		word, code := fields[0], fields[1]
		if !isEnglishLettersOnly(code) || isAllASCII(word) {
			continue
		}
		if len([]rune(word)) > 1 {
			words = append(words, DictEntry{code, word})
		} else {
			chars = append(chars, DictEntry{code, word})
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary: %w", err)
	}

	if err := writeCodeWordPairs(filepath.Join(config.TargetPath, "quick_words.txt"), words, config.Unique); err != nil {
		return err
	}
	return writeCodeWordPairs(filepath.Join(config.TargetPath, "quick_chars.txt"), chars, config.Unique)
}

func exportPopWords(config ExportConfig) error {
	dictPath := findDictFile(config.YuhaoPath, config.MethodName, config.Suffix, "pop")

	file, err := os.Open(dictPath)
	if err != nil {
		return fmt.Errorf("failed to open '%s': %w", dictPath, err)
	}
	defer file.Close()

	var words, chars []DictEntry

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		word, code := fields[0], fields[1]
		if !isEnglishLettersOnly(code) || isAllASCII(word) {
			continue
		}
		if len([]rune(word)) > 1 {
			words = append(words, DictEntry{code, word})
		} else {
			chars = append(chars, DictEntry{code, word})
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary: %w", err)
	}

	if err := writeCodeWordPairs(filepath.Join(config.TargetPath, "pop_words.txt"), words, config.Unique); err != nil {
		return err
	}
	return writeCodeWordPairs(filepath.Join(config.TargetPath, "pop_chars.txt"), chars, config.Unique)
}

func isEnglishLettersOnly(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

func isAllASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func extractZipToDir(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, file := range r.File {
		filePath := filepath.Join(destDir, file.Name)

		// Prevent ZipSlip
		if !strings.HasPrefix(filePath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", filePath)
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(filePath, os.ModePerm)
			continue
		}

		os.MkdirAll(filepath.Dir(filePath), os.ModePerm)

		src, _ := file.Open()
		defer src.Close()

		dst, _ := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		defer dst.Close()

		io.Copy(dst, src)
	}
	return nil
}
