import asyncio
import re
import os
from pathlib import Path
from urllib.parse import urljoin, urlparse
from crawl4ai import AsyncWebCrawler, CrawlerRunConfig, CacheMode


# --- 核心配置区 ---
START_URL = "https://bybit-exchange.github.io/docs/v5/guide"      # 起始 URL
URL_REGEX = r".*/docs/v5/.*"           # 只爬取包含符合特定正则的链接
MAX_PAGES = 1000
OUTPUT_BASE_DIR = "bybit_docs"
CONCURRENT_COUNT = 5 

# --- 网页内容解析配置 (在这里屏蔽菜单和侧边栏) ---
# 1. 排除模式：列出所有你不想看到的 HTML 标签、Class 或 ID
EXCLUDED_TAGS = [
    "nav", 
    "footer", 
    "header", 
    "aside", 
]

# 2. 聚焦模式：如果你只想抓取某个特定区域，填写它的 CSS 选择器。
# 例如 "main" 或 "#content"。如果设为 None，则抓取除上述排除项之外的整页。
CONTENT_SELECTORS = ['article']
# ------------------


def url_to_filepath(url, base_dir):
    """将 URL 映射为本地文件路径"""
    parsed = urlparse(normalize_url(url))
    path_str = parsed.path.strip("/")
    path_parts = [p for p in path_str.split('/') if p]
    if not path_parts:
        path_parts = ["index"]
    
    full_path = Path(base_dir) / parsed.netloc / Path(*path_parts)
    return full_path.with_suffix(".md")

def normalize_url(url):
    """统一规范 URL：移除 query/fragment，并去掉末尾斜杠"""
    parsed = urlparse(url)
    normalized = parsed._replace(query="", fragment="", params="").geturl()
    return normalized.rstrip("/")

async def save_to_markdown(result, base_dir):
    """保存爬取到的内容"""
    if not result.success or not result.markdown:
        return False

    file_path = url_to_filepath(result.url, base_dir)
    file_path.parent.mkdir(parents=True, exist_ok=True)
    
    try:
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(result.markdown)
        return True
    except Exception as e:
        print(f"❌ 写入文件失败 {result.url}: {e}")
        return False

async def exhaustive_crawl():
    visited = set()            
    to_visit = {START_URL}     
    processing = set()         
    
    regex = re.compile(URL_REGEX)
    
    # 构造爬虫配置
    config = CrawlerRunConfig(
        cache_mode=CacheMode.ENABLED,
        exclude_external_links=True,
        # 应用全局配置
        target_elements=CONTENT_SELECTORS,
        excluded_tags=EXCLUDED_TAGS,
        # 自动移除常见的遮罩层和弹窗
        remove_overlay_elements=True
    )

    Path(OUTPUT_BASE_DIR).mkdir(parents=True, exist_ok=True)

    async with AsyncWebCrawler() as crawler:
        while (to_visit or processing) and len(visited) < MAX_PAGES:
            current_batch = []
            while to_visit and len(current_batch) < CONCURRENT_COUNT:
                url = to_visit.pop()
                normalized_url = normalize_url(url)
                
                if normalized_url in visited or normalized_url in processing:
                    continue

                target_file = url_to_filepath(normalized_url, OUTPUT_BASE_DIR)
                if target_file.exists():
                    print(f"⏭️  跳过 (已存在): {normalized_url}")
                    visited.add(normalized_url)
                    continue

                current_batch.append(normalized_url)
                processing.add(normalized_url)
            
            if not current_batch:
                if not processing: break 
                await asyncio.sleep(0.5)
                continue

            print(f"🌐 正在爬取新页面: {len(current_batch)} 条...")
            
            results = await crawler.arun_many(current_batch, config=config)
            
            for result in results:
                curr_url = normalize_url(result.url)
                if curr_url in processing:
                    processing.remove(curr_url)
                
                if not result.success:
                    continue
                
                visited.add(curr_url)
                print(f"✅ 成功下载: {curr_url}")
                
                await save_to_markdown(result, OUTPUT_BASE_DIR)

                links = result.links or {}
                internal_links = links.get("internal", [])
                for link in internal_links:
                    href = link.get("href")
                    if not href: continue
                    
                    full_url = normalize_url(urljoin(result.url, href))
                    
                    if regex.search(full_url) and full_url not in visited and full_url not in processing:
                        to_visit.add(full_url)

    print(f"\n✨ 任务完成。当前本地库共计: {len(visited)} 个页面。")

if __name__ == "__main__":
    asyncio.run(exhaustive_crawl())
