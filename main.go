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
	var unique bool

	var exportCmd = &cobra.Command{
		Use:   "export [method name]",
		Short: "导出宇浩指定输入法的字根、简码",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cobra.CheckErr(export(args[0], sourceDir, targetDir, unique))
		},
	}

	exportCmd.Flags().StringVarP(&sourceDir, "source", "s", "", "宇浩发布的 zip 文件路径")
	_ = exportCmd.MarkFlagRequired("source")
	exportCmd.Flags().StringVarP(&targetDir, "target", "t", "./export", "导出路径")
	exportCmd.Flags().BoolVarP(&unique, "unique", "u", false, "当设置时，quick 和 pop 的输出中，一个编码 code 只输出一次")

	cmd.AddCommand(exportCmd)

	if err := cmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
