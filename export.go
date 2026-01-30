package main

import (
	"archive/zip"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	gkconfig "github.com/gookit/config/v2"
	"github.com/gookit/config/v2/json5"
)

// DictEntry represents a code-word pair [code, word]
type DictEntry [2]string

// TemplateMeta represents the full structure of a template.json5 file (includes ItemsMeta for generation)
type TemplateMeta struct {
	Name      string                `json:"name"`
	Version   string                `json:"version"`
	SVersion  string                `json:"sversion"`
	Font      TemplateFont          `json:"font"`
	Items     []map[string][]string `json:"items"`
	ItemsMeta []TemplateItemsMeta   `json:"items_meta"`
	Tabs      []TemplateTab         `json:"tabs"`
	Help      string                `json:"help"`
}

type TemplateItemsMeta struct {
	Category   []string `json:"category"`
	Prefix     []string `json:"prefix"`
	Suffix     []string `json:"suffix"`
	MinLength  int      `json:"min_length"`
	MaxLength  int      `json:"max_length"`
	WithSuffix string   `json:"with_suffix"`
}

// Template represents the structure for export (same as TemplateMeta but without ItemsMeta)
type Template struct {
	Name     string                `json:"name"`
	Version  string                `json:"version"`
	SVersion string                `json:"sversion"`
	Font     TemplateFont          `json:"font"`
	Items    []map[string][]string `json:"items"`
	Tabs     []TemplateTab         `json:"tabs"`
	Help     string                `json:"help"`
}

type TemplateFont struct {
	Name string `json:"name"`
	File string `json:"file"`
}

// TemplateTab represents a tab in the template
type TemplateTab struct {
	Label string `json:"label"`
	Type  string `json:"type"`
	Beg   int    `json:"beg,omitempty"`
	End   int    `json:"end,omitempty"`
}

// ExportConfig contains configuration for export operations
type ExportConfig struct {
	MethodName string
	YuhaoPath  string
	TargetPath string
}

func export(src, tar string) error {
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

	baseMethodName := parseMethodName(methodName)

	config := ExportConfig{
		MethodName: baseMethodName,
		YuhaoPath:  filepath.Join(tempDir, "schema/yuhao"),
		TargetPath: tar,
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
	}

	// Export template file if exists
	if err := exportTemplate(config); err != nil {
		return fmt.Errorf("failed to export template: %w", err)
	}

	return nil
}

func parseMethodName(methodName string) string {
	return methodName
}

// findSuffixedFiles finds files matching pattern: methodName_*.fileType.dict.yaml
// Returns map of suffix -> file path
func findSuffixedFiles(yuhaoPath, methodName, fileType string) map[string]string {
	suffixes := make(map[string]string)

	entries, err := os.ReadDir(yuhaoPath)
	if err != nil {
		return suffixes
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, methodName+"_") || !strings.HasSuffix(name, "."+fileType+".dict.yaml") {
			continue
		}
		// Extract suffix: methodName_suffix.fileType.dict.yaml -> suffix
		middle := strings.TrimPrefix(name, methodName+"_")
		suffix := strings.TrimSuffix(middle, "."+fileType+".dict.yaml")
		if suffix != "" && middle != suffix {
			suffixes[suffix] = filepath.Join(yuhaoPath, name)
		}
	}
	return suffixes
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

func writeCodeWordPairs(path string, entries []DictEntry) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create '%s': %w", path, err)
	}
	defer file.Close()

	sortByCode(entries)

	seenCodes := make(map[string]bool)
	for _, entry := range entries {
		if seenCodes[entry[0]] {
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
	dictPath := filepath.Join(config.YuhaoPath, config.MethodName+".roots.dict.yaml")

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
	// Export main quick file (no suffix)
	mainPath := filepath.Join(config.YuhaoPath, config.MethodName+".quick.dict.yaml")
	if _, err := os.Stat(mainPath); err == nil {
		if err := exportQuickWordsFromFile(config.YuhaoPath, mainPath, "", config); err != nil {
			return err
		}
	}

	// Find and export suffixed quick files
	suffixedFiles := findSuffixedFiles(config.YuhaoPath, config.MethodName, "quick")
	for suffix, filePath := range suffixedFiles {
		if err := exportQuickWordsFromFile(config.YuhaoPath, filePath, suffix, config); err != nil {
			return err
		}
	}
	return nil
}

func exportQuickWordsFromFile(yuhaoPath, dictPath, suffix string, config ExportConfig) error {
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

	suffixPrefix := ""
	if suffix != "" {
		suffixPrefix = "_" + suffix
	}

	if err := writeCodeWordPairs(filepath.Join(config.TargetPath, "quick_words"+suffixPrefix+".txt"), words); err != nil {
		return err
	}
	return writeCodeWordPairs(filepath.Join(config.TargetPath, "quick_chars"+suffixPrefix+".txt"), chars)
}

func exportPopWords(config ExportConfig) error {
	// Export main pop file (no suffix)
	mainPath := filepath.Join(config.YuhaoPath, config.MethodName+".pop.dict.yaml")
	if _, err := os.Stat(mainPath); err == nil {
		if err := exportPopWordsFromFile(mainPath, "", config); err != nil {
			return err
		}
	}

	// Find and export suffixed pop files
	suffixedFiles := findSuffixedFiles(config.YuhaoPath, config.MethodName, "pop")
	for suffix, filePath := range suffixedFiles {
		if err := exportPopWordsFromFile(filePath, suffix, config); err != nil {
			return err
		}
	}
	return nil
}

func exportPopWordsFromFile(dictPath, suffix string, config ExportConfig) error {
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

	suffixPrefix := ""
	if suffix != "" {
		suffixPrefix = "_" + suffix
	}

	if err := writeCodeWordPairs(filepath.Join(config.TargetPath, "pop_words"+suffixPrefix+".txt"), words); err != nil {
		return err
	}
	return writeCodeWordPairs(filepath.Join(config.TargetPath, "pop_chars"+suffixPrefix+".txt"), chars)
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
		if err := extractFile(file, destDir); err != nil {
			return err
		}
	}
	return nil
}

func extractFile(file *zip.File, destDir string) error {
	filePath := filepath.Join(destDir, file.Name)

	// Prevent ZipSlip
	if !strings.HasPrefix(filePath, filepath.Clean(destDir)+string(os.PathSeparator)) {
		return fmt.Errorf("illegal file path: %s", filePath)
	}

	if file.FileInfo().IsDir() {
		return os.MkdirAll(filePath, os.ModePerm)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return err
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

// exportTemplate reads methodName.template.json5, updates sversion, and writes to target directory
func exportTemplate(config ExportConfig) error {
	// Register JSON5 driver
	gkconfig.AddDriver(json5.Driver)

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Export main template file (no suffix)
	mainTemplatePath := filepath.Join(cwd, config.MethodName+".template.json5")
	if _, err := os.Stat(mainTemplatePath); err == nil {
		if err := exportTemplateFromFile(mainTemplatePath, config.MethodName+".json5", config); err != nil {
			return fmt.Errorf("failed to export main template: %w", err)
		}
	}

	// Find and export suffixed template files
	suffixedTemplates := findSuffixedTemplates(cwd, config.MethodName, "template.json5")
	for suffix, filePath := range suffixedTemplates {
		outputName := config.MethodName + "_" + suffix + ".json5"
		if err := exportTemplateFromFile(filePath, outputName, config); err != nil {
			return fmt.Errorf("failed to export template '%s': %w", outputName, err)
		}
	}

	return nil
}

// findSuffixedTemplates finds files matching pattern: methodName_*.suffix
// Returns map of suffix -> file path
func findSuffixedTemplates(cwd, methodName, suffix string) map[string]string {
	result := make(map[string]string)

	entries, err := os.ReadDir(cwd)
	if err != nil {
		return result
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, methodName+"_") || !strings.HasSuffix(name, "."+suffix) {
			continue
		}
		// Extract suffix: methodName_suffix.suffix -> suffix
		middle := strings.TrimPrefix(name, methodName+"_")
		fileSuffix := strings.TrimSuffix(middle, "."+suffix)
		if fileSuffix != "" && middle != fileSuffix {
			result[fileSuffix] = filepath.Join(cwd, name)
		}
	}
	return result
}

// exportTemplateFromFile reads a template file, updates sversion, and writes to target
func exportTemplateFromFile(templatePath, outputName string, config ExportConfig) error {
	// Read and parse JSON5 template using gookit/config
	var tmplMeta TemplateMeta
	err := gkconfig.LoadFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template file: %w", err)
	}
	if err := gkconfig.Decode(&tmplMeta); err != nil {
		return fmt.Errorf("failed to decode template file: %w", err)
	}

	// Update sversion
	newVersion, err := updateSVersion(tmplMeta.SVersion)
	if err != nil {
		return fmt.Errorf("failed to update sversion: %w", err)
	}
	tmplMeta.SVersion = newVersion

	// Convert to Template for JSON output (ItemsMeta will be excluded)
	tmpl := Template{
		Name:     tmplMeta.Name,
		Version:  tmplMeta.Version,
		SVersion: tmplMeta.SVersion,
		Font:     tmplMeta.Font,
		Items:    tmplMeta.Items,
		Tabs:     tmplMeta.Tabs,
		Help:     tmplMeta.Help,
	}

	// Write to output file with proper formatting
	outputPath := filepath.Join(config.TargetPath, outputName)
	outputData, err := json.MarshalIndent(tmpl, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal template: %w", err)
	}

	if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	return nil
}

// updateSVersion updates the sversion based on current date
// sversion format: "YYYY.M.D-seq" (e.g., "2026.1.29-1")
func updateSVersion(current string) (string, error) {
	// Parse current sversion
	var datePart string
	var seq int
	parts := strings.Split(current, "-")
	if len(parts) >= 2 {
		datePart = parts[0]
		if _, err := fmt.Sscanf(parts[1], "%d", &seq); err != nil {
			seq = 0
		}
	} else {
		datePart = current
		seq = 0
	}

	// Get current date in the same format
	now := time.Now()
	currentDate := fmt.Sprintf("%d.%d.%d", now.Year(), int(now.Month()), now.Day())

	// Compare dates and update sequence
	if datePart == currentDate {
		seq++
	} else {
		seq = 1
	}

	return fmt.Sprintf("%s-%d", currentDate, seq), nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
