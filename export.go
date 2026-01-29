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

func export(methodName, src, tar string) error {
	// 0. Check if methodName is empty
	if methodName == "" {
		return errors.New("method name cannot be empty")
	}

	// Parse methodName and suffix
	var baseMethodName, suffix string
	parts := strings.Split(methodName, ",")
	if len(parts) == 1 {
		baseMethodName = parts[0]
		suffix = ""
	} else if len(parts) == 2 {
		baseMethodName = parts[0]
		suffix = parts[1]
	} else {
		return errors.New("invalid method name format, expected 'name' or 'name,suffix'")
	}

	// Check if src is a zip file
	isZip := strings.HasSuffix(strings.ToLower(src), ".zip")

	var yuhaoPath string
	tempDir := ""

	if isZip {
		// Extract zip file to temporary directory
		tmpDir, err := os.MkdirTemp("", "yu_tool_")
		if err != nil {
			return fmt.Errorf("failed to create temporary directory: %w", err)
		}
		tempDir = tmpDir
		defer os.RemoveAll(tempDir) // Clean up temp directory

		err = extractZipToDir(src, tempDir)
		if err != nil {
			return fmt.Errorf("failed to extract zip file: %w", err)
		}

		yuhaoPath = filepath.Join(tempDir, "schema/yuhao")
	} else {
		// 1. Check if src folder and 'yuhao' folder under src exist
		if _, err := os.Stat(src); os.IsNotExist(err) {
			return fmt.Errorf("source directory '%s' does not exist", src)
		}

		yuhaoPath = filepath.Join(src, "yuhao")
		if _, err := os.Stat(yuhaoPath); os.IsNotExist(err) {
			return fmt.Errorf("yuhao directory '%s' does not exist", yuhaoPath)
		}
	}

	// 2. Check if tar folder exists, create it recursively if not
	if _, err := os.Stat(tar); os.IsNotExist(err) {
		if err := os.MkdirAll(tar, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", tar, err)
		}
	}

	// 3. Export root
	if err := exportRoot(baseMethodName, suffix, yuhaoPath, tar); err != nil {
		return fmt.Errorf("failed to export root: %w", err)
	}

	// 4. Export quick words
	if err := exportQuickWords(baseMethodName, suffix, yuhaoPath, tar); err != nil {
		return fmt.Errorf("failed to export quick words: %w", err)
	}

	// 5. Export pop words - ignore if file doesn't exist
	if err := exportPopWords(baseMethodName, suffix, yuhaoPath, tar); err != nil {
		// Check if error is due to file not existing
		if !strings.Contains(err.Error(), "no such file or directory") && !strings.Contains(err.Error(), "cannot find the file") {
			return fmt.Errorf("failed to export pop words: %w", err)
		}
		// If it's a "file not found" error, just log and continue
		fmt.Printf("Notice: Pop words file not found, skipping pop words export\n")
	}

	return nil
}

func exportRoot(methodName, suffix, yuhaoPath, tar string) error {
	// Try to find file with suffix first
	var dictFileName string
	if suffix != "" {
		dictFileNameWithSuffix := methodName + "_" + suffix + ".roots.dict.yaml"
		dictFilePathWithSuffix := filepath.Join(yuhaoPath, dictFileNameWithSuffix)
		if _, err := os.Stat(dictFilePathWithSuffix); err == nil {
			// File with suffix exists, use it
			dictFileName = dictFileNameWithSuffix
		} else {
			// File with suffix doesn't exist, fall back to original
			dictFileName = methodName + ".roots.dict.yaml"
		}
	} else {
		dictFileName = methodName + ".roots.dict.yaml"
	}

	dictFilePath := filepath.Join(yuhaoPath, dictFileName)

	file, err := os.Open(dictFilePath)
	if err != nil {
		return fmt.Errorf("failed to open dictionary file '%s': %w", dictFilePath, err)
	}
	defer file.Close()

	// Create output file
	outputFilePath := filepath.Join(tar, "roots.txt")
	outputFile, err := os.Create(outputFilePath)
	if err != nil {
		return fmt.Errorf("failed to create output file '%s': %w", outputFilePath, err)
	}
	defer outputFile.Close()

	// Map to store keyCodes and their corresponding words
	wordToKeyCodeMap := make(map[string][]string) // map: keyCode -> list of words

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		// Process only lines that start with '+'
		if strings.HasPrefix(line, "+") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				code := fields[1]
				words := fields[3]
				// Process the last field to extract key by removing '/lm' prefix
				lastField := fields[len(fields)-1]
				key := lastField[3:]
				keyCode := key + code

				// Add words corresponding to each keyCode
				for _, word := range []rune(words) {
					wordStr := string(word)
					wordToKeyCodeMap[keyCode] = append(wordToKeyCodeMap[keyCode], wordStr)
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary file: %w", err)
	}

	// Get list of unique keyCodes and sort them by length first, then alphabetically
	var sortedKeyCodes []string
	for keyCode := range wordToKeyCodeMap {
		sortedKeyCodes = append(sortedKeyCodes, keyCode)
	}
	sort.SliceStable(sortedKeyCodes, func(i, j int) bool {
		keyCodeI := sortedKeyCodes[i]
		keyCodeJ := sortedKeyCodes[j]
		if len(keyCodeI) != len(keyCodeJ) {
			return len(keyCodeI) < len(keyCodeJ) // Shorter keyCodes first
		}
		return keyCodeI < keyCodeJ // Then alphabetical
	})

	// Write sorted entries to output file
	for _, keyCode := range sortedKeyCodes {
		words := wordToKeyCodeMap[keyCode]
		for _, word := range words {
			entry := word + "\t" + keyCode
			_, err := outputFile.WriteString(entry + "\n")
			if err != nil {
				return fmt.Errorf("failed to write to output file: %w", err)
			}
		}
	}
	return nil
}

func exportQuickWords(methodName, suffix, yuhaoPath, tar string) error {
	// Try to find file with suffix first
	var dictFileName string
	if suffix != "" {
		dictFileNameWithSuffix := methodName + "_" + suffix + ".quick.dict.yaml"
		dictFilePathWithSuffix := filepath.Join(yuhaoPath, dictFileNameWithSuffix)
		if _, err := os.Stat(dictFilePathWithSuffix); err == nil {
			// File with suffix exists, use it
			dictFileName = dictFileNameWithSuffix
		} else {
			// File with suffix doesn't exist, fall back to original
			dictFileName = methodName + ".quick.dict.yaml"
		}
	} else {
		dictFileName = methodName + ".quick.dict.yaml"
	}

	dictFilePath := filepath.Join(yuhaoPath, dictFileName)

	file, err := os.Open(dictFilePath)
	if err != nil {
		return fmt.Errorf("failed to open dictionary file '%s': %w", dictFilePath, err)
	}
	defer file.Close()

	// Create output files
	wordsOutputPath := filepath.Join(tar, "quick_words.txt")
	charsOutputPath := filepath.Join(tar, "quick_chars.txt")

	wordsFile, err := os.Create(wordsOutputPath)
	if err != nil {
		return fmt.Errorf("failed to create words output file '%s': %w", wordsOutputPath, err)
	}
	defer wordsFile.Close()

	charsFile, err := os.Create(charsOutputPath)
	if err != nil {
		return fmt.Errorf("failed to create chars output file '%s': %w", charsOutputPath, err)
	}
	defer charsFile.Close()

	// Slices to store words and chars with their codes
	var wordsList [][2]string // [][word, code]
	var charsList [][2]string // [][char, code]

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// Only process lines with exactly 2 fields
		if len(fields) == 2 {
			wordOrChar := fields[0]
			code := fields[1]

			// Check if code contains only English letters and wordOrChar is not all ASCII
			if isEnglishLettersOnly(code) && !isAllASCII(wordOrChar) {
				runes := []rune(wordOrChar)
				if len(runes) > 1 {
					// It's a word (more than one rune)
					wordsList = append(wordsList, [2]string{wordOrChar, code})
				} else {
					// It's a character (single rune)
					charsList = append(charsList, [2]string{wordOrChar, code})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary file: %w", err)
	}

	// Sort both lists by code: shorter codes first, then alphabetically
	sort.SliceStable(wordsList, func(i, j int) bool {
		codeI := wordsList[i][1]
		codeJ := wordsList[j][1]
		if len(codeI) != len(codeJ) {
			return len(codeI) < len(codeJ) // Shorter codes first
		}
		return codeI < codeJ // Then alphabetical
	})

	sort.SliceStable(charsList, func(i, j int) bool {
		codeI := charsList[i][1]
		codeJ := charsList[j][1]
		if len(codeI) != len(codeJ) {
			return len(codeI) < len(codeJ) // Shorter codes first
		}
		return codeI < codeJ // Then alphabetical
	})

	// Write words to file
	for _, item := range wordsList {
		_, err := wordsFile.WriteString(item[1] + "\t" + item[0] + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to words file: %w", err)
		}
	}

	// Write chars to file
	for _, item := range charsList {
		_, err := charsFile.WriteString(item[1] + "\t" + item[0] + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to chars file: %w", err)
		}
	}

	return nil
}

func exportPopWords(methodName, suffix, yuhaoPath, tar string) error {
	// 1. Check if src folder and 'yuhao' folder under src exist
	if _, err := os.Stat(yuhaoPath); os.IsNotExist(err) {
		return fmt.Errorf("yuhao directory '%s' does not exist", yuhaoPath)
	}

	// 2. Check if tar folder exists, create it recursively if not
	if _, err := os.Stat(tar); os.IsNotExist(err) {
		if err := os.MkdirAll(tar, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", tar, err)
		}
	}

	// Try to find file with suffix first
	var dictFileName string
	if suffix != "" {
		dictFileNameWithSuffix := methodName + "_" + suffix + ".pop.dict.yaml"
		dictFilePathWithSuffix := filepath.Join(yuhaoPath, dictFileNameWithSuffix)
		if _, err := os.Stat(dictFilePathWithSuffix); err == nil {
			// File with suffix exists, use it
			dictFileName = dictFileNameWithSuffix
		} else {
			// File with suffix doesn't exist, fall back to original
			dictFileName = methodName + ".pop.dict.yaml"
		}
	} else {
		dictFileName = methodName + ".pop.dict.yaml"
	}

	dictFilePath := filepath.Join(yuhaoPath, dictFileName)

	file, err := os.Open(dictFilePath)
	if err != nil {
		return fmt.Errorf("failed to open dictionary file '%s': %w", dictFilePath, err)
	}
	defer file.Close()

	// Create output files
	popWordsOutputPath := filepath.Join(tar, "pop_words.txt")
	popCharsOutputPath := filepath.Join(tar, "pop_chars.txt")

	popWordsFile, err := os.Create(popWordsOutputPath)
	if err != nil {
		return fmt.Errorf("failed to create pop words output file '%s': %w", popWordsOutputPath, err)
	}
	defer popWordsFile.Close()

	popCharsFile, err := os.Create(popCharsOutputPath)
	if err != nil {
		return fmt.Errorf("failed to create pop chars output file '%s': %w", popCharsOutputPath, err)
	}
	defer popCharsFile.Close()

	// Slices to store words and chars with their codes
	var wordsList [][2]string // [][word, code]
	var charsList [][2]string // [][char, code]

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// Only process lines with exactly 2 fields
		if len(fields) == 2 {
			wordOrChar := fields[0]
			code := fields[1]

			// Check if code contains only English letters and wordOrChar is not all ASCII
			if isEnglishLettersOnly(code) && !isAllASCII(wordOrChar) {
				runes := []rune(wordOrChar)
				if len(runes) > 1 {
					// It's a word (more than one rune)
					wordsList = append(wordsList, [2]string{wordOrChar, code})
				} else {
					// It's a character (single rune)
					charsList = append(charsList, [2]string{wordOrChar, code})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary file: %w", err)
	}

	// Sort both lists by code: shorter codes first, then alphabetically (stable sort)
	sort.SliceStable(wordsList, func(i, j int) bool {
		codeI := wordsList[i][1]
		codeJ := wordsList[j][1]
		if len(codeI) != len(codeJ) {
			return len(codeI) < len(codeJ) // Shorter codes first
		}
		return codeI < codeJ // Then alphabetical
	})

	sort.SliceStable(charsList, func(i, j int) bool {
		codeI := charsList[i][1]
		codeJ := charsList[j][1]
		if len(codeI) != len(codeJ) {
			return len(codeI) < len(codeJ) // Shorter codes first
		}
		return codeI < codeJ // Then alphabetical
	})

	// Write words to file (code first, then word)
	for _, item := range wordsList {
		_, err := popWordsFile.WriteString(item[1] + "\t" + item[0] + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to pop words file: %w", err)
		}
	}

	// Write chars to file (code first, then char)
	for _, item := range charsList {
		_, err := popCharsFile.WriteString(item[1] + "\t" + item[0] + "\n")
		if err != nil {
			return fmt.Errorf("failed to write to pop chars file: %w", err)
		}
	}

	return nil
}

// Helper function to check if a string contains only English letters
func isEnglishLettersOnly(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return true
}

// Helper function to check if a string contains only ASCII characters
func isAllASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

// Helper function to extract zip file to a destination directory
func extractZipToDir(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, file := range r.File {
		filePath := filepath.Join(destDir, file.Name)

		// Check for ZipSlip vulnerability
		if !strings.HasPrefix(filePath, filepath.Clean(destDir)+string(os.PathSeparator)) {
			return fmt.Errorf("illegal file path: %s", filePath)
		}

		if file.FileInfo().IsDir() {
			// Make folder
			if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		// Make sure the directory exists
		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return err
		}

		// Extract file
		fileReader, err := file.Open()
		if err != nil {
			return err
		}

		targetFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			fileReader.Close() // Close fileReader if targetFile opening fails
			return err
		}

		if _, err := io.Copy(targetFile, fileReader); err != nil {
			fileReader.Close() // Close fileReader if copy fails
			targetFile.Close() // Close targetFile if copy fails
			return err
		}

		// Close both files after successful copy
		fileReader.Close()
		targetFile.Close()
	}
	return nil
}
