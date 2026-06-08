#!/usr/bin/env python3
"""
用法: python replace_in_dir.py <文件夹> <src> <dst>

第一轮: 递归替换所有文件内容中的整词 src -> dst（严格区分大小写，只匹配完整单词）
第二轮: 将所有名字严格等于 src 的文件夹重命名为 dst
"""

import os
import re
import sys


def is_binary(filepath):
    """通过检测是否含有 NULL 字节来判断文件是否为二进制文件"""
    try:
        with open(filepath, "rb") as f:
            chunk = f.read(8192)
        return b"\x00" in chunk
    except Exception:
        return True


def replace_in_files(folder, src, dst, pattern):
    """第一轮：替换所有文件内容中的 src -> dst（整词匹配）"""
    total_modified = 0
    total_skipped = 0
    total_errors = 0

    for root, dirs, files in os.walk(folder, topdown=True):
        # 让 os.walk 也能遍历以 . 开头的隐藏子目录
        dirs[:] = [d for d in dirs]  # 不过滤，全部遍历

        for filename in files:
            filepath = os.path.join(root, filename)

            if is_binary(filepath):
                total_skipped += 1
                continue

            try:
                with open(filepath, "r", encoding="utf-8", errors="replace") as f:
                    content = f.read()

                new_content = pattern.sub(dst, content)

                if new_content != content:
                    with open(filepath, "w", encoding="utf-8", errors="replace") as f:
                        f.write(new_content)
                    print(f"  [修改] {filepath}")
                    total_modified += 1
                else:
                    total_skipped += 1

            except Exception as e:
                print(f"  [错误] 处理文件 {filepath} 时出错: {e}", file=sys.stderr)
                total_errors += 1

    return total_modified, total_skipped, total_errors


def rename_dirs(folder, src, dst):
    """第二轮：将名字严格等于 src 的目录重命名为 dst（自底向上，避免路径冲突）"""
    # 先收集所有需要重命名的目录（自底向上，topdown=False）
    to_rename = []

    for root, dirs, files in os.walk(folder, topdown=False):
        for d in dirs:
            if d == src:
                old_path = os.path.join(root, d)
                new_path = os.path.join(root, dst)
                to_rename.append((old_path, new_path))

    total_renamed = 0
    total_errors = 0

    for old_path, new_path in to_rename:
        # 路径可能已因父目录更名而变化，跳过不存在的
        if not os.path.exists(old_path):
            print(f"  [跳过] 路径已不存在（可能已由父目录重命名）: {old_path}", file=sys.stderr)
            continue
        try:
            os.rename(old_path, new_path)
            print(f"  [重命名] {old_path}  ->  {new_path}")
            total_renamed += 1
        except Exception as e:
            print(f"  [错误] 重命名目录 {old_path} 时出错: {e}", file=sys.stderr)
            total_errors += 1

    return total_renamed, total_errors


def main():
    if len(sys.argv) != 4:
        print("用法: python replace_in_dir.py <文件夹> <src> <dst>")
        sys.exit(1)

    folder = sys.argv[1]
    src = sys.argv[2]
    dst = sys.argv[3]

    if not os.path.isdir(folder):
        print(f"错误: '{folder}' 不是一个有效的文件夹", file=sys.stderr)
        sys.exit(1)

    if not src:
        print("错误: src 不能为空", file=sys.stderr)
        sys.exit(1)

    # 构建整词匹配的正则（严格区分大小写）
    # \b 匹配单词边界，re.escape 处理 src 中可能含有的特殊字符
    pattern = re.compile(r"\b" + re.escape(src) + r"\b")

    print(f"文件夹  : {os.path.abspath(folder)}")
    print(f"src     : {src}")
    print(f"dst     : {dst}")
    print(f"正则    : {pattern.pattern}")
    print()

    # ── 第一轮：文件内容替换 ──────────────────────────────────────────────────
    print("=" * 60)
    print(f"第一轮：替换所有文件内容中的整词 '{src}' -> '{dst}'")
    print("=" * 60)
    modified, skipped, errors1 = replace_in_files(folder, src, dst, pattern)
    print(f"\n  已修改文件: {modified}  |  跳过(无匹配/二进制): {skipped}  |  错误: {errors1}")

    # ── 第二轮：目录重命名 ────────────────────────────────────────────────────
    print()
    print("=" * 60)
    print(f"第二轮：将名字严格等于 '{src}' 的目录重命名为 '{dst}'")
    print("=" * 60)
    renamed, errors2 = rename_dirs(folder, src, dst)
    print(f"\n  已重命名目录: {renamed}  |  错误: {errors2}")

    print()
    print("全部完成！")


if __name__ == "__main__":
    main()
