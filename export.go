package main

import (
	"bufio"
	"errors"
	"fmt"
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

	// 1. Check if src folder and 'yuhao' folder under src exist
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return fmt.Errorf("source directory '%s' does not exist", src)
	}

	yuhaoPath := filepath.Join(src, "yuhao")
	if _, err := os.Stat(yuhaoPath); os.IsNotExist(err) {
		return fmt.Errorf("yuhao directory '%s' does not exist", yuhaoPath)
	}

	// 2. Check if tar folder exists, create it recursively if not
	if _, err := os.Stat(tar); os.IsNotExist(err) {
		if err := os.MkdirAll(tar, 0755); err != nil {
			return fmt.Errorf("failed to create target directory '%s': %w", tar, err)
		}
	}

	// 3. Export root
	if err := exportRoot(methodName, yuhaoPath, tar); err != nil {
		return fmt.Errorf("failed to export root: %w", err)
	}

	// 4. Export quick words
	if err := exportQuickWords(methodName, yuhaoPath, tar); err != nil {
		return fmt.Errorf("failed to export quick words: %w", err)
	}

	return nil
}

func exportRoot(methodName, yuhaoPath, tar string) error {
	dictFileName := methodName + ".roots.dict.yaml"
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
				key := strings.TrimPrefix(lastField, "/lm")
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

	// Get list of unique keyCodes and sort them
	var sortedKeyCodes []string
	for keyCode := range wordToKeyCodeMap {
		sortedKeyCodes = append(sortedKeyCodes, keyCode)
	}
	sort.Strings(sortedKeyCodes)

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

// exportQuickWords 实现了如下功能：
// 1. 按 utf-8 读取 src/yuhao/methodName.quick.dict.yaml 文件，每行 split 后只留下列数为 2 且第二列全部是英文字母的行
// 2. split[0] 的内容转成 rune slice，如果大于 1 代表是个词，否则是字；split[1] 为字词的编码 code
// 3. 字词分开存储，并且各自按 code 升序排序，再将 code 和 字词的映射分别存储到 tar/quickWords.txt 和 tar/quickChars.txt 中
// 4. 注意 code 与字词之间用一个 tab 分割，记录之间用换行分割
// 5. 如果参数不满足需求，适当调整
func exportQuickWords(methodName, yuhaoPath, tar string) error {
	dictFileName := methodName + ".quick.dict.yaml"
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
	sort.Slice(wordsList, func(i, j int) bool {
		codeI := wordsList[i][1]
		codeJ := wordsList[j][1]
		if len(codeI) != len(codeJ) {
			return len(codeI) < len(codeJ) // Shorter codes first
		}
		return codeI < codeJ // Then alphabetical
	})

	sort.Slice(charsList, func(i, j int) bool {
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
