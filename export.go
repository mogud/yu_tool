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
	"regexp"
	"sort"
	"strings"
	"time"

	gkconfig "github.com/gookit/config/v2"
	"github.com/gookit/config/v2/json5"
)

// TemplateMeta represents the full structure of a template.json5 file (includes ItemsMeta for generation)
type TemplateMeta struct {
	Name          string              `json:"name" mapstructure:"name"`
	Version       string              `json:"version" mapstructure:"version"`
	ConfigVersion string              `json:"config_version" mapstructure:"config_version"`
	Fonts         []TemplateFont      `json:"fonts" mapstructure:"fonts"`
	KeyBindings   []KeyBinding        `json:"key_bindings" mapstructure:"key_bindings"`
	ItemsMeta     []TemplateItemsMeta `json:"items_meta" mapstructure:"items_meta"`
	Tabs          []TemplateTab       `json:"tabs" mapstructure:"tabs"`
	Help          string              `json:"help" mapstructure:"help"`
}

// Template represents the structure for export (same as TemplateMeta but without ItemsMeta)
type Template struct {
	Name          string                `json:"name"`
	Version       string                `json:"version"`
	ConfigVersion string                `json:"config_version"`
	Fonts         []TemplateFont        `json:"fonts"`
	KeyBindings   []KeyBinding          `json:"key_bindings"`
	Items         []map[string][]string `json:"items"`
	Tabs          []TemplateTab         `json:"tabs"`
	Help          string                `json:"help"`
}

// DictEntry represents a code-word pair [code, word]
type DictEntry [2]string

type KeyBinding struct {
	Key     string `json:"key" mapstructure:"key"`
	Command string `json:"command" mapstructure:"command"`
}

type TemplateItemsMeta struct {
	Category     []string `json:"category" mapstructure:"category"`
	Prefix       []string `json:"prefix" mapstructure:"prefix"`
	Suffix       []string `json:"suffix" mapstructure:"suffix"`
	MinLength    int      `json:"min_length" mapstructure:"min_length"`
	MaxLength    int      `json:"max_length" mapstructure:"max_length"`
	AppendSuffix string   `json:"append_suffix" mapstructure:"append_suffix"`
}

type TemplateFont struct {
	Name   string `json:"name" mapstructure:"name"`
	File   string `json:"file" mapstructure:"file"`
	Type   string `json:"type" mapstructure:"type"`
	Base64 string `json:"base64" mapstructure:"base64"`
}

// TemplateTab represents a tab in the template
type TemplateTab struct {
	Label string `json:"label" mapstructure:"label"`
	Type  string `json:"type" mapstructure:"type"`
	Beg   int    `json:"beg" mapstructure:"beg"`
	End   int    `json:"end" mapstructure:"end"`
}

// ExportConfig contains configuration for export operations
type ExportConfig struct {
	MethodName string
	Version    string
	YuhaoPath  string
	RootPath   string
	TargetPath string
	Update     bool
}

func export(src, tar, root string, update bool) error {
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

	// Extract version from source filename
	// Format: methodName_version.zip or methodName_suffix_version.zip
	version := extractVersionFromFilename(src)

	config := ExportConfig{
		MethodName: baseMethodName,
		Version:    version,
		YuhaoPath:  filepath.Join(tempDir, "schema/yuhao"),
		RootPath:   root,
		TargetPath: tar,
		Update:     update,
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

// extractVersionFromFilename extracts version from source filename
// Format: methodName_version.zip or methodName_suffix_version.zip
// Returns the second part after splitting by '_' and removing .zip suffix
func extractVersionFromFilename(filename string) string {
	baseName := filepath.Base(filename)
	// Remove .zip suffix
	baseName = strings.TrimSuffix(baseName, ".zip")
	// Split by '_'
	parts := strings.Split(baseName, "_")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
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
	outputPath := filepath.Join(config.TargetPath, "roots.txt")
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create '%s': %w", outputPath, err)
	}
	defer outputFile.Close()

	entries, err := readRootsFromCSV(config.RootPath)
	if err != nil {
		return fmt.Errorf("failed to read roots from CSV: %w", err)
	}

	// 写入排序后的条目
	sortByCode(entries)
	for _, entry := range entries {
		if _, err := outputFile.WriteString(entry[1] + "\t" + entry[0] + "\n"); err != nil {
			return fmt.Errorf("failed to write to '%s': %w", outputPath, err)
		}
	}

	return nil
}

// readRootsFromCSV 从 CSV 文件读取字根，每行第一列是字根，第二列是编码
func readRootsFromCSV(csvPath string) ([]DictEntry, error) {
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open '%s': %w", csvPath, err)
	}
	defer file.Close()

	var entries []DictEntry
	scanner := bufio.NewScanner(file)
	isFirstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		// 跳过头部
		if isFirstLine {
			isFirstLine = false
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			continue
		}
		// CSV 格式: font,code,pinyin (第一列是字根，第二列是编码)
		word := strings.TrimSpace(fields[0])
		code := strings.ToLower(strings.TrimSpace(fields[1]))
		if code != "" && word != "" {
			entries = append(entries, DictEntry{code, word})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading CSV: %w", err)
	}
	return entries, nil
}

func exportQuickWords(config ExportConfig) error {
	// Export main quick file (no suffix)
	mainPath := filepath.Join(config.YuhaoPath, config.MethodName+".quick.dict.yaml")
	if _, err := os.Stat(mainPath); err == nil {
		if err := exportQuickWordsFromFile(mainPath, "", config); err != nil {
			return err
		}
	}

	// Find and export suffixed quick files
	suffixedFiles := findSuffixedFiles(config.YuhaoPath, config.MethodName, "quick")
	for suffix, filePath := range suffixedFiles {
		if err := exportQuickWordsFromFile(filePath, suffix, config); err != nil {
			return err
		}
	}
	return nil
}

func exportQuickWordsFromFile(dictPath, suffix string, config ExportConfig) error {
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

// exportTemplate reads methodName.template.json5, updates configversion, and writes to target directory
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
		if err := exportTemplateFromFile(mainTemplatePath, config.MethodName+".json5", "", config); err != nil {
			return fmt.Errorf("failed to export main template: %w", err)
		}
	}

	// Find and export suffixed template files
	suffixedTemplates := findSuffixedTemplates(cwd, config.MethodName, "template.json5")
	for suffix, filePath := range suffixedTemplates {
		outputName := config.MethodName + "_" + suffix + ".json5"
		if err := exportTemplateFromFile(filePath, outputName, suffix, config); err != nil {
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

// generateItemsFromMeta generates Items based on ItemsMeta rules
// Category values are item names (CategoryItem), e.g., "quick_words", "pop_words", "roots"
// File format: "CategoryItem_methodNameSuffix.txt" or "CategoryItem.txt"
// roots.txt format: "word keyCode" (e.g., "土 GA")
// others format: "code word" (e.g., "ga 土")
func generateItemsFromMeta(itemsMeta []TemplateItemsMeta, targetPath, methodNameSuffix string) ([]map[string][]string, error) {
	items := make([]map[string][]string, len(itemsMeta))

	for i, meta := range itemsMeta {
		itemMap := make(map[string][]string)

		for _, categoryItem := range meta.Category {
			// Try different file patterns based on methodNameSuffix
			var filePatterns []string
			if methodNameSuffix != "" {
				filePatterns = []string{
					categoryItem + "_" + methodNameSuffix + ".txt",
					categoryItem + ".txt",
				}
			} else {
				filePatterns = []string{
					categoryItem + ".txt",
				}
			}

			for _, filePattern := range filePatterns {
				categoryFilePath := filepath.Join(targetPath, filePattern)
				if _, err := os.Stat(categoryFilePath); os.IsNotExist(err) {
					continue
				}

				// Read and parse the category file
				file, err := os.Open(categoryFilePath)
				if err != nil {
					continue
				}
				scanner := bufio.NewScanner(file)
				for scanner.Scan() {
					line := scanner.Text()
					fields := strings.Fields(line)
					if len(fields) != 2 {
						continue
					}

					var code, word string
					if categoryItem == "roots" {
						// roots.txt format: "word keyCode"
						word = fields[0]
						code = fields[1]
					} else {
						// others format: "code word"
						code = fields[0]
						word = fields[1]
					}

					// Check Prefix (code must contain one of the prefixes)
					if len(meta.Prefix) > 0 {
						hasPrefix := false
						for _, pre := range meta.Prefix {
							if strings.HasPrefix(code, pre) {
								hasPrefix = true
								break
							}
						}
						if !hasPrefix {
							continue
						}
					}

					// Check Suffix (code must contain one of the suffixes)
					if len(meta.Suffix) > 0 {
						hasSuffix := false
						for _, suf := range meta.Suffix {
							if strings.HasSuffix(code, suf) {
								hasSuffix = true
								break
							}
						}
						if !hasSuffix {
							continue
						}
					}

					// Check MinLength
					if meta.MinLength > 0 && len(code) < meta.MinLength {
						continue
					}

					// Check MaxLength
					if meta.MaxLength > 0 && len(code) > meta.MaxLength {
						continue
					}

					// Apply AppendSuffix
					if meta.AppendSuffix != "" {
						code = code + meta.AppendSuffix
					}

					// Add to item map
					itemMap[code] = append(itemMap[code], word)
				}
				file.Close()
			}
		}

		// Convert map to slice format
		items[i] = make(map[string][]string)
		for code, words := range itemMap {
			items[i][code] = words
		}
	}

	return items, nil
}

// exportTemplateFromFile reads a template file, updates configversion, and writes to target
func exportTemplateFromFile(templatePath, outputName, methodNameSuffix string, config ExportConfig) error {
	// Read and parse JSON5 template using gookit/config
	var tmplMeta TemplateMeta
	err := gkconfig.LoadFiles(templatePath)
	if err != nil {
		return fmt.Errorf("failed to parse template file: %w", err)
	}
	if err := gkconfig.Decode(&tmplMeta); err != nil {
		return fmt.Errorf("failed to decode template file: %w", err)
	}

	// Update configversion
	newVersion, err := updateConfigVersion(tmplMeta.ConfigVersion)
	if err != nil {
		return fmt.Errorf("failed to update configversion: %w", err)
	}
	tmplMeta.ConfigVersion = newVersion

	// Update original template file if --update flag is set
	if config.Update {
		if err := updateTemplateConfigVersion(templatePath, newVersion); err != nil {
			return fmt.Errorf("failed to update template file: %w", err)
		}
	}

	// Generate Items from ItemsMeta
	items, err := generateItemsFromMeta(tmplMeta.ItemsMeta, config.TargetPath, methodNameSuffix)
	if err != nil {
		return fmt.Errorf("failed to generate items: %w", err)
	}

	// Convert to Template for JSON output (ItemsMeta will be excluded)
	tmpl := Template{
		Name:          tmplMeta.Name,
		Version:       config.Version,
		ConfigVersion: tmplMeta.ConfigVersion,
		Fonts:         tmplMeta.Fonts,
		KeyBindings:   tmplMeta.KeyBindings,
		Items:         items,
		Tabs:          tmplMeta.Tabs,
		Help:          tmplMeta.Help,
	}

	// Use template's Version if config.Version is empty
	if tmpl.Version == "" {
		tmpl.Version = tmplMeta.Version
	}

	// Write to output file with proper formatting
	baseName := strings.TrimSuffix(outputName, ".json5")
	if config.Version != "" {
		outputName = baseName + "_" + config.Version + ".json5"
	}
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

// updateConfigVersion updates the configversion based on current date
// configversion format: "YYYY.M.D-seq" (e.g., "2026.1.29-1")
func updateConfigVersion(current string) (string, error) {
	// Parse current configversion
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

// updateTemplateConfigVersion updates the configversion in the original template JSON5 file
func updateTemplateConfigVersion(templatePath, newVersion string) error {
	content, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read template file: %w", err)
	}

	// Replace config_version value using regex
	// Match patterns like: config_version: "2026.1.29-1" or config_version:'2026.1.29-1'
	configversionRegex := regexp.MustCompile(`config_version\s*:\s*["'][^"']*["']`)
	newContent := configversionRegex.ReplaceAllString(string(content), fmt.Sprintf(`config_version: "%s"`, newVersion))

	if err := os.WriteFile(templatePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write template file: %w", err)
	}

	return nil
}
