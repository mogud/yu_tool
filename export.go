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

	//

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
	outputFilePath := filepath.Join(tar, "root.txt")
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
