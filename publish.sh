#!/bin/bash
set -e

# 参数解析
DO_ZIP=false
DO_LOCAL=false
for arg in "$@"; do
    if [ "$arg" = "-zip" ]; then
        DO_ZIP=true
    elif [ "$arg" = "-local" ]; then
        DO_LOCAL=true
    fi
done

# 创建 publish 目录并清空
mkdir -p ./publish
rm -rf ./publish/*
mkdir -p ./inputs

# 获取 CSV 文件
if [ "$DO_LOCAL" = true ]; then
    echo "Using local CSV files from ./assets..."
    cp ./assets/zigen-*.csv ./inputs/
else
    echo "Downloading CSV files..."
    csv_urls=(
        "https://shurufa.app/zigen-ling.csv"
        "https://shurufa.app/zigen-ming.csv"
        "https://shurufa.app/zigen-joy.csv"
    )
    csv_names=("zigen-ling.csv" "zigen-ming.csv" "zigen-joy.csv")
    download_success=true

    for i in "${!csv_urls[@]}"; do
        url="${csv_urls[$i]}"
        name="${csv_names[$i]}"
        echo "Downloading $url -> ./inputs/$name"
        if ! curl -o "./inputs/$name" "$url"; then
            echo "Failed to download $url"
            download_success=false
            break
        fi
    done

    if [ "$download_success" = false ]; then
        echo "Failed to download some CSV files, rollback..."

        rm -f ./inputs/*.csv
        cp ./assets/zigen-*.csv ./inputs/

    else
        echo "All CSV files downloaded successfully"

        cp ./inputs/zigen-*.csv ./assets/
    fi
fi


# 遍历 inputs 目录下的所有 zip 文件
for zip_file in ./inputs/*.zip; do
    if [ -f "$zip_file" ]; then
        echo "Processing: $zip_file"

        # 根据 zip 文件名确定 CSV 文件
        zip_name=$(basename "$zip_file")
        if [[ "$zip_name" == 靈明* ]]; then
            csv_file="./inputs/zigen-ling.csv"
        elif [[ "$zip_name" == 卿雲* ]]; then
            csv_file="./inputs/zigen-joy.csv"
        elif [[ "$zip_name" == 日月* ]]; then
            csv_file="./inputs/zigen-ming.csv"
        else
            echo "Unknown zip file pattern: $zip_name"
            continue
        fi

        go run ./main.go ./export.go export -s "$zip_file" -r "$csv_file" -t ./export -u

        cp ./export/*.json5 ./publish/
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

# 复制字体文件
cp "Yuniversus.ttf" "./publish/"

# 如果指定了 -zip 参数，打包 publish 目录
if [ "$DO_ZIP" = true ]; then
    date_str=$(date +%Y%m%d)
    base_name="输入法工具_${appVersion}_${date_str}"
    zip_name="${base_name}.zip"

    # 查找所有符合格式的现有 zip 文件
    best_file=""
    best_version=""
    best_date=""
    best_build=""

    for f in 输入法工具_*.zip; do
        if [ -f "$f" ]; then
            filename=$(basename "$f" .zip)

            # 解析: 输入法工具_{version}_{date}_{build}.zip
            version=$(echo "$filename" | sed -E 's/输入法工具_([0-9]+\.[0-9]+\.[0-9]+)_[0-9]{8}.*/\1/')
            date_part=$(echo "$filename" | sed -E 's/输入法工具_[0-9]+\.[0-9]+\.[0-9]+_([0-9]{8}).*/\1/')

            # 提取 build 号（如果有）
            if echo "$filename" | grep -qE '输入法工具_[0-9]+\.[0-9]+\.[0-9]+_[0-9]{8}_[0-9]+$'; then
                build=$(echo "$filename" | sed -E 's/输入法工具_[0-9]+\.[0-9]+\.[0-9]+_[0-9]{8}_([0-9]+)/\1/')
            else
                build="0"
            fi

            # 比较并选择最高的
            if [ -z "$best_file" ]; then
                best_file="$f"
                best_version="$version"
                best_date="$date_part"
                best_build="$build"
            else
                # 版本号比较
                if [ "$version" != "$best_version" ]; then
                    higher=$(printf '%s\n%s\n' "$version" "$best_version" | sort -V | tail -1)
                    if [ "$version" = "$higher" ]; then
                        best_file="$f"
                        best_version="$version"
                        best_date="$date_part"
                        best_build="$build"
                    fi
                elif [ "$date_part" != "$best_date" ]; then
                    # 日期比较
                    if [ "$date_part" -gt "$best_date" ]; then
                        best_file="$f"
                        best_version="$version"
                        best_date="$date_part"
                        best_build="$build"
                    fi
                elif [ "$build" -gt "$best_build" ]; then
                    # build 号比较
                    best_file="$f"
                    best_version="$version"
                    best_date="$date_part"
                    best_build="$build"
                fi
            fi
        fi
    done

    # 如果有匹配的现有文件且版本和日期都相同，则递增 build 号
    if [ -n "$best_file" ] && [ "$best_version" = "$appVersion" ] && [ "$best_date" = "$date_str" ]; then
        new_build=$((best_build + 1))
        zip_name="${base_name}_${new_build}.zip"
    fi

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
