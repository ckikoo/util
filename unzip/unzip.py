import zipfile
import os

def extract_zip(zip_path, extract_path):
    with zipfile.ZipFile(zip_path, 'r') as zip_ref:
        for member in zip_ref.namelist():
            try:
                # 尝试以GBK编码解码文件名并重新编码为UTF-8
                utf8name = member.encode('cp437').decode('gbk')
            except UnicodeDecodeError:
                # 如果解码失败，则使用原始文件名
                utf8name = member
            member_path = os.path.join(extract_path, utf8name)
            # 创建文件的目录（如果不存在）
            os.makedirs(os.path.dirname(member_path), exist_ok=True)
            # 提取文件内容并写入新文件
            with open(member_path, 'wb') as outfile:
                outfile.write(zip_ref.read(member))

def extract_all_zips(download_dir, extract_base_dir):
    for root, _, files in os.walk(download_dir):
        for file in files:
            if file.endswith('.zip'):
                zip_path = os.path.join(root, file)
                extract_path = os.path.join(extract_base_dir, os.path.splitext(file)[0])
                try:
                    extract_zip(zip_path, extract_path)
                except:
                    continue


if __name__ == "__main__":
    download_dir = 'download'  # 替换为你的下载目录路径
    extract_base_dir = 'extracted_files'  # 替换为你的解压基目录路径
    extract_all_zips(download_dir, extract_base_dir)
