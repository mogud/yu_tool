#!/bin/bash
set -e

# 参数解析
DO_ZIP=false
for arg in "$@"; do
    if [ "$arg" = "-zip" ]; then
        DO_ZIP=true
    fi
done

# 创建 publish 目录
mkdir -p ./publish
mkdir -p ./inputs

# 遍历 inputs 目录下的所有 zip 文件
for zip_file in ./inputs/*.zip; do
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

# 如果指定了 -zip 参数，打包 publish 目录
if [ "$DO_ZIP" = true ]; then
    date_str=$(date +%Y%m%d)
    zip_name="输入法工具_${appVersion}_${date_str}.zip"

    if [[ "$OSTYPE" == "msys" ]] || [[ "$OSTYPE" == "cygwin" ]] || [[ "$OSTYPE" == "win32" ]]; then
        # Windows
        powershell.exe -Command "Compress-Archive -Path './publish/*' -DestinationPath './$zip_name' -Force"
    else
        # Linux/Mac
        cd ./publish && zip -r "../$zip_name" . && cd ..
    fi

    echo "Done! Created: $zip_name"
else
    echo "Done! Files copied to ./publish"
fi
