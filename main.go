package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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

	// 3. Read the YAML dictionary file
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

				// Split words into runes and process each word
				for _, word := range []rune(words) {
					resultLine := string(word) + "\t" + key + code
					_, err := outputFile.WriteString(resultLine + "\n")
					if err != nil {
						return fmt.Errorf("failed to write to output file: %w", err)
					}
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading dictionary file: %w", err)
	}

	return nil
}

func main() {
	var cmd = &cobra.Command{
		Use:   "yu_tool",
		Short: "用来处理宇浩系列发布的二次导出",
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}

	var sourceDir string
	var targetDir string

	var exportCmd = &cobra.Command{
		Use:   "export [method name]",
		Short: "导出宇浩指定输入法的字根、简码",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(export(args[0], sourceDir, targetDir))
		},
	}

	exportCmd.Flags().StringVarP(&sourceDir, "source", "s", ".", "")
	exportCmd.Flags().StringVarP(&targetDir, "target", "t", "./export", "")

	cmd.AddCommand(exportCmd)

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
