import requests
from bs4 import BeautifulSoup
import os
import time
import random

def fetch_page(url, headers):
    try:
        response = requests.get(url, headers=headers)
        response.raise_for_status()  # 如果请求失败，抛出HTTPError
        return response.text
    except requests.RequestException as e:
        print(f"Error fetching the URL {url}: {e}")
        return None

def parse_page(html):
    soup = BeautifulSoup(html, 'html.parser')
    
    # 查找所有符合条件的小说链接
    books = soup.find_all('a', class_='green')
    
    # 提取小说名称
    book_names = [book.get_text() for book in books]
    
    return book_names

def save_to_file(book_names, filename):
    lock_filename = f"{filename}.lock"
    try:
        # 创建.lock文件
        with open(lock_filename, 'w') as lock_file:
            pass
        
        # 使用'w'模式写入文件，确保每次写入前清空文件内容
        with open(filename, 'w', encoding='utf-8') as f:
            for name in book_names:
                f.write(f"{name}\n")
        print(f"Book names have been written to {filename}")
    except IOError as e:
        print(f"Error writing to file {filename}: {e}")
    finally:
        # 删除.lock文件
        if os.path.exists(lock_filename):
            os.remove(lock_filename)

def get_last_page_number(start_url, headers):
    html = fetch_page(start_url, headers)
    if not html:
        return None

    soup = BeautifulSoup(html, 'html.parser')
    last_page_link = soup.select_one('.pages a.end')
    if last_page_link:
        last_page_url = last_page_link['href']
        last_page_number = int(last_page_url.split('_')[-1].split('.')[0])
        return last_page_number
    return None

def main():
    start_url = 'http://www.shuyy8.cc/kehuan/list_update_1.html'  # 替换为你要请求的起始URL
    
    headers = {
        "Connection": "keep-alive",
        "Cache-Control": "max-age=0",
        "User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
        "Accept-Encoding": "gzip, deflate",
        "Accept-Language": "zh-CN,zh;q=0.9",
        "Cookie": "client_key=F1F17934834AE26140BBADBE6AC6AA5D",
        "If-Modified-Since": "Thu, 30 May 2024 15:39:00 GMT"
    }
    
    if not os.path.exists("download"):
        os.makedirs("download")
    
    last_page = get_last_page_number(start_url, headers)
    if last_page is None:
        print("Failed to retrieve the last page number.")
        return
    
    for page_num in range(1, last_page + 1):
        page_url = f'http://www.shuyy8.cc/kehuan/list_update_{page_num}.html'
        html = fetch_page(page_url, headers)
        if html:
            book_names = parse_page(html)
            filename = f"download/{page_num}.txt"
            save_to_file(book_names, filename)
            sleep_time = random.randint(1, 5)
            print(f"Sleeping for {sleep_time} seconds...")
            time.sleep(sleep_time)

if __name__ == "__main__":
    main()
