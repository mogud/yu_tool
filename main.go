package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

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
		Use:   "export",
		Short: "导出宇浩输入法的字根、简码",
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(export(sourceDir, targetDir))
		},
	}

	exportCmd.Flags().StringVarP(&sourceDir, "source", "s", "", "宇浩发布的 zip 文件路径")
	_ = exportCmd.MarkFlagRequired("source")
	exportCmd.Flags().StringVarP(&targetDir, "target", "t", "./export", "导出路径")

	cmd.AddCommand(exportCmd)

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
