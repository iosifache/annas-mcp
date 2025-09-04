#!/usr/bin/env python3
import requests
from bs4 import BeautifulSoup
import socket

# Force IPv4
original_create_connection = socket.create_connection
def create_connection_ipv4_only(address, *args, **kwargs):
    return original_create_connection(address, *args, **{**kwargs, 'family': socket.AF_INET})
socket.create_connection = create_connection_ipv4_only

url = "https://annas-archive.org/search?q=python%20programming"
session = requests.Session()
session.headers.update({'User-Agent': 'annas-mcp/2.0'})

response = session.get(url, timeout=30)
soup = BeautifulSoup(response.content, 'html.parser')

print("=== Page Title ===")
print(soup.title.string if soup.title else "No title")

print("\n=== Looking for book containers ===")
containers = soup.find_all('div', class_='js-vim-focus')
print(f"Found {len(containers)} js-vim-focus divs")

print("\n=== Looking for MD5 links ===")
md5_links = soup.find_all('a', href=lambda x: x and x.startswith('/md5/'))
print(f"Found {len(md5_links)} MD5 links")

for i, link in enumerate(md5_links[:3]):
    print(f"{i+1}. {link.get('href')} -> {link.get_text(strip=True)[:50]}")

print("\n=== General structure ===")
main_content = soup.find('main') or soup.find('div', class_='main')
if main_content:
    divs = main_content.find_all('div')[:10]
    for i, div in enumerate(divs):
        classes = div.get('class', [])
        text = div.get_text(strip=True)[:100]
        print(f"{i+1}. Classes: {classes} | Text: {text}")
else:
    print("No main content found")