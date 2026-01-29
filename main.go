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
