#!/bin/bash
set -e

# 创建 publish 目录
mkdir -p ./publish

# 遍历当前目录下的所有 zip 文件
for zip_file in *.zip; do
    if [ -f "$zip_file" ]; then
        echo "Processing: $zip_file"

        go run ./main.go ./export.go export -s "$zip_file" -t ./gen

        cp ./gen/*.json5 ./publish/
    fi
done


# 从 HTML 文件中提取 appVersion
appVersion=$(grep -o 'appVersion = "[^"]*"' "输入法练习工具.html" | sed 's/.*= "//;s/"$//')

if [ -z "$appVersion" ]; then
    echo "Error: Could not extract appVersion from 输入法练习工具.html"
    exit 1
fi

echo "App version: $appVersion"

# 复制并重命名 HTML 文件
cp "输入法练习工具.html" "./publish/输入法练习工具_${appVersion}.html"

echo "Done! Files copied to ./publish"
